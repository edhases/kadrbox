import 'package:flutter/material.dart';
import 'package:flutter/foundation.dart';
import 'package:media_kit/media_kit.dart';
import 'package:window_manager/window_manager.dart';

import 'core/di/injection.dart';
import 'core/services/version_service.dart';
import 'core/utils/logger.dart';
import 'data/providers/provider_registry.dart';
import 'data/services/auth_service.dart';
import 'presentation/app.dart';

void main() async {
  WidgetsFlutterBinding.ensureInitialized();

  // Desktop window configuration (must be initialized early before engine renders)
  if (!kIsWeb &&
      (defaultTargetPlatform == TargetPlatform.windows ||
          defaultTargetPlatform == TargetPlatform.linux ||
          defaultTargetPlatform == TargetPlatform.macOS)) {
    try {
      await windowManager.ensureInitialized();

      const windowOptions = WindowOptions(
        size: Size(1280, 720),
        minimumSize: Size(800, 600),
        center: true,
        backgroundColor: Color(0xFF0F172A),
        skipTaskbar: false,
        titleBarStyle: TitleBarStyle.hidden,
        title: 'Oxide Film',
      );

      windowManager.waitUntilReadyToShow(windowOptions, () async {
        await windowManager.show();
        await windowManager.focus();
      });

      // Fallback: ensure window is revealed even if waitUntilReadyToShow is delayed
      Future.delayed(const Duration(milliseconds: 400), () async {
        try {
          if (!await windowManager.isVisible()) {
            await windowManager.show();
            await windowManager.focus();
          }
        } catch (_) {}
      });
    } catch (e) {
      Logger.e('WindowManager initialization failed', tag: 'Main', error: e);
    }
  }

  // Initialize MediaKit
  try {
    MediaKit.ensureInitialized();
  } catch (e) {
    Logger.e('MediaKit initialization failed', tag: 'Main', error: e);
  }

  // Initialize dependency injection with safety timeout
  try {
    await configureDependencies().timeout(const Duration(seconds: 5));
  } catch (e) {
    Logger.e('Dependency injection initialization timed out or failed', tag: 'Main', error: e);
  }

  // Initialize VersionService with safety timeout
  try {
    await VersionService.init().timeout(const Duration(seconds: 3));
  } catch (e) {
    Logger.w('VersionService initialization timed out or failed: $e', tag: 'Main');
  }

  // Proactively refresh auth session in background
  _refreshAuthSession();

  // Resolve provider URLs in background (detects domain changes)
  _resolveProviderUrls();

  runApp(const OxideFilmApp());
}

/// Resolve provider URLs in background
///
/// This detects domain changes (e.g., uaflix.net -> uafix.net)
/// by following HTTP redirects and caching results.
void _resolveProviderUrls() {
  try {
    final registry = getIt<ProviderRegistry>();
    // Run in background, don't block app startup
    registry
        .resolveProviderUrls()
        .then((_) {
          Logger.d('Provider URLs resolved', tag: 'Main');
        })
        .catchError((e) {
          Logger.w('URL resolution failed: $e', tag: 'Main');
        });
  } catch (e) {
    Logger.w('Failed to start URL resolution: $e', tag: 'Main');
  }
}

/// Proactively refresh auth session in background
void _refreshAuthSession() {
  try {
    final authService = getIt<AuthService>();
    if (authService.isAuthenticated) {
      authService
          .refreshAuth()
          .then((success) {
            if (success) {
              Logger.i('Auth session refreshed successfully', tag: 'Main');
            } else {
              Logger.w(
                'Auth session refresh failed (token might be expired)',
                tag: 'Main',
              );
            }
          })
          .catchError((e) {
            Logger.e('Error during auth refresh', tag: 'Main', error: e);
          });
    }
  } catch (e) {
    Logger.w('Failed to start auth refresh: $e', tag: 'Main');
  }
}
