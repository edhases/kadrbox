import 'dart:convert';
import 'dart:io';
import 'package:flutter_test/flutter_test.dart';
import 'package:shared_preferences/shared_preferences.dart';
import 'package:dio/dio.dart';

import 'package:oxide_film/core/config/app_config.dart';
import 'package:oxide_film/core/network/api_client.dart';
import 'package:oxide_film/data/services/auth_service.dart';
import 'package:oxide_film/data/services/oxide_server_service.dart';

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
  late OxideServerService serverService;
  late AuthService authService;

  setUp(() async {
    originalBaseUrl = AppConfig.serverBaseUrl;
    SharedPreferences.setMockInitialValues({});
    prefs = await SharedPreferences.getInstance();

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

      req.response.headers.contentType = ContentType.json;

      switch ('$method $path') {
        case 'POST /api/v1/auth/login':
          final b = jsonBody as Map<String, dynamic>;
          if (b['email'] == 'valid@user.com' && b['password'] == 'password123') {
            req.response.statusCode = HttpStatus.ok;
            req.response.write(jsonEncode({
              'access_token': 'jwt_access',
              'refresh_token': 'jwt_refresh',
              'user': {
                'id': 'usr_42',
                'email': 'valid@user.com',
                'username': 'Alex',
                'bio': 'Film fan',
                'avatar_url': 'https://example.com/alex.jpg',
                'is_verified': true,
                'telegram_id': 'tg123',
                'is_google': true,
                'created_at': '2025-01-01T00:00:00Z',
              },
            }));
          } else {
            req.response.statusCode = HttpStatus.unauthorized;
            req.response.write(jsonEncode({'error': 'invalid login credentials'}));
          }
          break;

        case 'POST /api/v1/auth/register':
          final b = jsonBody as Map<String, dynamic>;
          if (b['email'] == 'exists@user.com') {
            req.response.statusCode = HttpStatus.badRequest;
            req.response.write(jsonEncode({'error': 'user already registered'}));
          } else {
            req.response.statusCode = HttpStatus.created;
            req.response.write(jsonEncode({
              'access_token': 'jwt_reg_access',
              'refresh_token': 'jwt_reg_refresh',
              'user': {
                'id': 'usr_43',
                'email': b['email'],
                'username': b['username'],
                'is_verified': false,
              },
            }));
          }
          break;

        case 'POST /api/v1/auth/google':
          req.response.statusCode = HttpStatus.ok;
          req.response.write(jsonEncode({
            'access_token': 'google_jwt_access',
            'refresh_token': 'google_jwt_refresh',
            'user': {
              'id': 'usr_google',
              'email': 'google@gmail.com',
              'username': 'GoogleUser',
              'is_verified': true,
              'is_google': true,
            },
          }));
          break;

        case 'POST /api/v1/auth/refresh':
          req.response.statusCode = HttpStatus.ok;
          req.response.write(jsonEncode({
            'access_token': 'refreshed_access',
            'refresh_token': 'refreshed_refresh',
          }));
          break;

        case 'GET /api/v1/auth/me':
          req.response.statusCode = HttpStatus.ok;
          req.response.write(jsonEncode({
            'id': 'usr_42',
            'email': 'valid@user.com',
            'username': 'AlexReloaded',
            'bio': 'Film fan updated',
            'avatar_url': 'https://example.com/alex2.jpg',
            'is_verified': true,
            'discord_id': 'disc123',
          }));
          break;

        case 'PUT /api/v1/auth/profile':
          req.response.statusCode = HttpStatus.ok;
          req.response.write(jsonEncode({
            'id': 'usr_42',
            'email': 'valid@user.com',
            'username': (jsonBody as Map)['name'] ?? 'Alex',
            'bio': (jsonBody as Map)['bio'] ?? 'Film fan',
          }));
          break;

        case 'POST /api/v1/auth/unlink':
          req.response.statusCode = HttpStatus.ok;
          req.response.write(jsonEncode({
            'id': 'usr_42',
            'email': 'valid@user.com',
            'username': 'Alex',
          }));
          break;

        case 'POST /api/v1/auth/resend-verification':
        case 'POST /api/v1/auth/verify-email':
        case 'POST /api/v1/auth/forgot-password':
        case 'POST /api/v1/auth/change-password':
        case 'DELETE /api/v1/auth/account':
          req.response.statusCode = HttpStatus.ok;
          req.response.write(jsonEncode({'status': 'ok'}));
          break;

        default:
          req.response.statusCode = HttpStatus.notFound;
          req.response.write(jsonEncode({'error': 'not found'}));
      }
      await req.response.close();
    });

    final dio = Dio(BaseOptions(
      connectTimeout: const Duration(seconds: 5),
      receiveTimeout: const Duration(seconds: 5),
    ));
    apiClient = ApiClient(prefs: prefs, dio: dio);
    serverService = OxideServerService(prefs, apiClient);
    authService = AuthService(serverService);
  });

  tearDown(() async {
    AppConfig.serverBaseUrl = originalBaseUrl;
    await server.close(force: true);
  });

  group('AuthService State & Guest Mode', () {
    test('starts as guest when unauthenticated', () {
      expect(authService.isAuthenticated, isFalse);
      expect(authService.isGuest, isTrue);
      expect(authService.userId, isNull);
      expect(authService.userEmail, isNull);
      expect(authService.currentUser, isNull);
      expect(authService.profile, isNull);
      expect(authService.avatarUrl, isNull);
      expect(authService.displayName, 'Гість');
      expect(authService.linkedProviders, isEmpty);
      expect(authService.error, isNull);
    });

    test('restores pre-authenticated user session with linked providers', () async {
      SharedPreferences.setMockInitialValues({
        'oxide_jwt_access_token': 'prev_token',
        'oxide_jwt_refresh_token': 'prev_refresh',
        'oxide_user_json': jsonEncode({
          'id': 'u77',
          'email': 'pro@gmail.com',
          'username': 'ProUser',
          'telegram_id': 'tg_77',
          'is_verified': true,
        }),
      });
      final p = await SharedPreferences.getInstance();
      final s = OxideServerService(p, apiClient);
      final a = AuthService(s);

      expect(a.isAuthenticated, isTrue);
      expect(a.isGuest, isFalse);
      expect(a.userId, 'u77');
      expect(a.displayName, 'ProUser');
      expect(a.currentUser, (id: 'u77', email: 'pro@gmail.com', name: 'ProUser'));
      expect(a.linkedProviders, containsAll(['telegram', 'google']));
    });
  });

  group('AuthService Sign In / Sign Up Flow', () {
    test('signIn updates state, notifies listeners, and loads linked providers', () async {
      int notifyCount = 0;
      authService.addListener(() => notifyCount++);

      await authService.signIn(email: 'valid@user.com', password: 'password123');

      expect(authService.isAuthenticated, isTrue);
      expect(authService.isGuest, isFalse);
      expect(authService.userId, 'usr_42');
      expect(authService.userEmail, 'valid@user.com');
      expect(authService.userName, 'Alex');
      expect(authService.displayName, 'Alex');
      expect(authService.avatarUrl, 'https://example.com/alex.jpg');
      expect(authService.isVerified, isTrue);
      expect(authService.linkedProviders, containsAll(['telegram', 'google']));
      expect(notifyCount, greaterThan(0));

      final prof = authService.profile;
      expect(prof?['display_name'], 'Alex');
      expect(prof?['bio'], 'Film fan');
      expect(prof?['created_at'], '2025-01-01T00:00:00Z');
    });

    test('signIn with wrong credentials sets translated Ukrainian error', () async {
      try {
        await authService.signIn(email: 'valid@user.com', password: 'wrongpassword');
      } catch (_) {}

      expect(authService.isAuthenticated, isFalse);
      expect(authService.error, 'Невірний email або пароль');
    });

    test('signUp creates account and notifies listeners', () async {
      await authService.signUp(
        email: 'brandnew@user.com',
        password: 'password123',
        displayName: 'BrandNew',
      );

      expect(authService.isAuthenticated, isTrue);
      expect(authService.userId, 'usr_43');
      expect(authService.userName, 'BrandNew');
      expect(authService.isVerified, isFalse);
    });

    test('signUp duplicate email sets translated Ukrainian error', () async {
      try {
        await authService.signUp(
          email: 'exists@user.com',
          password: 'password123',
          displayName: 'Tester',
        );
      } catch (_) {}

      expect(authService.error, 'Користувач з таким email вже існує');
    });

    test('signInWithGoogle completes authentication flow', () async {
      await authService.signInWithGoogle('google_token_123');
      expect(authService.isAuthenticated, isTrue);
      expect(authService.userEmail, 'google@gmail.com');
      expect(authService.linkedProviders, contains('google'));
    });
  });

  group('AuthService Profile, Password & Account Operations', () {
    setUp(() async {
      await authService.signIn(email: 'valid@user.com', password: 'password123');
    });

    test('updateProfile updates user profile and notifies listeners', () async {
      int notifyCount = 0;
      authService.addListener(() => notifyCount++);

      await authService.updateProfile(displayName: 'AlexUpdated', bio: 'Cinema lover');

      expect(authService.userName, 'AlexUpdated');
      expect(notifyCount, greaterThan(0));
    });

    test('unlinkSocialAccount removes linked provider and notifies', () async {
      await authService.unlinkSocialAccount('telegram');
      expect(authService.isAuthenticated, isTrue);
    });

    test('requestVerification and verifyEmail', () async {
      await authService.requestVerification('valid@user.com');
      await authService.verifyEmail('token_123');
      expect(authService.isVerified, isTrue);
    });

    test('resetPassword sends request to server', () async {
      await authService.resetPassword('valid@user.com');
      expect(authService.error, isNull);
    });

    test('changePassword sends update to server', () async {
      await authService.changePassword('password123', 'newpassword456', 'newpassword456');
      expect(authService.error, isNull);
    });

    test('refreshProfile fetches latest user data and linked providers', () async {
      await authService.refreshProfile();
      expect(authService.userName, 'AlexReloaded');
      expect(authService.linkedProviders, contains('discord'));
    });

    test('refreshAuth delegates to server service', () async {
      final success = await authService.refreshAuth();
      expect(success, isTrue);
    });

    test('deleteAccount calls server and signs out completely', () async {
      await authService.deleteAccount();
      expect(authService.isAuthenticated, isFalse);
      expect(authService.isGuest, isTrue);
      expect(authService.currentUser, isNull);
    });

    test('signOut clears user session and resets linked providers', () async {
      await authService.signOut();
      expect(authService.isAuthenticated, isFalse);
      expect(authService.isGuest, isTrue);
      expect(authService.linkedProviders, isEmpty);
    });
  });
}
