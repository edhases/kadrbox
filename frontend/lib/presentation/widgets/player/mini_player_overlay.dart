import 'dart:async';
import 'package:flutter/material.dart';
import 'package:get_it/get_it.dart';
import 'package:go_router/go_router.dart';
import 'package:media_kit_video/media_kit_video.dart';
import 'package:kadrbox/data/services/video_player_service.dart';

class MiniPlayerOverlay extends StatefulWidget {
  final Widget child;

  const MiniPlayerOverlay({super.key, required this.child});

  @override
  State<MiniPlayerOverlay> createState() => _MiniPlayerOverlayState();
}

class _MiniPlayerOverlayState extends State<MiniPlayerOverlay> {
  final VideoPlayerService _videoPlayerService = GetIt.I<VideoPlayerService>();

  // Draggable position
  Offset _position = const Offset(20, 100);
  final double _width = 300; // 16:9 aspect ratio -> ~169 height
  final double _aspectRatio = 16 / 9;

  bool _showControls = false;
  Timer? _hideTimer;

  @override
  void initState() {
    super.initState();
    _videoPlayerService.addListener(_updateState);
  }

  @override
  void dispose() {
    _videoPlayerService.removeListener(_updateState);
    _hideTimer?.cancel();
    super.dispose();
  }

  void _updateState() {
    if (mounted) setState(() {});
  }

  void _startHideTimer() {
    _hideTimer?.cancel();
    _hideTimer = Timer(const Duration(seconds: 3), () {
      if (mounted) {
        setState(() {
          _showControls = false;
        });
      }
    });
  }

  void _onHover(PointerEvent event) {
    if (!_showControls) {
      setState(() {
        _showControls = true;
      });
    }
    _startHideTimer();
  }

  void _onExit(PointerEvent event) {
    _hideTimer?.cancel();
    setState(() {
      _showControls = false;
    });
  }

  void _onPanUpdate(DragUpdateDetails details) {
    setState(() {
      _position += details.delta;
    });
  }

  void _onPanEnd(DragEndDetails details) {
    if (!mounted) return;

    final size = MediaQuery.of(context).size;
    final padding = MediaQuery.of(context).padding;
    final height = _width / _aspectRatio;

    double dx = _position.dx;
    double dy = _position.dy;

    // Apply boundaries
    // Top boundary (safe area + some margin)
    final topLimit = padding.top + 10;
    if (dy < topLimit) dy = topLimit;

    // Bottom boundary (safe area + bottom navigation height approximation + margin)
    // We assume bottom navigation + safe area is at least 80-100px.
    final bottomLimit = size.height - height - padding.bottom - 80;
    if (dy > bottomLimit) dy = bottomLimit;

    // Snap to left or right edge
    final middleX = size.width / 2;
    final playerCenterX = dx + (_width / 2);

    if (playerCenterX < middleX) {
      dx = 10; // Snap left
    } else {
      dx = size.width - _width - 10; // Snap right
    }

    setState(() {
      _position = Offset(dx, dy);
    });
  }

  void _restoreFullscreen() {
    final controller = _videoPlayerService.controller;
    if (controller != null) {
      _videoPlayerService.maximize();
      // Navigate to player page
      context.pushNamed(
        'player',
        extra: {
          'url': controller.initialUrl,
          'title': controller.title,
          'streams': controller.streams,
          'mediaId': controller.mediaId,
          'providerId': controller.providerId,
          'posterUrl': controller.posterUrl,
          'mediaType': controller.mediaType,
          'season': controller.state.currentSeason,
          'episode': controller.state.currentEpisode,
          'episodeTitle': controller.state.currentEpisodeTitle,
        },
      );
    }
  }

  void _close() {
    _videoPlayerService.close();
  }

  @override
  Widget build(BuildContext context) {
    return Stack(
      children: [
        widget.child,
        if (_videoPlayerService.miniPlayerState == MiniPlayerState.minimized &&
            _videoPlayerService.controller != null &&
            _videoPlayerService.controller!.hasVideoController &&
            !_videoPlayerService.isDesktopPiP)
          Positioned(
            left: _position.dx,
            top: _position.dy,
            child: GestureDetector(
              onPanUpdate: _onPanUpdate,
              onPanEnd: _onPanEnd,
              onTap: _restoreFullscreen,
              child: Material(
                elevation: 8,
                borderRadius: BorderRadius.circular(12),
                clipBehavior: Clip.antiAlias,
                color: Colors.black,
                child: SizedBox(
                  width: _width,
                  height: _width / _aspectRatio,
                  child: Stack(
                    fit: StackFit.expand,
                    children: [
                      // Video
                      Video(
                        controller:
                            _videoPlayerService.controller!.videoController!,
                        fit: BoxFit.cover,
                        controls: NoVideoControls,
                      ),

                      // Controls layer with visibility
                      MouseRegion(
                        onHover: _onHover,
                        onExit: _onExit,
                        child: AnimatedOpacity(
                          opacity: _showControls ? 1.0 : 0.0,
                          duration: const Duration(milliseconds: 250),
                          child: Stack(
                            fit: StackFit.expand,
                            children: [
                              // Overlay gradient
                              Container(
                                decoration: BoxDecoration(
                                  gradient: LinearGradient(
                                    begin: Alignment.topCenter,
                                    end: Alignment.bottomCenter,
                                    colors: [
                                      Colors.black.withValues(alpha: 0.6),
                                      Colors.transparent,
                                      Colors.black.withValues(alpha: 0.6),
                                    ],
                                    stops: const [0.0, 0.2, 1.0],
                                  ),
                                ),
                              ),

                              // Controls Layout
                              Column(
                                children: [
                                  // Top bar with Maximize and Close
                                  Padding(
                                    padding: const EdgeInsets.symmetric(
                                      horizontal: 6,
                                      vertical: 4,
                                    ),
                                    child: Row(
                                      mainAxisAlignment:
                                          MainAxisAlignment.spaceBetween,
                                      children: [
                                        _MiniControlAction(
                                          icon: Icons.fullscreen,
                                          onTap: _restoreFullscreen,
                                          tooltip: 'Розгорнути',
                                        ),
                                        _MiniControlAction(
                                          icon: Icons.close,
                                          onTap: _close,
                                          tooltip: 'Закрити',
                                        ),
                                      ],
                                    ),
                                  ),

                                  const Spacer(),

                                  // Center controls: Rewind, Play/Pause, Forward
                                  Row(
                                    mainAxisAlignment: MainAxisAlignment.center,
                                    children: [
                                      _MiniControlAction(
                                        icon: Icons.replay_10,
                                        size: 24,
                                        onTap: () => _videoPlayerService
                                            .controller
                                            ?.seekBackward(),
                                      ),
                                      const SizedBox(width: 16),
                                      StreamBuilder<bool>(
                                        stream: _videoPlayerService
                                            .controller!
                                            .player
                                            .stream
                                            .playing,
                                        builder: (context, snapshot) {
                                          final playing =
                                              snapshot.data ?? false;
                                          return IconButton(
                                            onPressed: () {
                                              _videoPlayerService.controller
                                                  ?.playOrPause();
                                              _startHideTimer();
                                            },
                                            icon: Icon(
                                              playing
                                                  ? Icons.pause
                                                  : Icons.play_arrow,
                                              color: Colors.white,
                                              size: 32,
                                            ),
                                            style: IconButton.styleFrom(
                                              backgroundColor: Colors.black
                                                  .withOpacity(0.4),
                                            ),
                                          );
                                        },
                                      ),
                                      const SizedBox(width: 16),
                                      _MiniControlAction(
                                        icon: Icons.forward_10,
                                        size: 24,
                                        onTap: () => _videoPlayerService
                                            .controller
                                            ?.seekForward(),
                                      ),
                                    ],
                                  ),

                                  const Spacer(),
                                  const SizedBox(height: 8),
                                ],
                              ),

                              // Bottom Progress Bar
                              Positioned(
                                left: 0,
                                right: 0,
                                bottom: 0,
                                child: ValueListenableBuilder<Duration>(
                                  valueListenable: _videoPlayerService
                                      .controller!
                                      .positionNotifier,
                                  builder: (context, position, _) {
                                    final duration = _videoPlayerService
                                        .controller!
                                        .state
                                        .duration;
                                    if (duration.inSeconds == 0) {
                                      return const SizedBox();
                                    }
                                    final progress =
                                        position.inMilliseconds /
                                        duration.inMilliseconds;
                                    return Container(
                                      height: 2,
                                      color: Colors.white24,
                                      child: FractionallySizedBox(
                                        alignment: Alignment.centerLeft,
                                        widthFactor: progress.clamp(0.0, 1.0),
                                        child: Container(color: Colors.red),
                                      ),
                                    );
                                  },
                                ),
                              ),
                            ],
                          ),
                        ),
                      ),
                    ],
                  ),
                ),
              ),
            ),
          ),
      ],
    );
  }
}

class _MiniControlAction extends StatelessWidget {
  final IconData icon;
  final VoidCallback onTap;
  final String? tooltip;
  final double size;

  const _MiniControlAction({
    required this.icon,
    required this.onTap,
    this.tooltip,
    this.size = 20,
  });

  @override
  Widget build(BuildContext context) {
    return GestureDetector(
      onTap: onTap,
      child: Container(
        padding: const EdgeInsets.all(4),
        decoration: BoxDecoration(
          color: Colors.black.withOpacity(0.4),
          shape: BoxShape.circle,
        ),
        child: Icon(icon, color: Colors.white, size: size),
      ),
    );
  }
}
