import 'package:flutter/material.dart';

/// Spacing design tokens for consistent layouts across the app.
///
/// Usage:
/// ```dart
/// padding: EdgeInsets.all(AppSpacing.md)
/// SizedBox(height: AppSpacing.lg)
/// ```
class AppSpacing {
  AppSpacing._();

  // Base spacing scale (4-point grid)
  static const double none = 0;
  static const double xxs = 2;
  static const double xs = 4;
  static const double sm = 8;
  static const double md = 16;
  static const double lg = 24;
  static const double xl = 32;
  static const double xxl = 48;
  static const double xxxl = 64;

  // Semantic spacing
  static const double cardPadding = md;
  static const double cardMargin = sm;
  static const double pagePadding = md;
  static const double pageMarginHorizontal = lg;
  static const double sectionGap = lg;
  static const double itemGap = sm;
  static const double iconTextGap = sm;
  static const double buttonGap = xs;

  // Grid spacing
  static const double gridPadding = md;
  static const double gridCrossAxisSpacing = md;
  static const double gridMainAxisSpacing = md;

  // Responsive breakpoints
  static const double mobileBreakpoint = 600;
  static const double tabletBreakpoint = 900;
  static const double desktopBreakpoint = 1200;

  // Edge insets presets
  static const EdgeInsets cardInsets = EdgeInsets.all(cardPadding);
  static const EdgeInsets pageInsets = EdgeInsets.all(pagePadding);
  static const EdgeInsets pageInsetsHorizontal = EdgeInsets.symmetric(
    horizontal: pageMarginHorizontal,
  );
  static const EdgeInsets listItemInsets = EdgeInsets.symmetric(
    horizontal: md,
    vertical: sm,
  );

  // Gap helper for Column/Row children
  static Widget gap(double size) => SizedBox(height: size, width: size);
  static const Widget gapXs = SizedBox(height: xs, width: xs);
  static const Widget gapSm = SizedBox(height: sm, width: sm);
  static const Widget gapMd = SizedBox(height: md, width: md);
  static const Widget gapLg = SizedBox(height: lg, width: lg);
  static const Widget gapXl = SizedBox(height: xl, width: xl);

  /// Returns responsive padding based on screen width
  static EdgeInsets responsivePadding(BuildContext context) {
    final width = MediaQuery.sizeOf(context).width;
    if (width >= desktopBreakpoint) {
      return const EdgeInsets.symmetric(horizontal: xxl, vertical: lg);
    } else if (width >= tabletBreakpoint) {
      return const EdgeInsets.symmetric(horizontal: xl, vertical: md);
    } else if (width >= mobileBreakpoint) {
      return const EdgeInsets.symmetric(horizontal: lg, vertical: md);
    }
    return const EdgeInsets.all(md);
  }

  /// Returns responsive horizontal padding based on screen width
  static double responsiveHorizontal(BuildContext context) {
    final width = MediaQuery.sizeOf(context).width;
    if (width >= desktopBreakpoint) return xxl;
    if (width >= tabletBreakpoint) return xl;
    if (width >= mobileBreakpoint) return lg;
    return md;
  }
}
