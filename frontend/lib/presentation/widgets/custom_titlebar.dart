import 'package:flutter/material.dart';
import 'package:window_manager/window_manager.dart';

import '../theme/app_theme.dart';

/// Custom titlebar for desktop platforms (frameless window)
class CustomTitleBar extends StatelessWidget {
  const CustomTitleBar({super.key});

  @override
  Widget build(BuildContext context) {
    return Container(
      height: 36,
      decoration: BoxDecoration(
        color: AppTheme.darkSurface,
        border: Border(
          bottom: BorderSide(color: AppTheme.darkBorder, width: 1),
        ),
      ),
      child: Row(
        children: [
          // Drag area
          Expanded(
            child: GestureDetector(
              behavior: HitTestBehavior.translucent,
              onPanStart: (_) => windowManager.startDragging(),
              onDoubleTap: _toggleMaximize,
              child: Row(
                children: [
                  const SizedBox(width: 12),
                  // App icon
                  Container(
                    width: 20,
                    height: 20,
                    decoration: BoxDecoration(
                      gradient: const LinearGradient(
                        colors: [
                          AppTheme.primaryColor,
                          AppTheme.secondaryColor,
                        ],
                      ),
                      borderRadius: BorderRadius.circular(4),
                    ),
                    child: const Icon(
                      Icons.movie_filter,
                      size: 14,
                      color: Colors.white,
                    ),
                  ),
                  const SizedBox(width: 8),
                  Text(
                    'Oxide Film',
                    style: TextStyle(
                      color: AppTheme.textSecondary,
                      fontSize: 12,
                      fontWeight: FontWeight.w500,
                    ),
                  ),
                ],
              ),
            ),
          ),

          // Window controls
          _WindowButton(
            icon: Icons.remove,
            onPressed: () => windowManager.minimize(),
            tooltip: 'Згорнути',
          ),
          _WindowButton(
            icon: Icons.crop_square,
            onPressed: _toggleMaximize,
            tooltip: 'Розгорнути / Відновити',
          ),
          _WindowButton(
            icon: Icons.close,
            onPressed: () => windowManager.close(),
            tooltip: 'Закрити',
            isClose: true,
          ),
        ],
      ),
    );
  }

  Future<void> _toggleMaximize() async {
    final isMaximized = await windowManager.isMaximized();
    if (isMaximized) {
      await windowManager.unmaximize();
    } else {
      await windowManager.maximize();
    }
  }
}

class _WindowButton extends StatefulWidget {
  final IconData icon;
  final VoidCallback onPressed;
  final String tooltip;
  final bool isClose;

  const _WindowButton({
    required this.icon,
    required this.onPressed,
    required this.tooltip,
    this.isClose = false,
  });

  @override
  State<_WindowButton> createState() => _WindowButtonState();
}

class _WindowButtonState extends State<_WindowButton> {
  bool _isHovered = false;

  @override
  Widget build(BuildContext context) {
    Color bgColor = Colors.transparent;
    if (_isHovered) {
      bgColor = widget.isClose
          ? Colors.red
          : Colors.white.withValues(alpha: 0.1);
    }

    return Tooltip(
      message: widget.tooltip,
      child: MouseRegion(
        onEnter: (_) => setState(() => _isHovered = true),
        onExit: (_) => setState(() => _isHovered = false),
        child: GestureDetector(
          onTap: widget.onPressed,
          child: Container(
            width: 46,
            height: 36,
            color: bgColor,
            child: Icon(
              widget.icon,
              size: 16,
              color: _isHovered && widget.isClose
                  ? Colors.white
                  : AppTheme.textSecondary,
            ),
          ),
        ),
      ),
    );
  }
}
