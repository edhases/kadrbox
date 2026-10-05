import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:get_it/get_it.dart';

import '../../../data/services/settings_service.dart';
import '../../../domain/entities/entities.dart';

/// A wrapper widget that handles focus and keyboard events for D-Pad navigation.
/// Provides consistent visual feedback (scale, glow, border) for TV.
class FocusableCard extends StatefulWidget {
  final Widget child;
  final VoidCallback? onTap;
  final double borderRadius;
  final bool isSelected;
  final FocusNode? focusNode;

  const FocusableCard({
    super.key,
    required this.child,
    this.onTap,
    this.borderRadius = 8.0,
    this.isSelected = false,
    this.focusNode,
  });

  @override
  State<FocusableCard> createState() => _FocusableCardState();
}

class _FocusableCardState extends State<FocusableCard> {
  late final FocusNode _focusNode;
  bool _isFocused = false;
  final _settings = GetIt.instance<SettingsService>();

  UISettings get _ui => _settings.uiSettings;
  Color get _accentColor => Color(_ui.accentColor.colorValue);

  @override
  void initState() {
    super.initState();
    _focusNode = widget.focusNode ?? FocusNode();
    _focusNode.addListener(_onFocusChange);
  }

  @override
  void dispose() {
    // Only dispose if we created it
    if (widget.focusNode == null) {
      _focusNode.dispose();
    } else {
      _focusNode.removeListener(_onFocusChange);
    }
    super.dispose();
  }

  void _onFocusChange() {
    if (mounted) {
      setState(() {
        _isFocused = _focusNode.hasFocus;
      });
    }
  }

  @override
  Widget build(BuildContext context) {
    final animationsEnabled = _ui.animationsEnabled;

    return Focus(
      focusNode: _focusNode,
      onKeyEvent: (node, event) {
        if (event is KeyDownEvent &&
            (event.logicalKey == LogicalKeyboardKey.select ||
                event.logicalKey == LogicalKeyboardKey.enter)) {
          HapticFeedback.selectionClick();
          widget.onTap?.call();
          return KeyEventResult.handled;
        }
        return KeyEventResult.ignored;
      },
      child: GestureDetector(
        onTap: () {
          HapticFeedback.selectionClick();
          widget.onTap?.call();
        },
        child: AnimatedContainer(
          duration: animationsEnabled
              ? const Duration(milliseconds: 200)
              : Duration.zero,
          curve: Curves.easeOutCubic,
          transform: _isFocused && animationsEnabled
              ? (Matrix4.identity()
                  ..setEntry(0, 0, 1.05)
                  ..setEntry(1, 1, 1.05))
              : Matrix4.identity(),
          transformAlignment: Alignment.center,
          decoration: BoxDecoration(
            borderRadius: BorderRadius.circular(widget.borderRadius),
            boxShadow: _isFocused
                ? [
                    BoxShadow(
                      color: _accentColor.withValues(alpha: 0.4),
                      blurRadius: 16,
                      spreadRadius: 2,
                    ),
                  ]
                : null,
          ),
          child: Stack(
            fit: StackFit.passthrough,
            children: [
              widget.child,
              if (_isFocused)
                Positioned.fill(
                  child: Container(
                    decoration: BoxDecoration(
                      borderRadius: BorderRadius.circular(widget.borderRadius),
                      border: Border.all(color: _accentColor, width: 3),
                    ),
                  ),
                ),
            ],
          ),
        ),
      ),
    );
  }
}
