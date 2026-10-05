import 'package:flutter/material.dart';
import 'package:get_it/get_it.dart';
import 'package:go_router/go_router.dart';
import 'package:shared_preferences/shared_preferences.dart';

import '../../data/services/oxide_server_service.dart';
import '../../domain/entities/entities.dart';
import '../pages/home/home_page.dart';
import '../pages/details/details_page.dart';
import '../pages/search/search_page.dart';
import '../pages/settings/settings_page.dart';
import '../pages/settings/appearance_page.dart';
import '../pages/player/player_page.dart';
import '../pages/favorites/favorites_page.dart';
import '../pages/history/history_page.dart';
import '../pages/category/category_page.dart';
import '../pages/provider/provider_page.dart';
import '../pages/downloads/downloads_page.dart';
import '../pages/stats/stats_page.dart';
import '../pages/watch_party/watch_party_page.dart';
import '../pages/auth/login_page.dart';
import '../pages/auth/onboarding_page.dart';
import '../pages/auth/register_page.dart';
import '../pages/auth/verify_email_page.dart';
import '../pages/auth/profile_page.dart';
import '../pages/plugins/plugins_page.dart';
import '../widgets/player/mini_player_overlay.dart';

/// Application router configuration
class AppRouter {
  static bool _isFirstLaunch() {
    try {
      // Already signed in вЂ” onboarding is irrelevant.
      if (GetIt.instance<OxideServerService>().isAuthenticated) return false;
      final prefs = GetIt.instance<SharedPreferences>();
      return !OnboardingPage.isSeen(prefs);
    } catch (_) {
      return false;
    }
  }

  /// Safely decode mediaId path parameter.
  ///
  /// GoRouter already decodes path parameters in recent versions.
  /// Calling Uri.decodeComponent unconditionally throws FormatException
  /// ("Illegal percent encoding in URI") whenever the parameter already
  /// contains a '%' character (e.g. "100% Вовк" or JSON payload).
  static String safeDecodeMediaId(String raw) {
    try {
      return Uri.decodeComponent(raw);
    } catch (_) {
      return raw;
    }
  }

  static final GoRouter router = GoRouter(
    initialLocation: '/',
    redirect: (context, state) {
      final location = state.uri.toString();
      final onboarding =
          location == '/onboarding' || location.startsWith('/onboarding?');
      if (_isFirstLaunch() && !onboarding) return '/onboarding';
      if (!_isFirstLaunch() && onboarding) return '/';
      return null;
    },
    routes: [
      ShellRoute(
        builder: (context, state, child) {
          return MiniPlayerOverlay(child: child);
        },
        routes: [
          // Home
          GoRoute(
            path: '/',
            name: 'home',
            builder: (context, state) => const HomePage(),
          ),

          // Auth routes
          GoRoute(
            path: '/onboarding',
            name: 'onboarding',
            builder: (context, state) => const OnboardingPage(),
          ),
          GoRoute(
            path: '/login',
            name: 'login',
            builder: (context, state) => const LoginPage(),
          ),
          GoRoute(
            path: '/register',
            name: 'register',
            builder: (context, state) => const RegisterPage(),
          ),
          GoRoute(
            path: '/verify-email',
            name: 'verify-email',
            builder: (context, state) {
              final email = state.uri.queryParameters['email'] ?? '';
              return VerifyEmailPage(email: email);
            },
          ),
          GoRoute(
            path: '/profile',
            name: 'profile',
            builder: (context, state) => const ProfilePage(),
          ),

          // Deep link handler for OAuth2 redirect
          GoRoute(
            path: '/auth',
            name: 'oauth_callback',
            builder: (context, state) => Scaffold(
              appBar: AppBar(),
              body: Center(
                child: Column(
                  mainAxisSize: MainAxisSize.min,
                  children: [
                    const CircularProgressIndicator(),
                    const SizedBox(height: 16),
                    TextButton(
                      onPressed: () => context.go('/'),
                      child: const Text('Скасувати'),
                    ),
                  ],
                ),
              ),
            ),
          ),

          // Categories/Browse
          GoRoute(
            path: '/category',
            name: 'category',
            builder: (context, state) {
              final typeStr = state.uri.queryParameters['type'];
              ContentType? type;
              if (typeStr != null) {
                type = ContentType.values.firstWhere(
                  (t) => t.name == typeStr,
                  orElse: () => ContentType.movie,
                );
              }
              return CategoryPage(initialType: type);
            },
          ),

          // Single separate provider page
          GoRoute(
            path: '/provider/:providerId',
            name: 'provider',
            builder: (context, state) {
              final providerId = state.pathParameters['providerId']!;
              return ProviderPage(providerId: providerId);
            },
          ),

          // Media details - mediaId is URL-encoded to handle slashes
          GoRoute(
            path: '/details/:providerId/:mediaId',
            name: 'details',
            pageBuilder: (context, state) {
              final providerId = state.pathParameters['providerId']!;
              final rawMediaId = state.pathParameters['mediaId']!;
              final mediaId = safeDecodeMediaId(rawMediaId);
              return _buildPageWithThemeTransition(
                context: context,
                state: state,
                child: DetailsPage(providerId: providerId, mediaId: mediaId),
              );
            },
          ),

          // Video player - supports both query params and extra data
          GoRoute(
            path: '/player',
            name: 'player',
            builder: (context, state) {
              // Support both query params and extra data
              final extra = state.extra as Map<String, dynamic>?;
              final url =
                  extra?['url'] as String? ?? state.uri.queryParameters['url'];
              if (url == null || url.isEmpty) {
                return Scaffold(
                  appBar: AppBar(),
                  body: Center(
                    child: Column(
                      mainAxisSize: MainAxisSize.min,
                      children: [
                        const Text(
                          'РќРµРјР°С” URL РґР»СЏ РІС–РґС‚РІРѕСЂРµРЅРЅСЏ',
                        ),
                        const SizedBox(height: 12),
                        TextButton(
                          onPressed: () => context.go('/'),
                          child: const Text('РќР° РіРѕР»РѕРІРЅСѓ'),
                        ),
                      ],
                    ),
                  ),
                );
              }
              final title =
                  extra?['title'] ?? state.uri.queryParameters['title'];
              final subtitle =
                  extra?['subtitle'] ?? state.uri.queryParameters['subtitle'];
              final streams = extra?['streams'] as List<StreamSource>?;
              final mediaId = extra?['mediaId'] as String?;
              final providerId = extra?['providerId'] as String?;
              final posterUrl = extra?['posterUrl'] as String?;
              final mediaType = extra?['mediaType'] as ContentType?;
              final isOffline =
                  (extra?['isOffline'] as bool?) ??
                  state.uri.queryParameters['offline'] == 'true';
              final season = extra?['season'] as int?;
              final episode = extra?['episode'] as int?;
              final episodeTitle = extra?['episodeTitle'] as String?;

              return PlayerPage(
                url: url,
                title: title,
                subtitle: subtitle,
                streams: streams,
                mediaId: mediaId,
                providerId: providerId,
                posterUrl: posterUrl,
                mediaType: mediaType,
                season: season,
                episode: episode,
                episodeTitle: episodeTitle,
                isOffline: isOffline,
              );
            },
          ),

          // Search
          GoRoute(
            path: '/search',
            name: 'search',
            builder: (context, state) {
              final query = state.uri.queryParameters['q'];
              return SearchPage(initialQuery: query);
            },
          ),

          // Settings
          GoRoute(
            path: '/settings',
            name: 'settings',
            builder: (context, state) => const SettingsPage(),
          ),

          // Plugins / Sources
          GoRoute(
            path: '/plugins',
            name: 'plugins',
            builder: (context, state) => const PluginsPage(),
          ),

          // Appearance settings
          GoRoute(
            path: '/appearance',
            name: 'appearance',
            builder: (context, state) => const AppearancePage(),
          ),

          // Favorites
          GoRoute(
            path: '/favorites',
            name: 'favorites',
            builder: (context, state) => const FavoritesPage(),
          ),

          // History
          GoRoute(
            path: '/history',
            name: 'history',
            builder: (context, state) => const HistoryPage(),
          ),

          // Downloads (offline)
          GoRoute(
            path: '/downloads',
            name: 'downloads',
            builder: (context, state) => const DownloadsPage(),
          ),

          // Statistics
          GoRoute(
            path: '/stats',
            name: 'stats',
            builder: (context, state) => const StatsPage(),
          ),

          // Watch Party
          GoRoute(
            path: '/watch-party',
            name: 'watch-party',
            builder: (context, state) {
              final extra = state.extra as Map<String, dynamic>?;
              return WatchPartyPage(
                mediaUrl: extra?['mediaUrl'],
                mediaTitle: extra?['mediaTitle'],
              );
            },
          ),
        ],
      ),
    ],
  );

  static Page<dynamic> _buildPageWithThemeTransition({
    required BuildContext context,
    required GoRouterState state,
    required Widget child,
  }) {
    return CustomTransitionPage<void>(
      key: state.pageKey,
      child: child,
      opaque: true,
      transitionsBuilder: (context, animation, secondaryAnimation, child) {
        return FadeTransition(
          opacity: CurveTween(curve: Curves.easeInOut).animate(animation),
          child: child,
        );
      },
    );
  }
}
