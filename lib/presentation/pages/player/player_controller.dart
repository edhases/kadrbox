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

/// Player state that can be observed by UI
class PlayerState {
  final bool isInitialized;
  final bool isBuffering;
  final bool isPlaying;
  final bool hasError;
  final String? errorMessage;
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
    this.errorMessage,
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

  PlayerState copyWith({
    bool? isInitialized,
    bool? isBuffering,
    bool? isPlaying,
    bool? hasError,
    String? errorMessage,
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
      errorMessage: clearError ? null : (errorMessage ?? this.errorMessage),
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
  final List<StreamSubscription> _subscriptions = [];
  Timer? _saveProgressTimer;
  bool _isDisposed = false;

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

    // Set hardware decoding mode via native property
    if (_player.platform is NativePlayer) {
      try {
        // Enable auto-copy to decouple decoding from rendering (best for Windows resizing)
        await (_player.platform as NativePlayer).setProperty(
          'hwdec',
          'auto-copy',
        );
        Logger.d('Set hwdec: auto-copy (Safe Decoding Strategy)', tag: _tag);
      } catch (e) {
        Logger.w('Failed to set hwdec: $e', tag: _tag);
      }
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
    final currentStream = streams?.firstWhere(
      (s) => s.url == _state.currentUrl,
      orElse: () => StreamSource(url: _state.currentUrl),
    );
    await _player.open(
      Media(_state.currentUrl, httpHeaders: currentStream?.headers),
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
        if (error.isNotEmpty) {
          Logger.w('Player error: $error', tag: _tag);
          _state = _state.copyWith(
            hasError: true,
            errorMessage: error,
            isBuffering: false,
          );
          notifyListeners();
        }
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
      (_) => _saveProgress(),
    );
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

    await _player.open(Media(stream.url, httpHeaders: stream.headers));
    await _player.seek(currentPosition);
  }

  Future<void> setVideoTrack(VideoTrack track) async {
    Logger.i(
      'Selecting video track: ${track.id} (${track.w}x${track.h})',
      tag: _tag,
    );
    await _player.setVideoTrack(track);
  }

  Future<void> retryPlayback() async {
    _state = _state.copyWith(clearError: true, isBuffering: true);
    notifyListeners();

    final positionToSeek = _state.position;
    await _player.open(Media(_state.currentUrl));
    if (positionToSeek > Duration.zero) {
      await _player.seek(positionToSeek);
    }
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

    await _player.open(Media(stream.url, httpHeaders: stream.headers));
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
    _saveProgress();

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
