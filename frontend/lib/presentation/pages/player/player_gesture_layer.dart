import 'dart:async';
import 'package:flutter/gestures.dart';
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
  String? _overlayText;
  bool _isBoosted = false;
  Timer? _hideHudTimer;

  void _showVolumeHud(double volume) {
    _hideHudTimer?.cancel();
    setState(() {
      _isBoosted = volume > 100.0;
      if (_isBoosted) {
        _overlayText = 'Гучність: ${volume.toInt()}% (Boost)';
      } else {
        _overlayText = 'Гучність: ${volume.toInt()}%';
      }
    });

    _hideHudTimer = Timer(const Duration(milliseconds: 1200), () {
      if (mounted) {
        setState(() {
          _overlayText = null;
        });
      }
    });
  }

  void _handlePointerSignal(PointerSignalEvent event) {
    if (event is PointerScrollEvent) {
      // Scroll up: delta.dy < 0 -> increase volume (+5%)
      // Scroll down: delta.dy > 0 -> decrease volume (-5%)
      final delta = event.scrollDelta.dy < 0 ? 5.0 : -5.0;
      final newVolume = (widget.controller.state.volume + delta).clamp(0.0, 150.0);
      widget.controller.setVolume(newVolume);
      _showVolumeHud(newVolume);
    }
  }

  @override
  void dispose() {
    _hideHudTimer?.cancel();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    return Listener(
      onPointerSignal: _handlePointerSignal,
      behavior: HitTestBehavior.translucent,
      child: GestureDetector(
        onTap: widget.onTap,
        onDoubleTap: widget.onDoubleTap,
        onVerticalDragStart: (_) {
          _verticalDelta = 0;
        },
        onVerticalDragUpdate: (details) {
          // Adjust sensitivity
          _verticalDelta -= details.primaryDelta! / 200.0;

          final currentVolume = widget.controller.state.volume;
          final newVolume = (currentVolume + _verticalDelta * 100).clamp(
            0.0,
            150.0,
          );

          widget.controller.setVolume(newVolume);
          _showVolumeHud(newVolume);
        },
        onVerticalDragEnd: (_) {},
        behavior: HitTestBehavior.translucent,
        child: Stack(
          children: [
            widget.child,
            if (_overlayText != null)
              Center(
                child: Container(
                  padding: const EdgeInsets.symmetric(
                    horizontal: 20,
                    vertical: 10,
                  ),
                  decoration: BoxDecoration(
                    color: _isBoosted
                        ? Colors.black87
                        : Colors.black54,
                    border: _isBoosted
                        ? Border.all(color: Colors.orangeAccent, width: 1.5)
                        : null,
                    borderRadius: BorderRadius.circular(20),
                  ),
                  child: Row(
                    mainAxisSize: MainAxisSize.min,
                    children: [
                      Icon(
                        _isBoosted
                            ? Icons.campaign
                            : (widget.controller.state.volume == 0
                                ? Icons.volume_off
                                : Icons.volume_up),
                        color: _isBoosted ? Colors.orangeAccent : Colors.white,
                        size: 22,
                      ),
                      const SizedBox(width: 8),
                      Text(
                        _overlayText!,
                        style: TextStyle(
                          color: _isBoosted ? Colors.orangeAccent : Colors.white,
                          fontSize: 18,
                          fontWeight: FontWeight.bold,
                        ),
                      ),
                    ],
                  ),
                ),
              ),
          ],
        ),
      ),
    );
  }
}
