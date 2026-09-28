import 'dart:async';
import 'package:flutter/foundation.dart';
import 'package:pocketbase/pocketbase.dart';
import '../database/app_database.dart';
import '../database/dao/history_dao.dart';
import 'oxide_server_service.dart';
import 'pocketbase_service.dart';
import 'auth_service.dart';

/// Service for managing watch history with cloud sync
///
/// **Offline-first strategy:**
/// - Local Drift database is the PRIMARY source of truth
/// - Cloud (Oxide Server / PocketBase fallback) is used for backup and cross-device sync
/// - All reads come from local DB (fast)
/// - Writes go to local DB first, then sync to cloud in background
/// - On startup, pull latest from cloud and merge with local
class HistoryService extends ChangeNotifier {
  final HistoryDao _dao;
  final OxideServerService _server;
  final PocketBaseService _pocketBase;
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
  UnsubscribeFunc? _realtimeSub; // PocketBase realtime subscription

  HistoryService({
    required AppDatabase database,
    required OxideServerService server,
    required PocketBaseService pocketBase,
    required AuthService authService,
  }) : _dao = HistoryDao(database),
       _server = server,
       _pocketBase = pocketBase,
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

    // Subscribe to realtime updates (optional - fallback to periodic sync if fails)
    _subscribeToRealtime();
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
          final providerId = (data['providerId'] ?? data['provider_id']) as String?;
          if (mediaId == null || providerId == null) continue;

          final season = data['season'] as int?;
          final episode = data['episode'] as int?;

          final localItem = await _dao.getForMedia(
            mediaId,
            providerId,
            season: season,
            episode: episode,
          );

          final watchedAtStr = (data['watchedAt'] ?? data['watched_at']) as String?;
          final cloudWatchedAt = watchedAtStr != null ? DateTime.parse(watchedAtStr) : DateTime.now();

          if (localItem == null || localItem.watchedAt.isBefore(cloudWatchedAt)) {
            await _dao.saveProgress(
              mediaId: mediaId,
              providerId: providerId,
              title: (data['title'] ?? '') as String,
              posterUrl: (data['posterUrl'] ?? data['poster_url']) as String?,
              year: data['year'] as int?,
              mediaType: (data['mediaType'] ?? data['media_type'] ?? 'movie') as String,
              positionMs: (data['positionMs'] ?? data['position_ms'] ?? 0) as int,
              durationMs: (data['durationMs'] ?? data['duration_ms'] ?? 0) as int,
              season: season,
              episode: episode,
              episodeTitle: (data['episodeTitle'] ?? data['episode_title']) as String?,
              lastStreamUrl: (data['lastStreamUrl'] ?? data['last_stream_url']) as String?,
              voiceover: data['voiceover'] as String?,
              watchedAt: cloudWatchedAt,
            );
          }
        }
        debugPrint('✅ Synced ${serverRecords.length} items from Oxide Server');
        return;
      }

      // Fetch from cloud
      final cloudRecords = await _pocketBase.pb
          .collection('watch_history')
          .getList(page: 1, perPage: 500, sort: '-watched_at');

      for (final record in cloudRecords.items) {
        final data = record.data;

        final localItem = await _dao.getForMedia(
          data['media_id'] as String,
          data['provider_id'] as String,
          season: data['season'] as int?,
          episode: data['episode'] as int?,
        );

        final cloudWatchedAt = DateTime.parse(data['watched_at'] as String);

        if (localItem == null || localItem.watchedAt.isBefore(cloudWatchedAt)) {
          await _dao.saveProgress(
            mediaId: data['media_id'] as String,
            providerId: data['provider_id'] as String,
            title: data['title'] as String,
            posterUrl: data['poster_url'] as String?,
            year: data['year'] as int?,
            mediaType: data['media_type'] as String,
            positionMs: data['position_ms'] as int,
            durationMs: data['duration_ms'] as int,
            season: data['season'] as int?,
            episode: data['episode'] as int?,
            episodeTitle: data['episode_title'] as String?,
            lastStreamUrl: data['last_stream_url'] as String?,
            voiceover: data['voiceover'] as String?,
            watchedAt: cloudWatchedAt,
          );
        }
      }
      debugPrint('✅ Synced ${cloudRecords.items.length} items from PocketBase');
    } catch (e) {
      debugPrint('⚠️ Failed to pull from cloud: $e');
    } finally {
      _isSyncing = false;
      notifyListeners();
    }
  }

  /// Subscribe to realtime updates from PocketBase
  /// Optional feature - falls back to periodic sync if realtime fails
  Future<void> _subscribeToRealtime() async {
    if (!_pocketBase.isAuthenticated) {
      return;
    }

    final user = _authService.currentUser;
    if (user == null) {
      return;
    }

    try {
      // Subscribe to watch_history changes for current user only
      _realtimeSub = await _pocketBase.pb
          .collection('watch_history')
          .subscribe(
            '*',
            (e) => _handleRealtimeEvent(e),
            filter: 'user_id = "${user.id}"',
          );

      debugPrint('[History] ✓ Subscribed to realtime updates');
    } catch (e) {
      debugPrint(
        '[History] Realtime subscription failed (using periodic sync): $e',
      );
      // Not critical - periodic sync will handle updates
    }
  }

  /// Handle realtime events from PocketBase
  void _handleRealtimeEvent(RecordSubscriptionEvent e) {
    final record = e.record;
    if (record == null) return;

    debugPrint('[History] Realtime event: ${e.action} for ${record.id}');

    switch (e.action) {
      case 'create':
      case 'update':
        _mergeCloudRecord(record);
        break;
      case 'delete':
        _removeCloudRecord(record);
        break;
    }
  }

  /// Merge a single cloud record into local DB
  Future<void> _mergeCloudRecord(RecordModel record) async {
    try {
      final data = record.data;
      final cloudUpdatedAt = DateTime.parse(record.updated);

      // Check if we have this locally
      final local = await _dao.getForMedia(
        data['media_id'] as String,
        data['provider_id'] as String,
        season: data['season'] as int?,
        episode: data['episode'] as int?,
      );

      // If local is newer, skip (our changes will be synced via periodic)
      // If local set is later, merge cloud data into it
      // but keep local watched_at if it is newer
      if (local != null && local.watchedAt.isAfter(cloudUpdatedAt)) {
        // Local is newer, keep it (but maybe update other fields?)
        // For now, simple rule: newest wins
        return;
      }

      // Merge into local DB (use snake_case field names from PocketBase)
      await _dao.saveProgress(
        mediaId: data['media_id'] as String,
        providerId: data['provider_id'] as String,
        title: data['title'] as String,
        posterUrl: data['poster_url'] as String?,
        year: data['year'] as int?,
        mediaType: data['media_type'] as String,
        positionMs: data['position_ms'] as int,
        durationMs: data['duration_ms'] as int,
        season: data['season'] as int?,
        episode: data['episode'] as int?,
        episodeTitle: data['episode_title'] as String?,
        lastStreamUrl: data['last_stream_url'] as String?,
        voiceover: data['voiceover'] as String?,
        rating: (data['rating'] as num?)?.toDouble(),
        ratingSource: data['rating_source'] as String?,
        watchedAt: cloudUpdatedAt,
      );

      debugPrint('[History] ✓ Merged cloud record: ${data['title']}');
    } catch (e) {
      debugPrint('[History] Failed to merge cloud record: $e');
    }
  }

  /// Remove a record that was deleted in the cloud
  Future<void> _removeCloudRecord(RecordModel record) async {
    try {
      final data = record.data;
      await _dao.remove(
        data['media_id'] as String,
        data['provider_id'] as String,
      );
      debugPrint('[History] ✓ Removed deleted cloud record');
    } catch (e) {
      debugPrint('[History] Failed to remove cloud record: $e');
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

      for (final item in localHistory) {
        try {
          await _syncSingleItemToCloud(
            mediaId: item.mediaId,
            providerId: item.providerId,
            season: item.season,
            episode: item.episode,
          );
          synced++;
        } catch (e) {
          debugPrint('Failed to sync ${item.mediaId}: $e');
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

      // Check if exists in cloud (include season/episode for series)
      String filter =
          'user_id = "$userId" && media_id = "$mediaId" && provider_id = "$providerId"';
      if (season != null) filter += ' && season = $season';
      if (episode != null) filter += ' && episode = $episode';

      final existing = await _pocketBase.pb
          .collection('watch_history')
          .getList(filter: filter);

        final body = {
          'user_id': userId,
          'media_id': mediaId,
          'provider_id': providerId,
          'title': localItem.title.isEmpty ? 'Unknown' : localItem.title,
          'poster_url': (localItem.posterUrl?.startsWith('http') ?? false)
              ? localItem.posterUrl
              : null,
          'year': localItem.year,
          'media_type': localItem.mediaType,
          'position_ms': localItem.positionMs,
          'duration_ms': localItem.durationMs,
          'season': season,
          'episode': episode,
          'episode_title': localItem.episodeTitle,
          'last_stream_url':
              (localItem.lastStreamUrl?.startsWith('http') ?? false)
              ? localItem.lastStreamUrl
              : null,
          'voiceover': localItem.voiceover,
          'watched_at': localItem.watchedAt.toIso8601String(),
        };

        if (existing.items.isEmpty) {
          // Create new
          await _pocketBase.pb.collection('watch_history').create(body: body);
        } else {
          // Update existing
          await _pocketBase.pb
              .collection('watch_history')
              .update(existing.items.first.id, body: body);
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
    _realtimeSub?.call(); // Unsubscribe from realtime
    _historySubscription?.cancel();
    _continueSubscription?.cancel();
    _syncTimer?.cancel();
    super.dispose();
  }
}
