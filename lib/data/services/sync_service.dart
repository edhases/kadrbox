import 'dart:convert';
import 'dart:io';
import 'dart:isolate';

import 'package:file_picker/file_picker.dart';
import 'package:flutter/foundation.dart';
import 'package:path_provider/path_provider.dart';
import 'package:share_plus/share_plus.dart';

import '../../core/utils/logger.dart';
import '../database/app_database.dart';
import '../database/dao/favorites_dao.dart';
import '../database/dao/history_dao.dart';
import '../database/dao/settings_dao.dart';

/// Data synchronization service
///
/// Provides export/import functionality for user data
/// without requiring a backend server
class SyncService extends ChangeNotifier {
  static const String _tag = 'SyncService';
  final AppDatabase _database;
  late final FavoritesDao _favoritesDao;
  late final HistoryDao _historyDao;
  late final SettingsDao _settingsDao;

  bool _isExporting = false;
  bool get isExporting => _isExporting;

  bool _isImporting = false;
  bool get isImporting => _isImporting;

  String? _lastError;
  String? get lastError => _lastError;

  SyncService(this._database) {
    _favoritesDao = FavoritesDao(_database);
    _historyDao = HistoryDao(_database);
    _settingsDao = SettingsDao(_database);
  }

  /// Export all user data to JSON file
  Future<String?> exportData({
    bool includeFavorites = true,
    bool includeHistory = true,
    bool includeSettings = true,
  }) async {
    _isExporting = true;
    _lastError = null;
    notifyListeners();

    try {
      final data = <String, dynamic>{
        'version': 1,
        'exportedAt': DateTime.now().toIso8601String(),
        'app': 'OxideFilm',
      };

      if (includeFavorites) {
        final favorites = await _favoritesDao.getAll();
        data['favorites'] = favorites.map((f) => _favoriteToJson(f)).toList();
      }

      if (includeHistory) {
        final history = await _historyDao.getAll(limit: null);
        data['history'] = history.map((h) => _historyToJson(h)).toList();
      }

      if (includeSettings) {
        final settings = await _settingsDao.getAllSettings();
        data['settings'] = settings;
      }

      final jsonString = const JsonEncoder.withIndent('  ').convert(data);

      // Save to file
      final fileName =
          'oxide_film_backup_${DateTime.now().millisecondsSinceEpoch}.json';
      String filePath;

      if (kIsWeb) {
        // For web, trigger download
        filePath = fileName;
      } else {
        final directory = await getApplicationDocumentsDirectory();
        filePath = '${directory.path}/$fileName';
        final file = File(filePath);
        await file.writeAsString(jsonString);
      }

      _isExporting = false;
      notifyListeners();

      return filePath;
    } catch (e) {
      _lastError = e.toString();
      _isExporting = false;
      notifyListeners();
      return null;
    }
  }

  /// Export and share via system share dialog
  Future<void> exportAndShare() async {
    final filePath = await exportData();
    if (filePath == null) return;

    if (!kIsWeb) {
      await Share.shareXFiles([XFile(filePath)], subject: 'Oxide Film Backup');
    }
  }

  /// Import data from JSON file
  Future<bool> importData() async {
    _isImporting = true;
    _lastError = null;
    notifyListeners();

    try {
      // Pick file
      final result = await FilePicker.platform.pickFiles(
        type: FileType.custom,
        allowedExtensions: ['json'],
      );

      if (result == null || result.files.isEmpty) {
        _isImporting = false;
        notifyListeners();
        return false;
      }

      String jsonString;

      if (kIsWeb) {
        final bytes = result.files.first.bytes;
        if (bytes == null) throw Exception('Failed to read file');
        jsonString = utf8.decode(bytes);
      } else {
        final file = File(result.files.first.path!);
        jsonString = await file.readAsString();
      }

      final data =
          await Isolate.run(() => jsonDecode(jsonString))
              as Map<String, dynamic>;

      // Validate format
      if (data['app'] != 'OxideFilm') {
        throw Exception('Invalid backup file format');
      }

      // Import favorites
      if (data['favorites'] != null) {
        final favorites = data['favorites'] as List;
        for (final f in favorites) {
          await _importFavorite(f as Map<String, dynamic>);
        }
      }

      // Import history
      if (data['history'] != null) {
        final history = data['history'] as List;
        for (final h in history) {
          await _importHistory(h as Map<String, dynamic>);
        }
      }

      // Import settings
      if (data['settings'] != null) {
        final settings = data['settings'] as Map<String, dynamic>;
        for (final entry in settings.entries) {
          await _settingsDao.setSetting(entry.key, entry.value.toString());
        }
      }

      _isImporting = false;
      notifyListeners();
      return true;
    } catch (e) {
      _lastError = e.toString();
      _isImporting = false;
      notifyListeners();
      return false;
    }
  }

  /// Import from JSON string directly (e.g., from QR code)
  Future<bool> importFromJson(String jsonString) async {
    _isImporting = true;
    _lastError = null;
    notifyListeners();

    try {
      final data =
          await Isolate.run(() => jsonDecode(jsonString))
              as Map<String, dynamic>;

      if (data['app'] != 'OxideFilm') {
        throw Exception('Invalid backup format');
      }

      if (data['favorites'] != null) {
        final favorites = data['favorites'] as List;
        for (final f in favorites) {
          await _importFavorite(f as Map<String, dynamic>);
        }
      }

      if (data['history'] != null) {
        final history = data['history'] as List;
        for (final h in history) {
          await _importHistory(h as Map<String, dynamic>);
        }
      }

      if (data['settings'] != null) {
        final settings = data['settings'] as Map<String, dynamic>;
        for (final entry in settings.entries) {
          await _settingsDao.setSetting(entry.key, entry.value.toString());
        }
      }

      _isImporting = false;
      notifyListeners();
      return true;
    } catch (e) {
      _lastError = e.toString();
      _isImporting = false;
      notifyListeners();
      return false;
    }
  }

  /// Generate compact JSON for QR code (favorites only)
  Future<String> generateQrData() async {
    final favorites = await _favoritesDao.getAll();

    final data = {
      'app': 'OxideFilm',
      'v': 1,
      'f': favorites
          .map((f) => {'i': f.mediaId, 'p': f.providerId, 't': f.title})
          .toList(),
    };

    return jsonEncode(data);
  }

  Map<String, dynamic> _favoriteToJson(Favorite f) {
    return {
      'mediaId': f.mediaId,
      'providerId': f.providerId,
      'title': f.title,
      'posterUrl': f.posterUrl,
      'year': f.year,
      'mediaType': f.mediaType,
      'addedAt': f.addedAt.toIso8601String(),
    };
  }

  Map<String, dynamic> _historyToJson(WatchHistoryData h) {
    return {
      'mediaId': h.mediaId,
      'providerId': h.providerId,
      'title': h.title,
      'posterUrl': h.posterUrl,
      'year': h.year,
      'mediaType': h.mediaType,
      'positionMs': h.positionMs,
      'durationMs': h.durationMs,
      'season': h.season,
      'episode': h.episode,
      'episodeTitle': h.episodeTitle,
      'lastStreamUrl': h.lastStreamUrl,
      'voiceover': h.voiceover,
      'watchedAt': h.watchedAt.toIso8601String(),
    };
  }

  Future<void> _importFavorite(Map<String, dynamic> data) async {
    final mediaId = (data['mediaId'] ?? data['media_id'])?.toString();
    final providerId = (data['providerId'] ?? data['provider_id'])?.toString();
    final title = data['title']?.toString();
    if (mediaId == null || providerId == null || title == null) {
      Logger.w(
        'Skipping invalid favorite backup record: missing required fields',
        tag: _tag,
      );
      return;
    }

    try {
      final rawYear = data['year'];
      final year = rawYear is int
          ? rawYear
          : int.tryParse(rawYear?.toString() ?? '');
      await _favoritesDao.add(
        mediaId: mediaId,
        providerId: providerId,
        title: title,
        posterUrl: (data['posterUrl'] ?? data['poster_url'])?.toString(),
        year: year,
        mediaType:
            (data['mediaType'] ?? data['media_type'])?.toString() ?? 'unknown',
      );
    } catch (e) {
      Logger.w('Failed to import favorite $mediaId: $e', tag: _tag);
    }
  }

  Future<void> _importHistory(Map<String, dynamic> data) async {
    final mediaId = (data['mediaId'] ?? data['media_id'])?.toString();
    final providerId = (data['providerId'] ?? data['provider_id'])?.toString();
    final title = data['title']?.toString();
    if (mediaId == null || providerId == null || title == null) {
      Logger.w(
        'Skipping invalid history backup record: missing required fields',
        tag: _tag,
      );
      return;
    }

    try {
      final rawYear = data['year'];
      final year = rawYear is int
          ? rawYear
          : int.tryParse(rawYear?.toString() ?? '');
      final rawPos = data['positionMs'] ?? data['position_ms'];
      final positionMs = rawPos is int
          ? rawPos
          : (int.tryParse(rawPos?.toString() ?? '') ?? 0);
      final rawDur = data['durationMs'] ?? data['duration_ms'];
      final durationMs = rawDur is int
          ? rawDur
          : (int.tryParse(rawDur?.toString() ?? '') ?? 0);
      final rawSeason = data['season'];
      final season = rawSeason is int
          ? rawSeason
          : int.tryParse(rawSeason?.toString() ?? '');
      final rawEpisode = data['episode'];
      final episode = rawEpisode is int
          ? rawEpisode
          : int.tryParse(rawEpisode?.toString() ?? '');

      await _historyDao.saveProgress(
        mediaId: mediaId,
        providerId: providerId,
        title: title,
        posterUrl: (data['posterUrl'] ?? data['poster_url'])?.toString(),
        year: year,
        mediaType:
            (data['mediaType'] ?? data['media_type'])?.toString() ?? 'movie',
        positionMs: positionMs,
        durationMs: durationMs,
        season: season,
        episode: episode,
        episodeTitle: (data['episodeTitle'] ?? data['episode_title'])
            ?.toString(),
        lastStreamUrl: (data['lastStreamUrl'] ?? data['last_stream_url'])
            ?.toString(),
        voiceover: data['voiceover']?.toString(),
      );
    } catch (e) {
      Logger.w('Failed to import history $mediaId: $e', tag: _tag);
    }
  }

  /// Get export data statistics
  Future<Map<String, int>> getDataStats() async {
    final favorites = await _favoritesDao.count();
    final history = await _historyDao.count();

    return {'favorites': favorites, 'history': history};
  }
}
