import 'dart:async';
import 'package:flutter/painting.dart' show BoxFit;

import 'package:collection/collection.dart';
import 'package:flutter/foundation.dart';
import 'package:flutter/services.dart';
import 'package:media_kit/media_kit.dart';
import 'package:media_kit_video/media_kit_video.dart';
import 'package:wakelock_plus/wakelock_plus.dart';
import 'package:window_manager/window_manager.dart';

import '../../../core/utils/logger.dart';
import '../../../data/services/history_service.dart';
import '../../../data/services/settings_service.dart';
import '../../../data/services/watch_party_service.dart';
import '../../../domain/entities/entities.dart';
import 'playback_error.dart';

/// Player state that can be observed by UI
class PlayerState {
  final bool isInitialized;
  final bool isBuffering;
  final bool isPlaying;
  final bool hasError;
  final PlaybackError? error;
  final int? currentSeason;
  final int? currentEpisode;
  final String? currentEpisodeTitle;
  final Duration position;
  final Duration duration;
  final double volume;
  final double playbackSpeed;
  final BoxFit videoFit;
  final bool isFullscreen;
  final int textureKey;
  final bool isTransitioning;
  final String currentUrl;
  final String? currentVoiceover;
  final StreamQuality? currentQuality;
  final List<VideoTrack> videoTracks;
  final VideoTrack? selectedVideoTrack;
  final List<AudioTrack> audioTracks;
  final AudioTrack? selectedAudioTrack;

  const PlayerState({
    this.isInitialized = false,
    this.isBuffering = true,
    this.isPlaying = false,
    this.hasError = false,
    this.error,
    this.currentSeason,
    this.currentEpisode,
    this.currentEpisodeTitle,
    this.position = Duration.zero,
    this.duration = Duration.zero,
    this.volume = 100.0,
    this.playbackSpeed = 1.0,
    this.videoFit = BoxFit.contain,
    this.isFullscreen = false,
    this.textureKey = 0,
    this.isTransitioning = false,
    this.currentUrl = '',
    this.currentVoiceover,
    this.currentQuality,
    this.videoTracks = const [],
    this.selectedVideoTrack,
    this.audioTracks = const [],
    this.selectedAudioTrack,
  });

  /// Raw libmpv message, kept for compatibility with existing UI code.
  String? get errorMessage => error?.mpvMessage;

  PlayerState copyWith({
    bool? isInitialized,
    bool? isBuffering,
    bool? isPlaying,
    bool? hasError,
    PlaybackError? error,
    Duration? position,
    Duration? duration,
    double? volume,
    double? playbackSpeed,
    BoxFit? videoFit,
    bool? isFullscreen,
    int? textureKey,
    bool? isTransitioning,
    String? currentUrl,
    String? currentVoiceover,
    StreamQuality? currentQuality,
    List<VideoTrack>? videoTracks,
    VideoTrack? selectedVideoTrack,
    List<AudioTrack>? audioTracks,
    AudioTrack? selectedAudioTrack,
    int? currentSeason,
    int? currentEpisode,
    String? currentEpisodeTitle,
    bool clearError = false,
  }) {
    return PlayerState(
      isInitialized: isInitialized ?? this.isInitialized,
      isBuffering: isBuffering ?? this.isBuffering,
      isPlaying: isPlaying ?? this.isPlaying,
      hasError: clearError ? false : (hasError ?? this.hasError),
      error: clearError ? null : (error ?? this.error),
      position: position ?? this.position,
      duration: duration ?? this.duration,
      volume: volume ?? this.volume,
      playbackSpeed: playbackSpeed ?? this.playbackSpeed,
      videoFit: videoFit ?? this.videoFit,
      isFullscreen: isFullscreen ?? this.isFullscreen,
      textureKey: textureKey ?? this.textureKey,
      isTransitioning: isTransitioning ?? this.isTransitioning,
      currentUrl: currentUrl ?? this.currentUrl,
      currentVoiceover: currentVoiceover ?? this.currentVoiceover,
      currentQuality: currentQuality ?? this.currentQuality,
      videoTracks: videoTracks ?? this.videoTracks,
      selectedVideoTrack: selectedVideoTrack ?? this.selectedVideoTrack,
      audioTracks: audioTracks ?? this.audioTracks,
      selectedAudioTrack: selectedAudioTrack ?? this.selectedAudioTrack,
      currentSeason: currentSeason ?? this.currentSeason,
      currentEpisode: currentEpisode ?? this.currentEpisode,
      currentEpisodeTitle: currentEpisodeTitle ?? this.currentEpisodeTitle,
    );
  }
}

/// Controller that handles all player logic, separated from UI
///
/// Responsibilities:
/// - Media playback control (play, pause, seek)
/// - Stream/quality switching
/// - History saving
/// - Watch Party synchronization
/// - Fullscreen management
/// - WakeLock management
class PlayerController extends ChangeNotifier with WindowListener {
  static const String _tag = 'PlayerController';

  // Dependencies
  final HistoryService _historyService;
  final SettingsService _settingsService;
  final WatchPartyService _watchPartyService;

  // Optional player factory for testing (returns a media_kit Player)
  final Player Function()? _playerFactory;

  // Media info
  final String initialUrl;
  final String? title;
  final String? mediaId;
  final String? providerId;
  final String? posterUrl;
  final ContentType? mediaType;
  final int? initialSeason;
  final int? initialEpisode;
  final String? initialEpisodeTitle;
  final List<StreamSource>? streams;
  final bool isOffline;

  // Internal state
  late final Player _player;
  VideoController? _videoController;
  final List<StreamSubscription<dynamic>> _subscriptions = [];
  Timer? _saveProgressTimer;
  bool _isDisposed = false;

  /// True while a periodic progress save is running. [Timer.periodic] does not
  /// wait for its callback, and a save awaits a cloud sync whose timeouts run to
  /// tens of seconds, so overlapping ticks would let an older position land
  /// after a newer one and rewind the resume point.
  bool _saveInFlight = false;

  /// Index into [_retryLadder] of the header set that last worked.
  ///
  /// -1 means "nothing worked yet", so the next retry starts from step 0.
  /// Remembered for the lifetime of the controller so repeated retries are
  /// one shot instead of walking the whole ladder again.
  int _winningHeaderStep = -1;

  /// The exact header map used for the most recent open, kept so the error
  /// path can report what was actually sent.
  Map<String, String> _lastSentHeaders = const {};

  /// Default UA sent by the retry ladder.
  ///
  /// Kept short and stable — rotating it mid-ladder would change too many
  /// variables at once to stay debuggable.
  static const String _defaultUserAgent =
      'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 '
      '(KHTML, like Gecko) Chrome/124.0 Safari/537.36';

  /// Origin of the media URL, or null when the URL is not a real absolute URL.
  static String? _originOf(String url) {
    try {
      final u = Uri.parse(url);
      if (!u.hasScheme || u.host.isEmpty) return null;
      return u.origin;
    } catch (_) {
      return null;
    }
  }

  /// Host of the Referer/Origin header actually used, for diagnostics.
  static String? _playerHostOf(Map<String, String> headers) {
    for (final key in const ['Referer', 'Referrer', 'Origin']) {
      final v = headers[key] ?? headers[key.toLowerCase()];
      if (v == null || v.isEmpty) continue;
      try {
        final u = Uri.parse(v);
        if (u.host.isNotEmpty) return u.host;
      } catch (_) {
        // Ignore unparsable header values.
      }
    }
    return null;
  }

  /// Builds the header map for each rung of the retry ladder, in order.
  ///
  /// [base] is the [StreamSource]'s own headers and always wins — we escalate
  /// *around* what the provider told us, never against it.
  ///
  /// Rungs:
  ///   0. nothing            — some CDNs reject a Referer that is not theirs
  ///   1. Referer + UA       — the common case
  ///   2. Referer + Origin + UA — some CDNs check Origin even with a Referer
  ///   3. Referer = media URL's own origin only (no UA rewrite)
  List<Map<String, String>> _buildRetryLadder(
    String url,
    Map<String, String>? base,
  ) {
    final origin = _originOf(url);
    final baseHeaders = <String, String>{...?base};

    final none = <String, String>{};

    // Rung 1: Referer + UA (the common case).
    final refererUa = <String, String>{...baseHeaders};
    if (origin != null) refererUa['Referer'] = origin;
    refererUa['User-Agent'] = _defaultUserAgent;

    // Rung 2: add Origin — some CDNs check Origin even with a Referer.
    final refererOriginUa = <String, String>{...refererUa};
    if (origin != null) refererOriginUa['Origin'] = origin;

    // Rung 3: Referer set to the media URL's own origin, no UA rewrite.
    // Some CDNs only accept the directory the media lives in.
    final sameOrigin = <String, String>{...baseHeaders};
    if (origin != null) sameOrigin['Referer'] = origin;

    return [none, refererUa, refererOriginUa, sameOrigin];
  }

  /// The single place where a URL is handed to libmpv.
  ///
  /// Always logs URL, stream type and exact headers first — that single log
  /// line turns "Failed to recognize file format." into a one-line diagnosis.
  Future<void> _openMedia(
    String url,
    StreamSource? source,
    Map<String, String> headers, {
    required String reason,
  }) {
    final type = source?.type ?? StreamType.direct;
    final headerSummary = headers.isEmpty
        ? '<none>'
        : headers.entries.map((e) => '${e.key}=${e.value}').join('; ');
    Logger.i(
      'OPEN[$reason] url=$url type=${type.name} headers={$headerSummary}',
      tag: _tag,
    );
    _lastSentHeaders = Map<String, String>.unmodifiable(headers);
    return _player.open(
      Media(url, httpHeaders: headers.isEmpty ? null : headers),
    );
  }

  /// Keeps mpv's native `referrer` option in sync with the HTTP header.
  ///
  /// Several CDNs validate mpv's own `referrer` separately from the header
  /// map, so switching streams must update it too.
  ///
  /// Deliberately **not** awaited before the open: mpv reads `referrer` per
  /// HTTP request, so setting it on the same event-loop turn as the open is
  /// both sufficient and keeps [_openMedia] reachable synchronously (which
  /// several callers and tests depend on).
  void _applyMpvReferrer(String url) {
    final platform = _player.platform;
    if (platform is! NativePlayer) return;
    final origin = _originOf(url);
    if (origin == null) return;
    // Fire-and-forget: a failure here must not block playback.
    unawaited(
      platform
          .setProperty('referrer', origin)
          .then((_) {
            Logger.d('Set referrer: $origin', tag: _tag);
          })
          .catchError((Object e) {
            Logger.w('Failed to set referrer: $e', tag: _tag);
          }),
    );
  }

  // Test-only flag: when false avoid creating VideoController and subscribing to
  // Player.stream which can depend on native platform assets.
  final bool _setupPlayerStreams;

  // Observable state
  PlayerState _state = const PlayerState();
  PlayerState get state => _state;

  StreamSource? get currentSource {
    final url = _state.currentUrl;
    if (url.isEmpty) return null;

    if (streams != null && streams!.isNotEmpty) {
      try {
        return streams!.firstWhere((s) => s.url == url);
      } catch (_) {
        // Fallback to the first stream if URL doesn't match but we have streams
        return streams!.first;
      }
    }
    // If no streams list provided, create a dummy source from URL
    return StreamSource(url: url);
  }

  MediaItem? get mediaItem {
    // We allow downloading even if mediaId/providerId are missing by providing defaults,
    // though usually they should be present.
    return MediaItem(
      id: mediaId ?? 'unknown_${DateTime.now().millisecondsSinceEpoch}',
      providerId: providerId ?? 'unknown',
      title: title ?? 'Невідоме відео',
      posterUrl: posterUrl,
      type: mediaType ?? ContentType.unknown,
    );
  }

  bool _isTogglingFullscreen = false;
  bool get isTogglingFullscreen => _isTogglingFullscreen;

  // Synchronization for fullscreen transitions
  Completer<void>? _uiRenderedCompleter;
  Completer<void>? _fullscreenTransitionCompleter;

  // High-frequency updates
  final ValueNotifier<Duration> positionNotifier = ValueNotifier(Duration.zero);

  // Expose player and controller for Video widget
  Player get player => _player;
  VideoController? get videoController => _videoController;
  bool get hasVideoController => _videoController != null;

  // Video fit options
  static const List<BoxFit> fits = [BoxFit.contain, BoxFit.cover, BoxFit.fill];
  static const List<String> fitTooltips = [
    'Вписати',
    'Масштабувати (Cover)',
    'Розтягнути (Fill)',
  ];

  // Speed options
  static const List<double> speeds = [0.5, 0.75, 1.0, 1.25, 1.5, 2.0];

  /// [setupPlayerStreams] can be set to false in tests to avoid creating
  /// a real [VideoController] and subscribing to [Player.stream] which may
  /// depend on native platform components. Defaults to true.
  PlayerController({
    required this.initialUrl,
    required HistoryService historyService,
    required SettingsService settingsService,
    required WatchPartyService watchPartyService,
    Player Function()? playerFactory,
    this.title,
    this.mediaId,
    this.providerId,
    this.posterUrl,
    this.mediaType,
    this.initialSeason,
    this.initialEpisode,
    this.initialEpisodeTitle,
    this.streams,
    this.isOffline = false,
    bool setupPlayerStreams = true,
  }) : _historyService = historyService,
       _settingsService = settingsService,
       _watchPartyService = watchPartyService,
       _playerFactory = playerFactory,
       _setupPlayerStreams = setupPlayerStreams {
    _state = _state.copyWith(
      currentSeason: initialSeason,
      currentEpisode: initialEpisode,
      currentEpisodeTitle: initialEpisodeTitle,
    );
    _initCurrentStreamInfo();
    // Register window listener for fullscreen events
    windowManager.addListener(this);
  }

  // --- WindowListener Callbacks ---

  @override
  void onWindowEnterFullScreen() {
    if (_fullscreenTransitionCompleter != null &&
        !_fullscreenTransitionCompleter!.isCompleted) {
      Logger.d('Event received: Enter FullScreen', tag: _tag);
      _fullscreenTransitionCompleter!.complete();
    }
  }

  @override
  void onWindowLeaveFullScreen() {
    if (_fullscreenTransitionCompleter != null &&
        !_fullscreenTransitionCompleter!.isCompleted) {
      Logger.d('Event received: Leave FullScreen', tag: _tag);
      _fullscreenTransitionCompleter!.complete();
    }
  }

  // ... (existing _initCurrentStreamInfo and other methods)

  Future<void> toggleFullscreen() async {
    if (_isTogglingFullscreen) return; // Prevent re-entry
    _isTogglingFullscreen = true; // Block UI updates from didChangeMetrics

    Logger.d('Starting toggleFullscreen (Event Driven)', tag: _tag);

    // Initialize synchronization primitives
    _uiRenderedCompleter = Completer<void>();
    _fullscreenTransitionCompleter = Completer<void>();

    // 1. Signal UI to detach (hide video, show black placeholder)
    _state = _state.copyWith(isTransitioning: true);
    notifyListeners();

    // 2. Wait for UI to confirm it has rendered the black placeholder
    // This ensures the heavy video texture is removed from the scene graph.
    try {
      await _uiRenderedCompleter!.future.timeout(
        const Duration(milliseconds: 1000),
        onTimeout: () {
          Logger.w(
            'Timeout waiting for UI detachment (rendering placeholder)',
            tag: _tag,
          );
        },
      );
    } catch (e) {
      Logger.w('Error waiting for UI detachment: $e', tag: _tag);
    }

    final newFullscreenState = !_state.isFullscreen;
    final isDesktop =
        !kIsWeb &&
        (defaultTargetPlatform == TargetPlatform.windows ||
            defaultTargetPlatform == TargetPlatform.linux ||
            defaultTargetPlatform == TargetPlatform.macOS);

    try {
      if (isDesktop) {
        // 3. Perform window operation
        Logger.d(
          'Calling windowManager.setFullScreen($newFullscreenState)',
          tag: _tag,
        );
        await windowManager.setFullScreen(newFullscreenState);

        // Update internal state
        _state = _state.copyWith(isFullscreen: newFullscreenState);

        // 4. Wait for OS signal via WindowListener
        // This ensures the window has physically resized before we re-attach.
        try {
          await _fullscreenTransitionCompleter!.future.timeout(
            const Duration(milliseconds: 1000),
            onTimeout: () {
              Logger.w('Timeout waiting for OS fullscreen event', tag: _tag);
            },
          );
          // SAFETY BUFFER: Give the OS 100ms to finish visual animations/bounds changes.
          // Even if the event fired, the window rect might still be interpolated.
          await Future.delayed(const Duration(milliseconds: 100));
        } catch (e) {
          Logger.w('Error waiting for OS fullscreen event: $e', tag: _tag);
        }
      } else {
        // Mobile logic
        if (newFullscreenState) {
          await SystemChrome.setEnabledSystemUIMode(
            SystemUiMode.immersiveSticky,
          );
          await SystemChrome.setPreferredOrientations([
            DeviceOrientation.landscapeLeft,
            DeviceOrientation.landscapeRight,
          ]);
        } else {
          await SystemChrome.setEnabledSystemUIMode(SystemUiMode.edgeToEdge);
          await SystemChrome.setPreferredOrientations([
            DeviceOrientation.portraitUp,
            DeviceOrientation.portraitDown,
            DeviceOrientation.landscapeLeft,
            DeviceOrientation.landscapeRight,
          ]);
        }
        _state = _state.copyWith(isFullscreen: newFullscreenState);
      }
    } catch (e) {
      Logger.e('Fullscreen toggle failed', tag: _tag, error: e);
      // Rollback state in case of error
      _state = _state.copyWith(isFullscreen: !newFullscreenState);
    } finally {
      // 5. Restore UI - only after confirmed event
      Logger.d('Transition finished. Rendering video.', tag: _tag);

      _state = _state.copyWith(
        isTransitioning: false,
        textureKey: _state.textureKey + 1, // Force texture refresh
      );

      notifyListeners();

      // Cleanup
      _isTogglingFullscreen = false;
      _uiRenderedCompleter = null;
      _fullscreenTransitionCompleter = null;
    }
  }

  void _initCurrentStreamInfo() {
    var currentUrl = initialUrl;
    String? currentVoiceover;
    StreamQuality? currentQuality;

    if (streams != null && streams!.isNotEmpty) {
      // 1. Find initial stream (by URL) or default to first
      var current = streams!.firstWhere(
        (s) => s.url == currentUrl,
        orElse: () => streams!.first,
      );

      // 2. Apply Language Preference (if initial URL didn't specify one or we just defaulted)
      // If we are just starting (initialUrl is empty or just the first stream), try to find preferred language.
      // SettingsService has subtitleLanguage, but we can use it as a hint for audio too?
      // Or we can prefer 'ukr'/'ua' if the locale is 'uk'.
      final preferredLang =
          _settingsService.state.subtitleLanguage; // 'uk' by default
      if (preferredLang == 'uk' && (currentUrl.isEmpty)) {
        final ukrStream = streams!.firstWhereOrNull((s) {
          final v = s.voiceover?.toLowerCase() ?? '';
          final l = s.language?.toLowerCase() ?? '';
          return v.contains('ukr') ||
              v.contains('укр') ||
              v.contains('ua') ||
              l.contains('ukr') ||
              l.contains('ua');
        });
        if (ukrStream != null) {
          current = ukrStream;
          currentUrl = current.url;
        }
      }

      // 3. Apply Quality Preference
      final defaultQuality = _settingsService.state.defaultQuality;
      StreamQuality? targetQuality;

      switch (defaultQuality) {
        case DefaultQuality.q480p:
          targetQuality = StreamQuality.q480p;
          break;
        case DefaultQuality.q720p:
          targetQuality = StreamQuality.q720p;
          break;
        case DefaultQuality.q1080p:
          targetQuality = StreamQuality.q1080p;
          break;
        case DefaultQuality.q1440p:
          targetQuality = StreamQuality.q1440p;
          break;
        case DefaultQuality.auto:
          targetQuality = null;
          break;
      }

      if (targetQuality != null) {
        // Try to find target quality within the CURRENT voiceover first
        final preferredStream = streams!.firstWhere(
          (s) => s.quality == targetQuality && s.voiceover == current.voiceover,
          orElse: () => streams!.firstWhere(
            (s) => s.quality == targetQuality,
            orElse: () => current,
          ),
        );

        if (preferredStream != current) {
          current = preferredStream;
          currentUrl = current.url;
        }
      } else {
        // If Auto quality, and we are on a low quality stream, try to upgrade to best available
        // for this voiceover?
        // Or leave it as is. Usually "Auto" implies adaptive, but for static MP4s,
        // maybe we should pick 1080p/720p by default?
        // Let's pick the HIGHEST quality for the current voiceover if 'Auto' is selected
        // AND it's not HLS (if it's HLS, Auto means adaptive).

        if (current.type != StreamType.hls) {
          final bestQualityStream =
              streams!.where((s) => s.voiceover == current.voiceover).toList()
                ..sort(
                  (a, b) => b.quality.sortOrder.compareTo(a.quality.sortOrder),
                );

          if (bestQualityStream.isNotEmpty) {
            current = bestQualityStream.first;
            currentUrl = current.url;
          }
        }
      }

      currentVoiceover = current.voiceover;
      currentQuality = current.quality;
    }

    _state = _state.copyWith(
      currentUrl: currentUrl,
      currentVoiceover: currentVoiceover,
      currentQuality: currentQuality,
    );
  }

  /// Initialize player and start playback
  Future<void> initialize() async {
    _player = _playerFactory != null
        ? _playerFactory()
        : Player(
            configuration: const PlayerConfiguration(
              // Explicitly set log level to see MPV errors
              logLevel: MPVLogLevel.info,
            ),
          );

    // Set mpv options via the native property path already used for `hwdec`.
    if (_player.platform is NativePlayer) {
      final native = _player.platform as NativePlayer;

      // Enable auto-copy to decouple decoding from rendering (best for Windows
      // resizing).
      try {
        await native.setProperty('hwdec', 'auto-copy');
        Logger.d('Set hwdec: auto-copy (Safe Decoding Strategy)', tag: _tag);
      } catch (e) {
        Logger.w('Failed to set hwdec: $e', tag: _tag);
      }

      // CDN-facing defaults. Without a UA many CDNs answer with an HTML
      // challenge page, which libmpv then reports as
      // "Failed to recognize file format.".
      for (final option in <String, String>{
        'user-agent': _defaultUserAgent,
        // Seconds. mpv's default of 60 is far too long for a player that is
        // supposed to fail fast and offer a retry.
        'network-timeout': '15',
      }.entries) {
        try {
          await native.setProperty(option.key, option.value);
          Logger.d('Set ${option.key}: ${option.value}', tag: _tag);
        } catch (e) {
          Logger.w('Failed to set ${option.key}: $e', tag: _tag);
        }
      }

      // `referrer` is stream-specific; the real value is applied by
      // `_applyMpvReferrer` right before each open.
    }

    if (_setupPlayerStreams) {
      _videoController = VideoController(_player);
      Logger.d(
        'VideoController created: ${_videoController.hashCode}',
        tag: _tag,
      );

      _setupPlayerListeners();
      if (!isOffline) {
        _setupWatchPartySync();
      }
      _startProgressSaving();
    } else {
      // In test mode we skip creating VideoController and subscribing to streams.
      // Tests may still call player methods on the provided player instance.
    }

    // Enable wakelock and handle fullscreen only when player streams are
    // initialized (skip during unit tests where streams/platform channels are
    // not available).
    if (_setupPlayerStreams) {
      WakelockPlus.enable();

      // Auto fullscreen on Android & iOS
      if (!kIsWeb &&
          (defaultTargetPlatform == TargetPlatform.android ||
              defaultTargetPlatform == TargetPlatform.iOS)) {
        _state = _state.copyWith(isFullscreen: true);
        SystemChrome.setEnabledSystemUIMode(SystemUiMode.immersiveSticky);
        SystemChrome.setPreferredOrientations([
          DeviceOrientation.landscapeLeft,
          DeviceOrientation.landscapeRight,
        ]);
      }
    } else {
      // In test mode we avoid interacting with platform channels (wakelock,
      // fullscreen, etc.).
    }

    // Open media
    final currentStream =
        streams?.firstWhereOrNull((s) => s.url == _state.currentUrl) ??
        (streams != null && streams!.isNotEmpty
            ? streams!.first
            : StreamSource(url: _state.currentUrl));
    final initialHeaders = currentStream.headers ?? const <String, String>{};
    _applyMpvReferrer(_state.currentUrl);
    await _openMedia(
      _state.currentUrl,
      currentStream,
      initialHeaders,
      reason: 'initial',
    );

    // Resume from last position
    await _resumeLastPosition();

    // Initial sync for Watch Party client
    if (!isOffline &&
        _watchPartyService.state == WatchPartyState.connected &&
        !_watchPartyService.isHost) {
      _watchPartyService.requestSync();
      if (_watchPartyService.isPlaying) {
        _player.play();
      }
    }

    _state = _state.copyWith(isInitialized: true);
    notifyListeners();
  }

  void _setupPlayerListeners() {
    _subscriptions.add(
      _player.stream.playing.listen((playing) {
        if (_isDisposed) return;
        _state = _state.copyWith(isPlaying: playing);
        notifyListeners();

        // Toggle WakeLock based on playback state
        if (playing) {
          WakelockPlus.enable();
        } else {
          WakelockPlus.disable();
        }

        // Watch Party sync
        if (_watchPartyService.state == WatchPartyState.connected) {
          if (playing && !_watchPartyService.isPlaying) {
            _watchPartyService.play();
          } else if (!playing && _watchPartyService.isPlaying) {
            _watchPartyService.pause();
          }
        }
      }),
    );

    _subscriptions.add(
      _player.stream.completed.listen((completed) {
        if (_isDisposed) return;
        if (completed && _settingsService.state.autoPlayNext) {
          // Notify UI to pop with result
          _onPlaybackCompleted?.call();
        }
      }),
    );

    _subscriptions.add(
      _player.stream.position.listen((position) {
        if (_isDisposed) return;
        _state = _state.copyWith(position: position);
        // Do NOT notifyListeners() here to avoid rebuilding the whole UI 60fps
        positionNotifier.value = position;

        if (_watchPartyService.state == WatchPartyState.connected) {
          _watchPartyService.updateLocalPosition(position);
        }
      }),
    );

    _subscriptions.add(
      _player.stream.duration.listen((duration) {
        if (_isDisposed) return;
        _state = _state.copyWith(duration: duration);
        notifyListeners();
      }),
    );

    _subscriptions.add(
      _player.stream.buffering.listen((buffering) {
        if (_isDisposed) return;
        _state = _state.copyWith(isBuffering: buffering);
        notifyListeners();

        if (_watchPartyService.state == WatchPartyState.connected) {
          _watchPartyService.reportBuffering(buffering);
        }
      }),
    );

    _subscriptions.add(
      _player.stream.error.listen((error) {
        if (_isDisposed) return;
        if (error.isEmpty) return;

        final source = currentSource;
        final playbackError = PlaybackError.fromMpvMessage(
          error,
          providerId: providerId,
          url: _state.currentUrl,
          headersSent: _lastSentHeaders,
          streamType: source?.type ?? StreamType.direct,
          playerHost: _playerHostOf(_lastSentHeaders),
        );
        // ERROR level: playback failures must be visible in release builds.
        playbackError.log(tag: _tag);

        _state = _state.copyWith(
          hasError: true,
          error: playbackError,
          isBuffering: false,
        );
        notifyListeners();
      }),
    );

    _subscriptions.add(
      _player.stream.tracks.listen((tracks) {
        if (_isDisposed) return;
        _state = _state.copyWith(
          videoTracks: tracks.video,
          audioTracks: tracks.audio,
        );

        Logger.i(
          'Tracks detected: Audio=${tracks.audio.length}, Video=${tracks.video.length}, Subs=${tracks.subtitle.length}',
          tag: _tag,
        );
        for (var a in tracks.audio) {
          Logger.i(
            'Audio Track: ${a.id} - ${a.title} - ${a.language}',
            tag: _tag,
          );
        }

        notifyListeners();
      }),
    );

    _subscriptions.add(
      _player.stream.track.listen((track) {
        if (_isDisposed) return;
        _state = _state.copyWith(
          selectedVideoTrack: track.video,
          selectedAudioTrack: track.audio,
        );
        notifyListeners();
      }),
    );

    _subscriptions.add(
      _player.stream.volume.listen((volume) {
        if (_isDisposed) return;
        _state = _state.copyWith(volume: volume);
        notifyListeners();
      }),
    );
  }

  void _setupWatchPartySync() {
    if (_watchPartyService.state != WatchPartyState.connected) return;

    _watchPartyService.onPlayPauseChanged = (isPlaying) {
      if (_isDisposed) return;
      if (isPlaying) {
        if (_state.hasError) {
          Logger.i(
            'Auto-recovering from error due to Watch Party Play command',
            tag: _tag,
          );
          retryPlayback().then((_) => _player.play());
        } else if (!_player.state.playing) {
          _player.play();
        }
      } else if (!isPlaying && _player.state.playing) {
        _player.pause();
      }
    };

    _watchPartyService.onSeek = (position) {
      if (_isDisposed) return;
      if ((_player.state.position - position).abs() >
          const Duration(seconds: 2)) {
        _player.seek(position);
      }
    };

    _watchPartyService.onSpeedChanged = (speed) {
      if (_isDisposed) return;
      if ((_state.playbackSpeed - speed).abs() > 0.01) {
        _state = _state.copyWith(playbackSpeed: speed);
        _player.setRate(speed);
        notifyListeners();
      }
    };

    _watchPartyService.onSyncStatusChanged = (mode, driftMs) {
      if (_isDisposed) return;
      notifyListeners();
    };

    _watchPartyService.onQualityAdjustRequested = (delta) {
      if (_isDisposed) return;
      if (delta < 0) {
        tryReduceQuality();
      }
    };
  }

  void _startProgressSaving() {
    _saveProgressTimer = Timer.periodic(
      const Duration(seconds: 10),
      (_) => unawaited(_saveProgressGuarded()),
    );
  }

  /// Single-flight wrapper around [_saveProgress] for the periodic timer.
  Future<void> _saveProgressGuarded() async {
    if (_isDisposed || _saveInFlight) return;
    _saveInFlight = true;
    try {
      await _saveProgress();
    } finally {
      _saveInFlight = false;
    }
  }

  Future<void> _saveProgress() async {
    // For local/offline files:
    // If [isOffline] is true OR URL starts with file://, use mediaId/providerId from params
    // If it's a regular online play, use whatever we have.
    final isLocalFile = isOffline || initialUrl.startsWith('file://');
    final effectiveMediaId = mediaId;
    final effectiveProviderId = providerId;

    if (effectiveMediaId == null || effectiveProviderId == null) return;
    if (_state.duration.inSeconds < 1) return;

    try {
      await _historyService.saveProgress(
        mediaId: effectiveMediaId,
        providerId: effectiveProviderId,
        title: title ?? 'Невідомо',
        posterUrl: posterUrl,
        mediaType: isLocalFile ? 'local_video' : 'movie',
        position: _state.position,
        duration: _state.duration,
        lastStreamUrl: _state.currentUrl,
        voiceover: _state.currentVoiceover,
      );
    } catch (e) {
      Logger.w('Failed to save progress: $e', tag: _tag);
    }
  }

  Future<void> _resumeLastPosition() async {
    if (!_settingsService.state.rememberPosition) return;

    final effectiveMediaId = mediaId;
    final effectiveProviderId = providerId;

    if (effectiveMediaId == null || effectiveProviderId == null) return;

    try {
      final lastPosition = await _historyService.getLastPosition(
        effectiveMediaId,
        effectiveProviderId,
      );

      if (lastPosition != null && lastPosition.inSeconds > 5) {
        // Wait for duration to be known
        int attempts = 0;
        while (_state.duration == Duration.zero && attempts < 20) {
          await Future.delayed(const Duration(milliseconds: 250));
          attempts++;
        }

        final expectedDuration = _state.duration.inSeconds > 0
            ? _state.duration
            : const Duration(minutes: 90);

        // Safety check: ensure we don't resume if the video is significantly shorter
        // than the saved position (e.g. wrong file or different version)
        if (lastPosition > expectedDuration) {
          Logger.w(
            'Saved position ($lastPosition) > duration ($expectedDuration). Ignoring.',
            tag: _tag,
          );
          return;
        }

        final progress = lastPosition.inSeconds / expectedDuration.inSeconds;

        if (progress < 0.95) {
          Logger.i('Resuming at $lastPosition', tag: _tag);
          await _player.seek(lastPosition);
          _onPositionResumed?.call(lastPosition);
        }
      }
    } catch (e) {
      Logger.w('Failed to resume position: $e', tag: _tag);
    }
  }

  // =========================================================================
  // PUBLIC API
  // =========================================================================

  /// Callbacks for UI events
  VoidCallback? _onPlaybackCompleted;
  void Function(Duration)? _onPositionResumed;
  void Function(String)? _onQualityReduced;

  set onPlaybackCompleted(VoidCallback? callback) =>
      _onPlaybackCompleted = callback;
  set onPositionResumed(void Function(Duration)? callback) =>
      _onPositionResumed = callback;
  set onQualityReduced(void Function(String)? callback) =>
      _onQualityReduced = callback;

  void playOrPause() {
    // Prefer controller's own state when deciding play/pause to avoid
    // depending on native Player.state during unit tests.
    final wasPlaying = _state.isPlaying;
    _player.playOrPause();

    if (_watchPartyService.state == WatchPartyState.connected) {
      if (wasPlaying) {
        _watchPartyService.pause();
      } else {
        _watchPartyService.play();
      }
    }
  }

  void seek(Duration position) {
    _player.seek(position);
    if (_watchPartyService.state == WatchPartyState.connected) {
      _watchPartyService.seek(position);
    }
  }

  void seekForward([Duration duration = const Duration(seconds: 10)]) {
    seek(_state.position + duration);
  }

  void seekBackward([Duration duration = const Duration(seconds: 10)]) {
    seek(_state.position - duration);
  }

  bool hasMultipleAudioTracks() {
    return _state.audioTracks.length > 1;
  }

  void setAudioTrack(AudioTrack track) {
    Logger.i(
      'Setting audio track: ${track.id} - ${track.title} - ${track.language}',
      tag: _tag,
    );
    _player.setAudioTrack(track);
  }

  void setSpeed(double speed) {
    Logger.d('Setting playback speed: $speed', tag: _tag);
    _state = _state.copyWith(playbackSpeed: speed);
    _player.setRate(speed);
    notifyListeners();

    if (_watchPartyService.state == WatchPartyState.connected) {
      if (_watchPartyService.playbackSpeed != speed) {
        _watchPartyService.setSpeed(speed);
      }
    }
  }

  void setVolume(double volume) {
    _player.setVolume(volume);
  }

  void cycleFit() {
    final currentIndex = fits.indexOf(_state.videoFit);
    final nextIndex = (currentIndex + 1) % fits.length;
    _state = _state.copyWith(videoFit: fits[nextIndex]);
    notifyListeners();
  }

  /// Called by UI Frame Callback when it has finished building with the new state.
  void notifyUIUpdated() {
    if (_uiRenderedCompleter != null && !_uiRenderedCompleter!.isCompleted) {
      // Only complete if we are in the "hiding" phase.
      // The logic in toggleFullscreen sets isTransitioning=true, then waits for this.
      if (_state.isTransitioning) {
        Logger.d('UI Updated Signal Received (Video Hidden)', tag: _tag);
        _uiRenderedCompleter!.complete();
      }
    }
  }

  /// Force a frame refresh by doing a micro-seek.
  /// Call this if video freezes but audio continues.
  Future<void> forceTextureRefresh() async {
    if (!_setupPlayerStreams) return;
    Logger.d(
      'Forcing frame refresh via texture key increment and seek',
      tag: _tag,
    );

    _state = _state.copyWith(textureKey: _state.textureKey + 1);
    notifyListeners();

    final currentPos = _state.position;
    await _player.seek(currentPos);
    Logger.d('Frame refresh completed', tag: _tag);
  }

  Future<void> switchStream(StreamSource stream) async {
    Logger.i(
      'Switching stream to: ${stream.quality.displayName} | ${stream.voiceover} | ${stream.url}',
      tag: _tag,
    );

    // Only return if BOTH url and voiceover are the same
    if (stream.url == _state.currentUrl &&
        stream.voiceover == _state.currentVoiceover) {
      Logger.d(
        'Stream URL and voiceover match current, skipping switch',
        tag: _tag,
      );
      return;
    }

    Logger.i(
      'SWITCHING STREAM: Voiceover="${stream.voiceover}", URL="${stream.url}"',
      tag: _tag,
    );

    final currentPosition = _state.position;
    _state = _state.copyWith(
      currentUrl: stream.url,
      currentVoiceover: stream.voiceover,
      currentQuality: stream.quality,
      isBuffering: true,
    );
    notifyListeners();

    _applyMpvReferrer(stream.url);
    await _openMedia(
      stream.url,
      stream,
      stream.headers ?? const <String, String>{},
      reason: 'switch',
    );
    await _player.seek(currentPosition);
  }

  Future<void> setVideoTrack(VideoTrack track) async {
    Logger.i(
      'Selecting video track: ${track.id} (${track.w}x${track.h})',
      tag: _tag,
    );
    await _player.setVideoTrack(track);
  }

  /// Re-resolves the [StreamSource] backing [url] from the current streams.
  ///
  /// This is what makes retry behave like `switchStream`: without it the URL
  /// was reopened with no `httpHeaders` at all, silently stripping the
  /// Referer/User-Agent that the CDN required.
  StreamSource? _resolveSourceFor(String url) {
    if (streams == null || streams!.isEmpty) return null;
    return streams!.firstWhereOrNull((s) => s.url == url);
  }

  /// Re-resolves the stream, applies the first promising header set and opens.
  ///
  /// Escalates through [_buildRetryLadder] until playback starts or the
  /// ladder is exhausted; the winning rung is remembered so subsequent
  /// retries are a single open instead of the whole walk.
  Future<void> retryPlayback() async {
    _state = _state.copyWith(clearError: true, isBuffering: true);
    notifyListeners();

    final positionToSeek = _state.position;
    final url = _state.currentUrl;
    final source = _resolveSourceFor(url);
    final ladder = _buildRetryLadder(url, source?.headers);

    // Resume from the rung that worked before, if any.
    final start = _winningHeaderStep >= 0 ? _winningHeaderStep : 0;

    for (var step = start; step < ladder.length; step++) {
      if (_isDisposed) return;

      _applyMpvReferrer(url);
      await _openMedia(url, source, ladder[step], reason: 'retry:$step');

      if (await _waitForPlaybackStart()) {
        if (step != _winningHeaderStep) {
          Logger.i(
            'Retry ladder: step $step succeeded (headers=${ladder[step].isEmpty ? '<none>' : ladder[step].keys.join(",")})',
            tag: _tag,
          );
          _winningHeaderStep = step;
        }
        if (positionToSeek > Duration.zero) {
          await _player.seek(positionToSeek);
        }
        return;
      }

      Logger.w('Retry ladder: step $step failed, escalating', tag: _tag);
    }

    Logger.e(
      'Retry ladder exhausted for $url after ${ladder.length} attempts',
      tag: _tag,
    );
  }

  /// Waits briefly to see whether the just-opened media actually starts.
  ///
  /// Returns true as soon as we see evidence of playback, false if libmpv
  /// reports an error or nothing happens within the window.
  Future<bool> _waitForPlaybackStart() async {
    if (!_setupPlayerStreams) return true;

    final deadline = DateTime.now().add(const Duration(seconds: 4));
    while (!_isDisposed && DateTime.now().isBefore(deadline)) {
      await Future<void>.delayed(const Duration(milliseconds: 150));
      if (_state.hasError) return false;
      if (_state.position > Duration.zero) return true;
      // A track list implies the demuxer accepted the container.
      if (_state.videoTracks.isNotEmpty || _state.audioTracks.isNotEmpty) {
        return true;
      }
      if (_state.duration > Duration.zero) return true;
    }
    // Timed out without an explicit error: treat as success so we do not
    // burn the whole ladder on a slow-but-working CDN.
    return !_state.hasError;
  }

  Future<void> switchStreamWithRetry(StreamSource stream) async {
    Logger.i('Switching stream with retry: ${stream.url}', tag: _tag);
    _state = _state.copyWith(
      clearError: true,
      isBuffering: true,
      currentUrl: stream.url,
      currentVoiceover: stream.voiceover,
      currentQuality: stream.quality,
    );
    notifyListeners();

    _applyMpvReferrer(stream.url);
    await _openMedia(
      stream.url,
      stream,
      stream.headers ?? const <String, String>{},
      reason: 'switch-retry',
    );
  }

  /// Try to reduce quality when buffering issues detected
  void tryReduceQuality() {
    // Try HLS tracks first
    if (_state.videoTracks.length > 1 && _state.selectedVideoTrack != null) {
      final sortedTracks = List<VideoTrack>.from(_state.videoTracks)
        ..sort((a, b) => (b.h ?? 0).compareTo(a.h ?? 0));

      final currentIndex = sortedTracks.indexOf(_state.selectedVideoTrack!);
      if (currentIndex < sortedTracks.length - 1) {
        final lowerTrack = sortedTracks[currentIndex + 1];
        Logger.i(
          'Reducing quality to ${lowerTrack.h}p due to buffering',
          tag: _tag,
        );
        _player.setVideoTrack(lowerTrack);
        _onQualityReduced?.call('${lowerTrack.h}p');
        return;
      }
    }

    // Try stream sources
    if (streams != null && streams!.length > 1) {
      final sortedStreams = List<StreamSource>.from(streams!)
        ..sort((a, b) => b.quality.sortOrder.compareTo(a.quality.sortOrder));

      final currentIndex = sortedStreams.indexWhere(
        (s) => s.url == _state.currentUrl,
      );
      if (currentIndex >= 0 && currentIndex < sortedStreams.length - 1) {
        final lowerStream = sortedStreams[currentIndex + 1];
        Logger.i(
          'Reducing quality to ${lowerStream.quality.displayName} due to buffering',
          tag: _tag,
        );
        switchStream(lowerStream);
        _onQualityReduced?.call(lowerStream.quality.displayName);
      }
    }
  }

  // =========================================================================
  // HELPER METHODS
  // =========================================================================

  bool hasMultipleQualities() {
    if (_state.videoTracks.length > 1) return true;
    if (streams == null || streams!.length <= 1) return false;
    final qualities = streams!.map((s) => s.quality).toSet();
    return qualities.length > 1;
  }

  bool hasMultipleVoiceovers() {
    if (streams == null || streams!.length <= 1) return false;
    final voiceovers = streams!
        .where((s) => s.voiceover != null)
        .map((s) => s.voiceover)
        .toSet();
    return voiceovers.length > 1;
  }

  /// For tests: set available video tracks and currently selected track
  @visibleForTesting
  void setVideoTracksForTest(List<VideoTrack> tracks, VideoTrack selected) {
    _state = _state.copyWith(videoTracks: tracks, selectedVideoTrack: selected);
  }

  /// For tests: drive position/duration without a real media backend.
  @visibleForTesting
  void setPlaybackForTest({Duration? position, Duration? duration}) {
    _state = _state.copyWith(position: position, duration: duration);
  }

  /// For tests: run one periodic-saver tick through the single-flight guard.
  @visibleForTesting
  Future<void> tickProgressSaveForTest() => _saveProgressGuarded();

  /// For tests: whether a periodic save is currently in flight.
  @visibleForTesting
  bool get saveInFlightForTest => _saveInFlight;

  /// For tests: whether [dispose] has already run.
  @visibleForTesting
  bool get disposedForTest => _isDisposed;

  String buildSubtitleText(String? fallbackSubtitle) {
    final parts = <String>[];
    if (_state.currentQuality != null) {
      if (_state.selectedVideoTrack !=
              null && // Keep this null check as selectedVideoTrack can be null
          _state.videoTracks.length > 1 &&
          _state.selectedVideoTrack!.w != null &&
          _state.selectedVideoTrack!.h != null) {
        parts.add('${_state.selectedVideoTrack!.h}p');
      } else if (_state.currentQuality != null) {
        parts.add(_state.currentQuality!.displayName);
      }
    }
    if (_state.currentVoiceover != null) {
      parts.add(_state.currentVoiceover!);
    }
    if (parts.isEmpty && fallbackSubtitle != null) {
      return fallbackSubtitle;
    }
    return parts.join(' • ');
  }

  WatchPartyService get watchPartyService => _watchPartyService;
  SettingsService get settingsService => _settingsService;

  @override
  void dispose() {
    windowManager.removeListener(this);
    if (_isDisposed) return;
    _isDisposed = true;

    // Cancel subscriptions
    for (final sub in _subscriptions) {
      sub.cancel();
    }
    _subscriptions.clear();

    // Clear Watch Party callbacks
    _watchPartyService.onPlayPauseChanged = null;
    _watchPartyService.onSeek = null;
    _watchPartyService.onSpeedChanged = null;
    _watchPartyService.onSyncStatusChanged = null;
    _watchPartyService.onQualityAdjustRequested = null;

    // Disable wakelock
    WakelockPlus.disable();

    // Stop progress saving
    _saveProgressTimer?.cancel();
    // Deliberately bypasses [_saveProgressGuarded]: this is the last chance to
    // persist the position and the guard drops everything once _isDisposed.
    unawaited(_saveProgress());

    // Stop player safely
    try {
      _player.stop();
      _player.dispose();
    } catch (e) {
      Logger.w('Error disposing player: $e', tag: _tag);
    }

    // Restore orientation on Android & iOS
    if (!kIsWeb &&
        (defaultTargetPlatform == TargetPlatform.android ||
            defaultTargetPlatform == TargetPlatform.iOS)) {
      SystemChrome.setEnabledSystemUIMode(SystemUiMode.edgeToEdge);
      SystemChrome.setPreferredOrientations([
        DeviceOrientation.portraitUp,
        DeviceOrientation.portraitDown,
        DeviceOrientation.landscapeLeft,
        DeviceOrientation.landscapeRight,
      ]);
    }

    super.dispose();
  }
}
