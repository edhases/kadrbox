// Regression tests for session-handling defects in `ApiClient` and
// `OxideServerService`:
//
//  * no 401 interceptor, so the first 401 after the 15-minute access-token TTL
//    signed the user out instead of refreshing and replaying;
//  * `_isUnauthorized` substring-matched `toString()`, so a socket error whose
//    text happened to contain "401" destroyed a valid session;
//  * the retry interceptor replayed non-idempotent POSTs (a retried
//    `/sync/favorites/toggle` toggles back, a retried `/auth/refresh` presents a
//    consumed token and yields a 401 -> sign-out);
//  * `signOut()` did not cancel an in-flight refresh, so the startup refresh in
//    `main.dart` re-installed the token after the sign-out;
//  * `_user` was an untyped `Map` with implicit dynamic -> String? casts.

import 'dart:async';
import 'dart:convert';
import 'dart:io';

import 'package:flutter_test/flutter_test.dart';
import 'package:shared_preferences/shared_preferences.dart';

import 'package:oxide_film/core/config/app_config.dart';
import 'package:oxide_film/core/error/exceptions.dart';
import 'package:oxide_film/core/network/api_client.dart';
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
  late OxideServerService service;

  /// Per-path request counters keyed by `"$method $path"`.
  final hits = <String, int>{};
  bool failRefresh = false;
  bool refreshGateOpen = true;
  Completer<void>? refreshGate;

  /// Number of upcoming `GET /sync/history` requests that must answer 401.
  int historyFailures = 0;

  /// When set, every `GET /sync/history` answers this status (used for 503).
  int? historyFixedStatus;

  /// Status for `POST /sync/favorites/toggle`; null = 200.
  int? favoritesToggleStatus;

  const historyPath = '/api/v1/sync/history';
  const favoritesTogglePath = '/api/v1/sync/favorites/toggle';

  setUp(() async {
    originalBaseUrl = AppConfig.serverBaseUrl;
    SharedPreferences.setMockInitialValues({});
    prefs = await SharedPreferences.getInstance();

    hits.clear();
    failRefresh = false;
    refreshGateOpen = true;
    refreshGate = null;
    historyFailures = 0;
    historyFixedStatus = null;
    favoritesToggleStatus = null;

    server = await HttpServer.bind(InternetAddress.loopbackIPv4, 0);
    AppConfig.serverBaseUrl = 'http://127.0.0.1:${server.port}';

    unawaited(
      server.forEach((req) async {
        final key = '${req.method} ${req.uri.path}';
        hits[key] = (hits[key] ?? 0) + 1;
        req.response.headers.contentType = ContentType.json;

        switch (key) {
          case 'POST /api/v1/auth/login':
            req.response.statusCode = HttpStatus.ok;
            req.response.write(
              jsonEncode({
                'access_token': 'access_1',
                'refresh_token': 'refresh_1',
                'user': {'id': 'u1', 'email': 'u@e.com', 'is_verified': true},
              }),
            );
          case 'POST /api/v1/auth/refresh':
            if (!refreshGateOpen) {
              await (refreshGate?.future ?? Future<void>.value());
            }
            if (failRefresh) {
              req.response.statusCode = HttpStatus.unauthorized;
              req.response.write(jsonEncode({'error': 'invalid refresh'}));
            } else {
              req.response.statusCode = HttpStatus.ok;
              req.response.write(
                jsonEncode({
                  'access_token': 'access_2',
                  'refresh_token': 'refresh_2',
                }),
              );
            }
          case 'GET $historyPath':
            final fixed = historyFixedStatus;
            if (fixed != null) {
              req.response.statusCode = fixed;
              req.response.write(jsonEncode({'error': 'nope'}));
            } else if (historyFailures > 0) {
              historyFailures--;
              req.response.statusCode = HttpStatus.unauthorized;
              req.response.write(jsonEncode({'error': 'invalid or expired'}));
            } else {
              req.response.statusCode = HttpStatus.ok;
              req.response.write(
                jsonEncode([
                  {'media_id': 'm1', 'provider_id': 'uakino', 'title': 'M1'},
                ]),
              );
            }
          case 'POST $favoritesTogglePath':
            final status = favoritesToggleStatus;
            req.response.statusCode = status ?? HttpStatus.ok;
            req.response.write(jsonEncode({'is_favorite': status == null}));
          default:
            req.response.statusCode = HttpStatus.ok;
            req.response.write(jsonEncode({'status': 'ok'}));
        }
        await req.response.close();
      }),
    );

    // No `dio:` seam here on purpose: this exercises the real interceptor chain
    // (cookies + retry + 401 refresh/replay).
    apiClient = ApiClient();
    service = OxideServerService(prefs, apiClient);
  });

  tearDown(() async {
    apiClient.dispose();
    AppConfig.serverBaseUrl = originalBaseUrl;
    await server.close(force: true);
  });

  Future<void> signIn() => service.signIn('u@e.com', 'secret');

  group('isUnauthorizedError', () {
    test('is true only for a 401 ServerException', () {
      expect(
        service.isUnauthorizedError(
          const ServerException(
            message: 'x',
            statusCode: 401,
            code: 'HTTP_401',
          ),
        ),
        isTrue,
      );
    });

    test(
      'is false for a 503 ServerException (server outage != auth failure)',
      () {
        expect(
          service.isUnauthorizedError(
            const ServerException(
              message: 'x',
              statusCode: 503,
              code: 'HTTP_503',
            ),
          ),
          isFalse,
        );
      },
    );

    test('is false for a socket error whose text mentions 401', () {
      // The old implementation substring-matched `toString()`, so a byte count
      // like "read 401 bytes" triggered _handleAuthExpired() -> signOut().
      final socketish = StateError(
        'SocketException: Connection closed, read 401 bytes from host',
      );
      expect(service.isUnauthorizedError(socketish), isFalse);
      expect(socketish.toString().contains('401'), isTrue);

      expect(
        service.isUnauthorizedError(Exception('HTTP_401 Unauthorized')),
        isFalse,
      );
      expect(
        service.isUnauthorizedError(Exception('invalid or expired token')),
        isFalse,
      );
      expect(
        service.isUnauthorizedError(
          const NetworkException(message: 'timeout', code: 'TIMEOUT'),
        ),
        isFalse,
      );
      expect(service.isUnauthorizedError('https://host/401/x'), isFalse);
    });

    test('a 503 during sync keeps the session alive', () async {
      await signIn();
      historyFixedStatus = HttpStatus.serviceUnavailable;

      final result = await service.getHistory();
      expect(result, isEmpty);
      expect(
        service.isAuthenticated,
        isTrue,
        reason: '503 must not be treated as an auth failure',
      );
      expect(
        hits['POST /api/v1/auth/refresh'],
        isNull,
        reason: 'a 503 must never trigger a token refresh',
      );
      expect(hits['GET $historyPath'], greaterThanOrEqualTo(1));
    });
  });

  group('401 -> refresh -> replay', () {
    test('refreshes once and replays the request exactly once', () async {
      await signIn();
      expect(service.accessToken, 'access_1');

      // Exactly one 401, then the replay succeeds.
      historyFailures = 1;
      final history = await service.getHistory();

      expect(history, hasLength(1));
      expect(history.first['media_id'], 'm1');
      expect(
        hits['GET $historyPath'],
        2,
        reason: 'original + exactly 1 replay',
      );
      expect(hits['POST /api/v1/auth/refresh'], 1);
      expect(service.isAuthenticated, isTrue);
      expect(
        service.accessToken,
        'access_2',
        reason: 'the refreshed token must be installed',
      );

      // A subsequent call needs no further refresh.
      final again = await service.getHistory();
      expect(again, hasLength(1));
      expect(hits['POST /api/v1/auth/refresh'], 1);
    });

    test('a refresh that itself 401s signs the user out', () async {
      await signIn();
      failRefresh = true;
      historyFailures = 10;

      var expired = false;
      service.onAuthExpired = () => expired = true;

      await expectLater(service.getHistory(), throwsA(isA<ServerException>()));

      expect(service.isAuthenticated, isFalse);
      expect(service.accessToken, isNull);
      expect(prefs.getString('oxide_jwt_access_token'), isNull);
      expect(expired, isTrue);
    });

    test('concurrent 401s share a single refresh', () async {
      await signIn();
      historyFailures = 3;

      await Future.wait([
        service.getHistory().catchError((_) => <Map<String, dynamic>>[]),
        service.getHistory().catchError((_) => <Map<String, dynamic>>[]),
        service.getHistory().catchError((_) => <Map<String, dynamic>>[]),
      ]);

      expect(
        hits['POST /api/v1/auth/refresh'],
        1,
        reason: 'single-flight refresh',
      );
      expect(
        hits['GET $historyPath'],
        6,
        reason: '3 originals + 3 replays, no refresh storm',
      );
    });

    test('a replay that 401s again signs out and never loops', () async {
      await signIn();
      historyFailures = 99;
      var expired = false;
      service.onAuthExpired = () => expired = true;

      await expectLater(service.getHistory(), throwsA(isA<ServerException>()));

      expect(
        hits['GET $historyPath'],
        2,
        reason: 'one original + one replay; no refresh loop',
      );
      expect(hits['POST /api/v1/auth/refresh'], 1);
      expect(service.isAuthenticated, isFalse);
      expect(expired, isTrue);
    });
  });

  group('retry policy', () {
    test('does NOT retry a non-idempotent POST on 5xx', () async {
      await signIn();
      favoritesToggleStatus = HttpStatus.internalServerError;

      await service.toggleFavorite(
        mediaId: 'm1',
        providerId: 'uakino',
        title: 'M1',
        mediaType: 'movie',
      );

      expect(
        hits['POST $favoritesTogglePath'],
        1,
        reason: 'a retried toggle would flip the favorite back',
      );
    });

    test(
      'does NOT retry /auth/refresh (a replay presents a consumed token)',
      () async {
        await signIn();
        failRefresh = true;

        final ok = await service.refreshAuth();
        expect(ok, isFalse);
        expect(hits['POST /api/v1/auth/refresh'], 1);
      },
    );
  });

  group('signOut vs in-flight refresh', () {
    test(
      'a refresh completing after signOut does not restore the session',
      () async {
        await signIn();

        // Hold the refresh response open.
        refreshGateOpen = false;
        refreshGate = Completer<void>();

        final refresh = service.refreshAuth();
        // Wait until the request is actually parked on the gate.
        while (hits['POST /api/v1/auth/refresh'] == null ||
            (hits['POST /api/v1/auth/refresh'] ?? 0) < 1) {
          await Future<void>.delayed(const Duration(milliseconds: 5));
        }

        await service.signOut();
        expect(service.isAuthenticated, isFalse);

        // Let the refresh response land.
        refreshGate!.complete();
        refreshGateOpen = true;
        final result = await refresh;

        expect(result, isFalse);
        expect(
          service.isAuthenticated,
          isFalse,
          reason: 'the refresh must not resurrect the session',
        );
        expect(service.accessToken, isNull);
        expect(
          prefs.getString('oxide_jwt_access_token'),
          isNull,
          reason: 'tokens must stay cleared on disk too',
        );
      },
    );
  });

  group('SessionUser.fromJson', () {
    test('coerces id given as an int', () {
      final user = SessionUser.fromJson({'id': 42, 'email': 'a@b.c'});
      expect(user.id, '42');
      expect(user.email, 'a@b.c');
    });

    test('coerces id given as a String', () {
      final user = SessionUser.fromJson({'id': 'u-7', 'email': 'a@b.c'});
      expect(user.id, 'u-7');
    });

    test('never throws on hostile or empty payloads', () {
      expect(SessionUser.fromJson(null).id, isNull);
      expect(SessionUser.fromJson('not a map').email, isNull);
      expect(SessionUser.fromJson(<String, dynamic>{}).isVerified, isFalse);
      expect(
        SessionUser.fromJson({
          'id': <String>['weird'],
          'is_verified': 'true',
        }).isVerified,
        isTrue,
      );
      expect(SessionUser.fromJson({'id': 3.0}).id, '3.0');
      expect(
        SessionUser.fromJson({'avatar': 'https://a/b.png'}).avatarUrl,
        'https://a/b.png',
      );
    });

    test('keeps the raw map for callers reading untyped keys', () {
      final user = SessionUser.fromJson({'id': 1, 'telegram_id': 55});
      expect(user.raw['telegram_id'], 55);
    });
  });
}
