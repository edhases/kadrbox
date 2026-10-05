import 'package:drift/drift.dart';

import '../app_database.dart';

part 'search_history_dao.g.dart';

/// DAO for search history operations
@DriftAccessor(tables: [SearchHistoryTable])
class SearchHistoryDao extends DatabaseAccessor<AppDatabase>
    with _$SearchHistoryDaoMixin {
  SearchHistoryDao(super.db);

  /// Get all search history ordered by last searched
  Future<List<SearchHistoryTableData>> getAll() {
    return (select(searchHistoryTable)..orderBy([
          (t) => OrderingTerm(
            expression: t.lastSearchedAt,
            mode: OrderingMode.desc,
          ),
        ]))
        .get();
  }

  /// Get recent searches (limit to 100)
  Future<List<SearchHistoryTableData>> getRecent({int limit = 100}) {
    return (select(searchHistoryTable)
          ..orderBy([
            (t) => OrderingTerm(
              expression: t.lastSearchedAt,
              mode: OrderingMode.desc,
            ),
          ])
          ..limit(limit))
        .get();
  }

  /// Get successful searches only (for autocomplete)
  Future<List<SearchHistoryTableData>> getSuccessful({int limit = 20}) {
    return (select(searchHistoryTable)
          ..where((t) => t.wasSuccessful.equals(true))
          ..orderBy([
            (t) => OrderingTerm(
              expression: t.searchCount,
              mode: OrderingMode.desc,
            ),
            (t) => OrderingTerm(
              expression: t.lastSearchedAt,
              mode: OrderingMode.desc,
            ),
          ])
          ..limit(limit))
        .get();
  }

  /// Search history by query prefix (for autocomplete)
  Future<List<SearchHistoryTableData>> searchByPrefix(
    String prefix, {
    int limit = 10,
  }) {
    final normalizedPrefix = prefix.toLowerCase().trim();
    return (select(searchHistoryTable)
          ..where((t) => t.normalizedQuery.like('$normalizedPrefix%'))
          ..where((t) => t.wasSuccessful.equals(true))
          ..orderBy([
            (t) => OrderingTerm(
              expression: t.searchCount,
              mode: OrderingMode.desc,
            ),
          ])
          ..limit(limit))
        .get();
  }

  /// Add or update search query
  Future<void> addSearch({
    required String query,
    required String normalizedQuery,
    required int resultCount,
  }) async {
    final existing =
        await (select(searchHistoryTable)
              ..where((t) => t.normalizedQuery.equals(normalizedQuery)))
            .getSingleOrNull();

    if (existing != null) {
      // Update existing entry
      await (update(
        searchHistoryTable,
      )..where((t) => t.id.equals(existing.id))).write(
        SearchHistoryTableCompanion(
          query: Value(query), // Update to latest query form
          resultCount: Value(resultCount),
          wasSuccessful: Value(resultCount > 0),
          searchCount: Value(existing.searchCount + 1),
          lastSearchedAt: Value(DateTime.now()),
        ),
      );
    } else {
      // Insert new entry
      await into(searchHistoryTable).insert(
        SearchHistoryTableCompanion.insert(
          query: query,
          normalizedQuery: normalizedQuery,
          resultCount: Value(resultCount),
          wasSuccessful: Value(resultCount > 0),
        ),
      );

      // Cleanup old entries (keep only 100)
      await _cleanupOldEntries();
    }
  }

  /// Delete a search history entry
  Future<void> deleteById(int id) {
    return (delete(searchHistoryTable)..where((t) => t.id.equals(id))).go();
  }

  /// Clear all search history
  Future<void> clearAll() {
    return delete(searchHistoryTable).go();
  }

  /// Get most frequent failed searches (for typo detection)
  Future<List<SearchHistoryTableData>> getFailedSearches({int limit = 50}) {
    return (select(searchHistoryTable)
          ..where((t) => t.wasSuccessful.equals(false))
          ..orderBy([
            (t) => OrderingTerm(
              expression: t.searchCount,
              mode: OrderingMode.desc,
            ),
          ])
          ..limit(limit))
        .get();
  }

  /// Cleanup old entries, keeping only the most recent 100
  Future<void> _cleanupOldEntries() async {
    final count = await searchHistoryTable.count().getSingle();

    if (count > 100) {
      // Get IDs to keep (100 most recent)
      final toKeep =
          await (select(searchHistoryTable)
                ..orderBy([
                  (t) => OrderingTerm(
                    expression: t.lastSearchedAt,
                    mode: OrderingMode.desc,
                  ),
                ])
                ..limit(100))
              .get();

      final keepIds = toKeep.map((e) => e.id).toSet();

      // Delete entries not in keepIds
      await (delete(
        searchHistoryTable,
      )..where((t) => t.id.isNotIn(keepIds))).go();
    }
  }

  /// Watch search history changes
  Stream<List<SearchHistoryTableData>> watchRecent({int limit = 20}) {
    return (select(searchHistoryTable)
          ..orderBy([
            (t) => OrderingTerm(
              expression: t.lastSearchedAt,
              mode: OrderingMode.desc,
            ),
          ])
          ..limit(limit))
        .watch();
  }
}
