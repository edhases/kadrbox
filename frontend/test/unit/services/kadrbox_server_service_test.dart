import 'dart:convert';
import 'dart:io';
import 'package:flutter_test/flutter_test.dart';
import 'package:shared_preferences/shared_preferences.dart';
import 'package:dio/dio.dart';

import 'package:kadrbox/core/config/app_config.dart';
import 'package:kadrbox/core/error/exceptions.dart';
import 'package:kadrbox/core/network/api_client.dart';
import 'package:kadrbox/data/services/kadrbox_server_service.dart';

class _AllowAllHttpOverrides extends HttpOverrides {
  @override
  HttpClient createHttpClient(SecurityContext? context) {
    return super.createHttpClient(context);
  }
}

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();
  HttpOverrides.global = _AllowAllHttpOverrides();

  late HttpServer server;
  late String originalBaseUrl;
  late SharedPreferences prefs;
  late ApiClient apiClient;
  late KadrboxServerService service;

  // Captured requests for assertions
  final capturedRequests = <String, dynamic>{};
  int refreshCallCount = 0;
  bool return401OnRefresh = false;
  bool return401OnSync = false;

  setUp(() async {
    originalBaseUrl = AppConfig.serverBaseUrl;
    SharedPreferences.setMockInitialValues({});
    prefs = await SharedPreferences.getInstance();

    refreshCallCount = 0;
    return401OnRefresh = false;
    return401OnSync = false;
    capturedRequests.clear();

    // Start real loopback HTTP server
    server = await HttpServer.bind(InternetAddress.loopbackIPv4, 0);
    AppConfig.serverBaseUrl = 'http://127.0.0.1:${server.port}';

    server.listen((HttpRequest req) async {
      final path = req.uri.path;
      final method = req.method;

      String bodyStr = '';
      if (req.contentLength > 0) {
        bodyStr = await utf8.decodeStream(req);
      }
      dynamic jsonBody;
      if (bodyStr.isNotEmpty) {
        try {
          jsonBody = jsonDecode(bodyStr);
        } catch (_) {
          jsonBody = bodyStr;
        }
      }
      capturedRequests['$method $path'] = {
        'headers': req.headers,
        'body': jsonBody,
        'query': req.uri.queryParameters,
      };

      req.response.headers.contentType = ContentType.json;

      if (return401OnSync && path.startsWith('/api/v1/sync/')) {
        req.response.statusCode = HttpStatus.unauthorized;
        req.response.write(jsonEncode({'error': 'invalid or expired token'}));
        await req.response.close();
        return;
      }

      switch ('$method $path') {
        case 'POST /api/v1/auth/login':
          final b = jsonBody as Map<String, dynamic>;
          if (b['email'] == 'valid@user.com' && b['password'] == 'secret') {
            req.response.statusCode = HttpStatus.ok;
            req.response.write(
              jsonEncode({
                'access_token': 'jwt_access_123',
                'refresh_token': 'jwt_refresh_456',
                'user': {
                  'id': 'u100',
                  'email': 'valid@user.com',
                  'username': 'Valik',
                  'bio': 'Test bio',
                  'avatar_url': 'https://example.com/avatar.png',
                  'is_verified': true,
                },
              }),
            );
          } else {
            req.response.statusCode = HttpStatus.unauthorized;
            req.response.write(jsonEncode({'error': 'invalid credentials'}));
          }
          break;

        case 'POST /api/v1/auth/register':
          req.response.statusCode = HttpStatus.created;
          req.response.write(
            jsonEncode({
              'access_token': 'reg_jwt_access',
              'refresh_token': 'reg_jwt_refresh',
              'user': {
                'id': 'u101',
                'email': jsonBody['email'],
                'username': jsonBody['username'],
                'is_verified': false,
              },
            }),
          );
          break;

        case 'POST /api/v1/auth/google':
        case 'POST /api/v1/auth/discord':
        case 'POST /api/v1/auth/telegram':
          req.response.statusCode = HttpStatus.ok;
          req.response.write(
            jsonEncode({
              'access_token': 'oauth_access_token',
              'refresh_token': 'oauth_refresh_token',
              'user': {
                'id': 'oauth_user_1',
                'email': 'oauth@user.com',
                'username': 'OAuthUser',
                'is_verified': true,
              },
            }),
          );
          break;

        case 'POST /api/v1/auth/refresh':
          refreshCallCount++;
          if (return401OnRefresh) {
            req.response.statusCode = HttpStatus.unauthorized;
            req.response.write(
              jsonEncode({'error': 'invalid or expired refresh token'}),
            );
          } else {
            req.response.statusCode = HttpStatus.ok;
            req.response.write(
              jsonEncode({
                'access_token': 'refreshed_access_token',
                'refresh_token': 'refreshed_refresh_token',
              }),
            );
          }
          break;

        case 'GET /api/v1/auth/me':
          req.response.statusCode = HttpStatus.ok;
          req.response.write(
            jsonEncode({
              'id': 'u100',
              'email': 'valid@user.com',
              'username': 'ValikUpdated',
              'bio': 'Updated bio',
              'avatar_url': 'https://example.com/avatar2.png',
              'is_verified': true,
            }),
          );
          break;

        case 'PUT /api/v1/auth/profile':
          req.response.statusCode = HttpStatus.ok;
          req.response.write(
            jsonEncode({
              'id': 'u100',
              'email': 'valid@user.com',
              'username': jsonBody['name'] ?? 'Valik',
              'bio': jsonBody['bio'] ?? '',
              'avatar_url': jsonBody['avatar'] ?? '',
            }),
          );
          break;

        case 'POST /api/v1/auth/verify-email':
        case 'POST /api/v1/auth/resend-verification':
        case 'POST /api/v1/auth/forgot-password':
        case 'POST /api/v1/auth/reset-password':
        case 'POST /api/v1/auth/change-password':
        case 'DELETE /api/v1/auth/account':
          req.response.statusCode = HttpStatus.ok;
          req.response.write(jsonEncode({'status': 'ok'}));
          break;

        case 'POST /api/v1/auth/unlink':
          req.response.statusCode = HttpStatus.ok;
          req.response.write(
            jsonEncode({
              'id': 'u100',
              'email': 'valid@user.com',
              'username': 'Valik',
              'is_verified': true,
            }),
          );
          break;

        case 'GET /api/v1/sync/history':
          req.response.statusCode = HttpStatus.ok;
          req.response.write(
            jsonEncode([
              {
                'media_id': 'm1',
                'provider_id': 'src_a',
                'title': 'Test Movie',
                'position_ms': 120000,
                'duration_ms': 7200000,
              },
            ]),
          );
          break;

        case 'POST /api/v1/sync/history':
          req.response.statusCode = HttpStatus.ok;
          req.response.write(jsonEncode({'status': 'ok'}));
          break;

        case 'GET /api/v1/sync/continue-watching':
          req.response.statusCode = HttpStatus.ok;
          req.response.write(
            jsonEncode([
              {
                'media_id': 'm1',
                'provider_id': 'src_a',
                'title': 'Test Movie',
                'position_ms': 120000,
              },
            ]),
          );
          break;

        case 'GET /api/v1/sync/favorites':
          req.response.statusCode = HttpStatus.ok;
          req.response.write(
            jsonEncode([
              {
                'media_id': 'fav1',
                'provider_id': 'src_a',
                'title': 'Fav Film',
                'media_type': 'movie',
              },
            ]),
          );
          break;

        case 'POST /api/v1/sync/favorites/toggle':
          req.response.statusCode = HttpStatus.ok;
          req.response.write(jsonEncode({'is_favorite': true}));
          break;

        case 'DELETE /api/v1/sync/favorites':
          req.response.statusCode = HttpStatus.ok;
          req.response.write(jsonEncode({'status': 'ok'}));
          break;

        default:
          req.response.statusCode = HttpStatus.notFound;
          req.response.write(jsonEncode({'error': 'not found'}));
      }
      await req.response.close();
    });

    final dio = Dio(
      BaseOptions(
        connectTimeout: const Duration(seconds: 5),
        receiveTimeout: const Duration(seconds: 5),
      ),
    );
    apiClient = ApiClient(prefs: prefs, dio: dio);
    service = KadrboxServerService(prefs, apiClient);
  });

  tearDown(() async {
    AppConfig.serverBaseUrl = originalBaseUrl;
    await server.close(force: true);
  });

  group('KadrboxServerService Initialization and State Restoration', () {
    test('starts unauthenticated when SharedPreferences is empty', () {
      expect(service.isAuthenticated, isFalse);
      expect(service.accessToken, isNull);
      expect(service.userId, isNull);
      expect(service.userEmail, isNull);
      expect(service.userName, isNull);
      expect(service.avatar, isNull);
      expect(service.bio, isNull);
      expect(service.isVerified, isFalse);
      expect(service.user, isNull);
    });

    test('restores session and user info from SharedPreferences', () async {
      final initialData = {
        'oxide_jwt_access_token': 'restored_token',
        'oxide_jwt_refresh_token': 'restored_refresh',
        'oxide_user_json': jsonEncode({
          'id': 'u999',
          'email': 'saved@user.com',
          'username': 'SavedUser',
          'avatar_url': 'https://saved.avatar.png',
          'bio': 'Saved bio',
          'is_verified': true,
        }),
      };
      SharedPreferences.setMockInitialValues(initialData);
      final restoredPrefs = await SharedPreferences.getInstance();
      final restoredService = KadrboxServerService(restoredPrefs, apiClient);

      expect(restoredService.isAuthenticated, isTrue);
      expect(restoredService.accessToken, 'restored_token');
      expect(restoredService.userId, 'u999');
      expect(restoredService.userEmail, 'saved@user.com');
      expect(restoredService.userName, 'SavedUser');
      expect(restoredService.avatar, 'https://saved.avatar.png');
      expect(restoredService.bio, 'Saved bio');
      expect(restoredService.isVerified, isTrue);
    });

    test('handles corrupted user JSON gracefully without throwing', () async {
      SharedPreferences.setMockInitialValues({
        'oxide_jwt_access_token': 'restored_token',
        'oxide_user_json': '{not-valid-json',
      });
      final restoredPrefs = await SharedPreferences.getInstance();
      final restoredService = KadrboxServerService(restoredPrefs, apiClient);

      expect(restoredService.isAuthenticated, isTrue);
      expect(restoredService.user, isNull);
      expect(restoredService.userId, isNull);
    });
  });

  group('KadrboxServerService Authentication & Profile', () {
    test('signIn saves tokens, user data, and updates apiClient', () async {
      await service.signIn('valid@user.com', 'secret');

      expect(service.isAuthenticated, isTrue);
      expect(service.accessToken, 'jwt_access_123');
      expect(service.userId, 'u100');
      expect(service.userEmail, 'valid@user.com');
      expect(service.userName, 'Valik');
      expect(service.isVerified, isTrue);

      // Verify persistent storage
      expect(prefs.getString('oxide_jwt_access_token'), 'jwt_access_123');
      expect(prefs.getString('oxide_jwt_refresh_token'), 'jwt_refresh_456');
      expect(prefs.getString('oxide_user_json'), contains('valid@user.com'));

      // Verify payload sent
      final req = capturedRequests['POST /api/v1/auth/login'];
      expect(req['body']['email'], 'valid@user.com');
      expect(req['body']['password'], 'secret');
    });

    test(
      'signIn with invalid credentials throws ServerException / rethrows',
      () async {
        expect(
          () => service.signIn('wrong@user.com', 'badpassword'),
          throwsA(isA<ServerException>()),
        );
        expect(service.isAuthenticated, isFalse);
      },
    );

    test('signUp registers and stores session', () async {
      await service.signUp(
        email: 'new@user.com',
        password: 'password123',
        name: 'Newbie',
      );

      expect(service.isAuthenticated, isTrue);
      expect(service.accessToken, 'reg_jwt_access');
      expect(service.userName, 'Newbie');
      expect(service.isVerified, isFalse);

      final req = capturedRequests['POST /api/v1/auth/register'];
      expect(req['body']['email'], 'new@user.com');
      expect(req['body']['username'], 'Newbie');
    });

    test('signInWithGoogle saves tokens and user', () async {
      await service.signInWithGoogle('google_token_abc');
      expect(service.isAuthenticated, isTrue);
      expect(service.accessToken, 'oauth_access_token');
      expect(service.userEmail, 'oauth@user.com');
    });

    test(
      'signInWithDiscordCode sends code and optional redirect_uri',
      () async {
        await service.signInWithDiscordCode(
          'discord_code_123',
          redirectUri: 'http://localhost/cb',
        );
        expect(service.isAuthenticated, isTrue);
        final req = capturedRequests['POST /api/v1/auth/discord'];
        expect(req['body']['code'], 'discord_code_123');
        expect(req['body']['redirect_uri'], 'http://localhost/cb');
      },
    );

    test('signInWithTelegramData sends telegram credentials map', () async {
      await service.signInWithTelegramData({'id': 12345, 'hash': 'abcde'});
      expect(service.isAuthenticated, isTrue);
      final req = capturedRequests['POST /api/v1/auth/telegram'];
      expect(req['body']['id'], 12345);
    });

    test(
      'verifyEmail calls verify-email and marks user verified locally',
      () async {
        await service.signIn('valid@user.com', 'secret');
        await service.verifyEmail('token_xyz');

        expect(service.isVerified, isTrue);
        expect(
          capturedRequests.containsKey('POST /api/v1/auth/verify-email'),
          isTrue,
        );
      },
    );

    test(
      'resendVerification, requestPasswordReset, resetPassword, changePassword',
      () async {
        await service.signIn('valid@user.com', 'secret');

        await service.resendVerification('valid@user.com');
        expect(
          capturedRequests.containsKey('POST /api/v1/auth/resend-verification'),
          isTrue,
        );

        await service.requestPasswordReset('valid@user.com');
        expect(
          capturedRequests.containsKey('POST /api/v1/auth/forgot-password'),
          isTrue,
        );

        await service.resetPassword('reset_tok', 'new_pass');
        expect(
          capturedRequests.containsKey('POST /api/v1/auth/reset-password'),
          isTrue,
        );

        await service.changePassword('old_pass', 'new_pass');
        expect(
          capturedRequests.containsKey('POST /api/v1/auth/change-password'),
          isTrue,
        );
      },
    );

    test('fetchMe updates user profile from server', () async {
      await service.signIn('valid@user.com', 'secret');
      final me = await service.fetchMe();

      expect(me, isNotNull);
      expect(me?['username'], 'ValikUpdated');
      expect(service.userName, 'ValikUpdated');
      expect(service.bio, 'Updated bio');
    });

    test('updateProfile modifies profile and persists update', () async {
      await service.signIn('valid@user.com', 'secret');
      await service.updateProfile(name: 'NewValik', bio: 'Brand new bio');

      expect(service.userName, 'NewValik');
      expect(service.bio, 'Brand new bio');
      final req = capturedRequests['PUT /api/v1/auth/profile'];
      expect(req['body']['name'], 'NewValik');
      expect(req['body']['bio'], 'Brand new bio');
    });

    test('unlinkProvider sends unlink request and updates profile', () async {
      await service.signIn('valid@user.com', 'secret');
      await service.unlinkProvider('telegram');

      expect(capturedRequests.containsKey('POST /api/v1/auth/unlink'), isTrue);
    });

    test('deleteAccount calls endpoint and signs out', () async {
      await service.signIn('valid@user.com', 'secret');
      expect(service.isAuthenticated, isTrue);

      await service.deleteAccount();
      expect(service.isAuthenticated, isFalse);
      expect(service.accessToken, isNull);
      expect(prefs.getString('oxide_jwt_access_token'), isNull);
    });

    test(
      'signOut clears tokens, user state, and calls onAuthExpired',
      () async {
        await service.signIn('valid@user.com', 'secret');
        bool expiredCalled = false;
        service.onAuthExpired = () => expiredCalled = true;

        await service.signOut();

        expect(service.isAuthenticated, isFalse);
        expect(service.accessToken, isNull);
        expect(service.user, isNull);
        expect(prefs.getString('oxide_jwt_access_token'), isNull);
        expect(prefs.getString('oxide_jwt_refresh_token'), isNull);
        expect(prefs.getString('oxide_user_json'), isNull);
        expect(expiredCalled, isTrue);
      },
    );
  });

  group('KadrboxServerService refreshAuth & Single-Flight Dedup', () {
    test(
      'refreshAuth returns false and clears session if no refresh_token',
      () async {
        bool expiredCalled = false;
        service.onAuthExpired = () => expiredCalled = true;

        final result = await service.refreshAuth();
        expect(result, isFalse);
        expect(expiredCalled, isTrue);
      },
    );

    test('refreshAuth updates tokens successfully', () async {
      await service.signIn('valid@user.com', 'secret');
      final result = await service.refreshAuth();

      expect(result, isTrue);
      expect(service.accessToken, 'refreshed_access_token');
      expect(
        prefs.getString('oxide_jwt_access_token'),
        'refreshed_access_token',
      );
    });

    test(
      'refreshAuth single-flight deduplicates concurrent requests',
      () async {
        await service.signIn('valid@user.com', 'secret');

        // Launch 5 concurrent refreshAuth calls
        final futures = List.generate(5, (_) => service.refreshAuth());
        final results = await Future.wait(futures);

        expect(results.every((r) => r == true), isTrue);
        // Only 1 actual HTTP request must have been made
        expect(refreshCallCount, 1);
      },
    );

    test(
      'refreshAuth on 401 triggers onAuthExpired and clears session',
      () async {
        await service.signIn('valid@user.com', 'secret');
        return401OnRefresh = true;

        bool expiredCalled = false;
        service.onAuthExpired = () => expiredCalled = true;

        final result = await service.refreshAuth();

        expect(result, isFalse);
        expect(service.isAuthenticated, isFalse);
        expect(expiredCalled, isTrue);
      },
    );
  });

  group('KadrboxServerService Cloud Sync (History & Favorites)', () {
    setUp(() async {
      await service.signIn('valid@user.com', 'secret');
    });

    test('getHistory returns parsed items list', () async {
      final history = await service.getHistory(limit: 50);
      expect(history.length, 1);
      expect(history.first['title'], 'Test Movie');
      expect(history.first['position_ms'], 120000);
    });

    test('saveHistoryProgress sends correct payload to server', () async {
      await service.saveHistoryProgress(
        mediaId: 'm123',
        providerId: 'src_a',
        title: 'Film 123',
        posterUrl: 'https://poster.jpg',
        year: 2024,
        mediaType: 'movie',
        positionMs: 60000,
        durationMs: 120000,
        season: 1,
        episode: 2,
        episodeTitle: 'Ep 2',
      );

      final req = capturedRequests['POST /api/v1/sync/history'];
      expect(req['body']['media_id'], 'm123');
      expect(req['body']['title'], 'Film 123');
      expect(req['body']['position_ms'], 60000);
      expect(req['body']['duration_ms'], 120000);
      expect(req['body']['season'], 1);
      expect(req['body']['episode'], 2);
    });

    test('getContinueWatching returns active in-progress items', () async {
      final items = await service.getContinueWatching();
      expect(items.length, 1);
      expect(items.first['media_id'], 'm1');
    });

    test('getFavorites returns favorites list', () async {
      final favs = await service.getFavorites();
      expect(favs.length, 1);
      expect(favs.first['title'], 'Fav Film');
    });

    test('toggleFavorite returns new state', () async {
      final isFav = await service.toggleFavorite(
        mediaId: 'fav1',
        providerId: 'src_a',
        title: 'Fav Film',
        mediaType: 'movie',
      );
      expect(isFav, isTrue);
    });

    test('removeFavorite sends delete request', () async {
      await service.removeFavorite(mediaId: 'fav1', providerId: 'src_a');
      expect(
        capturedRequests.containsKey('DELETE /api/v1/sync/favorites'),
        isTrue,
      );
    });

    test('401 on sync calls onAuthExpired, signs out, and rethrows', () async {
      return401OnSync = true;
      bool expiredCalled = false;
      service.onAuthExpired = () => expiredCalled = true;

      // Await the thrown error rather than asserting on a callback that returns
      // a Future: `expect(() => ..., throwsA(...))` on an async body does not
      // await it, so the previous `await Future.delayed(10ms)` was the only thing
      // ordering the assertions — and 10ms is not enough for the three
      // SharedPreferences removals inside signOut() when the suite runs in
      // parallel.
      //
      // _withAuthRecovery rethrows only *after* `await _handleAuthExpired()`,
      // which awaits signOut(), which calls onAuthExpired last. So the error
      // surfacing is itself the completion signal: when this await returns,
      // the callback has definitely already run.
      await expectLater(service.getHistory(), throwsA(isA<ServerException>()));

      expect(service.isAuthenticated, isFalse);
      expect(expiredCalled, isTrue);
    });
  });
}
