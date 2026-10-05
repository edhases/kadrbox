// Runtime tests for the (media, provider, season, episode) logical-key fix.
//
// SQLite treats NULLs as distinct from each other, so the former
// `uniqueKeys => [{mediaId, providerId, season, episode}]` only ever deduped
// episode rows. Movie rows (season/episode NULL) could be inserted many times.
// The structural fix is a UNIQUE index over the COALESCE'd expression plus a
// de-duplicating upgrade step.

import 'package:drift/drift.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:oxide_film/data/database/app_database.dart';

import '../helpers/in_memory_db.dart';

Future<void> insertHistory(
  AppDatabase db, {
  required String mediaId,
  required String providerId,
  String? title,
  int? season,
  int? episode,
  int positionMs = 0,
}) {
  return db
      .into(db.watchHistory)
      .insert(
        WatchHistoryCompanion.insert(
          mediaId: mediaId,
          providerId: providerId,
          title: title ?? 'T',
          mediaType: 'movie',
          positionMs: Value(positionMs),
          season: Value(season),
          episode: Value(episode),
        ),
      );
}

Future<void> insertDownload(
  AppDatabase db, {
  required String mediaId,
  required String providerId,
  int? season,
  int? episode,
}) {
  return db
      .into(db.downloads)
      .insert(
        DownloadsCompanion.insert(
          mediaId: mediaId,
          providerId: providerId,
          title: 'T',
          mediaType: 'movie',
          streamUrl: 'https://cdn/s.mkv',
          localPath: '/tmp/s.mkv',
          quality: '720p',
          season: Value(season),
          episode: Value(episode),
        ),
      );
}

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();

  group('logical key schema', () {
    late AppDatabase db;

    setUp(() {
      db = createTestAppDatabase();
    });

    tearDown(() async {
      await db.close();
    });

    test('schemaVersion is 11', () {
      expect(db.schemaVersion, 11);
    });

    test('a duplicate movie row (NULL season/episode) is rejected', () async {
      await insertHistory(db, mediaId: 'm', providerId: 'p', title: 'first');

      await expectLater(
        insertHistory(db, mediaId: 'm', providerId: 'p', title: 'second'),
        throwsA(anything),
      );

      final rows = await db.select(db.watchHistory).get();
      expect(rows.length, 1);
      expect(rows.single.title, 'first');
    });

    test(
      'a duplicate download row (NULL season/episode) is rejected',
      () async {
        await insertDownload(db, mediaId: 'm', providerId: 'p');
        await expectLater(
          insertDownload(db, mediaId: 'm', providerId: 'p'),
          throwsA(anything),
        );
        expect((await db.select(db.downloads).get()).length, 1);
      },
    );

    test('episode rows keep their per-episode identity', () async {
      await insertHistory(
        db,
        mediaId: 's',
        providerId: 'p',
        season: 1,
        episode: 1,
      );
      await insertHistory(
        db,
        mediaId: 's',
        providerId: 'p',
        season: 1,
        episode: 2,
      );
      await insertHistory(db, mediaId: 's', providerId: 'p');

      expect((await db.select(db.watchHistory).get()).length, 3);

      await expectLater(
        insertHistory(db, mediaId: 's', providerId: 'p', season: 1, episode: 2),
        throwsA(anything),
      );
    });

    test('the same title from another provider is a distinct row', () async {
      await insertHistory(db, mediaId: 'm', providerId: 'p1');
      await insertHistory(db, mediaId: 'm', providerId: 'p2');
      expect((await db.select(db.watchHistory).get()).length, 2);
    });
  });

  group('migration to schema 11', () {
    late AppDatabase db;

    setUp(() async {
      db = createTestAppDatabase();
      // Simulate a pre-migration database: drop the expression indexes so the
      // duplicate rows below can be inserted, exactly as an upgrading install
      // would have them.
      await db.customStatement(
        'DROP INDEX IF EXISTS idx_watch_history_logical_key',
      );
      await db.customStatement(
        'DROP INDEX IF EXISTS idx_downloads_logical_key',
      );
    });

    tearDown(() async {
      await db.close();
    });

    test('de-duplicates movie rows and keeps the newest', () async {
      await insertHistory(
        db,
        mediaId: 'm',
        providerId: 'p',
        title: 'oldest',
        positionMs: 100,
      );
      await insertHistory(
        db,
        mediaId: 'm',
        providerId: 'p',
        title: 'middle',
        positionMs: 200,
      );
      await insertHistory(
        db,
        mediaId: 'm',
        providerId: 'p',
        title: 'newest',
        positionMs: 300,
      );
      expect((await db.select(db.watchHistory).get()).length, 3);

      await db.migration.onUpgrade(db.createMigrator(), 10, 11);

      final rows = await db.select(db.watchHistory).get();
      expect(rows.length, 1);
      expect(rows.single.title, 'newest');
      expect(rows.single.positionMs, 300);
    });

    test('de-duplicates downloads too', () async {
      await insertDownload(db, mediaId: 'm', providerId: 'p');
      await insertDownload(db, mediaId: 'm', providerId: 'p');
      await insertDownload(db, mediaId: 'm', providerId: 'p');
      expect((await db.select(db.downloads).get()).length, 3);

      await db.migration.onUpgrade(db.createMigrator(), 10, 11);

      expect((await db.select(db.downloads).get()).length, 1);
    });

    test(
      'keeps distinct episodes while collapsing duplicate episodes',
      () async {
        await insertHistory(
          db,
          mediaId: 's',
          providerId: 'p',
          season: 1,
          episode: 1,
          title: 's1e1-old',
        );
        await insertHistory(
          db,
          mediaId: 's',
          providerId: 'p',
          season: 1,
          episode: 1,
          title: 's1e1-new',
        );
        await insertHistory(
          db,
          mediaId: 's',
          providerId: 'p',
          season: 1,
          episode: 2,
          title: 's1e2',
        );
        await insertHistory(db, mediaId: 's', providerId: 'p', title: 'movie');

        await db.migration.onUpgrade(db.createMigrator(), 10, 11);

        final rows = await db.select(db.watchHistory).get();
        expect(rows.length, 3);
        final titles = rows.map((r) => r.title).toSet();
        expect(titles, {'s1e1-new', 's1e2', 'movie'});
      },
    );

    test('re-installs the unique indexes, so duplicates fail again', () async {
      await insertHistory(db, mediaId: 'm', providerId: 'p', title: 'old');
      await db.migration.onUpgrade(db.createMigrator(), 10, 11);

      await expectLater(
        insertHistory(db, mediaId: 'm', providerId: 'p', title: 'new'),
        throwsA(anything),
      );

      await insertDownload(db, mediaId: 'dm', providerId: 'dp');
      await expectLater(
        insertDownload(db, mediaId: 'dm', providerId: 'dp'),
        throwsA(anything),
      );
    });

    test('is idempotent across repeated runs', () async {
      await insertHistory(db, mediaId: 'm', providerId: 'p', title: 'old');
      await insertHistory(db, mediaId: 'm', providerId: 'p', title: 'new');

      await db.migration.onUpgrade(db.createMigrator(), 10, 11);
      await db.migration.onUpgrade(db.createMigrator(), 10, 11);

      final rows = await db.select(db.watchHistory).get();
      expect(rows.length, 1);
      expect(rows.single.title, 'new');
    });

    test('the < 10 hdrezka purge still runs for older databases', () async {
      await insertHistory(db, mediaId: 'rezka', providerId: 'hdrezka');
      await db.migration.onUpgrade(db.createMigrator(), 9, 11);

      final rows = await db.select(db.watchHistory).get();
      expect(rows.where((r) => r.providerId == 'hdrezka'), isEmpty);
      // ...and the dedup step ran on the way through.
      expect((await db.select(db.favorites).get()), isEmpty);
    });
  });
}
