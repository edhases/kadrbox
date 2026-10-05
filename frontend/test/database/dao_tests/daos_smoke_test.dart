// DAO smoke tests using in-memory database helper

import 'package:flutter_test/flutter_test.dart';
import 'package:oxide_film/data/database/dao/history_dao.dart';
import 'package:oxide_film/data/database/dao/favorites_dao.dart';

import '../../helpers/in_memory_db.dart';

void main() {
  group('DAO smoke tests (in-memory DB)', () {
    test(
      'HistoryDao.getContinueWatching filters progress between 5% and 95%',
      () async {
        final db = createTestAppDatabase();
        final historyDao = HistoryDao(db);

        // Seed entries with different progress:
        // 1) 1% -> position 10, duration 1000
        await historyDao.saveProgress(
          mediaId: 'm1',
          providerId: 'p1',
          title: 'LowProgress',
          posterUrl: null,
          year: 2020,
          mediaType: 'movie',
          positionMs: 10,
          durationMs: 1000,
        );

        // 2) 10% -> position 100, duration 1000
        await historyDao.saveProgress(
          mediaId: 'm2',
          providerId: 'p1',
          title: 'TenPercent',
          posterUrl: null,
          year: 2020,
          mediaType: 'movie',
          positionMs: 100,
          durationMs: 1000,
        );

        // 3) 50% -> position 500, duration 1000
        await historyDao.saveProgress(
          mediaId: 'm3',
          providerId: 'p1',
          title: 'HalfWay',
          posterUrl: null,
          year: 2020,
          mediaType: 'movie',
          positionMs: 500,
          durationMs: 1000,
        );

        // 4) 96% -> position 960, duration 1000
        await historyDao.saveProgress(
          mediaId: 'm4',
          providerId: 'p1',
          title: 'AlmostDone',
          posterUrl: null,
          year: 2020,
          mediaType: 'movie',
          positionMs: 960,
          durationMs: 1000,
        );

        final results = await historyDao.getContinueWatching(limit: 10);

        // Expect only entries with 10% and 50% progress
        expect(results.length, 2);
        final ids = results.map((r) => r.mediaId).toSet();
        expect(ids, containsAll({'m2', 'm3'}));

        await db.close();
      },
    );

    test(
      'HistoryDao.saveProgress updates existing entry instead of inserting duplicate',
      () async {
        final db = createTestAppDatabase();
        final historyDao = HistoryDao(db);

        await historyDao.saveProgress(
          mediaId: 'upd1',
          providerId: 'prov',
          title: 'Initial',
          posterUrl: null,
          year: 2021,
          mediaType: 'movie',
          positionMs: 1000,
          durationMs: 2000,
        );

        // Update existing
        await historyDao.saveProgress(
          mediaId: 'upd1',
          providerId: 'prov',
          title: 'Initial',
          posterUrl: null,
          year: 2021,
          mediaType: 'movie',
          positionMs: 1500,
          durationMs: 2000,
        );

        final entry = await historyDao.getForMedia('upd1', 'prov');
        expect(entry, isNotNull);
        expect(entry!.positionMs, 1500);

        // Ensure only one record exists for that media/provider
        final all = await historyDao.getAll();
        final same = all
            .where((e) => e.mediaId == 'upd1' && e.providerId == 'prov')
            .toList();
        expect(same.length, 1);

        await db.close();
      },
    );

    test(
      'FavoritesDao.add/remove/toggle and watchIsFavorite stream behavior',
      () async {
        final db = createTestAppDatabase();
        final favDao = FavoritesDao(db);

        // Initially not favorite
        final isFavBefore = await favDao.isFavorite('fav1', 'prov');
        expect(isFavBefore, isFalse);

        // Start watching stream
        final stream = favDao.watchIsFavorite('fav1', 'prov');
        final expectStream = expectLater(
          stream,
          emitsInOrder([false, true, false]),
        );

        // Add favorite
        final added = await favDao.add(
          mediaId: 'fav1',
          providerId: 'prov',
          title: 'My Favorite',
          posterUrl: null,
          year: 2022,
          rating: 8.0,
          ratingSource: 'Site',
          mediaType: 'movie',
        );
        expect(added, isNonZero);

        // Now toggle (should remove)
        final toggled = await favDao.toggle(
          mediaId: 'fav1',
          providerId: 'prov',
          title: 'My Favorite',
          posterUrl: null,
          year: 2022,
          rating: 8.0,
          ratingSource: 'Site',
          mediaType: 'movie',
        );
        expect(toggled, isFalse);

        await expectStream;

        await db.close();
      },
    );
  });
}
