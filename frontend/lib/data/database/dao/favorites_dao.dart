import 'package:drift/drift.dart';
import '../app_database.dart';

/// Data Access Object for favorites
class FavoritesDao {
  final AppDatabase _db;

  FavoritesDao(this._db);

  /// Get all favorites
  Future<List<Favorite>> getAll() async {
    final query = _db.select(_db.favorites)
      ..orderBy([(t) => OrderingTerm.desc(t.addedAt)]);
    return query.get();
  }

  /// Get favorites by media type
  Future<List<Favorite>> getByType(String mediaType) async {
    final query = _db.select(_db.favorites)
      ..where((t) => t.mediaType.equals(mediaType))
      ..orderBy([(t) => OrderingTerm.desc(t.addedAt)]);
    return query.get();
  }

  /// Check if item is in favorites
  Future<bool> isFavorite(String mediaId, String providerId) async {
    final query = _db.select(_db.favorites)
      ..where(
        (t) => t.mediaId.equals(mediaId) & t.providerId.equals(providerId),
      );
    final result = await query.getSingleOrNull();
    return result != null;
  }

  /// Get a single favorite
  Future<Favorite?> get(String mediaId, String providerId) async {
    final query = _db.select(_db.favorites)
      ..where(
        (t) => t.mediaId.equals(mediaId) & t.providerId.equals(providerId),
      );
    return query.getSingleOrNull();
  }

  /// Add to favorites
  Future<int> add({
    required String mediaId,
    required String providerId,
    required String title,
    String? posterUrl,
    int? year,
    double? rating,
    String? ratingSource,
    required String mediaType,
  }) async {
    return _db
        .into(_db.favorites)
        .insert(
          FavoritesCompanion.insert(
            mediaId: mediaId,
            providerId: providerId,
            title: title,
            posterUrl: Value(posterUrl),
            year: Value(year),
            rating: Value(rating),
            ratingSource: Value(ratingSource),
            mediaType: mediaType,
          ),
          mode: InsertMode.insertOrIgnore,
        );
  }

  /// Remove from favorites
  Future<int> remove(String mediaId, String providerId) async {
    return (_db.delete(_db.favorites)..where(
          (t) => t.mediaId.equals(mediaId) & t.providerId.equals(providerId),
        ))
        .go();
  }

  /// Toggle favorite status
  Future<bool> toggle({
    required String mediaId,
    required String providerId,
    required String title,
    String? posterUrl,
    int? year,
    double? rating,
    String? ratingSource,
    required String mediaType,
  }) async {
    final exists = await isFavorite(mediaId, providerId);
    if (exists) {
      await remove(mediaId, providerId);
      return false;
    } else {
      await add(
        mediaId: mediaId,
        providerId: providerId,
        title: title,
        posterUrl: posterUrl,
        year: year,
        rating: rating,
        ratingSource: ratingSource,
        mediaType: mediaType,
      );
      return true;
    }
  }

  /// Watch all favorites
  Stream<List<Favorite>> watchAll() {
    final query = _db.select(_db.favorites)
      ..orderBy([(t) => OrderingTerm.desc(t.addedAt)]);
    return query.watch();
  }

  /// Watch favorite status for specific item
  Stream<bool> watchIsFavorite(String mediaId, String providerId) {
    final query = _db.select(_db.favorites)
      ..where(
        (t) => t.mediaId.equals(mediaId) & t.providerId.equals(providerId),
      );
    return query.watchSingleOrNull().map((row) => row != null);
  }

  /// Get favorites count
  Future<int> count() async {
    final countExp = _db.favorites.id.count();
    final query = _db.selectOnly(_db.favorites)..addColumns([countExp]);
    final result = await query.getSingle();
    return result.read(countExp) ?? 0;
  }

  /// Clear all favorites
  Future<int> clearAll() async {
    return _db.delete(_db.favorites).go();
  }
}
