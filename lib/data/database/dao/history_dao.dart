import 'package:drift/drift.dart';
import '../app_database.dart';

/// Data Access Object for watch history
class HistoryDao {
  final AppDatabase _db;

  HistoryDao(this._db);

  /// Get all history entries
  Future<List<WatchHistoryData>> getAll({int? limit}) async {
    var query = _db.select(_db.watchHistory)
      ..orderBy([(t) => OrderingTerm.desc(t.watchedAt)]);
    if (limit != null) {
      query = query..limit(limit);
    }
    return query.get();
  }

  /// WHERE clause matching the logical history key
  /// `(mediaId, providerId, season, episode)`.
  ///
  /// Note SQLite treats NULLs as DISTINCT in unique indexes, so nothing at the
  /// schema level prevents several rows for the same movie key. Every read must
  /// therefore tolerate duplicates.
  static Expression<bool> _keyMatches(
    WatchHistory t,
    String mediaId,
    String providerId,
    int? season,
    int? episode,
  ) {
    return t.mediaId.equals(mediaId) &
        t.providerId.equals(providerId) &
        (season != null ? t.season.equals(season) : t.season.isNull()) &
        (episode != null ? t.episode.equals(episode) : t.episode.isNull());
  }

  /// Canonical string form of the logical history key, used for dedupe checks.
  /// A `season`/`episode` of -1 stands in for SQL NULL, mirroring the
  /// `COALESCE(season, -1)` form the partial unique index should use.
  static String historyKey(
    String mediaId,
    String providerId, {
    int? season,
    int? episode,
  }) => '$mediaId|$providerId|${season ?? -1}|${episode ?? -1}';

  /// Get history for specific media.
  ///
  /// Deliberately avoids `getSingleOrNull()`: it throws when more than one row
  /// matches, and duplicates are reachable because NULL season/episode columns
  /// are not deduplicated by the unique index. Ordering deterministically and
  /// taking the first row makes a duplicate harmless instead of fatal.
  Future<WatchHistoryData?> getForMedia(
    String mediaId,
    String providerId, {
    int? season,
    int? episode,
  }) async {
    final query = _db.select(_db.watchHistory)
      ..where((t) => _keyMatches(t, mediaId, providerId, season, episode))
      ..orderBy([(t) => OrderingTerm.desc(t.watchedAt)])
      ..limit(1);

    final rows = await query.get();
    return rows.isEmpty ? null : rows.first;
  }

  /// Get continue watching list (items with progress > 5% and < 95%)
  Future<List<WatchHistoryData>> getContinueWatching({int limit = 20}) async {
    final query = _db.select(_db.watchHistory)
      ..where(
        (t) =>
            t.positionMs.isBiggerThanValue(0) &
            t.durationMs.isBiggerThanValue(0),
      )
      ..orderBy([(t) => OrderingTerm.desc(t.watchedAt)])
      ..limit(limit);

    final results = await query.get();

    // Filter in Dart for progress percentage
    return results.where((item) {
      if (item.durationMs == 0) return false;
      final progress = item.positionMs / item.durationMs;
      return progress > 0.05 && progress < 0.95;
    }).toList();
  }

  /// Save watch progress
  Future<void> saveProgress({
    required String mediaId,
    required String providerId,
    required String title,
    String? posterUrl,
    int? year,
    required String mediaType,
    required int positionMs,
    required int durationMs,
    int? season,
    int? episode,
    String? episodeTitle,
    String? lastStreamUrl,
    String? voiceover,
    double? rating,
    String? ratingSource,
    DateTime? watchedAt, // Optional: for cloud sync merge
  }) async {
    // Read-then-write must be atomic: `player_controller`'s 10s periodic saver
    // does not await, so two overlapping calls both used to miss the existing
    // row and both INSERT (SQLite cannot dedupe NULL season/episode, so the
    // unique index does not help either) — producing duplicate rows that later
    // made `getSingleOrNull()` throw.
    await _db.transaction(() async {
      final existingRows =
          await (_db.select(_db.watchHistory)
                ..where(
                  (t) => _keyMatches(t, mediaId, providerId, season, episode),
                )
                ..orderBy([(t) => OrderingTerm.desc(t.watchedAt)])
                ..limit(1))
              .get();
      final existing = existingRows.isEmpty ? null : existingRows.first;

      if (existing != null) {
        // Update existing
        await (_db.update(
          _db.watchHistory,
        )..where((t) => t.id.equals(existing.id))).write(
          WatchHistoryCompanion(
            title: Value(title),
            posterUrl: Value(posterUrl),
            year: Value(year),
            mediaType: Value(mediaType),
            positionMs: Value(positionMs),
            durationMs: Value(durationMs),
            episodeTitle: Value(episodeTitle),
            lastStreamUrl: Value(lastStreamUrl),
            voiceover: Value(voiceover),
            rating: Value(rating),
            ratingSource: Value(ratingSource),
            watchedAt: Value(watchedAt ?? DateTime.now()),
          ),
        );
      } else {
        // Insert new
        await _db
            .into(_db.watchHistory)
            .insert(
              WatchHistoryCompanion.insert(
                mediaId: mediaId,
                providerId: providerId,
                title: title,
                posterUrl: Value(posterUrl),
                year: Value(year),
                mediaType: mediaType,
                positionMs: Value(positionMs),
                durationMs: Value(durationMs),
                season: Value(season),
                episode: Value(episode),
                episodeTitle: Value(episodeTitle),
                lastStreamUrl: Value(lastStreamUrl),
                voiceover: Value(voiceover),
                rating: Value(rating),
                ratingSource: Value(ratingSource),
                watchedAt: Value(DateTime.now()),
              ),
            );
      }
    });
  }

  /// Remove duplicates from history.
  ///
  /// Safe to call at any time: it runs in a transaction, keys rows on the real
  /// logical key (`mediaId`, `providerId`, `COALESCE(season,-1)`,
  /// `COALESCE(episode,-1)`) and keeps the most recently watched row per key.
  Future<int> cleanupDuplicates() async {
    return _db.transaction(() async {
      final allHistory =
          await (_db.select(_db.watchHistory)..orderBy([
                (t) => OrderingTerm.desc(t.watchedAt),
                // Deterministic tie-break so two rows written in the same
                // millisecond always resolve to the same survivor.
                (t) => OrderingTerm.desc(t.id),
              ]))
              .get();

      final seenKeys = <String>{};
      final idsToDelete = <int>[];

      for (final item in allHistory) {
        final key = historyKey(
          item.mediaId,
          item.providerId,
          season: item.season,
          episode: item.episode,
        );
        if (!seenKeys.add(key)) {
          // Older than the survivor already recorded for this key.
          idsToDelete.add(item.id);
        }
      }

      if (idsToDelete.isEmpty) return 0;

      return (_db.delete(
        _db.watchHistory,
      )..where((t) => t.id.isIn(idsToDelete))).go();
    });
  }

  /// Get last watched position for media
  Future<Duration?> getLastPosition(
    String mediaId,
    String providerId, {
    int? season,
    int? episode,
  }) async {
    final entry = await getForMedia(
      mediaId,
      providerId,
      season: season,
      episode: episode,
    );
    if (entry == null || entry.positionMs == 0) return null;
    return Duration(milliseconds: entry.positionMs);
  }

  /// Clear history entry
  Future<int> remove(String mediaId, String providerId) async {
    return (_db.delete(_db.watchHistory)..where(
          (t) => t.mediaId.equals(mediaId) & t.providerId.equals(providerId),
        ))
        .go();
  }

  /// Clear all history
  Future<int> clearAll() async {
    return _db.delete(_db.watchHistory).go();
  }

  /// Watch history stream
  Stream<List<WatchHistoryData>> watchAll({int? limit}) {
    var query = _db.select(_db.watchHistory)
      ..orderBy([(t) => OrderingTerm.desc(t.watchedAt)]);
    if (limit != null) {
      query = query..limit(limit);
    }
    return query.watch();
  }

  /// Watch continue watching list
  Stream<List<WatchHistoryData>> watchContinueWatching({int limit = 10}) {
    final query = _db.select(_db.watchHistory)
      ..where(
        (t) =>
            t.positionMs.isBiggerThanValue(0) &
            t.durationMs.isBiggerThanValue(0),
      )
      ..orderBy([(t) => OrderingTerm.desc(t.watchedAt)])
      ..limit(limit * 2); // Get more to filter

    return query.watch().map((results) {
      return results
          .where((item) {
            if (item.durationMs == 0) return false;
            final progress = item.positionMs / item.durationMs;
            return progress > 0.05 && progress < 0.95;
          })
          .take(limit)
          .toList();
    });
  }

  /// Get history count
  Future<int> count() async {
    final countExp = _db.watchHistory.id.count();
    final query = _db.selectOnly(_db.watchHistory)..addColumns([countExp]);
    final result = await query.getSingle();
    return result.read(countExp) ?? 0;
  }
}
