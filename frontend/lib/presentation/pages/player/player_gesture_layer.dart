import 'package:flutter/material.dart';
import 'player_controller.dart';

class PlayerGestureLayer extends StatefulWidget {
  final PlayerController controller;
  final VoidCallback? onTap;
  final VoidCallback? onDoubleTap;
  final Widget child;

  const PlayerGestureLayer({
    super.key,
    required this.controller,
    this.onTap,
    this.onDoubleTap,
    required this.child,
  });

  @override
  State<PlayerGestureLayer> createState() => _PlayerGestureLayerState();
}

class _PlayerGestureLayerState extends State<PlayerGestureLayer> {
  double _verticalDelta = 0;
  bool _isDragging = false;
  String? _overlayText;

  @override
  Widget build(BuildContext context) {
    return GestureDetector(
      onTap: widget.onTap,
      onDoubleTap: widget.onDoubleTap,
      onVerticalDragStart: (_) {
        _verticalDelta = 0;
        _isDragging = true;
      },
      onVerticalDragUpdate: (details) {
        // Adjust sensitivity
        _verticalDelta -= details.primaryDelta! / 200.0;

        final currentVolume = widget.controller.state.volume;
        final newVolume = (currentVolume + _verticalDelta * 100).clamp(
          0.0,
          100.0,
        );

        widget.controller.setVolume(newVolume);

        setState(() {
          _overlayText = 'Гучність: ${newVolume.toInt()}%';
        });
      },
      onVerticalDragEnd: (_) {
        setState(() {
          _isDragging = false;
          _overlayText = null;
        });
      },
      behavior: HitTestBehavior.translucent,
      child: Stack(
        children: [
          widget.child,
          if (_isDragging && _overlayText != null)
            Center(
              child: Container(
                padding: const EdgeInsets.symmetric(
                  horizontal: 20,
                  vertical: 10,
                ),
                decoration: BoxDecoration(
                  color: Colors.black54,
                  borderRadius: BorderRadius.circular(20),
                ),
                child: Text(
                  _overlayText!,
                  style: const TextStyle(
                    color: Colors.white,
                    fontSize: 18,
                    fontWeight: FontWeight.bold,
                  ),
                ),
              ),
            ),
        ],
      ),
    );
  }
}
