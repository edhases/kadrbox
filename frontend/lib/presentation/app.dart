import 'dart:ui';
import 'package:flutter/material.dart';
import 'package:flutter_localizations/flutter_localizations.dart';
import 'package:get_it/get_it.dart';

import '../core/l10n/app_strings.dart';
import '../data/services/settings_service.dart';
import 'theme/app_theme.dart';
import 'router/app_router.dart';
import 'widgets/update_guard.dart';

// Re-export for convenience
export '../data/services/settings_service.dart' show AppThemeMode;
export '../core/l10n/app_strings.dart' show AppLocale, AppStrings;

/// Main application widget
class KadrboxApp extends StatefulWidget {
  const KadrboxApp({super.key});

  @override
  State<KadrboxApp> createState() => _KadrboxAppState();
}

class _KadrboxAppState extends State<KadrboxApp> {
  final _settings = GetIt.instance<SettingsService>();

  @override
  void initState() {
    super.initState();
    _settings.addListener(_onSettingsChanged);
  }

  @override
  void dispose() {
    _settings.removeListener(_onSettingsChanged);
    super.dispose();
  }

  void _onSettingsChanged() {
    if (mounted) setState(() {});
  }

  @override
  Widget build(BuildContext context) {
    final accentColor = _settings.uiSettings.accentColor;
    final themeMode = _settings.state.flutterThemeMode;
    final isAmoled = _settings.state.theme == AppThemeMode.amoled;
    final locale = _settings.state.locale;

    return MaterialApp.router(
      title: 'Kadrbox',
      debugShowCheckedModeBanner: false,
      routerConfig: AppRouter.router,
      scrollBehavior: const _AppScrollBehavior(),
      theme: AppTheme.lightTheme,
      darkTheme: isAmoled
          ? AppTheme.buildAmoledTheme(accent: accentColor)
          : AppTheme.buildDarkTheme(accent: accentColor),
      themeMode: themeMode,
      locale: locale.locale,
      supportedLocales: const [Locale('uk'), Locale('en')],
      builder: (context, child) {
        return UpdateGuard(child: child!);
      },
      localizationsDelegates: [
        const AppStringsDelegate(),
        GlobalMaterialLocalizations.delegate,
        GlobalWidgetsLocalizations.delegate,
        GlobalCupertinoLocalizations.delegate,
      ],
    );
  }
}

/// Custom scroll behavior to enable mouse dragging for horizontal lists
class _AppScrollBehavior extends MaterialScrollBehavior {
  const _AppScrollBehavior();

  @override
  Set<PointerDeviceKind> get dragDevices => {
    PointerDeviceKind.touch,
    PointerDeviceKind.mouse,
    PointerDeviceKind.trackpad,
  };
}
