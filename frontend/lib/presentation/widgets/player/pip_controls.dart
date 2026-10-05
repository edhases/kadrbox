import 'package:flutter/material.dart';
import 'package:window_manager/window_manager.dart';

import '../../../data/services/video_player_service.dart';
import '../../pages/player/player_controller.dart';

class PiPControls extends StatefulWidget {
  final PlayerController controller;
  final VideoPlayerService videoPlayerService;

  const PiPControls({
    super.key,
    required this.controller,
    required this.videoPlayerService,
  });

  @override
  State<PiPControls> createState() => _PiPControlsState();
}

class _PiPControlsState extends State<PiPControls> {
  bool _isHovered = false;

  @override
  Widget build(BuildContext context) {
    return MouseRegion(
      onEnter: (_) => setState(() => _isHovered = true),
      onExit: (_) => setState(() => _isHovered = false),
      child: AnimatedOpacity(
        opacity: _isHovered ? 1.0 : 0.0,
        duration: const Duration(milliseconds: 200),
        child: Container(
          color: Colors.black.withOpacity(0.5),
          child: Stack(
            children: [
              // Generic drag area for the whole window
              Positioned.fill(
                child: GestureDetector(
                  onPanStart: (_) {
                    windowManager.startDragging();
                  },
                  child: Container(color: Colors.transparent),
                ),
              ),

              // Center Controls
              Center(
                child: Row(
                  mainAxisSize: MainAxisSize.min,
                  children: [
                    // Backward 10s
                    IconButton(
                      icon: const Icon(Icons.replay_10, color: Colors.white),
                      onPressed: () => widget.controller.seekBackward(),
                      tooltip: '-10s',
                    ),
                    const SizedBox(width: 8),
                    // Play/Pause
                    IconButton(
                      iconSize: 32,
                      icon: Icon(
                        widget.controller.state.isPlaying
                            ? Icons.pause_circle_filled
                            : Icons.play_circle_filled,
                        color: Colors.white,
                      ),
                      onPressed: widget.controller.playOrPause,
                      tooltip: widget.controller.state.isPlaying
                          ? 'Пауза'
                          : 'Грати',
                    ),
                    const SizedBox(width: 8),
                    // Forward 10s
                    IconButton(
                      icon: const Icon(Icons.forward_10, color: Colors.white),
                      onPressed: () => widget.controller.seekForward(),
                      tooltip: '+10s',
                    ),
                  ],
                ),
              ),

              // Top Right Controls (Maximize & Close)
              Positioned(
                top: 0,
                right: 0,
                child: Row(
                  mainAxisSize: MainAxisSize.min,
                  children: [
                    IconButton(
                      icon: const Icon(
                        Icons.open_in_full,
                        color: Colors.white,
                        size: 20,
                      ),
                      onPressed: () => widget.videoPlayerService.maximize(),
                      tooltip: 'Розгорнути',
                    ),
                    IconButton(
                      icon: const Icon(
                        Icons.close,
                        color: Colors.white,
                        size: 20,
                      ),
                      onPressed: () => windowManager.minimize(),
                      tooltip: 'Згорнути',
                    ),
                  ],
                ),
              ),
            ],
          ),
        ),
      ),
    );
  }
}
