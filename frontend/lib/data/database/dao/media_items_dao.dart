import 'package:drift/drift.dart';
import '../../../domain/entities/media_item.dart';
import '../app_database.dart';

/// Data Access Object for media items metadata cache
class MediaItemsDao {
  final AppDatabase _db;

  MediaItemsDao(this._db);

  /// Get cached media item metadata
  Future<MediaItem?> get(String id, String providerId) async {
    final query = _db.select(_db.storedMediaItems)
      ..where((t) => t.id.equals(id) & t.providerId.equals(providerId));
    final row = await query.getSingleOrNull();
    if (row == null) return null;
    return _mapToEntity(row);
  }

  /// Upsert media item metadata
  Future<void> upsert(MediaItem item) async {
    await _db
        .into(_db.storedMediaItems)
        .insert(
          StoredMediaItemsCompanion.insert(
            id: item.id,
            providerId: item.providerId,
            title: item.title,
            originalTitle: Value(item.originalTitle),
            posterUrl: Value(item.posterUrl),
            year: Value(item.year),
            rating: Value(item.rating),
            mediaType: item.type.name,
            description: Value(item.description),
            genres: Value(item.genres?.join(',')),
            country: Value(item.country),
            updatedAt: Value(DateTime.now()),
          ),
          mode: InsertMode.insertOrReplace,
        );
  }

  MediaItem _mapToEntity(StoredMediaItem row) {
    return MediaItem(
      id: row.id,
      providerId: row.providerId,
      title: row.title,
      originalTitle: row.originalTitle,
      posterUrl: row.posterUrl,
      year: row.year,
      rating: row.rating,
      type: _parseType(row.mediaType),
      description: row.description,
      genres: row.genres?.split(',').where((s) => s.isNotEmpty).toList(),
      country: row.country,
    );
  }

  ContentType _parseType(String type) {
    return ContentType.values.firstWhere(
      (e) => e.name == type,
      orElse: () => ContentType.unknown,
    );
  }
}
