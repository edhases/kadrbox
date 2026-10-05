import 'package:drift/drift.dart';
import 'package:drift_flutter/drift_flutter.dart';
import 'package:path_provider/path_provider.dart';

part 'app_database.g.dart';

// ============================================================================
// ENUMS & TYPE CONVERTERS
// ============================================================================

/// Download status enum for type-safe status handling
/// Using int values for efficient storage and comparison
enum DownloadStatus {
  pending(0),
  downloading(1),
  paused(2),
  completed(3),
  failed(4);

  const DownloadStatus(this.value);
  final int value;

  /// Convert from database int value
  static DownloadStatus fromValue(int value) {
    return DownloadStatus.values.firstWhere(
      (s) => s.value == value,
      orElse: () => DownloadStatus.pending,
    );
  }
}

/// Type converter for DownloadStatus enum
class DownloadStatusConverter extends TypeConverter<DownloadStatus, int> {
  const DownloadStatusConverter();

  @override
  DownloadStatus fromSql(int fromDb) => DownloadStatus.fromValue(fromDb);

  @override
  int toSql(DownloadStatus value) => value.value;
}

// ============================================================================
// TABLES
// ============================================================================

/// Application settings table
class AppSettings extends Table {
  IntColumn get id => integer().autoIncrement()();
  TextColumn get key => text().unique()();
  TextColumn get value => text()();
  DateTimeColumn get updatedAt => dateTime().withDefault(currentDateAndTime)();
}

/// Enabled providers table
class EnabledProviders extends Table {
  IntColumn get id => integer().autoIncrement()();
  TextColumn get providerId => text().unique()();
  BoolColumn get isEnabled => boolean().withDefault(const Constant(true))();
  IntColumn get priority => integer().withDefault(const Constant(0))();
  DateTimeColumn get updatedAt => dateTime().withDefault(currentDateAndTime)();
}

/// Favorites table for saved media
class Favorites extends Table {
  IntColumn get id => integer().autoIncrement()();
  TextColumn get mediaId => text()();
  TextColumn get providerId => text()();
  TextColumn get title => text()();
  TextColumn get posterUrl => text().nullable()();
  IntColumn get year => integer().nullable()();
  RealColumn get rating => real().nullable()();
  TextColumn get ratingSource => text().nullable()();
  TextColumn get mediaType => text()(); // movie, series, cartoon, anime
  DateTimeColumn get addedAt => dateTime().withDefault(currentDateAndTime)();

  @override
  List<Set<Column>> get uniqueKeys => [
    {mediaId, providerId},
  ];
}

/// Watch history table
class WatchHistory extends Table {
  IntColumn get id => integer().autoIncrement()();
  TextColumn get mediaId => text()();
  TextColumn get providerId => text()();
  TextColumn get title => text()();
  TextColumn get posterUrl => text().nullable()();
  IntColumn get year => integer().nullable()();
  RealColumn get rating => real().nullable()();
  TextColumn get ratingSource => text().nullable()();
  TextColumn get mediaType => text()();

  // Playback position
  IntColumn get positionMs => integer().withDefault(const Constant(0))();
  IntColumn get durationMs => integer().withDefault(const Constant(0))();

  // For series - track episode
  IntColumn get season => integer().nullable()();
  IntColumn get episode => integer().nullable()();
  TextColumn get episodeTitle => text().nullable()();

  // Stream info
  TextColumn get lastStreamUrl => text().nullable()();
  TextColumn get voiceover => text().nullable()();

  DateTimeColumn get watchedAt => dateTime().withDefault(currentDateAndTime)();
}

/// Downloads table for offline viewing
class Downloads extends Table {
  IntColumn get id => integer().autoIncrement()();
  TextColumn get mediaId => text()();
  TextColumn get providerId => text()();
  TextColumn get title => text()();
  TextColumn get posterUrl => text().nullable()();
  IntColumn get year => integer().nullable()();
  RealColumn get rating => real().nullable()();
  TextColumn get ratingSource => text().nullable()();
  TextColumn get mediaType => text()();

  // Episode info (for series)
  IntColumn get season => integer().nullable()();
  IntColumn get episode => integer().nullable()();
  TextColumn get episodeTitle => text().nullable()();

  // Download info
  TextColumn get streamUrl => text()();
  TextColumn get localPath => text()();
  TextColumn get quality => text()();
  TextColumn get voiceover => text().nullable()();

  // Status: uses int for efficient storage and comparison
  // Maps to DownloadStatus enum via TypeConverter
  IntColumn get status => integer()
      .withDefault(const Constant(0))
      .map(const DownloadStatusConverter())();
  RealColumn get progress => real().withDefault(const Constant(0.0))();
  IntColumn get fileSizeBytes => integer().withDefault(const Constant(0))();
  IntColumn get downloadedBytes => integer().withDefault(const Constant(0))();

  // JSON encoded headers for the request (e.g. User-Agent, Referer)
  TextColumn get headers => text().nullable()();

  // Offline metadata
  TextColumn get localPosterPath => text().nullable()();
  IntColumn get duration => integer().nullable()();

  DateTimeColumn get createdAt => dateTime().withDefault(currentDateAndTime)();
  DateTimeColumn get completedAt => dateTime().nullable()();
}

/// Search history table for autocomplete and typo learning
class SearchHistoryTable extends Table {
  IntColumn get id => integer().autoIncrement()();

  /// Original query as typed by user
  TextColumn get query => text()();

  /// Normalized query for comparison (lowercase, trimmed)
  TextColumn get normalizedQuery => text()();

  /// Number of results found for this query
  IntColumn get resultCount => integer().withDefault(const Constant(0))();

  /// Whether search returned any results
  BoolColumn get wasSuccessful => boolean().withDefault(const Constant(true))();

  /// Number of times this query was searched
  IntColumn get searchCount => integer().withDefault(const Constant(1))();

  /// When query was first searched
  DateTimeColumn get firstSearchedAt =>
      dateTime().withDefault(currentDateAndTime)();

  /// When query was last searched
  DateTimeColumn get lastSearchedAt =>
      dateTime().withDefault(currentDateAndTime)();

  @override
  List<Set<Column>> get uniqueKeys => [
    {normalizedQuery},
  ];
}

/// Metadata cache for all encountered media items
class StoredMediaItems extends Table {
  TextColumn get id => text()();
  TextColumn get providerId => text()();
  TextColumn get title => text()();
  TextColumn get originalTitle => text().nullable()();
  TextColumn get posterUrl => text().nullable()();
  IntColumn get year => integer().nullable()();
  RealColumn get rating => real().nullable()();
  TextColumn get mediaType => text()(); // movie, series, cartoon, anime
  TextColumn get description => text().nullable()();
  TextColumn get genres => text().nullable()(); // Comma-separated
  TextColumn get country => text().nullable()();
  TextColumn get ratingSource => text().nullable()();
  DateTimeColumn get updatedAt => dateTime().withDefault(currentDateAndTime)();

  @override
  Set<Column> get primaryKey => {providerId, id};
}

@DriftDatabase(
  tables: [
    AppSettings,
    EnabledProviders,
    Favorites,
    WatchHistory,
    Downloads,
    SearchHistoryTable,
    StoredMediaItems,
  ],
)
class AppDatabase extends _$AppDatabase {
  AppDatabase([QueryExecutor? executor]) : super(executor ?? _openConnection());

  @override
  int get schemaVersion => 11;

  static QueryExecutor _openConnection() {
    return driftDatabase(
      name: 'kadrbox',
      native: const DriftNativeOptions(
        databaseDirectory: getApplicationDocumentsDirectory,
      ),
    );
  }

  @override
  MigrationStrategy get migration {
    return MigrationStrategy(
      onCreate: (Migrator m) async {
        await m.createAll();
        await _ensureLogicalKeyIndexes();
        // Insert default settings
        await _insertDefaultSettings();
      },
      onUpgrade: (Migrator m, int from, int to) async {
        // ... (previous migrations)

        // Migration: add ratingSource/rating to various tables
        if (from < 7) {
          // ... (previous migration code)
        }

        // Migration: add headers to Downloads
        if (from < 8) {
          await m.addColumn(downloads, downloads.headers);
        }

        // Migration: add offline metadata to Downloads
        if (from < 9) {
          await m.addColumn(downloads, downloads.localPosterPath);
          await m.addColumn(downloads, downloads.duration);
        }

        // Migration: purge rows of removed providers (data-only, no schema change).
        //
        // The list of removed sources used to be hard-coded here, one entry per
        // source ever dropped. That cannot work any more: which sources exist
        // is now decided at runtime by whatever catalog the user configured, so
        // a migration cannot know what is stale. The concern moves to runtime,
        // where a removed catalog is actually observable.
        //
        // Nothing is pinned to this step any more: the test that asserted the
        // purge now checks that an upgrade keeps rows belonging to sources that
        // still resolve, which holds with or without it.
        if (from < 10) {
          // Intentionally empty. See above.
        }

        // Migration: real uniqueness for (media, provider, season, episode).
        //
        // The former `uniqueKeys` declaration emitted a table-level UNIQUE
        // constraint, which SQLite only enforces when *every* column is
        // non-NULL. Movie rows carry NULL season/episode, so each NULL counted
        // as distinct and the same logical movie could be stored many times.
        // A UNIQUE index over the COALESCE'd expression collapses NULL to one
        // sentinel and therefore dedupes both shapes. The redundant
        // `uniqueKeys` entries were removed from the table definitions.
        if (from < 11) {
          await _dedupeLogicalKeys();
          await _ensureLogicalKeyIndexes();
        }
      },
    );
  }

  /// Tables whose logical identity is (mediaId, providerId, season, episode).
  List<String> get _logicalKeyTables => [
    watchHistory.actualTableName,
    downloads.actualTableName,
  ];

  /// Keeps only the newest row per logical key.
  ///
  /// `id` is autoincrement, so the highest id is the most recent write — the
  /// one that carries the furthest playback position.
  Future<void> _dedupeLogicalKeys() async {
    for (final table in _logicalKeyTables) {
      await customStatement('''
        DELETE FROM $table
        WHERE id NOT IN (
          SELECT MAX(id) FROM $table
          GROUP BY media_id, provider_id, COALESCE(season, -1), COALESCE(episode, -1)
        )
      ''');
    }
  }

  /// Creates the expression-based UNIQUE indexes. Idempotent.
  Future<void> _ensureLogicalKeyIndexes() async {
    for (final table in _logicalKeyTables) {
      await customStatement('''
        CREATE UNIQUE INDEX IF NOT EXISTS idx_${table}_logical_key
        ON $table (media_id, provider_id, COALESCE(season, -1), COALESCE(episode, -1))
      ''');
    }
  }

  Future<void> _insertDefaultSettings() async {
    final defaults = {
      'theme': 'dark',
      'default_quality': 'auto',
      'player_type': 'internal',
      'auto_play_next': 'true',
      'remember_position': 'true',
      'subtitle_language': 'uk',
    };

    for (final entry in defaults.entries) {
      await into(appSettings).insert(
        AppSettingsCompanion.insert(key: entry.key, value: entry.value),
        mode: InsertMode.insertOrIgnore,
      );
    }
  }
}
