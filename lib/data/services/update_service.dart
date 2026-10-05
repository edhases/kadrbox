import 'dart:convert';
import 'dart:io';

import 'package:crypto/crypto.dart';
import 'package:dio/dio.dart';
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

  /// OTA manifest location.
  ///
  /// MUST be an immutable ref (a release tag, or a path pinned to a release).
  /// It used to be `.../master/update.json` and `master` does not exist in this
  /// repository, so every single check 404'd.
  ///
  /// NOTE: this points at `dev` because that is the ref the manifest is
  /// actually committed to. Move it to a release tag when releases are cut.
  ///
  /// RESIDUAL RISK: `raw.githubusercontent.com` serves whatever is committed at
  /// that ref right now. A digest of the manifest pinned in this binary was
  /// tried and removed: it does not add protection, because anyone who can
  /// rewrite the manifest can also rewrite this constant in the next build, and
  /// it makes every manifest edit require an app release — which had already
  /// produced a client that could not check for updates at all. The control
  /// that does hold is the per-artifact `sha256` below, verified against the
  /// downloaded bytes and mandatory. Real protection needs signed root metadata
  /// (TUF) with key rotation.
  static const String _updateJsonUrl =
      'https://raw.githubusercontent.com/edhases/oxide_film/dev/update.json';

  /// Per-platform escape hatch for the download digest check.
  ///
  /// Must stay `false` in release builds. `installUpdate` hands the downloaded
  /// file to the OS installer, so with this set to `true` a compromised
  /// download host is arbitrary code execution. It exists so a platform with no
  /// publishable digest can be developed against locally; it is not a supported
  /// configuration.
  static const bool kAllowUnverifiedDownloads = false;

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

      final response = await _dio.get<List<int>>(
        url,
        options: Options(
          // Fetch raw bytes: the trust anchor is a digest over the exact
          // response body, so it must be hashed before it is parsed.
          responseType: ResponseType.bytes,
          receiveTimeout: const Duration(seconds: 10),
          sendTimeout: const Duration(seconds: 10),
          // The manifest decides what gets executed on the device. Do not let a
          // proxy or a captive portal answer for it.
          headers: const {'Cache-Control': 'no-cache'},
        ),
      );

      if (response.statusCode != 200) {
        Logger.w(
          'Failed to fetch update.json: ${response.statusCode}',
          tag: _tag,
        );
        return (UpdateCheckResult.error, null);
      }

      final body = response.data;
      if (body == null || body.isEmpty) {
        Logger.w('Empty update.json response', tag: _tag);
        return (UpdateCheckResult.error, null);
      }

      Object? data = jsonDecode(utf8.decode(body));
      if (data is! Map<String, dynamic>) {
        Logger.w('update.json is not a JSON object', tag: _tag);
        return (UpdateCheckResult.error, null);
      }

      final updateInfo = UpdateInfo.fromJson(data);
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
      // "Cannot reach the manifest" and "you are current" are different facts.
      // Reporting 404 as upToDate is what let the dead `master` URL masquerade
      // as a healthy updater for the entire life of the feature.
      if (e.response?.statusCode == 404) {
        Logger.e(
          'update.json not found at $_updateJsonUrl. The OTA manifest URL is '
          'wrong or the ref does not exist.',
          tag: _tag,
        );
        return (UpdateCheckResult.error, null);
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
      if (downloadUrl.isEmpty) {
        throw UnsupportedError('No download URL for current platform');
      }
      // The digest is what stands between a hostile mirror and the OS
      // installer. An absent digest used to skip verification silently and
      // install anyway; it is now a hard failure unless the compile-time
      // escape hatch above is explicitly turned on.
      if (expectedHash.isEmpty && !kAllowUnverifiedDownloads) {
        throw StateError(
          'Manifest carries no SHA-256 for this platform; refusing to install '
          'an unverified binary',
        );
      }
      if (!downloadUrl.startsWith('https://')) {
        throw StateError('Refusing to download an update over $downloadUrl');
      }
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
