import 'dart:convert';
import 'dart:io';
import 'package:crypto/crypto.dart';
import 'package:dio/dio.dart';
import 'package:injectable/injectable.dart';
import 'package:open_filex/open_filex.dart';
import 'package:path_provider/path_provider.dart';
import 'package:permission_handler/permission_handler.dart';

import '../../core/utils/logger.dart';
import '../../domain/entities/entities.dart';
import '../../core/services/version_service.dart';
import 'settings_service.dart';

/// Result of an update check
enum UpdateCheckResult { upToDate, updateAvailable, forcedUpdate, error }

/// Service for handling OTA updates via GitHub Releases
@lazySingleton
class UpdateService {
  final SettingsService _settingsService;
  final Dio _dio = Dio(
    BaseOptions(
      connectTimeout: const Duration(seconds: 15),
      sendTimeout: const Duration(seconds: 15),
      receiveTimeout: const Duration(seconds: 30),
    ),
  );
  static const String _tag = 'UpdateService';

  // URL to update.json on GitHub
  static const String _updateJsonUrl =
      'https://raw.githubusercontent.com/edhases/oxide_film/master/update.json';

  UpdateService(this._settingsService);

  /// Get current app version code
  Future<int> getCurrentVersionCode() async {
    final versionCode = VersionService.versionCode;
    Logger.d(
      'Current app: ${VersionService.versionName} (code: $versionCode)',
      tag: _tag,
    );
    return versionCode;
  }

  /// Get current app version name
  Future<String> getCurrentVersionName() async {
    return VersionService.versionName;
  }

  /// Check for available updates
  /// [force] - if true, always check even if auto-update is disabled
  /// Returns UpdateInfo if update available, null if up to date or error
  Future<(UpdateCheckResult, UpdateInfo?)> checkForUpdate({
    bool force = false,
  }) async {
    try {
      // Check if auto-update notifications are enabled (unless forced)
      if (!force && !_settingsService.state.updateNotify) {
        return (UpdateCheckResult.upToDate, null);
      }

      // Fetch update.json from GitHub (add timestamp to bust cache)
      final url = '$_updateJsonUrl?t=${DateTime.now().millisecondsSinceEpoch}';

      final response = await _dio.get(
        url,
        options: Options(
          responseType: ResponseType.json,
          receiveTimeout: const Duration(seconds: 10),
          sendTimeout: const Duration(seconds: 10),
        ),
      );

      if (response.statusCode != 200) {
        Logger.w(
          'Failed to fetch update.json: ${response.statusCode}',
          tag: _tag,
        );
        return (UpdateCheckResult.error, null);
      }

      Object data = response.data;
      if (data is String) {
        data = jsonDecode(data);
      }

      final updateInfo = UpdateInfo.fromJson(data as Map<String, dynamic>);
      final platformInfo = updateInfo.forCurrentPlatform;

      if (platformInfo == null) {
        Logger.d('No update info for current platform', tag: _tag);
        return (UpdateCheckResult.upToDate, null);
      }

      final currentVersionCode = await getCurrentVersionCode();

      Logger.i(
        'Current: $currentVersionCode, Remote: ${platformInfo.versionCode}',
        tag: _tag,
      );

      // Check if forced update is required
      if (platformInfo.isForcedUpdate(currentVersionCode)) {
        Logger.w('Forced update required', tag: _tag);
        return (UpdateCheckResult.forcedUpdate, updateInfo);
      }

      // Check if newer version available
      if (platformInfo.isNewerThan(currentVersionCode)) {
        Logger.i('Update available: ${platformInfo.versionName}', tag: _tag);
        return (UpdateCheckResult.updateAvailable, updateInfo);
      }

      Logger.d('App is up to date', tag: _tag);
      return (UpdateCheckResult.upToDate, null);
    } on DioException catch (e) {
      if (e.response?.statusCode == 404) {
        Logger.d('No update.json found (404), assuming up to date', tag: _tag);
        return (UpdateCheckResult.upToDate, null);
      }
      Logger.e('Error checking for updates', tag: _tag, error: e);
      return (UpdateCheckResult.error, null);
    } catch (e) {
      Logger.e('Error checking for updates', tag: _tag, error: e);
      return (UpdateCheckResult.error, null);
    }
  }

  /// Download update file (APK for Android, EXE for Windows)
  /// Returns the downloaded file path, or null if failed
  Future<File?> downloadUpdate(
    UpdateInfo updateInfo, {
    void Function(int received, int total)? onProgress,
  }) async {
    try {
      final platformInfo = updateInfo.forCurrentPlatform;
      if (platformInfo == null) {
        throw UnsupportedError('Platform not supported for OTA updates');
      }

      final downloadUrl = platformInfo.url;
      final expectedHash = platformInfo.sha256;
      final extension = Platform.isAndroid ? 'apk' : 'exe';

      // Clean up old updates before downloading
      await cleanupOldUpdates(excludeVersionCode: platformInfo.versionCode);

      final tempDir = await getTemporaryDirectory();
      final fileName = 'oxide_update_${platformInfo.versionCode}.$extension';
      final filePath = '${tempDir.path}/$fileName';
      final file = File(filePath);

      // Check if file already exists and verify checksum
      if (await file.exists() && expectedHash.isNotEmpty) {
        Logger.d('File already exists, verifying checksum...', tag: _tag);
        final bytes = await file.readAsBytes();
        final digest = sha256.convert(bytes);
        final computedHash = digest.toString().toLowerCase();

        if (computedHash == expectedHash.toLowerCase()) {
          Logger.i('Existing file verified, skipping download', tag: _tag);
          onProgress?.call(bytes.length, bytes.length);
          return file;
        } else {
          Logger.w('Existing file corrupted, re-downloading...', tag: _tag);
          await file.delete();
        }
      }

      // Download file
      await _dio.download(
        downloadUrl,
        filePath,
        onReceiveProgress: onProgress,
        options: Options(receiveTimeout: const Duration(minutes: 10)),
      );

      // Verify SHA-256 if provided
      if (expectedHash.isNotEmpty) {
        final bytes = await file.readAsBytes();
        final digest = sha256.convert(bytes);
        final computedHash = digest.toString().toLowerCase();

        if (computedHash != expectedHash.toLowerCase()) {
          Logger.e(
            'SHA-256 mismatch! Expected: $expectedHash, Computed: $computedHash',
            tag: _tag,
          );
          await file.delete();
          return null;
        }
        Logger.i('SHA-256 verified successfully', tag: _tag);
      }

      return file;
    } catch (e) {
      Logger.e('Error downloading update', tag: _tag, error: e);
      return null;
    }
  }

  /// Install the downloaded update
  /// Returns true if the operation was successful
  Future<bool> installUpdate(File updateFile) async {
    try {
      if (!await updateFile.exists()) {
        Logger.w('Update file not found: ${updateFile.path}', tag: _tag);
        return false;
      }

      if (Platform.isAndroid) {
        // Check for install permission on Android 8.0+
        final status = await Permission.requestInstallPackages.status;
        if (!status.isGranted) {
          Logger.i('Requesting install packages permission...', tag: _tag);
          final result = await Permission.requestInstallPackages.request();
          if (!result.isGranted) {
            Logger.w('Install permission denied', tag: _tag);
            return false;
          }
        }
      }

      Logger.i(
        'Opening update for installation: ${updateFile.path}',
        tag: _tag,
      );

      // open_filex handles APK installation on Android and Opening files on Windows
      final result = await OpenFilex.open(
        updateFile.path,
        type: Platform.isAndroid
            ? 'application/vnd.android.package-archive'
            : null,
      );

      Logger.d(
        'Install result: ${result.type}, message: ${result.message}',
        tag: _tag,
      );

      return result.type == ResultType.done;
    } catch (e) {
      Logger.e('Error installing update', tag: _tag, error: e);
      return false;
    }
  }

  /// Clean up old downloaded updates
  Future<void> cleanupOldUpdates({int? excludeVersionCode}) async {
    try {
      final tempDir = await getTemporaryDirectory();
      final dir = Directory(tempDir.path);

      await for (final entity in dir.list()) {
        if (entity is File) {
          final isUpdateFile = entity.path.contains('oxide_update_');
          final matchesExclude =
              excludeVersionCode != null &&
              entity.path.contains('oxide_update_$excludeVersionCode');

          if (isUpdateFile && !matchesExclude) {
            Logger.d('Deleting old update file: ${entity.path}', tag: _tag);
            await entity.delete();
          }
        }
      }
    } catch (e) {
      Logger.w('Error cleaning up old updates: $e', tag: _tag);
    }
  }
}
