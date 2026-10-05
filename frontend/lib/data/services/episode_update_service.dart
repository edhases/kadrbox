import 'dart:async';
import 'package:flutter/material.dart';

import '../database/app_database.dart';
import '../database/dao/favorites_dao.dart';
import '../providers/provider_registry.dart';

/// Information about a new episode
class NewEpisodeInfo {
  final String mediaId;
  final String providerId;
  final String title;
  final String? posterUrl;
  final int season;
  final int episode;
  final String? episodeTitle;
  final DateTime discoveredAt;

  const NewEpisodeInfo({
    required this.mediaId,
    required this.providerId,
    required this.title,
    this.posterUrl,
    required this.season,
    required this.episode,
    this.episodeTitle,
    required this.discoveredAt,
  });

  String get episodeLabel =>
      'S${season.toString().padLeft(2, '0')}E${episode.toString().padLeft(2, '0')}';
}

/// Service for checking and notifying about new episodes
class EpisodeUpdateService extends ChangeNotifier {
  final FavoritesDao _favoritesDao;
  final ProviderRegistry _registry;

  List<NewEpisodeInfo> _newEpisodes = [];
  bool _isChecking = false;
  DateTime? _lastCheck;
  String? _error;

  List<NewEpisodeInfo> get newEpisodes => _newEpisodes;
  bool get isChecking => _isChecking;
  DateTime? get lastCheck => _lastCheck;
  bool get hasNewEpisodes => _newEpisodes.isNotEmpty;
  int get newEpisodesCount => _newEpisodes.length;
  String? get error => _error;

  // Store last known episode counts: key = "mediaId:providerId" -> lastEpisode
  final Map<String, int> _lastKnownEpisodes = {};

  EpisodeUpdateService(AppDatabase database, this._registry)
    : _favoritesDao = FavoritesDao(database);

  /// Check for new episodes in favorite series
  Future<void> checkForUpdates({bool force = false}) async {
    if (_isChecking) return;

    // Only check every 30 minutes unless forced
    if (!force && _lastCheck != null) {
      final sinceLastCheck = DateTime.now().difference(_lastCheck!);
      if (sinceLastCheck.inMinutes < 30) {
        return;
      }
    }

    _isChecking = true;
    _error = null;
    notifyListeners();

    try {
      final favorites = await _favoritesDao.getAll();

      // Filter only series-type content
      final series = favorites
          .where(
            (f) =>
                f.mediaType == 'series' ||
                f.mediaType == 'anime' ||
                f.mediaType == 'cartoon',
          )
          .toList();

      final newEpisodesList = <NewEpisodeInfo>[];

      for (final item in series) {
        try {
          final provider = _registry.getById(item.providerId);
          if (provider == null) continue;

          // Get details to check episode count
          final details = await provider.getDetails(item.mediaId);

          final seasons = details.seasons;
          if (seasons == null || seasons.isEmpty) continue;

          // Get the latest episode info
          int latestEpisode = 0;
          int latestSeason = 0;
          String? latestEpisodeTitle;

          for (final season in seasons) {
            if (season.episodes.isNotEmpty) {
              final maxEp = season.episodes.reduce(
                (a, b) => a.number > b.number ? a : b,
              );
              if (season.number > latestSeason ||
                  (season.number == latestSeason &&
                      maxEp.number > latestEpisode)) {
                latestSeason = season.number;
                latestEpisode = maxEp.number;
                latestEpisodeTitle = maxEp.title;
              }
            }
          }

          if (latestEpisode == 0) continue;

          // Check if this is new
          final key = '${item.mediaId}:${item.providerId}';
          final lastKnown = _lastKnownEpisodes[key];

          if (lastKnown != null) {
            // Calculate total episode number for comparison
            final currentTotal = _calculateTotalEpisodes(
              latestSeason,
              latestEpisode,
            );

            if (currentTotal > lastKnown) {
              newEpisodesList.add(
                NewEpisodeInfo(
                  mediaId: item.mediaId,
                  providerId: item.providerId,
                  title: item.title,
                  posterUrl: item.posterUrl,
                  season: latestSeason,
                  episode: latestEpisode,
                  episodeTitle: latestEpisodeTitle,
                  discoveredAt: DateTime.now(),
                ),
              );
            }
          }

          // Update last known
          _lastKnownEpisodes[key] = _calculateTotalEpisodes(
            latestSeason,
            latestEpisode,
          );
        } catch (e) {
          debugPrint('Failed to check updates for ${item.title}: $e');
        }
      }

      _newEpisodes = newEpisodesList;
      _lastCheck = DateTime.now();
      _isChecking = false;
      notifyListeners();
    } catch (e) {
      _error = e.toString();
      _isChecking = false;
      notifyListeners();
    }
  }

  int _calculateTotalEpisodes(int season, int episode) {
    // Simple calculation: assume max 100 episodes per season
    return season * 100 + episode;
  }

  /// Mark an episode as seen (remove from new list)
  void markAsSeen(String mediaId, String providerId) {
    _newEpisodes.removeWhere(
      (e) => e.mediaId == mediaId && e.providerId == providerId,
    );
    notifyListeners();
  }

  /// Clear all new episode notifications
  void clearAll() {
    _newEpisodes.clear();
    notifyListeners();
  }

  /// Get formatted last check time
  String get lastCheckFormatted {
    if (_lastCheck == null) return 'Ніколи';

    final now = DateTime.now();
    final diff = now.difference(_lastCheck!);

    if (diff.inMinutes < 1) return 'Щойно';
    if (diff.inMinutes < 60) return '${diff.inMinutes} хв тому';
    if (diff.inHours < 24) return '${diff.inHours} год тому';
    return '${diff.inDays} дн тому';
  }
}
