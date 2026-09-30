import 'dart:async';
import 'package:flutter/foundation.dart';
import '../database/app_database.dart';
import '../database/dao/history_dao.dart';
import 'oxide_server_service.dart';
import 'auth_service.dart';

/// Service for managing watch history with cloud sync
///
/// **Offline-first strategy:**
/// - Local Drift database is the PRIMARY source of truth
/// - Cloud (Oxide Server) is used for backup and cross-device sync
/// - All reads come from local DB (fast)
/// - Writes go to local DB first, then sync to cloud in background
/// - On startup, pull latest from cloud and merge with local
class HistoryService extends ChangeNotifier {
  final HistoryDao _dao;
  final OxideServerService _server;
  final AuthService _authService;

  List<WatchHistoryData> _history = [];
  List<WatchHistoryData> get history => _history;

  List<WatchHistoryData> _continueWatching = [];
  List<WatchHistoryData> get continueWatching => _continueWatching;

  final bool _isLoading = false;
  bool get isLoading => _isLoading;

  bool _isSyncing = false;
  bool get isSyncing => _isSyncing;

  StreamSubscription<List<WatchHistoryData>>? _historySubscription;
  StreamSubscription<List<WatchHistoryData>>? _continueSubscription;
  Timer? _syncTimer;

  HistoryService({
    required AppDatabase database,
    required OxideServerService server,
    required AuthService authService,
  }) : _dao = HistoryDao(database),
       _server = server,
       _authService = authService {
    _init();
  }

  void _init() {
    // Cleanup duplicates on startup (fire and forget)
    _dao.cleanupDuplicates().then((count) {
      if (count > 0) {
        debugPrint('Cleaned up $count duplicate history entries');
        notifyListeners();
      }
    });

    _historySubscription = _dao.watchAll(limit: 50).listen((items) {
      _history = items;
      notifyListeners();
    });

    _continueSubscription = _dao.watchContinueWatching(limit: 20).listen((
      items,
    ) {
      _continueWatching = items;
      notifyListeners();
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
    );
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
    notifyListeners();
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

  // ============================================================================
  // Cloud Sync Methods
  // ============================================================================

  /// Pull latest history from cloud and merge with local
  Future<void> _pullFromCloud() async {
    if (!_authService.isAuthenticated) return;

    try {
      _isSyncing = true;
      notifyListeners();

      if (_server.isAuthenticated) {
        final serverRecords = await _server.getHistory();
        for (final data in serverRecords) {
          final mediaId = (data['mediaId'] ?? data['media_id']) as String?;
          final providerId =
              (data['providerId'] ?? data['provider_id']) as String?;
          if (mediaId == null || providerId == null) continue;

          final season = data['season'] as int?;
          final episode = data['episode'] as int?;

          final localItem = await _dao.getForMedia(
            mediaId,
            providerId,
            season: season,
            episode: episode,
          );

          final watchedAtStr =
              (data['watchedAt'] ?? data['watched_at']) as String?;
          final cloudWatchedAt = watchedAtStr != null
              ? DateTime.parse(watchedAtStr)
              : DateTime.now();

          if (localItem == null ||
              localItem.watchedAt.isBefore(cloudWatchedAt)) {
            await _dao.saveProgress(
              mediaId: mediaId,
              providerId: providerId,
              title: (data['title'] ?? '') as String,
              posterUrl: (data['posterUrl'] ?? data['poster_url']) as String?,
              year: data['year'] as int?,
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
        debugPrint('✅ Synced ${serverRecords.length} items from Oxide Server');
        return;
      }
    } catch (e) {
      final msg = e.toString();
      if (msg.contains('HTTP_401') || msg.contains('invalid or expired token')) {
        debugPrint('⚠️ Cloud pull: token expired, skipping sync');
      } else {
        debugPrint('⚠️ Failed to pull from cloud: $e');
      }
    } finally {
      _isSyncing = false;
      notifyListeners();
    }
  }

  /// Sync all local history to cloud
  Future<void> _syncToCloud() async {
    if (!_authService.isAuthenticated) return;

    try {
      _isSyncing = true;
      notifyListeners();

      final localHistory = await _dao.getAll(limit: 500);
      int synced = 0;
      bool tokenExpired = false;

      for (final item in localHistory) {
        if (tokenExpired) break; // зупиняємо якщо токен протух — не спамимо

        try {
          await _syncSingleItemToCloud(
            mediaId: item.mediaId,
            providerId: item.providerId,
            season: item.season,
            episode: item.episode,
          );
          synced++;
        } catch (e) {
          final msg = e.toString();
          // Якщо токен протух — зупиняємо весь батч, не пробуємо решту
          if (msg.contains('HTTP_401') || msg.contains('invalid or expired token')) {
            debugPrint('Cloud sync: token expired, stopping batch sync');
            tokenExpired = true;
          } else {
            debugPrint('Failed to sync ${item.mediaId}: $e');
          }
        }
      }

      debugPrint('✅ Synced $synced/${localHistory.length} items to cloud');
    } catch (e) {
      debugPrint('⚠️ Failed to sync to cloud: $e');
    } finally {
      _isSyncing = false;
      notifyListeners();
    }
  }

  /// Sync a single item to cloud (called after local save)
  Future<void> _syncSingleItemToCloud({
    required String mediaId,
    required String providerId,
    int? season,
    int? episode,
  }) async {
    if (!_authService.isAuthenticated) return;

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
    }
  }

  /// Force full sync now (pull + push)
  Future<void> syncNow() async {
    await _pullFromCloud();
    await _syncToCloud();
  }

  @override
  void dispose() {
    _historySubscription?.cancel();
    _continueSubscription?.cancel();
    _syncTimer?.cancel();
    super.dispose();
  }
}
