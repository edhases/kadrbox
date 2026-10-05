// Tests for SyncService export/import functionality

import 'dart:convert';
import 'dart:io';

import 'package:flutter_test/flutter_test.dart';

import 'package:oxide_film/data/services/sync_service.dart';
import 'package:oxide_film/data/database/dao/favorites_dao.dart';
import 'package:oxide_film/data/database/dao/history_dao.dart';
import 'package:oxide_film/data/database/dao/settings_dao.dart';

import '../helpers/in_memory_db.dart';
import '../helpers/file_system_mocks.dart';

void main() {
  group('SyncService export/import', () {
    test(
      'exportData writes JSON with favorites/history/settings keys',
      () async {
        final db = createTestAppDatabase();
        final favoritesDao = FavoritesDao(db);
        final historyDao = HistoryDao(db);
        final settingsDao = SettingsDao(db);

        // Seed data
        await favoritesDao.add(
          mediaId: 'f1',
          providerId: 'prov',
          title: 'Fav1',
          posterUrl: null,
          year: 2020,
          rating: 7.5,
          ratingSource: 'Site',
          mediaType: 'movie',
        );

        await favoritesDao.add(
          mediaId: 'f2',
          providerId: 'prov',
          title: 'Fav2',
          posterUrl: null,
          year: 2021,
          rating: 8.0,
          ratingSource: 'Site',
          mediaType: 'movie',
        );

        await historyDao.saveProgress(
          mediaId: 'h1',
          providerId: 'prov',
          title: 'Hist1',
          posterUrl: null,
          year: 2020,
          mediaType: 'movie',
          positionMs: 1000,
          durationMs: 4000,
        );

        await settingsDao.setSetting('theme', 'dark');
        await settingsDao.setSetting('default_quality', 'q720p');

        // Prepare test documents directory
        final tmp = await Directory.systemTemp.createTemp('oxide_sync_test');
        await setMockDocumentsDirectory(tmp);

        final service = SyncService(db);

        final filePath = await service.exportData(
          includeFavorites: true,
          includeHistory: true,
          includeSettings: true,
        );

        expect(filePath, isNotNull);
        final file = File(filePath!);
        expect(await file.exists(), isTrue);

        final content = await file.readAsString();
        final data = jsonDecode(content) as Map<String, dynamic>;

        expect(data.containsKey('favorites'), isTrue);
        expect((data['favorites'] as List).length, 2);
        expect(data.containsKey('history'), isTrue);
        expect((data['history'] as List).length, 1);
        expect(data.containsKey('settings'), isTrue);
        expect((data['settings'] as Map).containsKey('theme'), isTrue);

        // Cleanup
        await file.delete();
        await tmp.delete(recursive: true);
        await db.close();
      },
    );

    test(
      'importFromJson imports favorites/history and returns true for valid payload',
      () async {
        final db = createTestAppDatabase();
        final favoritesDao = FavoritesDao(db);
        final historyDao = HistoryDao(db);

        final service = SyncService(db);

        final payload = {
          'app': 'OxideFilm',
          'version': 1,
          'favorites': [
            {
              'mediaId': 'fi1',
              'providerId': 'prov',
              'title': 'Imported Fav',
              'posterUrl': null,
              'year': 2019,
              'rating': 7.0,
              'ratingSource': 'Site',
              'mediaType': 'movie',
            },
          ],
          'history': [
            {
              'mediaId': 'hi1',
              'providerId': 'prov',
              'title': 'Imported Hist',
              'posterUrl': null,
              'year': 2019,
              'mediaType': 'movie',
              'positionMs': 2000,
              'durationMs': 4000,
            },
          ],
        };

        final jsonString = jsonEncode(payload);

        final result = await service.importFromJson(jsonString);
        expect(result, isTrue);

        final favs = await favoritesDao.getAll();
        expect(favs.length, 1);
        expect(favs.first.mediaId, 'fi1');

        final hist = await historyDao.getAll();
        expect(hist.length, 1);
        expect(hist.first.mediaId, 'hi1');

        await db.close();
      },
    );
  });
}
