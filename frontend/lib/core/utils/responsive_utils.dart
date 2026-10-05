import 'package:flutter/foundation.dart';
import 'package:flutter/material.dart';
import 'package:get_it/get_it.dart';

import '../../data/services/settings_service.dart';

/// Device type classification
enum DeviceType {
  /// Phone: < 600px width
  phone,

  /// Tablet: 600-900px width
  tablet,

  /// Desktop: > 900px width, mouse/keyboard
  desktop,

  /// TV: > 900px width, D-Pad/remote
  tv;

  String get displayName {
    switch (this) {
      case DeviceType.phone:
        return 'Телефон';
      case DeviceType.tablet:
        return 'Планшет';
      case DeviceType.desktop:
        return 'ПК/Ноутбук';
      case DeviceType.tv:
        return 'Телевізор';
    }
  }

  String get key => name;

  static DeviceType fromString(String? value) {
    return DeviceType.values.firstWhere(
      (e) => e.key == value,
      orElse: () => DeviceType.phone,
    );
  }
}

/// Screen size classification (independent of device type)
enum ScreenSize {
  /// < 360px - very small phones
  compact,

  /// 360-600px - regular phones
  small,

  /// 600-900px - large phones landscape, small tablets
  medium,

  /// 900-1200px - tablets, small desktops
  large,

  /// > 1200px - desktops, TVs
  extraLarge,
}

/// Orientation helper
enum ScreenOrientation { portrait, landscape }

/// Responsive utilities for adaptive UI
class ResponsiveUtils {
  ResponsiveUtils._();

  // Breakpoints
  static const double compactWidth = 360;
  static const double phoneMaxWidth = 600;
  static const double tabletMaxWidth = 900;
  static const double desktopMaxWidth = 1200;
  static const double tvMinWidth = 1280;

  /// Get screen size classification
  static ScreenSize getScreenSize(BuildContext context) {
    final width = MediaQuery.of(context).size.width;
    return getScreenSizeFromWidth(width);
  }

  /// Get screen size from width value
  static ScreenSize getScreenSizeFromWidth(double width) {
    if (width < compactWidth) return ScreenSize.compact;
    if (width < phoneMaxWidth) return ScreenSize.small;
    if (width < tabletMaxWidth) return ScreenSize.medium;
    if (width < desktopMaxWidth) return ScreenSize.large;
    return ScreenSize.extraLarge;
  }

  /// Get orientation
  static ScreenOrientation getOrientation(BuildContext context) {
    final size = MediaQuery.of(context).size;
    return size.width > size.height
        ? ScreenOrientation.landscape
        : ScreenOrientation.portrait;
  }

  /// Auto-detect device type based on platform and screen size
  /// Can be overridden by user preference from settings
  static DeviceType detectDeviceType(
    BuildContext context, {
    DeviceType? override,
  }) {
    // Check settings override first
    if (override == null) {
      try {
        final settings = GetIt.instance<SettingsService>();
        final userOverride = settings.state.deviceTypeOverride;
        if (userOverride != 'auto') {
          return DeviceType.fromString(userOverride);
        }
      } catch (_) {
        // Settings not available yet, use auto-detect
      }
    }

    if (override != null) return override;

    final width = MediaQuery.of(context).size.width;

    // Check platform first
    if (!kIsWeb) {
      if (defaultTargetPlatform == TargetPlatform.android ||
          defaultTargetPlatform == TargetPlatform.iOS) {
        // Mobile platforms
        if (width < phoneMaxWidth) {
          return DeviceType.phone;
        }
        return DeviceType.tablet;
      }

      if (defaultTargetPlatform == TargetPlatform.windows ||
          defaultTargetPlatform == TargetPlatform.linux ||
          defaultTargetPlatform == TargetPlatform.macOS) {
        return DeviceType.desktop;
      }
    }

    // Web or fallback - use screen size
    if (width < phoneMaxWidth) return DeviceType.phone;
    if (width < tabletMaxWidth) return DeviceType.tablet;
    return DeviceType.desktop;
  }

  /// Check if device is compact (very small screen)
  static bool isCompact(BuildContext context) {
    return MediaQuery.of(context).size.width < compactWidth;
  }

  /// Check if should use mobile layout
  static bool isMobile(BuildContext context) {
    return MediaQuery.of(context).size.width < phoneMaxWidth;
  }

  /// Check if should use tablet layout
  static bool isTablet(BuildContext context) {
    final width = MediaQuery.of(context).size.width;
    return width >= phoneMaxWidth && width < tabletMaxWidth;
  }

  /// Check if should use desktop layout
  static bool isDesktop(BuildContext context) {
    return MediaQuery.of(context).size.width >= tabletMaxWidth;
  }

  /// Check if device is in landscape mode
  static bool isLandscape(BuildContext context) {
    return getOrientation(context) == ScreenOrientation.landscape;
  }

  /// Get adaptive padding based on screen size
  static EdgeInsets getAdaptivePadding(BuildContext context) {
    final size = getScreenSize(context);
    switch (size) {
      case ScreenSize.compact:
        return const EdgeInsets.all(8);
      case ScreenSize.small:
        return const EdgeInsets.all(12);
      case ScreenSize.medium:
        return const EdgeInsets.all(16);
      case ScreenSize.large:
        return const EdgeInsets.all(20);
      case ScreenSize.extraLarge:
        return const EdgeInsets.all(24);
    }
  }

  /// Get adaptive icon size
  static double getAdaptiveIconSize(
    BuildContext context, {
    bool forTV = false,
  }) {
    if (forTV) return 32; // TV needs larger icons

    final size = getScreenSize(context);
    switch (size) {
      case ScreenSize.compact:
        return 20;
      case ScreenSize.small:
        return 24;
      case ScreenSize.medium:
        return 24;
      case ScreenSize.large:
        return 28;
      case ScreenSize.extraLarge:
        return 28;
    }
  }

  /// Get adaptive button height (minimum 48dp for TV)
  static double getAdaptiveButtonHeight(
    BuildContext context,
    DeviceType deviceType,
  ) {
    if (deviceType == DeviceType.tv) return 56;

    final size = getScreenSize(context);
    switch (size) {
      case ScreenSize.compact:
        return 36;
      case ScreenSize.small:
        return 40;
      case ScreenSize.medium:
        return 44;
      case ScreenSize.large:
        return 48;
      case ScreenSize.extraLarge:
        return 48;
    }
  }

  /// Get adaptive font size multiplier
  static double getFontScaleFactor(
    BuildContext context,
    DeviceType deviceType,
  ) {
    if (deviceType == DeviceType.tv) return 1.3;

    final size = getScreenSize(context);
    switch (size) {
      case ScreenSize.compact:
        return 0.85;
      case ScreenSize.small:
        return 1.0;
      case ScreenSize.medium:
        return 1.0;
      case ScreenSize.large:
        return 1.1;
      case ScreenSize.extraLarge:
        return 1.1;
    }
  }

  /// Get chat panel width (adaptive)
  static double getChatPanelWidth(BuildContext context) {
    final width = MediaQuery.of(context).size.width;
    final size = getScreenSize(context);

    switch (size) {
      case ScreenSize.compact:
      case ScreenSize.small:
        return width * 0.85; // Almost full width on phones
      case ScreenSize.medium:
        return width * 0.4; // 40% on tablets
      case ScreenSize.large:
        return 320; // Fixed width on desktop
      case ScreenSize.extraLarge:
        return 360; // Slightly larger on big screens
    }
  }

  /// Get player controls bottom padding
  static double getPlayerBottomPadding(BuildContext context) {
    final size = getScreenSize(context);
    switch (size) {
      case ScreenSize.compact:
        return 8;
      case ScreenSize.small:
        return 12;
      case ScreenSize.medium:
        return 16;
      case ScreenSize.large:
      case ScreenSize.extraLarge:
        return 16;
    }
  }

  /// Check if should show full controls or compact version
  static bool shouldShowCompactControls(BuildContext context) {
    final size = getScreenSize(context);
    final orientation = getOrientation(context);

    // Show compact controls on small screens in portrait
    if (size == ScreenSize.compact) return true;
    if (size == ScreenSize.small && orientation == ScreenOrientation.portrait) {
      return true;
    }
    return false;
  }

  /// Get maximum expand height for app bar on details page
  static double getDetailsAppBarHeight(BuildContext context) {
    final size = getScreenSize(context);
    final height = MediaQuery.of(context).size.height;

    switch (size) {
      case ScreenSize.compact:
        return height * 0.35;
      case ScreenSize.small:
        return height * 0.4;
      case ScreenSize.medium:
        return 300;
      case ScreenSize.large:
      case ScreenSize.extraLarge:
        return 350;
    }
  }
}

/// Extension for easy access in widgets
extension ResponsiveContext on BuildContext {
  ScreenSize get screenSize => ResponsiveUtils.getScreenSize(this);
  ScreenOrientation get screenOrientation =>
      ResponsiveUtils.getOrientation(this);
  bool get isMobile => ResponsiveUtils.isMobile(this);
  bool get isTablet => ResponsiveUtils.isTablet(this);
  bool get isDesktop => ResponsiveUtils.isDesktop(this);
  bool get isLandscape => ResponsiveUtils.isLandscape(this);
  bool get isCompact => ResponsiveUtils.isCompact(this);

  EdgeInsets get adaptivePadding => ResponsiveUtils.getAdaptivePadding(this);
  double get adaptiveIconSize => ResponsiveUtils.getAdaptiveIconSize(this);
  double get chatPanelWidth => ResponsiveUtils.getChatPanelWidth(this);
}
