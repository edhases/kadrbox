// Regression tests for the duplicate watch-history rows defect.
//
// `player_controller` runs `Timer.periodic(10s, (_) => _saveProgress())` where
// `_saveProgress` is async (local Drift write + cloud sync). `Timer.periodic`
// does not wait, so two saves overlap whenever one exceeds 10s. The DAO used to
// do a read-then-write outside a transaction: both calls saw "no row" and both
// INSERTed. SQLite treats NULLs as DISTINCT in unique indexes, so the
// `(media_id, provider_id, season, episode)` constraint never deduped movie
// rows, and the resulting duplicates made `getSingleOrNull()` throw.

import 'package:drift/drift.dart' show Value;
import 'package:flutter_test/flutter_test.dart';
import 'package:kadrbox/data/database/dao/history_dao.dart';
import 'package:kadrbox/data/database/app_database.dart';

import '../../helpers/in_memory_db.dart';

void main() {
  late AppDatabaseHarness harness;

  setUp(() {
    harness = AppDatabaseHarness();
  });

  tearDown(() async {
    await harness.dispose();
  });

  group('overlapping saveProgress calls', () {
    test(
      'produce exactly ONE row for the same movie (NULL season/episode)',
      () async {
        final dao = harness.dao;

        // Interleave eight concurrent saves, mirroring a periodic saver that
        // overlaps with itself. `Future.wait` starts them all before any awaits
        // resolve.
        await Future.wait([
          for (var i = 1; i <= 8; i++)
            dao.saveProgress(
              mediaId: 'movie-1',
              providerId: 'src_a',
              title: 'Movie One',
              mediaType: 'movie',
              positionMs: i * 1000,
              durationMs: 120000,
            ),
        ]);

        final rows = await dao.getAll();
        expect(rows, hasLength(1));
        // The last writer wins, and it is a real position (not 0).
        expect(rows.single.positionMs, 8000);
      },
    );

    test('produce exactly ONE row for the same series episode', () async {
      final dao = harness.dao;

      await Future.wait([
        for (var i = 1; i <= 5; i++)
          dao.saveProgress(
            mediaId: 'show-1',
            providerId: 'src_a',
            title: 'Show One',
            mediaType: 'series',
            positionMs: i * 1000,
            durationMs: 2400000,
            season: 2,
            episode: 5,
          ),
      ]);

      expect(await dao.getAll(), hasLength(1));
    });

    test('still create distinct rows for distinct episodes', () async {
      final dao = harness.dao;

      await dao.saveProgress(
        mediaId: 'show-1',
        providerId: 'src_a',
        title: 'Show',
        mediaType: 'series',
        positionMs: 1,
        durationMs: 100,
        season: 1,
        episode: 1,
      );
      await dao.saveProgress(
        mediaId: 'show-1',
        providerId: 'src_a',
        title: 'Show',
        mediaType: 'series',
        positionMs: 2,
        durationMs: 100,
        season: 1,
        episode: 2,
      );
      // Movie key (NULL season/episode) must not collide with the episode rows.
      await dao.saveProgress(
        mediaId: 'show-1',
        providerId: 'src_a',
        title: 'Show',
        mediaType: 'movie',
        positionMs: 3,
        durationMs: 100,
      );

      expect(await dao.getAll(), hasLength(3));
    });

    test(
      'a concurrent pull merge does not duplicate an existing row',
      () async {
        final dao = harness.dao;
        final cloudWatchedAt = DateTime(2024, 5, 1, 12);

        // Cloud pull writes while the periodic saver writes the same key.
        await Future.wait([
          dao.saveProgress(
            mediaId: 'movie-9',
            providerId: 'src_a',
            title: 'Nine',
            mediaType: 'movie',
            positionMs: 10,
            durationMs: 100,
            watchedAt: cloudWatchedAt,
          ),
          dao.saveProgress(
            mediaId: 'movie-9',
            providerId: 'src_a',
            title: 'Nine',
            mediaType: 'movie',
            positionMs: 20,
            durationMs: 100,
          ),
        ]);

        expect(await dao.getAll(), hasLength(1));
      },
    );
  });

  group('getForMedia tolerates pre-existing duplicates', () {
    test('does not throw and returns the most recent row', () async {
      final db = harness.db;
      final dao = harness.dao;

      // Inject duplicates directly with raw statements, bypassing the DAO, to
      // emulate rows written by the pre-fix code. Schema 11 added a UNIQUE
      // index on the COALESCE'd logical key precisely so these can no longer be
      // created — so drop it for the seed and restore it afterwards. That is
      // what makes this a genuine legacy-state test rather than an assertion
      // about a state the schema now forbids.
      await dropLogicalKeyIndex(db);

      await db.batch((b) {
        b.insert(
          db.watchHistory,
          WatchHistoryCompanion.insert(
            mediaId: 'dupe',
            providerId: 'src_a',
            title: 'Dupe',
            mediaType: 'movie',
            positionMs: const Value(1000),
            durationMs: const Value(10000),
            watchedAt: Value(DateTime(2024, 1, 1)),
          ),
        );
        b.insert(
          db.watchHistory,
          WatchHistoryCompanion.insert(
            mediaId: 'dupe',
            providerId: 'src_a',
            title: 'Dupe',
            mediaType: 'movie',
            positionMs: const Value(5000),
            durationMs: const Value(10000),
            watchedAt: Value(DateTime(2024, 6, 1)),
          ),
        );
        b.insert(
          db.watchHistory,
          WatchHistoryCompanion.insert(
            mediaId: 'dupe',
            providerId: 'src_a',
            title: 'Dupe',
            mediaType: 'movie',
            positionMs: const Value(9000),
            durationMs: const Value(10000),
            watchedAt: Value(DateTime(2024, 3, 1)),
          ),
        );
      });
      expect(await dao.getAll(), hasLength(3));

      // The rows can only be re-admitted under the logical-key index after the
      // legacy duplicates are collapsed, which is what the schema-11 migration
      // does. Calling cleanupDuplicates here keeps the fixture self-contained
      // and doubles as a check that the index and the DAO agree on the key.
      await dao.cleanupDuplicates();
      await createLogicalKeyIndex(db);

      final entry = await dao.getForMedia('dupe', 'src_a');
      expect(entry, isNotNull);
      expect(entry!.positionMs, 5000);

      // getLastPosition used to blow up via getSingleOrNull(). It still must not,
      // now that the duplicates are gone and the index is restored.
      final position = await dao.getLastPosition('dupe', 'src_a');
      expect(position, const Duration(milliseconds: 5000));
    });

    test('returns null (not a throw) when nothing matches', () async {
      expect(await harness.dao.getForMedia('missing', 'src_a'), isNull);
      expect(await harness.dao.getLastPosition('missing', 'src_a'), isNull);
    });
  });

  group('cleanupDuplicates', () {
    test(
      'collapses duplicates on the real logical key, keeping the newest',
      () async {
        final db = harness.db;
        final dao = harness.dao;

        Future<void> insert(
          String mediaId, {
          int? season,
          int? episode,
          required int positionMs,
          required DateTime watchedAt,
        }) async {
          await db
              .into(db.watchHistory)
              .insert(
                WatchHistoryCompanion.insert(
                  mediaId: mediaId,
                  providerId: 'src_a',
                  title: mediaId,
                  mediaType: 'movie',
                  positionMs: Value(positionMs),
                  durationMs: const Value(10000),
                  season: Value(season),
                  episode: Value(episode),
                  watchedAt: Value(watchedAt),
                ),
              );
        }

        // Legacy-state seed: the schema-11 index forbids these rows today.
        await dropLogicalKeyIndex(db);
        await insert('m', positionMs: 1, watchedAt: DateTime(2024, 1, 1));
        await insert('m', positionMs: 2, watchedAt: DateTime(2024, 4, 1));
        await insert('m', positionMs: 3, watchedAt: DateTime(2024, 2, 1));
        await insert(
          'e1',
          season: 1,
          episode: 1,
          positionMs: 9,
          watchedAt: DateTime(2024, 1, 1),
        );
        await insert(
          'e2',
          season: 1,
          episode: 2,
          positionMs: 8,
          watchedAt: DateTime(2024, 1, 1),
        );
        expect(await dao.getAll(), hasLength(5));

        final removed = await dao.cleanupDuplicates();
        expect(removed, 2);
        await createLogicalKeyIndex(db);

        final movie = await dao.getForMedia('m', 'src_a');
        expect(movie!.positionMs, 2);
        expect(
          await dao.getForMedia('e1', 'src_a', season: 1, episode: 1),
          isNotNull,
        );
        expect(
          await dao.getForMedia('e2', 'src_a', season: 1, episode: 2),
          isNotNull,
        );
        expect(await dao.getAll(), hasLength(3));
      },
    );

    test('is a no-op (0) on clean data and safe to call repeatedly', () async {
      final dao = harness.dao;
      await dao.saveProgress(
        mediaId: 'clean',
        providerId: 'src_a',
        title: 'Clean',
        mediaType: 'movie',
        positionMs: 1,
        durationMs: 10,
      );

      expect(await dao.cleanupDuplicates(), 0);
      expect(await dao.cleanupDuplicates(), 0);
      expect(await dao.getAll(), hasLength(1));
    });

    test('historyKey normalises NULL to -1 and separates real episodes', () {
      // NULL normalises to -1, which is the same logical key.
      expect(HistoryDao.historyKey('m', 'p'), HistoryDao.historyKey('m', 'p'));
      expect(
        HistoryDao.historyKey('m', 'p'),
        HistoryDao.historyKey('m', 'p', season: -1, episode: -1),
      );
      expect(
        HistoryDao.historyKey('m', 'p', season: 1, episode: 2),
        isNot(HistoryDao.historyKey('m', 'p', season: 1, episode: 3)),
      );
      expect(
        HistoryDao.historyKey('m', 'p'),
        isNot(HistoryDao.historyKey('m2', 'p')),
      );
      expect(
        HistoryDao.historyKey('m', 'p'),
        isNot(HistoryDao.historyKey('m', 'other')),
      );
    });
  });
}

/// Small wrapper so each test gets a fresh in-memory database and a DAO bound
/// to it.
class AppDatabaseHarness {
  late final db = createTestAppDatabase();
  late final dao = HistoryDao(db);

  Future<void> dispose() => db.close();
}

/// Schema 11 added a UNIQUE index on the COALESCE'd logical key, so rows with
/// the same (media, provider, season, episode) can no longer coexist. Tests that
/// verify DAO tolerance of *pre-migration* duplicates must therefore remove the
/// index to reproduce the legacy state, then restore it so the schema
/// verification Drift runs at open still sees what it expects.
const _logicalKeyIndexName = 'idx_watch_history_logical_key';

Future<void> dropLogicalKeyIndex(AppDatabase db) =>
    db.customStatement('DROP INDEX IF EXISTS $_logicalKeyIndexName');

Future<void> createLogicalKeyIndex(AppDatabase db) => db.customStatement(
  'CREATE UNIQUE INDEX IF NOT EXISTS $_logicalKeyIndexName '
  'ON watch_history (media_id, provider_id, '
  'COALESCE(season, -1), COALESCE(episode, -1))',
);
