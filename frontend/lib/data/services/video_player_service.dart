import 'package:flutter/foundation.dart';
import 'package:flutter/material.dart';
import 'package:flutter/scheduler.dart';
import 'package:flutter/services.dart';
import 'package:get_it/get_it.dart';
import 'package:window_manager/window_manager.dart';
import 'package:oxide_film/domain/entities/entities.dart';
import 'package:oxide_film/presentation/pages/player/player_controller.dart';
import 'package:oxide_film/data/services/history_service.dart';
import 'package:oxide_film/data/services/settings_service.dart';
import 'package:oxide_film/data/services/watch_party_service.dart';

enum MiniPlayerState { hidden, minimized, fullscreen }

class VideoPlayerService extends ChangeNotifier {
  static const _platform = MethodChannel('com.oxidefilm.oxide_film/pip');

  PlayerController? _controller;
  MiniPlayerState _miniPlayerState = MiniPlayerState.hidden;
  bool _isNativePiP = false;
  bool _isDesktopPiP = false;
  Rect? _windowBoundsBeforePiP;

  VideoPlayerService() {
    _platform.setMethodCallHandler(_handlePlatformCall);
  }

  PlayerController? get controller => _controller;
  MiniPlayerState get miniPlayerState => _miniPlayerState;
  bool get isNativePiP => _isNativePiP || _isDesktopPiP;
  bool get isDesktopPiP => _isDesktopPiP;

  bool get isPlaying => _controller?.state.isPlaying ?? false;

  /// Opens a media item and initializes the player
  Future<void> open({
    required String url,
    String? title,
    String? mediaId,
    String? providerId,
    String? posterUrl,
    ContentType? mediaType,
    int? season,
    int? episode,
    String? episodeTitle,
    List<StreamSource>? streams,
    bool isOffline = false,
  }) async {
    final newController = PlayerController(
      initialUrl: url,
      historyService: GetIt.I<HistoryService>(),
      settingsService: GetIt.I<SettingsService>(),
      watchPartyService: GetIt.I<WatchPartyService>(),
      title: title,
      streams: streams,
      mediaId: mediaId,
      providerId: providerId,
      posterUrl: posterUrl,
      mediaType: mediaType,
      initialSeason: season,
      initialEpisode: episode,
      initialEpisodeTitle: episodeTitle,
      isOffline: isOffline,
    );

    setController(newController);

    await _controller!.initialize();

    _controller!.addListener(_safeNotifyListeners);
  }

  void setController(PlayerController controller) {
    if (_controller == controller) return;
    if (_controller != null) {
      _controller!.removeListener(_safeNotifyListeners);
      _controller!.dispose();
    }
    _controller = controller;
    _miniPlayerState = MiniPlayerState.fullscreen;
    _safeNotifyListeners();
    _enableAutoPiPIfSupported(true);
  }

  /// Minimizes the player to a floating window
  void minimize() {
    if (_controller == null) return;
    _enableAutoPiPIfSupported(false);
    _miniPlayerState = MiniPlayerState.minimized;
    _safeNotifyListeners();
  }

  /// Maximizes the player back to fullscreen (usually triggers navigation to PlayerPage)
  Future<void> maximize() async {
    if (_controller == null) return;

    // Reset flag immediately to restore UI responsiveness
    // even if window resizing takes time or fails.
    final wasDesktopPiP = _isDesktopPiP;
    _isDesktopPiP = false;
    _miniPlayerState = MiniPlayerState.fullscreen;
    _safeNotifyListeners();

    if (wasDesktopPiP && !kIsWeb) {
      final isDesktop =
          defaultTargetPlatform == TargetPlatform.windows ||
          defaultTargetPlatform == TargetPlatform.linux ||
          defaultTargetPlatform == TargetPlatform.macOS;
      if (isDesktop) {
        try {
          await windowManager.setAlwaysOnTop(false);
          await windowManager.setMinimumSize(const Size(800, 600));
          if (_windowBoundsBeforePiP != null) {
            await windowManager.setBounds(_windowBoundsBeforePiP!);
            _windowBoundsBeforePiP = null;
          }
        } catch (e) {
          debugPrint("Failed to maximize window: $e");
        }
      }
    }

    _enableAutoPiPIfSupported(true);
  }

  /// Closes the player and clears state
  Future<void> close() async {
    _enableAutoPiPIfSupported(false);
    _miniPlayerState = MiniPlayerState.hidden;
    notifyListeners();

    if (_controller != null) {
      _controller!.removeListener(_safeNotifyListeners);
      _controller!.dispose();
      _controller = null;
    }
  }

  Future<void> enterNativePiP() async {
    final isDesktop =
        !kIsWeb &&
        (defaultTargetPlatform == TargetPlatform.windows ||
            defaultTargetPlatform == TargetPlatform.linux ||
            defaultTargetPlatform == TargetPlatform.macOS);

    if (isDesktop) {
      await _enterDesktopPiP();
      return;
    }

    try {
      if (!kIsWeb && defaultTargetPlatform == TargetPlatform.android) {
        await _platform.invokeMethod('enterPiP');
      }
    } on PlatformException catch (e) {
      debugPrint("Failed to enter PiP: '${e.message}'.");
    }
  }

  void _safeNotifyListeners() {
    if (SchedulerBinding.instance.schedulerPhase ==
        SchedulerPhase.persistentCallbacks) {
      SchedulerBinding.instance.addPostFrameCallback((_) => notifyListeners());
    } else {
      notifyListeners();
    }
  }

  Future<void> _enterDesktopPiP() async {
    if (kIsWeb) return;
    try {
      _windowBoundsBeforePiP = await windowManager.getBounds();
      await windowManager.setMinimumSize(Size.zero);
      await windowManager.setSize(const Size(480, 270));
      await windowManager.setAlignment(Alignment.bottomRight);
      await windowManager.setAlwaysOnTop(true);
      // Let's keep current position but resize.
      _isDesktopPiP = true;
      _miniPlayerState = MiniPlayerState.minimized;
      _safeNotifyListeners();
    } catch (e) {
      debugPrint("Failed to enter Desktop PiP: $e");
    }
  }

  Future<void> enableAutoPiP(bool enable) async {
    await _enableAutoPiPIfSupported(enable);
  }

  Future<void> _enableAutoPiPIfSupported(bool enable) async {
    if (kIsWeb || defaultTargetPlatform != TargetPlatform.android) return;
    try {
      await _platform.invokeMethod('setAutoEnterEnabled', enable);
    } on PlatformException catch (e) {
      debugPrint("Failed to set auto PiP: '${e.message}'.");
    }
  }

  Future<dynamic> _handlePlatformCall(MethodCall call) async {
    switch (call.method) {
      case 'onPiPStatusChanged':
        _isNativePiP = call.arguments as bool;
        notifyListeners();
        break;
    }
  }
}
