import 'dart:async';
import 'dart:convert';
import 'dart:io';

import 'package:dio/dio.dart';
import 'package:flutter/foundation.dart';
import 'package:path_provider/path_provider.dart';
import 'package:semaphore/semaphore.dart';

import 'package:connectivity_plus/connectivity_plus.dart';
import 'package:wakelock_plus/wakelock_plus.dart';
import '../../core/network/api_client.dart';
import '../../core/utils/logger.dart';
import '../database/app_database.dart';
import '../database/dao/downloads_dao.dart';
import '../../domain/entities/entities.dart';
import '../../core/utils/translit_utils.dart';
import 'settings_service.dart';

/// Download task info
class DownloadTask {
  final Download download;
  final CancelToken cancelToken;
  final Completer<void>? completer;

  DownloadTask({
    required this.download,
    required this.cancelToken,
    this.completer,
  });
}

/// Service for managing offline downloads
class DownloadService extends ChangeNotifier {
  static const String _tag = 'DownloadService';

  final DownloadsDao _dao;
  final ApiClient _apiClient;
  final SettingsService _settings;
  final Map<int, DownloadTask> _activeTasks = {};

  List<Download> _downloads = [];
  bool _isLoading = false;
  String? _lastError;

  // Limit concurrent downloads to 3
  final _downloadSemaphore = LocalSemaphore(3);

  DownloadService(AppDatabase database, this._apiClient, this._settings)
    : _dao = DownloadsDao(database) {
    _loadDownloads();
    _listenToDownloads();
  }

  // Getters
  List<Download> get downloads => _downloads;
  List<Download> get completed =>
      _downloads.where((d) => d.status == DownloadStatus.completed).toList();
  List<Download> get active => _downloads
      .where(
        (d) =>
            d.status == DownloadStatus.pending ||
            d.status == DownloadStatus.downloading ||
            d.status == DownloadStatus.paused,
      )
      .toList();
  bool get isLoading => _isLoading;
  String? get lastError => _lastError;

  /// Check if we can download (mobile/desktop only, no web)
  bool get canDownload => !kIsWeb;

  Future<void> _loadDownloads() async {
    _isLoading = true;
    notifyListeners();

    try {
      _downloads = await _dao.getAll();
      _isLoading = false;
      notifyListeners();
    } catch (e) {
      _lastError = e.toString();
      _isLoading = false;
      notifyListeners();
    }
  }

  StreamSubscription<List<Download>>? _downloadsSub;
  Timer? _initTimer;
  bool _isDisposed = false;

  void _listenToDownloads() {
    _downloadsSub = _dao.watchAll().listen((downloads) {
      if (_isDisposed) return;
      _downloads = downloads;
      notifyListeners();
    });
    _initTimer = Timer(const Duration(seconds: 2), () {
      if (!_isDisposed) _init();
    });
  }

  void _init() async {
    if (_isDisposed) return;
    try {
      final downloads = await _dao.getAll();
      if (_isDisposed) return;
      final interrupted = downloads.where(
        (d) =>
            d.status == DownloadStatus.downloading ||
            d.status == DownloadStatus.pending,
      );

      for (final download in interrupted) {
        resumeDownload(download.id);
      }
    } catch (e) {
      Logger.w('Failed to auto-resume downloads: $e', tag: _tag);
    }
  }

  @override
  void dispose() {
    _isDisposed = true;
    _initTimer?.cancel();
    _downloadsSub?.cancel();
    super.dispose();
  }

  /// Update active tasks and wakelock
  void _updateActiveTasks(int id, DownloadTask? task) {
    if (task == null) {
      _activeTasks.remove(id);
    } else {
      _activeTasks[id] = task;
    }

    // Toggle wakelock based on active downloads
    if (_activeTasks.isNotEmpty) {
      WakelockPlus.enable();
    } else {
      WakelockPlus.disable();
    }
  }

  /// Add to download queue
  Future<Directory> _getDownloadsDir() async {
    final String customPath = _settings.state.downloadPath;

    Directory downloadsDir;
    if (customPath.isNotEmpty) {
      downloadsDir = Directory(customPath);
    } else {
      final appDir = await getApplicationDocumentsDirectory();
      downloadsDir = Directory('${appDir.path}/downloads');
    }

    if (!await downloadsDir.exists()) {
      await downloadsDir.create(recursive: true);
    }
    return downloadsDir;
  }

  /// Generate local file path for download
  Future<String> _generateLocalPath(
    MediaItem item, {
    int? season,
    int? episode,
    String? extension,
  }) async {
    final dir = await _getDownloadsDir();
    final translitTitle = TranslitUtils.translit(item.title);

    String filename = translitTitle;
    if (season != null && episode != null) {
      filename += '_s${season}e$episode';
    }
    filename += '.${extension ?? "mp4"}';

    return '${dir.path}/$filename';
  }

  final Map<int, String> _speeds = {};
  final Map<int, String> _remaining = {};
  final Map<int, double> _rawSpeeds = {}; // Bytes per second

  // ... (existing code)

  /// Get download speed for active task
  String getDownloadSpeed(int id) => _speeds[id] ?? '';

  /// Get remaining time for active task
  String getRemainingTime(int id) => _remaining[id] ?? '';

  /// Get local poster path if available
  String? getLocalPosterPath(String mediaId, String providerId) {
    try {
      final download = _downloads.firstWhere(
        (d) => d.mediaId == mediaId && d.providerId == providerId,
      );
      if (download.localPosterPath != null &&
          File(download.localPosterPath!).existsSync()) {
        return download.localPosterPath;
      }
    } catch (_) {}
    return null;
  }

  /// Cache poster image locally
  Future<String?> _cachePoster(String? url, String mediaId) async {
    if (url == null || url.isEmpty) return null;

    try {
      final dir = await _getDownloadsDir();
      final postersDir = Directory('${dir.path}/posters');
      if (!await postersDir.exists()) {
        await postersDir.create(recursive: true);
      }

      final extension = url.split('.').last.split('?').first;
      // Sanitize extension
      final safeExt =
          extension.length > 4 ||
              !['jpg', 'jpeg', 'png', 'webp'].contains(extension)
          ? 'jpg'
          : extension;

      final filename = 'poster_$mediaId.$safeExt';
      final file = File('${postersDir.path}/$filename');

      if (await file.exists()) {
        return file.path;
      }

      final response = await _apiClient.dio.get<List<int>>(
        url,
        options: Options(responseType: ResponseType.bytes),
      );

      await file.writeAsBytes(response.data!);
      return file.path;
    } catch (e) {
      Logger.w('Failed to cache poster for $mediaId', tag: _tag, error: e);
      return null;
    }
  }

  /// Start downloading media
  Future<bool> downloadContent({
    required MediaItem item,
    required StreamSource source,
    int? season,
    int? episode,
    String? episodeTitle,
    int? duration,
  }) async {
    if (!canDownload) {
      _lastError = 'Завантаження недоступне на цій платформі';
      return false;
    }

    // Check for HLS
    if (source.url.toLowerCase().contains('.m3u8') ||
        source.type == StreamType.hls) {
      _lastError = 'Завантаження HLS-потоків (.m3u8) поки не підтримується';
      return false;
    }

    // Validate URL scheme (must be http or https)
    final uri = Uri.tryParse(source.url);
    if (uri == null ||
        !uri.hasScheme ||
        (uri.scheme != 'http' && uri.scheme != 'https')) {
      _lastError = 'Некоректне посилання для завантаження';
      Logger.e('Invalid download URL: ${source.url}', tag: _tag);
      return false;
    }

    try {
      // Check if already downloaded
      final existing = await _dao.getByMediaId(
        item.id,
        item.providerId,
        season: season,
        episode: episode,
      );

      if (existing != null &&
          (existing.status == DownloadStatus.completed ||
              existing.status == DownloadStatus.downloading)) {
        _lastError = 'Вже завантажується або завантажено';
        return false;
      }

      // Generate local path
      final localPath = await _generateLocalPath(
        item,
        season: season,
        episode: episode,
      );

      // Cache poster
      final localPosterPath = await _cachePoster(
        item.posterUrl,
        item.uniqueId.replaceAll(':', '_'),
      );

      // Add to database
      final id = await _dao.add(
        mediaId: item.id,
        providerId: item.providerId,
        title: item.title,
        posterUrl: item.posterUrl,
        year: item.year,
        mediaType: item.type.name,
        season: season,
        episode: episode,
        episodeTitle: episodeTitle,
        streamUrl: source.url,
        localPath: localPath,
        quality: source.quality.displayName,
        voiceover: source.voiceover,
        headers: source.headers,
        localPosterPath: localPosterPath,
        duration: duration,
      );

      // Start actual download
      _startDownloadTask(id, source.url, localPath);

      return true;
    } catch (e) {
      _lastError = e.toString();
      Logger.e('Download failed', tag: _tag, error: e);
      return false;
    }
  }

  /// Start the actual download task using Dio streams for resuming
  void _startDownloadTask(int id, String url, String localPath) async {
    final cancelToken = CancelToken();
    final completer = Completer<void>();

    try {
      // QUEUE: Wait for a slot
      await _downloadSemaphore.acquire();

      await _dao.updateStatus(id, DownloadStatus.downloading);

      // Get download record for task tracking
      final list = await _dao.getAll();
      final download = list.firstWhere((d) => d.id == id);

      _updateActiveTasks(
        id,
        DownloadTask(
          download: download,
          cancelToken: cancelToken,
          completer: completer,
        ),
      );

      // Connectivity check for Wi-Fi only
      if (_settings.state.onlyWifiDownload) {
        final connectivityResult = await Connectivity().checkConnectivity();
        if (!connectivityResult.contains(ConnectivityResult.wifi)) {
          Logger.w('Download deferred: Not on Wi-Fi', tag: _tag);
          await _dao.updateStatus(id, DownloadStatus.paused);
          _updateActiveTasks(id, null);
          _downloadSemaphore.release();
          return;
        }
      }

      // RESUME LOGIC: Check existing file size
      final file = File(localPath);
      int startByte = 0;
      if (await file.exists()) {
        startByte = await file.length();
      }

      // Parse headers if available
      Map<String, dynamic>? requestHeaders;
      if (download.headers != null) {
        try {
          requestHeaders =
              jsonDecode(download.headers!) as Map<String, dynamic>;
        } catch (e) {
          Logger.w('Failed to parse download headers', tag: _tag, error: e);
        }
      }

      // Start download stream
      final response = await _apiClient.dio.get<ResponseBody>(
        url,
        cancelToken: cancelToken,
        options: Options(
          headers: {
            ...?requestHeaders,
            if (startByte > 0) 'Range': 'bytes=$startByte-',
          },
          responseType: ResponseType.stream,
          followRedirects: true,
          receiveTimeout: const Duration(hours: 2),
        ),
      );

      final totalInResponse =
          int.tryParse(response.headers.value('content-length') ?? '') ?? 0;
      final fileTotalSize = startByte + totalInResponse;

      // Open file for appending
      final sink = file.openWrite(mode: FileMode.append);
      int receivedBytes = startByte;

      final progressStopwatch = Stopwatch()..start();
      final speedStopwatch = Stopwatch()..start();
      int bytesSinceLastSpeedCheck = 0;

      try {
        await for (final chunk in response.data!.stream) {
          sink.add(chunk);
          receivedBytes += chunk.length;
          bytesSinceLastSpeedCheck += chunk.length;

          // Update speed every second
          if (speedStopwatch.elapsedMilliseconds > 1000) {
            final speed =
                bytesSinceLastSpeedCheck /
                (speedStopwatch.elapsedMilliseconds / 1000);
            _rawSpeeds[id] = speed;
            _speeds[id] = '${formatSize(speed.toInt())}/s';

            // Calculate remaining time
            if (speed > 0 && fileTotalSize > 0) {
              final remainingBytes = fileTotalSize - receivedBytes;
              final remainingSeconds = remainingBytes / speed;
              _remaining[id] = _formatTimeRemaining(remainingSeconds.toInt());
            } else {
              _remaining[id] = '';
            }

            speedStopwatch.reset();
            bytesSinceLastSpeedCheck = 0;
            notifyListeners();
          }

          // Update progress periodically (every 500ms or so to avoid DB overload)
          if (progressStopwatch.elapsedMilliseconds > 500) {
            final progress = fileTotalSize > 0
                ? receivedBytes / fileTotalSize
                : 0.0;
            await _dao.updateProgress(
              id,
              progress: progress,
              downloadedBytes: receivedBytes,
              fileSizeBytes: fileTotalSize > 0 ? fileTotalSize : null,
            );
            progressStopwatch.reset();
          }
        }

        await sink.flush();
        await sink.close();

        // Final update - IMPORTANT: use receivedBytes, not fileTotalSize which might be 0
        await _dao.updateProgress(
          id,
          progress: 1.0,
          downloadedBytes: receivedBytes,
          fileSizeBytes: receivedBytes,
        );
        await _dao.updateStatus(id, DownloadStatus.completed);
      } catch (e) {
        await sink.close();
        rethrow;
      }

      _speeds.remove(id);
      _rawSpeeds.remove(id);
      _remaining.remove(id);
      _updateActiveTasks(id, null);
      completer.complete();
      Logger.i('Download completed: $localPath', tag: _tag);
    } on DioException catch (e) {
      _speeds.remove(id);
      _updateActiveTasks(id, null);

      if (e.type == DioExceptionType.cancel) {
        Logger.d('Download cancelled/paused: $id', tag: _tag);
        return;
      }

      Logger.e('Download task failed (Dio)', tag: _tag, error: e);
      await _dao.updateStatus(id, DownloadStatus.failed);
      completer.completeError(e);
    } catch (e) {
      _speeds.remove(id);
      _updateActiveTasks(id, null);
      Logger.e('Download task failed', tag: _tag, error: e);
      await _dao.updateStatus(id, DownloadStatus.failed);
      completer.completeError(e);
    } finally {
      _speeds.remove(id);
      _rawSpeeds.remove(id);
      _remaining.remove(id);
      // QUEUE: Release slot
      _downloadSemaphore.release();
    }
  }

  /// Pause download
  Future<void> pauseDownload(int id) async {
    final task = _activeTasks[id];
    if (task != null) {
      task.cancelToken.cancel('Paused by user');
      _activeTasks.remove(id);
    }
    await _dao.updateStatus(id, DownloadStatus.paused);
  }

  /// Resume download
  Future<void> resumeDownload(int id) async {
    final download = _downloads.firstWhere((d) => d.id == id);
    _startDownloadTask(id, download.streamUrl, download.localPath);
  }

  /// Cancel and delete download
  Future<void> cancelDownload(int id) async {
    // Cancel active task
    final task = _activeTasks[id];
    if (task != null) {
      task.cancelToken.cancel('Cancelled by user');
      _activeTasks.remove(id);
    }

    // Delete file if exists
    try {
      final download = _downloads.firstWhere((d) => d.id == id);
      final file = File(download.localPath);
      if (await file.exists()) {
        await file.delete();
      }
    } catch (e) {
      Logger.w('Failed to delete file', tag: _tag);
    }

    // Remove from database
    await _dao.delete(id);
  }

  /// Delete completed download
  Future<void> deleteDownload(int id) async {
    await cancelDownload(id);
  }

  /// Get local file path for offline playback
  String? getLocalPath(
    String mediaId,
    String providerId, {
    int? season,
    int? episode,
  }) {
    try {
      final download = _downloads.firstWhere(
        (d) =>
            d.mediaId == mediaId &&
            d.providerId == providerId &&
            d.season == season &&
            d.episode == episode &&
            d.status == DownloadStatus.completed,
      );
      return download.localPath;
    } catch (_) {
      return null;
    }
  }

  /// Check if media is available offline
  bool isAvailableOffline(
    String mediaId,
    String providerId, {
    int? season,
    int? episode,
  }) {
    return getLocalPath(
          mediaId,
          providerId,
          season: season,
          episode: episode,
        ) !=
        null;
  }

  /// Get download status for media
  DownloadStatus? getStatus(
    String mediaId,
    String providerId, {
    int? season,
    int? episode,
  }) {
    try {
      final download = _downloads.firstWhere(
        (d) =>
            d.mediaId == mediaId &&
            d.providerId == providerId &&
            d.season == season &&
            d.episode == episode,
      );
      return download.status;
    } catch (_) {
      return null;
    }
  }

  /// Get total downloaded size in bytes
  Future<int> getTotalSize() => _dao.getTotalSize();

  /// Format size for display
  String formatSize(int bytes) {
    if (bytes < 1024) return '$bytes B';
    if (bytes < 1024 * 1024) return '${(bytes / 1024).toStringAsFixed(1)} KB';
    if (bytes < 1024 * 1024 * 1024) {
      return '${(bytes / (1024 * 1024)).toStringAsFixed(1)} MB';
    }
    return '${(bytes / (1024 * 1024 * 1024)).toStringAsFixed(2)} GB';
  }

  /// Format time remaining
  String _formatTimeRemaining(int seconds) {
    if (seconds <= 0) return '';
    if (seconds < 60) return '${seconds}s';
    if (seconds < 3600) {
      final m = seconds ~/ 60;
      final s = seconds % 60;
      return '${m}m ${s}s';
    }
    final h = seconds ~/ 3600;
    final m = (seconds % 3600) ~/ 60;
    return '${h}h ${m}m';
  }

  /// Clear all downloads
  Future<void> clearAll() async {
    // Cancel all active downloads
    for (final id in _activeTasks.keys.toList()) {
      await cancelDownload(id);
    }

    // Delete all files
    for (final download in _downloads) {
      try {
        final file = File(download.localPath);
        if (await file.exists()) {
          await file.delete();
        }
      } catch (e) {
        Logger.w('Failed to delete file', tag: _tag);
      }
    }

    // Delete posters directory
    try {
      final dir = await _getDownloadsDir();
      final postersDir = Directory('${dir.path}/posters');
      if (await postersDir.exists()) {
        await postersDir.delete(recursive: true);
      }
    } catch (e) {
      Logger.w('Failed to delete posters directory', tag: _tag);
    }

    // Clear database
    await _dao.clearAll();
  }
}
