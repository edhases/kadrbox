import 'dart:async';
import 'package:flutter/foundation.dart';
import '../../core/utils/logger.dart';
import '../../domain/entities/entities.dart';
import '../database/app_database.dart';
import '../database/dao/favorites_dao.dart';
import 'oxide_server_service.dart';
import 'auth_service.dart';

/// Service for managing favorites with cloud sync
class FavoritesService extends ChangeNotifier {
  final FavoritesDao _dao;
  final OxideServerService _server;
  final AuthService _authService;

  List<Favorite> _favorites = [];
  List<Favorite> get favorites => _favorites;

  final bool _isLoading = false;
  bool get isLoading => _isLoading;

  bool _isSyncing = false;
  bool get isSyncing => _isSyncing;

  StreamSubscription<List<Favorite>>? _subscription;
  Timer? _syncTimer;

  FavoritesService({
    required AppDatabase database,
    required OxideServerService server,
    required AuthService authService,
  }) : _dao = FavoritesDao(database),
       _server = server,
       _authService = authService {
    _init();
  }

  void _init() {
    Logger.d('Initializing favorites service...', tag: 'Favorites');
    _subscription = _dao.watchAll().listen((items) {
      Logger.d('Favorites updated: ${items.length} items', tag: 'Favorites');
      _favorites = items;
      notifyListeners();
    });

    // Pull from cloud on startup (1 раз)
    _pullFromCloud();

    // Setup periodic sync every 5 minutes (не щосекунди!)
    _syncTimer = Timer.periodic(const Duration(minutes: 5), (_) {
      _syncToCloud();
    });
  }

  /// Check if item is in favorites
  bool isFavorite(String mediaId, String providerId) {
    return _favorites.any(
      (f) => f.mediaId == mediaId && f.providerId == providerId,
    );
  }

  /// Toggle favorite status (offline-first)
  Future<bool> toggle(MediaItem item) async {
    Logger.d('toggle: ${item.id} (${item.title})', tag: 'Favorites');

    // Save to local DB first (fast)
    final result = await _dao.toggle(
      mediaId: item.id,
      providerId: item.providerId,
      title: item.title,
      posterUrl: item.posterUrl,
      year: item.year,
      mediaType: item.type.name,
    );

    Logger.d('toggle result: $result', tag: 'Favorites');

    // Sync to cloud in background
    _syncSingleItem(item.id, item.providerId, result);

    return result;
  }

  /// Add to favorites (offline-first)
  Future<void> add(MediaItem item) async {
    // Save to local DB first
    await _dao.add(
      mediaId: item.id,
      providerId: item.providerId,
      title: item.title,
      posterUrl: item.posterUrl,
      year: item.year,
      mediaType: item.type.name,
    );

    // Sync to cloud in background
    _syncSingleItem(item.id, item.providerId, true);
  }

  /// Remove from favorites (offline-first)
  Future<void> remove(String mediaId, String providerId) async {
    // Remove from local DB first
    await _dao.remove(mediaId, providerId);

    // Sync to cloud in background
    _syncSingleItem(mediaId, providerId, false);
  }

  /// Get favorites by type
  Future<List<Favorite>> getByType(ContentType type) async {
    return _dao.getByType(type.name);
  }

  /// Get favorites count
  Future<int> get count => _dao.count();

  /// Clear all favorites
  Future<void> clearAll() async {
    await _dao.clearAll();
    // Sync deletion to cloud
    _syncToCloud();
  }

  /// Watch favorite status for specific item
  Stream<bool> watchIsFavorite(String mediaId, String providerId) {
    return _dao.watchIsFavorite(mediaId, providerId);
  }

  // ==================== Cloud Sync Methods ====================

  /// Pull favorites from cloud and merge with local (called on startup)
  Future<void> _pullFromCloud() async {
    if (!_authService.isAuthenticated) return;

    try {
      final user = _authService.currentUser;
      if (user == null) return;

      debugPrint('[Favorites] Pulling from cloud...');

      if (_server.isAuthenticated) {
        final records = await _server.getFavorites();
        debugPrint('[Favorites] Found ${records.length} cloud favorites from Oxide Server');

        for (final cloudData in records) {
          final mediaId = (cloudData['mediaId'] ?? cloudData['media_id']) as String?;
          final providerId = (cloudData['providerId'] ?? cloudData['provider_id']) as String?;
          if (mediaId == null || providerId == null) continue;

          final localExists = await _dao.isFavorite(mediaId, providerId);
          if (!localExists) {
            await _dao.add(
              mediaId: mediaId,
              providerId: providerId,
              title: (cloudData['title'] ?? '') as String,
              posterUrl: (cloudData['posterUrl'] ?? cloudData['poster_url']) as String?,
              year: cloudData['year'] as int?,
              mediaType: (cloudData['mediaType'] ?? cloudData['media_type'] ?? 'movie') as String,
              rating: (cloudData['rating'] as num?)?.toDouble(),
              ratingSource: (cloudData['ratingSource'] ?? cloudData['rating_source']) as String?,
            );
          }
        }
        debugPrint('[Favorites] Server cloud pull complete');
      }
    } catch (e) {
      debugPrint('[Favorites] Pull error: $e');
    }
  }

  /// Sync local favorites to cloud (periodic)
  Future<void> _syncToCloud() async {
    if (!_authService.isAuthenticated || _isSyncing) return;

    try {
      _isSyncing = true;
      notifyListeners();

      final user = _authService.currentUser;
      if (user == null) return;

      final localFavs = await _dao.getAll();
      debugPrint('[Favorites] Syncing ${localFavs.length} favorites to cloud');

      for (final fav in localFavs) {
        await _syncSingleItemToCloud(fav, true);
      }

      debugPrint('[Favorites] Sync to cloud complete');
    } catch (e) {
      debugPrint('[Favorites] Sync error: $e');
    } finally {
      _isSyncing = false;
      notifyListeners();
    }
  }

  /// Sync single item to cloud in background
  void _syncSingleItem(String mediaId, String providerId, bool isFavorite) {
    if (!_authService.isAuthenticated) return;

    // Run in background (don't await)
    Future.microtask(() async {
      try {
        final localFav = await _dao.get(mediaId, providerId);
        await _syncSingleItemToCloud(localFav, isFavorite);
      } catch (e) {
        debugPrint('[Favorites] Single item sync error: $e');
      }
    });
  }

  /// Helper to sync single favorite to cloud
  Future<void> _syncSingleItemToCloud(Favorite? fav, bool isFavorite) async {
    final user = _authService.currentUser;
    if (user == null || fav == null) return;

    try {
      if (_server.isAuthenticated) {
        if (isFavorite) {
          await _server.toggleFavorite(
            mediaId: fav.mediaId,
            providerId: fav.providerId,
            title: fav.title.isEmpty ? 'Unknown' : fav.title,
            posterUrl: (fav.posterUrl?.startsWith('http') ?? false)
                ? fav.posterUrl
                : null,
            year: fav.year,
            mediaType: fav.mediaType,
            rating: fav.rating,
            ratingSource: fav.ratingSource,
          );
        } else {
          await _server.removeFavorite(
            mediaId: fav.mediaId,
            providerId: fav.providerId,
          );
        }
      }
    } catch (e) {
      debugPrint('[Favorites] Cloud sync error: $e');
    }
  }

  /// Manual sync trigger (для користувача)
  Future<void> syncNow() async {
    await _syncToCloud();
  }

  @override
  void dispose() {
    _subscription?.cancel();
    _syncTimer?.cancel();
    super.dispose();
  }
}
