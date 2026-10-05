import 'package:flutter/material.dart';
import 'package:go_router/go_router.dart';
import '../../../../core/l10n/app_strings.dart';
import '../../../../data/services/settings_service.dart';
import '../../../theme/app_theme.dart';

import '../../../widgets/settings_widgets.dart';

/// Collection of dialogs used in the settings page
class SettingsDialogs {
  SettingsDialogs._();

  static void showLanguageDialog(
    BuildContext context,
    SettingsService settings,
    AppStrings strings,
  ) {
    showModalBottomSheet(
      context: context,
      backgroundColor: AppTheme.surfaceColor,
      shape: const RoundedRectangleBorder(
        borderRadius: BorderRadius.vertical(top: Radius.circular(16)),
      ),
      builder: (context) => SelectionSheet<AppLocale>(
        title: strings.selectLanguage,
        items: AppLocale.values,
        selectedItem: settings.state.locale,
        onSelected: (locale) {
          settings.setLocale(locale);
          context.pop();
        },
        itemBuilder: (locale) {
          return SelectionItem(
            title: locale.displayName,
            subtitle: locale == AppLocale.uk ? 'Ukrainian' : 'English',
            icon: Icons.language,
            isSelected: settings.state.locale == locale,
          );
        },
      ),
    );
  }

  static void showThemeDialog(
    BuildContext context,
    SettingsService settings,
    AppStrings strings,
  ) {
    showModalBottomSheet(
      context: context,
      backgroundColor: AppTheme.surfaceColor,
      shape: const RoundedRectangleBorder(
        borderRadius: BorderRadius.vertical(top: Radius.circular(16)),
      ),
      builder: (context) => SelectionSheet<AppThemeMode>(
        title: strings.theme,
        items: AppThemeMode.values,
        selectedItem: settings.state.theme,
        onSelected: (mode) {
          settings.setTheme(mode);
          context.pop();
        },
        itemBuilder: (mode) {
          return SelectionItem(
            title: mode.displayName,
            icon: _getThemeIcon(mode),
            isSelected: settings.state.theme == mode,
          );
        },
      ),
    );
  }

  static IconData _getThemeIcon(AppThemeMode theme) {
    switch (theme) {
      case AppThemeMode.dark:
        return Icons.dark_mode;
      case AppThemeMode.amoled:
        return Icons.smartphone;
      case AppThemeMode.light:
        return Icons.light_mode;
      case AppThemeMode.system:
        return Icons.settings_brightness;
    }
  }

  static void showQualityDialog(
    BuildContext context,
    SettingsService settings,
    AppStrings strings,
  ) {
    showModalBottomSheet(
      context: context,
      backgroundColor: AppTheme.surfaceColor,
      shape: const RoundedRectangleBorder(
        borderRadius: BorderRadius.vertical(top: Radius.circular(16)),
      ),
      builder: (context) => SelectionSheet<DefaultQuality>(
        title: strings.defaultQuality,
        items: DefaultQuality.values,
        selectedItem: settings.state.defaultQuality,
        onSelected: (quality) {
          settings.setDefaultQuality(quality);
          context.pop();
        },
        itemBuilder: (quality) {
          return SelectionItem(
            title: quality.displayName,
            icon: Icons.high_quality,
            isSelected: settings.state.defaultQuality == quality,
          );
        },
      ),
    );
  }

  static void showPlayerTypeDialog(
    BuildContext context,
    SettingsService settings,
    AppStrings strings,
  ) {
    showModalBottomSheet(
      context: context,
      backgroundColor: AppTheme.surfaceColor,
      shape: const RoundedRectangleBorder(
        borderRadius: BorderRadius.vertical(top: Radius.circular(16)),
      ),
      builder: (context) => SelectionSheet<PlayerType>(
        title: strings.player,
        items: PlayerType.values,
        selectedItem: settings.state.playerType,
        onSelected: (type) {
          settings.setPlayerType(type);
          context.pop();
        },
        itemBuilder: (type) {
          return SelectionItem(
            title: type.displayName,
            icon: type == PlayerType.internal
                ? Icons.smart_display
                : Icons.open_in_new,
            isSelected: settings.state.playerType == type,
          );
        },
      ),
    );
  }

  static void showSpeedDialog(
    BuildContext context,
    SettingsService settings,
    AppStrings strings,
  ) {
    final speeds = [0.5, 0.75, 1.0, 1.25, 1.5, 1.75, 2.0];
    showModalBottomSheet(
      context: context,
      backgroundColor: AppTheme.surfaceColor,
      shape: const RoundedRectangleBorder(
        borderRadius: BorderRadius.vertical(top: Radius.circular(16)),
      ),
      builder: (context) => SelectionSheet<double>(
        title: strings.defaultSpeed,
        items: speeds,
        selectedItem: settings.state.defaultSpeed,
        onSelected: (speed) {
          settings.setDefaultSpeed(speed);
          context.pop();
        },
        itemBuilder: (speed) {
          return SelectionItem(
            title: '${speed}x',
            icon: Icons.speed,
            isSelected: settings.state.defaultSpeed == speed,
          );
        },
      ),
    );
  }

  static void showNextEpisodeDelayDialog(
    BuildContext context,
    SettingsService settings,
    AppStrings strings,
  ) {
    final delays = [0, 5, 10, 15, 20];
    showModalBottomSheet(
      context: context,
      backgroundColor: AppTheme.surfaceColor,
      shape: const RoundedRectangleBorder(
        borderRadius: BorderRadius.vertical(top: Radius.circular(16)),
      ),
      builder: (context) => SelectionSheet<int>(
        title: strings.nextEpisodeDelay,
        items: delays,
        selectedItem: settings.state.nextEpisodeDelay,
        onSelected: (delay) {
          settings.setNextEpisodeDelay(delay);
          context.pop();
        },
        itemBuilder: (delay) {
          return SelectionItem(
            title: '$delay ${strings.seconds}',
            icon: Icons.timer,
            isSelected: settings.state.nextEpisodeDelay == delay,
          );
        },
      ),
    );
  }

  static void showFullscreenDelayDialog(
    BuildContext context,
    SettingsService settings,
    AppStrings strings,
  ) {
    final delays = [0, 500, 1000, 1500, 2000, 3000];
    showModalBottomSheet(
      context: context,
      backgroundColor: AppTheme.surfaceColor,
      shape: const RoundedRectangleBorder(
        borderRadius: BorderRadius.vertical(top: Radius.circular(16)),
      ),
      builder: (context) => SelectionSheet<int>(
        title: strings.fullscreenDelay,
        items: delays,
        selectedItem: settings.state.fullscreenTransitionDelay,
        onSelected: (delay) {
          settings.setFullscreenTransitionDelay(delay);
          context.pop();
        },
        itemBuilder: (delay) {
          return SelectionItem(
            title: '$delay ${strings.milliseconds}',
            icon: Icons.fullscreen,
            isSelected: settings.state.fullscreenTransitionDelay == delay,
          );
        },
      ),
    );
  }

  static void showDeviceTypeDialog(
    BuildContext context,
    SettingsService settings,
    AppStrings strings,
  ) {
    final Map<String?, String> deviceTypes = {
      null: strings.system,
      'desktop': 'Desktop',
      'mobile': 'Mobile',
      'tv': 'TV',
    };
    final Map<String?, IconData> deviceIcons = {
      null: Icons.settings_system_daydream,
      'desktop': Icons.desktop_windows,
      'mobile': Icons.smartphone,
      'tv': Icons.tv,
    };

    showModalBottomSheet(
      context: context,
      backgroundColor: AppTheme.surfaceColor,
      shape: const RoundedRectangleBorder(
        borderRadius: BorderRadius.vertical(top: Radius.circular(16)),
      ),
      builder: (context) => SelectionSheet<String?>(
        title: strings.deviceTypeOverride,
        items: deviceTypes.keys.toList(),
        selectedItem: settings.state.deviceTypeOverride == 'auto'
            ? null
            : settings.state.deviceTypeOverride,
        onSelected: (type) {
          settings.setDeviceTypeOverride(type ?? 'auto');
          context.pop();
        },
        itemBuilder: (type) {
          return SelectionItem(
            title: deviceTypes[type] ?? type.toString(),
            icon: deviceIcons[type] ?? Icons.device_unknown,
            isSelected: settings.state.deviceTypeOverride == (type ?? 'auto'),
          );
        },
      ),
    );
  }
}
