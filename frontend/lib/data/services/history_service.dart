import 'dart:async';
import 'package:flutter/foundation.dart';
import '../../core/error/exceptions.dart';
import '../database/app_database.dart';
import '../database/dao/history_dao.dart';
import 'kadrbox_server_service.dart';
import 'auth_service.dart';

/// Service for managing watch history with cloud sync
///
/// **Offline-first strategy:**
/// - Local Drift database is the PRIMARY source of truth
/// - Cloud (Kadrbox Server) is used for backup and cross-device sync
/// - All reads come from local DB (fast)
/// - Writes go to local DB first, then sync to cloud in background
/// - On startup, pull latest from cloud and merge with local
class HistoryService extends ChangeNotifier {
  final HistoryDao _dao;
  final KadrboxServerService _server;
  final AuthService _authService;

  List<WatchHistoryData> _history = [];
  List<WatchHistoryData> get history => _history;

  List<WatchHistoryData> _continueWatching = [];
  List<WatchHistoryData> get continueWatching => _continueWatching;

  final bool _isLoading = false;
  bool get isLoading => _isLoading;

  /// Direction-tracked sync state. Previously a single `_isSyncing` boolean was
  /// flipped by both pull and push, so whichever finished first cleared it while
  /// the other was still writing. The direction flags stay private: adding new
  /// public members here breaks the hand-rolled `MockHistoryService` in
  /// `test/unit/services/recommendation_service_test.dart`
  /// (`extends ChangeNotifier implements HistoryService`), which has no
  /// `noSuchMethod`.
  bool _isPulling = false;
  bool _isPushing = false;

  /// True while a cloud pull or push is running.
  bool get isSyncing => _isPulling || _isPushing;

  /// The single cloud sync currently in flight (pull or push). Cloud pull and
  /// cloud push both mutate the same history keys, so they must never overlap.
  Future<void>? _inFlightSync;

  bool _cloudSyncDisabled = false;
  bool _disposed = false;
  VoidCallback? _authListener;

  StreamSubscription<List<WatchHistoryData>>? _historySubscription;
  StreamSubscription<List<WatchHistoryData>>? _continueSubscription;
  Timer? _syncTimer;

  HistoryService({
    required AppDatabase database,
    required KadrboxServerService server,
    required AuthService authService,
  }) : _dao = HistoryDao(database),
       _server = server,
       _authService = authService {
    _init();
  }

  /// Notifies listeners unless the service has been disposed. Sync callbacks can
  /// land after `dispose()` (timer callbacks, in-flight HTTP), and calling
  /// `notifyListeners()` then throws.
  void _notify() {
    if (_disposed) return;
    notifyListeners();
  }

  /// Runs [body] as the only cloud sync in flight, or returns the future of the
  /// one already running instead of starting a second pass.
  ///
  /// The auth listener can fire a pull while the 5-minute timer is pushing 500
  /// items; without this guard both write the same keys concurrently and used to
  /// reset the shared `_isSyncing` flag out from under each other.
  Future<void> _runExclusive(Future<void> Function() body) {
    final existing = _inFlightSync;
    if (existing != null) return existing;

    final completer = Completer<void>();
    // Publish the future *before* running the body: an async body may complete
    // synchronously up to its first await, which would otherwise clear the
    // guard before it is assigned and wedge it forever.
    _inFlightSync = completer.future;
    Future<void>(() async {
      try {
        await body();
        completer.complete();
      } catch (e) {
        debugPrint('⚠️ History cloud sync failed: $e');
        completer.complete();
      } finally {
        _inFlightSync = null;
      }
    });
    return completer.future;
  }

  void _init() {
    _authListener = () {
      if (!_authService.isAuthenticated) {
        _cloudSyncDisabled = true;
      } else {
        _cloudSyncDisabled = false;
        _pullFromCloud();
      }
    };
    _authService.addListener(_authListener!);
    // Cleanup duplicates on startup (fire and forget)
    _dao.cleanupDuplicates().then((count) {
      if (count > 0) {
        debugPrint('Cleaned up $count duplicate history entries');
        _notify();
      }
    });

    _historySubscription = _dao.watchAll(limit: 50).listen((items) {
      _history = items;
      _notify();
    });

    _continueSubscription = _dao.watchContinueWatching(limit: 20).listen((
      items,
    ) {
      _continueWatching = items;
      _notify();
    });

    // Pull latest from cloud on startup
    _pullFromCloud();

    // Setup periodic sync every 5 minutes
    _syncTimer = Timer.periodic(const Duration(minutes: 5), (_) {
      _syncToCloud();
    });
  }

  /// Save watch progress (offline-first)
  /// Saves to local DB immediately, then syncs to cloud in background
  Future<void> saveProgress({
    required String mediaId,
    required String providerId,
    required String title,
    String? posterUrl,
    int? year,
    required String mediaType,
    required Duration position,
    required Duration duration,
    int? season,
    int? episode,
    String? episodeTitle,
    String? lastStreamUrl,
    String? voiceover,
  }) async {
    // Save to local DB first (fast)
    await _dao.saveProgress(
      mediaId: mediaId,
      providerId: providerId,
      title: title,
      posterUrl: posterUrl,
      year: year,
      mediaType: mediaType,
      positionMs: position.inMilliseconds,
      durationMs: duration.inMilliseconds,
      season: season,
      episode: episode,
      episodeTitle: episodeTitle,
      lastStreamUrl: lastStreamUrl,
      voiceover: voiceover,
    );

    // Sync to cloud in background (don't await)
    _syncSingleItemToCloud(
      mediaId: mediaId,
      providerId: providerId,
      season: season,
      episode: episode,
    ).catchError((e) {
      // Ignored: _cloudSyncDisabled is handled inside _syncSingleItemToCloud
    });
  }

  /// Get last position for media
  Future<Duration?> getLastPosition(
    String mediaId,
    String providerId, {
    int? season,
    int? episode,
  }) async {
    return _dao.getLastPosition(
      mediaId,
      providerId,
      season: season,
      episode: episode,
    );
  }

  /// Get history entry for media
  Future<WatchHistoryData?> getForMedia(
    String mediaId,
    String providerId, {
    int? season,
    int? episode,
  }) async {
    return _dao.getForMedia(
      mediaId,
      providerId,
      season: season,
      episode: episode,
    );
  }

  /// Remove from history
  Future<void> remove(String mediaId, String providerId) async {
    await _dao.remove(mediaId, providerId);
  }

  /// Clear all history
  Future<void> clearAll() async {
    await _dao.clearAll();
    _notify();
  }

  /// Get history count
  Future<int> get count => _dao.count();

  /// Calculate progress percentage
  double getProgress(WatchHistoryData item) {
    if (item.durationMs == 0) return 0;
    return (item.positionMs / item.durationMs).clamp(0.0, 1.0);
  }

  /// Format remaining time
  String formatRemaining(WatchHistoryData item) {
    final remaining = Duration(milliseconds: item.durationMs - item.positionMs);
    if (remaining.inHours > 0) {
      return '${remaining.inHours}г ${remaining.inMinutes % 60}хв залишилось';
    }
    return '${remaining.inMinutes}хв залишилось';
  }

  bool _isAuthError(dynamic e) {
    // `ApiClient` always turns a `badResponse` into a `ServerException` carrying
    // the status code, so a typed check is sufficient. Substring matching on
    // `toString()` produced false positives (a socket error mentioning "401
    // bytes") that needlessly disabled cloud sync.
    return e is ServerException && e.statusCode == 401;
  }

  // ============================================================================
  // Cloud Sync Methods
  // ============================================================================

  /// Pull latest history from cloud and merge with local
  Future<void> _pullFromCloud() {
    return _runExclusive(() async {
      if (!_authService.isAuthenticated ||
          !_server.isAuthenticated ||
          _cloudSyncDisabled) {
        return;
      }

      try {
        _isPulling = true;
        _notify();

        final serverRecords = await _server.getHistory();
        for (final data in serverRecords) {
          final mediaId = (data['mediaId'] ?? data['media_id']) as String?;
          final providerId =
              (data['providerId'] ?? data['provider_id']) as String?;
          if (mediaId == null || providerId == null) continue;

          final season = (data['season'] as num?)?.toInt();
          final episode = (data['episode'] as num?)?.toInt();

          final localItem = await _dao.getForMedia(
            mediaId,
            providerId,
            season: season,
            episode: episode,
          );

          final watchedAtStr =
              (data['watchedAt'] ?? data['watched_at']) as String?;
          final cloudWatchedAt = watchedAtStr != null
              ? DateTime.tryParse(watchedAtStr) ?? DateTime.now()
              : DateTime.now();

          if (localItem == null ||
              localItem.watchedAt.isBefore(cloudWatchedAt)) {
            await _dao.saveProgress(
              mediaId: mediaId,
              providerId: providerId,
              title: (data['title'] ?? '') as String,
              posterUrl: (data['posterUrl'] ?? data['poster_url']) as String?,
              year: (data['year'] as num?)?.toInt(),
              mediaType:
                  (data['mediaType'] ?? data['media_type'] ?? 'movie')
                      as String,
              positionMs:
                  (data['positionMs'] ?? data['position_ms'] ?? 0) as int,
              durationMs:
                  (data['durationMs'] ?? data['duration_ms'] ?? 0) as int,
              season: season,
              episode: episode,
              episodeTitle:
                  (data['episodeTitle'] ?? data['episode_title']) as String?,
              lastStreamUrl:
                  (data['lastStreamUrl'] ?? data['last_stream_url']) as String?,
              voiceover: data['voiceover'] as String?,
              watchedAt: cloudWatchedAt,
            );
          }
        }
        debugPrint('✅ Synced ${serverRecords.length} items from Kadrbox Server');
      } catch (e) {
        if (_isAuthError(e)) {
          _cloudSyncDisabled = true;
          debugPrint('⚠️ Cloud pull: token expired, skipping sync');
        } else {
          debugPrint('⚠️ Failed to pull from cloud: $e');
        }
      } finally {
        _isPulling = false;
        _notify();
      }
    });
  }

  /// Sync all local history to cloud
  Future<void> _syncToCloud() {
    return _runExclusive(() async {
      if (!_authService.isAuthenticated ||
          !_server.isAuthenticated ||
          _cloudSyncDisabled) {
        return;
      }

      try {
        _isPushing = true;
        _notify();

        final localHistory = await _dao.getAll(limit: 500);
        int synced = 0;
        int failed = 0;

        for (final item in localHistory) {
          if (_cloudSyncDisabled || !_server.isAuthenticated) break;

          try {
            await _syncSingleItemToCloud(
              mediaId: item.mediaId,
              providerId: item.providerId,
              season: item.season,
              episode: item.episode,
            );
            synced++;
          } catch (e) {
            if (_isAuthError(e)) {
              debugPrint(
                'Cloud sync: token expired, stopping batch sync immediately',
              );
              _cloudSyncDisabled = true;
              break;
            } else {
              failed++;
              debugPrint('Failed to sync ${item.mediaId}: $e');
            }
          }
        }

        if (failed > 0) {
          debugPrint(
            '⚠️ Synced $synced/${localHistory.length} items to cloud, $failed failed',
          );
        } else {
          debugPrint('✅ Synced $synced/${localHistory.length} items to cloud');
        }
      } catch (e) {
        debugPrint('⚠️ Failed to sync to cloud: $e');
      } finally {
        _isPushing = false;
        _notify();
      }
    });
  }

  /// Sync a single item to cloud (called after local save)
  Future<void> _syncSingleItemToCloud({
    required String mediaId,
    required String providerId,
    int? season,
    int? episode,
  }) async {
    if (!_authService.isAuthenticated ||
        !_server.isAuthenticated ||
        _cloudSyncDisabled) {
      return;
    }

    try {
      final localItem = await _dao.getForMedia(
        mediaId,
        providerId,
        season: season,
        episode: episode,
      );

      if (localItem == null) return;

      final userId = _authService.currentUser?.id;
      if (userId == null) return;

      if (_server.isAuthenticated) {
        await _server.saveHistoryProgress(
          mediaId: mediaId,
          providerId: providerId,
          title: localItem.title.isEmpty ? 'Unknown' : localItem.title,
          posterUrl: (localItem.posterUrl?.startsWith('http') ?? false)
              ? localItem.posterUrl
              : null,
          year: localItem.year,
          mediaType: localItem.mediaType,
          positionMs: localItem.positionMs,
          durationMs: localItem.durationMs,
          season: season,
          episode: episode,
          episodeTitle: localItem.episodeTitle,
          lastStreamUrl: (localItem.lastStreamUrl?.startsWith('http') ?? false)
              ? localItem.lastStreamUrl
              : null,
          voiceover: localItem.voiceover,
          watchedAt: localItem.watchedAt,
        );
        return;
      }
    } catch (e) {
      debugPrint('Failed to sync item to cloud: $e');
      if (_isAuthError(e)) {
        _cloudSyncDisabled = true;
        rethrow;
      }
      // Re-throw so the batch loop can count the failure. Swallowing it here
      // made every batch report "Synced N/N" no matter how many items the
      // server had rejected, which hid a real 500 from the sync report.
      rethrow;
    }
  }

  /// Force full sync now (pull + push)
  Future<void> syncNow() async {
    await _pullFromCloud();
    await _syncToCloud();
  }

  @override
  void dispose() {
    if (_disposed) return;
    _disposed = true;
    if (_authListener != null) {
      _authService.removeListener(_authListener!);
    }
    _historySubscription?.cancel();
    _continueSubscription?.cancel();
    _syncTimer?.cancel();
    super.dispose();
  }
}
