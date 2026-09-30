import 'dart:async';
import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:get_it/get_it.dart';
import 'package:go_router/go_router.dart';
import 'package:media_kit_video/media_kit_video.dart';

import '../../../core/utils/logger.dart';
import '../../../data/services/video_player_service.dart';
import '../../../data/services/watch_party_service.dart';
import '../../../domain/entities/entities.dart';
import 'player_chat_overlay.dart';
import 'player_controller.dart';
import 'player_controls.dart';
import 'player_gesture_layer.dart';
import 'playback_error.dart';
import '../../widgets/player/pip_controls.dart';
import '../../widgets/common/app_error_widget.dart';

/// Video player page with full controls
class PlayerPage extends StatefulWidget {
  final String url;
  final String? title;
  final String? subtitle;
  final List<StreamSource>? streams;
  final String? mediaId;
  final String? providerId;
  final String? posterUrl;
  final ContentType? mediaType;
  final int? season;
  final int? episode;
  final String? episodeTitle;
  final bool isOffline;

  const PlayerPage({
    super.key,
    required this.url,
    this.title,
    this.subtitle,
    this.streams,
    this.mediaId,
    this.providerId,
    this.posterUrl,
    this.mediaType,
    this.season,
    this.episode,
    this.episodeTitle,
    this.isOffline = false,
  });

  @override
  State<PlayerPage> createState() => _PlayerPageState();
}

class _PlayerPageState extends State<PlayerPage> with WidgetsBindingObserver {
  late final VideoPlayerService _videoPlayerService;
  final FocusNode _focusNode = FocusNode();

  // UI State managed by Page, not Controller (purely visual toggles)
  bool _showControls = true;
  bool _showChat = false;
  int _newChatMessages = 0;
  int _lastSeenMessageCount = 0;
  bool _shouldMinimize = true;
  Timer? _hideControlsTimer; // Timer for auto-hiding controls
  DateTime? _lastManualHideTime; // Cooldown for auto-showing

  @override
  void initState() {
    super.initState();
    WidgetsBinding.instance.addObserver(this);
    _videoPlayerService = GetIt.I<VideoPlayerService>();
    _videoPlayerService.addListener(_onServiceUpdate);

    _initPlayer();

    _focusNode.requestFocus();
    _scheduleHideControls();
  }

  Future<void> _initPlayer() async {
    final currentController = _videoPlayerService.controller;

    // Check if we can reuse the existing controller
    if (currentController != null &&
        currentController.mediaId == widget.mediaId &&
        currentController.providerId == widget.providerId &&
        currentController.initialUrl == widget.url) {
      // Already playing this content, just maximize
      _videoPlayerService.maximize();
      _setupControllerCallbacks(currentController);
    } else {
      // New content
      await _videoPlayerService.open(
        url: widget.url,
        title: widget.title,
        streams: widget.streams,
        mediaId: widget.mediaId,
        providerId: widget.providerId,
        posterUrl: widget.posterUrl,
        mediaType: widget.mediaType,
        season: widget.season,
        episode: widget.episode,
        episodeTitle: widget.episodeTitle,
        isOffline: widget.isOffline,
      );

      if (_videoPlayerService.controller != null) {
        _setupControllerCallbacks(_videoPlayerService.controller!);
      }
    }
  }

  void _setupControllerCallbacks(PlayerController controller) {
    controller.onPlaybackCompleted = () {
      _shouldMinimize = false;
      _videoPlayerService.close();
      if (mounted) context.pop(true);
    };

    controller.onPositionResumed = (pos) {
      if (mounted) {
        ScaffoldMessenger.of(context).showSnackBar(
          SnackBar(
            content: Text('Продовжено з ${_formatDuration(pos)}'),
            duration: const Duration(seconds: 2),
          ),
        );
      }
    };

    controller.onQualityReduced = (quality) {
      if (mounted) {
        ScaffoldMessenger.of(context).showSnackBar(
          SnackBar(
            content: Text('Якість знижено до $quality'),
            duration: const Duration(seconds: 2),
          ),
        );
      }
    };

    // Watch Party Listener for Chat
    controller.watchPartyService.addListener(_onWatchPartyUpdate);
  }

  void _onServiceUpdate() {
    if (mounted) setState(() {});
  }

  @override
  void didChangeMetrics() {
    Logger.d('PlayerPage: didChangeMetrics called', tag: 'PlayerPage');

    final controller = _videoPlayerService.controller;
    if (controller == null) return;

    // FLOOD PREVENTION: Skip heavy rebuilds while the window is animating
    if (controller.isTogglingFullscreen) {
      Logger.d(
        'PlayerPage: Skipping rebuild during fullscreen transition',
        tag: 'PlayerPage',
      );
      return;
    }

    if (mounted) {
      WidgetsBinding.instance.addPostFrameCallback((_) {
        if (mounted) {
          Logger.d(
            'PlayerPage: postFrameCallback triggered rebuild',
            tag: 'PlayerPage',
          );
          setState(() {});
        }
      });
    }
    super.didChangeMetrics();
  }

  void _onWatchPartyUpdate() {
    if (!mounted) return;
    final controller = _videoPlayerService.controller;
    if (controller == null) return;

    // Track new messages when chat is closed
    final totalMessages = controller.watchPartyService.chatMessages.length;
    if (!_showChat && totalMessages > _lastSeenMessageCount) {
      if (mounted) {
        setState(() {
          _newChatMessages = totalMessages - _lastSeenMessageCount;
        });
      }
    }
  }

  void _toggleControls() {
    _hideControlsTimer?.cancel();
    setState(() {
      _showControls = !_showControls;
      if (!_showControls) {
        _lastManualHideTime = DateTime.now();
      } else {
        _lastManualHideTime = null; // Reset when manually shown
      }
    });

    if (_showControls) {
      _scheduleHideControls();
    }
  }

  void _scheduleHideControls() {
    _hideControlsTimer?.cancel();
    _hideControlsTimer = Timer(const Duration(seconds: 5), () {
      final controller = _videoPlayerService.controller;
      if (mounted &&
          _showControls &&
          controller != null &&
          !controller.state.isBuffering) {
        if (controller.state.isPlaying) {
          setState(() => _showControls = false);
        }
      }
    });
  }

  void _toggleChatOverlay() {
    final controller = _videoPlayerService.controller;
    if (controller == null) return;

    setState(() {
      _showChat = !_showChat;
      if (_showChat) {
        _newChatMessages = 0;
        _lastSeenMessageCount =
            controller.watchPartyService.chatMessages.length;
      }
    });
  }

  @override
  void dispose() {
    _hideControlsTimer?.cancel();
    WidgetsBinding.instance.removeObserver(this);
    _videoPlayerService.removeListener(_onServiceUpdate);

    final controller = _videoPlayerService.controller;
    if (controller != null) {
      controller.watchPartyService.removeListener(_onWatchPartyUpdate);

      // Leave Watch Party room when exiting player
      if (controller.watchPartyService.state == WatchPartyState.connected ||
          controller.watchPartyService.state == WatchPartyState.hosting) {
        controller.watchPartyService.leaveRoom();
      }
    }

    // Minimize player instead of disposing
    if (_shouldMinimize) {
      _videoPlayerService.minimize();
    }

    _focusNode.dispose();
    super.dispose();
  }

  void _handleKeyEvent(KeyEvent event) {
    final controller = _videoPlayerService.controller;
    if (controller == null) return;

    final FocusNode? currentFocus = FocusManager.instance.primaryFocus;
    if (_showChat ||
        (currentFocus != null &&
            currentFocus.context?.widget is EditableText)) {
      return;
    }

    if (event is KeyDownEvent) {
      switch (event.logicalKey) {
        case LogicalKeyboardKey.space:
        case LogicalKeyboardKey.enter:
          controller.playOrPause();
          _showControlsTemp();
          break;
        case LogicalKeyboardKey.arrowRight:
          controller.seekForward();
          _showControlsTemp();
          break;
        case LogicalKeyboardKey.arrowLeft:
          controller.seekBackward();
          _showControlsTemp();
          break;
        case LogicalKeyboardKey.keyJ:
          controller.seekBackward();
          _showControlsTemp();
          break;
        case LogicalKeyboardKey.keyK:
          controller.playOrPause();
          _showControlsTemp();
          break;
        case LogicalKeyboardKey.keyL:
          controller.seekForward();
          _showControlsTemp();
          break;
        case LogicalKeyboardKey.escape:
          if (controller.state.isFullscreen) {
            controller.toggleFullscreen();
          } else {
            context.pop();
          }
          break;
      }
    }
  }

  void _showControlsTemp() {
    if (!_showControls) {
      // If manually hidden, only show again automatically after 3 seconds
      if (_lastManualHideTime != null) {
        final now = DateTime.now();
        if (now.difference(_lastManualHideTime!) < const Duration(seconds: 3)) {
          return;
        }
      }
      setState(() => _showControls = true);
    }
    _scheduleHideControls();
  }

  String _formatDuration(Duration duration) {
    String twoDigits(int n) => n.toString().padLeft(2, '0');
    final hours = duration.inHours;
    final minutes = duration.inMinutes.remainder(60);
    final seconds = duration.inSeconds.remainder(60);
    return hours > 0
        ? '$hours:${twoDigits(minutes)}:${twoDigits(seconds)}'
        : '${twoDigits(minutes)}:${twoDigits(seconds)}';
  }

  @override
  Widget build(BuildContext context) {
    final controller = _videoPlayerService.controller;

    if (controller == null) {
      return const Scaffold(
        backgroundColor: Colors.black,
        body: Center(child: CircularProgressIndicator()),
      );
    }

    Logger.d(
      'PlayerPage: build called. isFullscreen: ${controller.state.isFullscreen}',
      tag: 'PlayerPage',
    );
    return PopScope(
      canPop: false,
      onPopInvokedWithResult: (didPop, result) async {
        if (didPop) return;

        if (controller.state.isFullscreen) {
          await controller.toggleFullscreen();
          return;
        }

        if (context.mounted) {
          context.pop(result);
        }
      },
      child: Scaffold(
        backgroundColor: Colors.black,
        body: KeyboardListener(
          focusNode: _focusNode,
          onKeyEvent: _handleKeyEvent,
          child: MouseRegion(
            onHover: (_) => _showControlsTemp(),
            child: AnimatedBuilder(
              animation: controller,
              builder: (context, _) {
                final state = controller.state;

                if (!state.isInitialized) {
                  return const Center(child: CircularProgressIndicator());
                }

                return Stack(
                  fit: StackFit.expand,
                  children: [
                    // Video Layer
                    if (!state.hasError)
                      Stack(
                        fit: StackFit.expand,
                        children: [
                          if (!state.isTransitioning &&
                              controller.hasVideoController)
                            Video(
                              key: ValueKey('video_${state.textureKey}'),
                              controller: controller.videoController!,
                              fit: state.videoFit,
                              fill: Colors.black,
                              controls: NoVideoControls,
                            )
                          else
                            Builder(
                              builder: (context) {
                                WidgetsBinding.instance.addPostFrameCallback((
                                  _,
                                ) {
                                  Future.microtask(() {
                                    controller.notifyUIUpdated();
                                  });
                                });
                                return const SizedBox.expand(
                                  child: ColoredBox(color: Colors.black),
                                );
                              },
                            ),
                        ],
                      ),

                    // Error Layer — below the chrome layers so the control bar stays
                    // visible; both the gesture and controls layers stop
                    // accepting input while `hasError` is set (see
                    // PlayerControls), leaving the error buttons clickable.
                    if (state.hasError) _buildErrorWidget(state.error),

                    // Buffering Layer
                    if (state.isBuffering && !state.hasError)
                      const Center(
                        child: CircularProgressIndicator(color: Colors.white),
                      ),

                    // Gesture Layer — kept mounted in the error state so the chrome
                    // (quality/track/URL inspection) is still reachable. It
                    // ignores input while `hasError` is set so the error
                    // buttons underneath win hit-testing.
                    if (!_videoPlayerService.isNativePiP)
                      IgnorePointer(
                        ignoring: state.hasError,
                        child: PlayerGestureLayer(
                          controller: controller,
                          onTap: _toggleControls,
                          onDoubleTap: controller.toggleFullscreen,
                          child: Container(color: Colors.transparent),
                        ),
                      ),

                    // Controls Layer — same reasoning; PlayerControls renders
                    // but is non-interactive while an error is showing.
                    if (!_videoPlayerService.isNativePiP)
                      PlayerControls(
                        controller: controller,
                        showControls: _showControls,
                        onToggleControls: _toggleControls,
                        showChat: _showChat,
                        onToggleChat: _toggleChatOverlay,
                        newChatMessages: _newChatMessages,
                        onEnterPiP: _videoPlayerService.enterNativePiP,
                      ),

                    // Chat Layer
                    if (_showChat)
                      Positioned(
                        top: 0,
                        bottom: 0,
                        right: 0,
                        child: PlayerChatOverlay(
                          service: controller.watchPartyService,
                          onClose: _toggleChatOverlay,
                        ),
                      ),

                    // Watch Party UI (Sync Status)
                    if (!_videoPlayerService.isNativePiP &&
                        controller.watchPartyService.state ==
                            WatchPartyState.connected)
                      _buildWatchPartyFloatingUI(),

                    // Visibility Toggle — reachable in the error state too
                    if (!_videoPlayerService.isNativePiP)
                      Positioned(
                        top: 60 + MediaQuery.paddingOf(context).top,
                        right: 16,
                        child: IconButton(
                          onPressed: _toggleControls,
                          style: IconButton.styleFrom(
                            backgroundColor: Colors.black.withValues(
                              alpha: 0.4,
                            ),
                            foregroundColor: Colors.white,
                          ),
                          icon: Icon(
                            _showControls
                                ? Icons.visibility_off
                                : Icons.visibility,
                          ),
                          tooltip: _showControls
                              ? 'Сховати інтерфейс'
                              : 'Показати інтерфейс',
                        ),
                      ),

                    // Desktop PiP Controls
                    if (_videoPlayerService.isDesktopPiP)
                      PiPControls(
                        controller: controller,
                        videoPlayerService: _videoPlayerService,
                      ),
                  ],
                );
              },
            ),
          ),
        ),
      ),
    );
  }

  /// Host of [url], or an empty string when it cannot be parsed.
  static String _hostOf(String url) {
    try {
      final u = Uri.parse(url);
      return u.host;
    } catch (_) {
      return '';
    }
  }

  /// Human-readable label for an alternative source.
  ///
  /// Previously this was `voiceover ?? quality.displayName`, which rendered as
  /// the identical "Авто" for every source with unknown quality — the
  /// duplicate-chips bug in the audit.
  static String _sourceLabel(StreamSource source) {
    final parts = <String>[
      if (source.voiceover != null && source.voiceover!.trim().isNotEmpty)
        source.voiceover!.trim(),
      if (source.quality != StreamQuality.unknown) source.quality.displayName,
    ];
    if (parts.isEmpty) return 'Джерело';
    return parts.join(' • ');
  }

  Widget _buildErrorWidget(PlaybackError? error) {
    final controller = _videoPlayerService.controller;
    if (controller == null) return const SizedBox();

    final all = widget.streams ?? const <StreamSource>[];

    // De-duplicate by resolved URL: the backend frequently returns the same
    // CDN URL under several quality/voiceover labels.
    final seenUrls = <String>{controller.state.currentUrl};
    final alternatives = <StreamSource>[];
    for (final s in all) {
      if (!seenUrls.add(s.url)) continue;
      alternatives.add(s);
    }

    return Positioned.fill(
      child: Container(
        // Opaque behind the error copy only — the control bar above/below
        // stays visible so quality/track/URL inspection remains available.
        color: Colors.black,
        child: AppErrorWidget.playback(
          title: error?.title ?? 'Помилка відтворення',
          message: error?.message,
          onRetry: controller.retryPlayback,
          onBack: () => context.pop(),
          details: error?.toDiagnosticString(logTail: Logger.recentText),
          onCopyDetails: () => Logger.i(
            'User copied playback diagnostics for '
            '${error?.url ?? controller.state.currentUrl}',
            tag: 'PlayerPage',
          ),
          alternativeActions: alternatives.isEmpty
              ? null
              : alternatives.take(6).map((stream) {
                  // A source classified as an HTML page cannot be played by
                  // libmpv — render it disabled with an explanation rather
                  // than offering it as a retry target.
                  final isIframe = stream.type == StreamType.iframe;
                  final host = _hostOf(stream.url);
                  final chip = ActionChip(
                    avatar: Icon(
                      isIframe ? Icons.html : Icons.play_circle_outline,
                      size: 18,
                      color: isIframe ? Colors.orange : null,
                    ),
                    label: Text(
                      host.isEmpty
                          ? _sourceLabel(stream)
                          : '${_sourceLabel(stream)} · $host',
                    ),
                    onPressed: isIframe ? null : () => _openAlternative(stream),
                  );
                  if (!isIframe) return chip;
                  return Tooltip(
                    message:
                        'Це HTML-сторінка, а не відеофайл. Відтворення неможливе.',
                    child: chip,
                  );
                }).toList(),
        ),
      ),
    );
  }

  void _openAlternative(StreamSource stream) {
    final controller = _videoPlayerService.controller;
    if (controller == null) return;
    Logger.i(
      'User picked alternative source: ${stream.url} (type=${stream.type.name})',
      tag: 'PlayerPage',
    );
    controller.switchStreamWithRetry(stream);
  }

  Widget _buildWatchPartyFloatingUI() {
    return Positioned(
      right: _showChat ? 316 : 16,
      bottom: 100,
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.end,
        mainAxisSize: MainAxisSize.min,
        children: [_buildSyncIndicator()],
      ),
    );
  }

  Widget _buildSyncIndicator() {
    final controller = _videoPlayerService.controller;
    if (controller == null) return const SizedBox();

    final service = controller.watchPartyService;
    final mode = service.correctionMode;

    if (service.isHost) {
      return Tooltip(
        message: '${service.participants.length} учасників',
        child: Container(
          padding: const EdgeInsets.all(8),
          decoration: BoxDecoration(
            color: Colors.black.withValues(alpha: 0.4),
            shape: BoxShape.circle,
          ),
          child: Badge(
            label: Text('${service.participants.length}'),
            backgroundColor: Colors.green,
            child: const Icon(Icons.groups, color: Colors.white, size: 20),
          ),
        ),
      );
    }

    Color iconColor;
    IconData icon;
    String tooltip;

    switch (mode) {
      case SyncCorrectionMode.none:
        iconColor = Colors.green;
        icon = Icons.check_circle;
        tooltip = 'Синхронізовано';
        break;
      case SyncCorrectionMode.speedUp:
        iconColor = Colors.orange;
        icon = Icons.fast_forward;
        tooltip = 'Наздоганяємо...';
        break;
      case SyncCorrectionMode.slowDown:
        iconColor = Colors.orange;
        icon = Icons.slow_motion_video;
        tooltip = 'Уповільнюємо...';
        break;
      case SyncCorrectionMode.hardSeek:
        iconColor = Colors.red;
        icon = Icons.sync_problem;
        tooltip = 'Пересинхронізація...';
        break;
    }

    return Tooltip(
      message: tooltip,
      child: Container(
        padding: const EdgeInsets.all(8),
        decoration: BoxDecoration(
          color: Colors.black.withValues(alpha: 0.4),
          shape: BoxShape.circle,
        ),
        child: Icon(icon, color: iconColor, size: 20),
      ),
    );
  }
}
