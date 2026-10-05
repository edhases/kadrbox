import 'dart:convert';
import 'dart:io';
import 'dart:isolate';

import 'package:file_picker/file_picker.dart';
import 'package:flutter/foundation.dart';
import 'package:intl/intl.dart';
import 'package:path_provider/path_provider.dart';
import 'package:share_plus/share_plus.dart';

import '../../core/utils/logger.dart';
import '../../domain/entities/entities.dart';
import '../database/app_database.dart';
import 'favorites_service.dart';
import 'history_service.dart';

/// Service for exporting and importing user data
class DataTransferService {
  final HistoryService _historyService;
  final FavoritesService _favoritesService;

  DataTransferService(this._historyService, this._favoritesService);

  /// Export data to JSON file
  Future<void> exportData() async {
    final file = await _writeBackupFile();

    if (!kIsWeb &&
        (Platform.isWindows || Platform.isLinux || Platform.isMacOS)) {
      // Desktop: Save dialog
      final String? outputFile = await FilePicker.platform.saveFile(
        dialogTitle: 'Зберегти резервну копію',
        fileName: file.path.split(Platform.pathSeparator).last,
        allowedExtensions: ['json'],
        type: FileType.custom,
      );

      if (outputFile != null) {
        await file.copy(outputFile);
        Logger.i('Backup saved to $outputFile', tag: 'DataTransfer');
      }
    } else {
      // Mobile: Share sheet
      await Share.shareXFiles([XFile(file.path)], text: 'Oxide Backup');
    }
  }

  /// Share backup file via system share sheet (mobile) or save dialog (desktop).
  /// Unlike [exportData], never silently saves: always returns the file path.
  Future<String> shareBackup() async {
    final file = await _writeBackupFile();

    if (!kIsWeb &&
        (Platform.isWindows || Platform.isLinux || Platform.isMacOS)) {
      final String? outputFile = await FilePicker.platform.saveFile(
        dialogTitle: 'Зберегти резервну копію',
        fileName: file.path.split(Platform.pathSeparator).last,
        allowedExtensions: ['json'],
        type: FileType.custom,
      );
      if (outputFile != null) {
        await file.copy(outputFile);
        Logger.i('Backup saved to $outputFile', tag: 'DataTransfer');
        return outputFile;
      }
      throw Exception('Збереження скасовано');
    }

    await Share.shareXFiles([XFile(file.path)], text: 'Oxide Backup');
    return file.path;
  }

  /// Build the backup JSON and write it to a temp file.
  Future<File> _writeBackupFile() async {
    try {
      final history = _historyService.history;
      final favorites = _favoritesService.favorites;

      final data = {
        'version': 1,
        'timestamp': DateTime.now().toIso8601String(),
        'history': history.map((e) => _historyToJson(e)).toList(),
        'favorites': favorites.map((e) => _favoriteToJson(e)).toList(),
      };

      final jsonString = jsonEncode(data);
      final fileName =
          'oxide_backup_${DateFormat('yyyyMMdd_HHmm').format(DateTime.now())}.json';

      final tempDir = await getTemporaryDirectory();
      final file = File('${tempDir.path}/$fileName');
      await file.writeAsString(jsonString);
      return file;
    } catch (e, stack) {
      Logger.e(
        'Export failed',
        error: e,
        stackTrace: stack,
        tag: 'DataTransfer',
      );
      rethrow;
    }
  }

  /// Import data from JSON file
  Future<void> importData() async {
    try {
      final result = await FilePicker.platform.pickFiles(
        type: FileType.custom,
        allowedExtensions: ['json'],
      );

      if (result != null && result.files.single.path != null) {
        final file = File(result.files.single.path!);
        final jsonString = await file.readAsString();
        final data = await Isolate.run(
          () => jsonDecode(jsonString) as Map<String, dynamic>,
        );

        await _restoreData(data);
      }
    } catch (e, stack) {
      Logger.e(
        'Import failed',
        error: e,
        stackTrace: stack,
        tag: 'DataTransfer',
      );
      rethrow;
    }
  }

  Future<void> _restoreData(Map<String, dynamic> data) async {
    final historyList = (data['history'] as List?) ?? [];
    final favoritesList = (data['favorites'] as List?) ?? [];

    Logger.i(
      'Restoring ${historyList.length} history items and ${favoritesList.length} favorites',
      tag: 'DataTransfer',
    );

    // Restore History
    for (final item in historyList) {
      try {
        await _historyService.saveProgress(
          mediaId: item['mediaId'],
          providerId: item['providerId'],
          title: item['title'],
          posterUrl: item['posterUrl'],
          year: item['year'],
          mediaType: item['mediaType'],
          position: Duration(milliseconds: item['positionMs'] ?? 0),
          duration: Duration(milliseconds: item['durationMs'] ?? 0),
          season: item['season'],
          episode: item['episode'],
          episodeTitle: item['episodeTitle'],
          lastStreamUrl: item['lastStreamUrl'],
          voiceover: item['voiceover'],
        );
      } catch (e) {
        Logger.w(
          'Failed to restore history item: ${item['title']}',
          error: e,
          tag: 'DataTransfer',
        );
      }
    }

    // Restore Favorites
    for (final item in favoritesList) {
      try {
        final mediaItem = MediaItem(
          id: item['mediaId'],
          providerId: item['providerId'],
          title: item['title'],
          posterUrl: item['posterUrl'],
          year: item['year'],
          type: ContentType.values.firstWhere(
            (e) => e.name == item['mediaType'],
            orElse: () => ContentType.movie,
          ),
          rating: item['rating'],
        );
        await _favoritesService.add(mediaItem);
      } catch (e) {
        Logger.w(
          'Failed to restore favorite item: ${item['title']}',
          error: e,
          tag: 'DataTransfer',
        );
      }
    }
  }

  // Helpers for serialization since Drift classes might not have toJson customizable enough
  Map<String, dynamic> _historyToJson(WatchHistoryData item) {
    return {
      'mediaId': item.mediaId,
      'providerId': item.providerId,
      'title': item.title,
      'posterUrl': item.posterUrl,
      'year': item.year,
      'mediaType': item.mediaType,
      'positionMs': item.positionMs,
      'durationMs': item.durationMs,
      'season': item.season,
      'episode': item.episode,
      'episodeTitle': item.episodeTitle,
      'lastStreamUrl': item.lastStreamUrl,
      'voiceover': item.voiceover,
      'watchedAt': item.watchedAt.toIso8601String(),
    };
  }

  Map<String, dynamic> _favoriteToJson(Favorite item) {
    return {
      'mediaId': item.mediaId,
      'providerId': item.providerId,
      'title': item.title,
      'posterUrl': item.posterUrl,
      'year': item.year,
      'rating': item.rating,
      'mediaType': item.mediaType,
      'addedAt': item.addedAt.toIso8601String(),
    };
  }
}
