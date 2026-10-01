import 'dart:async';
import 'package:flutter_test/flutter_test.dart';
import 'package:mocktail/mocktail.dart';
import 'package:oxide_film/core/error/exceptions.dart';
import 'package:oxide_film/core/network/api_client.dart';
import 'package:oxide_film/data/database/app_database.dart';
import 'package:oxide_film/data/database/dao/favorites_dao.dart';
import 'package:oxide_film/data/services/auth_service.dart';
import 'package:oxide_film/data/services/favorites_service.dart';
import 'package:oxide_film/data/services/history_service.dart';
import 'package:oxide_film/data/services/oxide_server_service.dart';
import 'package:shared_preferences/shared_preferences.dart';

import '../../helpers/in_memory_db.dart';

class MockOxideServerService extends Mock implements OxideServerService {}
class MockAuthService extends Mock implements AuthService {}
class MockApiClient extends Mock implements ApiClient {}

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();

  group('401 Retry Storm Prevention', () {
    late AppDatabase db;
    late MockOxideServerService mockServer;
    late MockAuthService mockAuth;

    setUp(() {
      db = createTestAppDatabase();
      mockServer = MockOxideServerService();
      mockAuth = MockAuthService();

      when(() => mockAuth.isAuthenticated).thenReturn(true);
      when(() => mockAuth.currentUser).thenReturn(
        (id: 'user_1', email: 'test@example.com', name: 'Tester'),
      );
      when(() => mockAuth.addListener(any())).thenReturn(null);
      when(() => mockAuth.removeListener(any())).thenReturn(null);
      when(() => mockServer.isAuthenticated).thenReturn(true);
      when(() => mockServer.getHistory(limit: any(named: 'limit'))).thenAnswer(
        (_) async => [],
      );
      when(() => mockServer.getFavorites(limit: any(named: 'limit'))).thenAnswer(
        (_) async => [],
      );
      when(
        () => mockServer.saveHistoryProgress(
          mediaId: any(named: 'mediaId'),
          providerId: any(named: 'providerId'),
          title: any(named: 'title'),
          posterUrl: any(named: 'posterUrl'),
          year: any(named: 'year'),
          mediaType: any(named: 'mediaType'),
          positionMs: any(named: 'positionMs'),
          durationMs: any(named: 'durationMs'),
          season: any(named: 'season'),
          episode: any(named: 'episode'),
          episodeTitle: any(named: 'episodeTitle'),
          lastStreamUrl: any(named: 'lastStreamUrl'),
          voiceover: any(named: 'voiceover'),
          watchedAt: any(named: 'watchedAt'),
        ),
      ).thenAnswer((_) async {});
      when(
        () => mockServer.toggleFavorite(
          mediaId: any(named: 'mediaId'),
          providerId: any(named: 'providerId'),
          title: any(named: 'title'),
          posterUrl: any(named: 'posterUrl'),
          year: any(named: 'year'),
          mediaType: any(named: 'mediaType'),
          rating: any(named: 'rating'),
          ratingSource: any(named: 'ratingSource'),
        ),
      ).thenAnswer((_) async => true);
    });

    tearDown(() async {
      await db.close();
    });

    test('HistoryService stops batch immediately upon first 401 ServerException', () async {
      final historyService = HistoryService(
        database: db,
        server: mockServer,
        authService: mockAuth,
      );

      // Add 5 items to local DB
      for (int i = 1; i <= 5; i++) {
        await historyService.saveProgress(
          mediaId: 'item_$i',
          providerId: 'uakino',
          title: 'Movie $i',
          mediaType: 'movie',
          position: Duration(minutes: i),
          duration: const Duration(hours: 2),
        );
      }

      int calls = 0;
      when(
        () => mockServer.saveHistoryProgress(
          mediaId: any(named: 'mediaId'),
          providerId: any(named: 'providerId'),
          title: any(named: 'title'),
          posterUrl: any(named: 'posterUrl'),
          year: any(named: 'year'),
          mediaType: any(named: 'mediaType'),
          positionMs: any(named: 'positionMs'),
          durationMs: any(named: 'durationMs'),
          season: any(named: 'season'),
          episode: any(named: 'episode'),
          episodeTitle: any(named: 'episodeTitle'),
          lastStreamUrl: any(named: 'lastStreamUrl'),
          voiceover: any(named: 'voiceover'),
          watchedAt: any(named: 'watchedAt'),
        ),
      ).thenAnswer((_) async {
        calls++;
        throw const ServerException(
          message: 'invalid or expired token',
          statusCode: 401,
          code: 'HTTP_401',
        );
      });

      // Trigger sync
      await historyService.syncNow();

      // Only 1 attempt should have been made before batch stopped!
      expect(calls, equals(1));

      // Subsequent sync call should be blocked by sticky _cloudSyncDisabled
      await historyService.syncNow();
      expect(calls, equals(1));

      historyService.dispose();
    });

    test('FavoritesService stops batch immediately upon first 401 ServerException', () async {
      final favoritesService = FavoritesService(
        database: db,
        server: mockServer,
        authService: mockAuth,
      );

      // Add 5 favorites directly to DAO
      final favoritesDao = FavoritesDao(db);
      for (int i = 1; i <= 5; i++) {
        await favoritesDao.add(
          mediaId: 'fav_$i',
          providerId: 'uakino',
          title: 'Favorite $i',
          mediaType: 'movie',
        );
      }

      int calls = 0;
      when(
        () => mockServer.toggleFavorite(
          mediaId: any(named: 'mediaId'),
          providerId: any(named: 'providerId'),
          title: any(named: 'title'),
          posterUrl: any(named: 'posterUrl'),
          year: any(named: 'year'),
          mediaType: any(named: 'mediaType'),
          rating: any(named: 'rating'),
          ratingSource: any(named: 'ratingSource'),
        ),
      ).thenAnswer((_) async {
        calls++;
        throw const ServerException(
          message: 'invalid or expired token',
          statusCode: 401,
          code: 'HTTP_401',
        );
      });

      // Trigger sync
      await favoritesService.syncNow();

      // Only 1 attempt should have been made before batch stopped!
      expect(calls, equals(1));

      // Subsequent sync call should be blocked by sticky _cloudSyncDisabled
      await favoritesService.syncNow();
      expect(calls, equals(1));

      favoritesService.dispose();
    });

    test('OxideServerService refreshAuth deduplicates concurrent calls (single-flight)', () async {
      SharedPreferences.setMockInitialValues({
        'oxide_jwt_access_token': 'old_access',
        'oxide_jwt_refresh_token': 'valid_refresh',
      });
      final prefs = await SharedPreferences.getInstance();
      final mockApi = MockApiClient();

      int postCalls = 0;
      final completer = Completer<Map<String, dynamic>>();

      when(
        () => mockApi.post(
          any(),
          data: any(named: 'data'),
          headers: any(named: 'headers'),
          queryParameters: any(named: 'queryParameters'),
        ),
      ).thenAnswer((_) {
        postCalls++;
        return completer.future;
      });

      final serverService = OxideServerService(prefs, mockApi);

      // Launch 5 concurrent refreshAuth calls
      final futures = Future.wait([
        serverService.refreshAuth(),
        serverService.refreshAuth(),
        serverService.refreshAuth(),
        serverService.refreshAuth(),
        serverService.refreshAuth(),
      ]);

      // Exactly 1 network request should be in-flight
      expect(postCalls, equals(1));

      completer.complete({
        'access_token': 'new_access',
        'refresh_token': 'new_refresh',
      });

      final results = await futures;
      expect(results, everyElement(isTrue));
      expect(postCalls, equals(1));
      expect(serverService.accessToken, equals('new_access'));
    });
  });
}
