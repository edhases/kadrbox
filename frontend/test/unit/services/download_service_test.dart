import 'dart:convert';
import 'dart:io' as io;
import 'package:flutter_test/flutter_test.dart';
import 'package:dio/dio.dart';

import 'package:kadrbox/core/network/api_client.dart';
import 'package:kadrbox/data/database/app_database.dart';
import 'package:kadrbox/data/services/download_service.dart';
import 'package:kadrbox/data/services/settings_service.dart';
import 'package:kadrbox/domain/entities/entities.dart';

import 'package:wakelock_plus_platform_interface/wakelock_plus_platform_interface.dart';

import '../../helpers/in_memory_db.dart';

class _AllowAllHttpOverrides extends io.HttpOverrides {
  @override
  io.HttpClient createHttpClient(io.SecurityContext? context) {
    return super.createHttpClient(context);
  }
}

class _FakeWakelockPlusPlatform extends WakelockPlusPlatformInterface {
  bool _enabled = false;
  @override
  Future<bool> get enabled async => _enabled;
  @override
  Future<void> toggle({required bool enable}) async {
    _enabled = enable;
  }
}

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();
  io.HttpOverrides.global = _AllowAllHttpOverrides();
  WakelockPlusPlatformInterface.instance = _FakeWakelockPlusPlatform();

  late AppDatabase db;
  late SettingsService settingsService;
  late ApiClient apiClient;
  late DownloadService downloadService;
  late io.HttpServer server;
  late io.Directory tempDir;

  setUp(() async {
    db = createTestAppDatabase();
    settingsService = SettingsService(db);
    // Allow settings to initialize
    await Future.delayed(const Duration(milliseconds: 30));

    tempDir = io.Directory.systemTemp.createTempSync('download_test_');
    await settingsService.setDownloadPath(tempDir.path);
    // Disable wifi-only requirement for tests
    await settingsService.setOnlyWifiDownload(false);

    // Setup local HTTP server for poster and video streaming
    server = await io.HttpServer.bind(io.InternetAddress.loopbackIPv4, 0);
    server.listen((io.HttpRequest req) async {
      if (req.uri.path == '/poster.jpg') {
        req.response.headers.contentType = io.ContentType('image', 'jpeg');
        req.response.add([
          0xFF,
          0xD8,
          0xFF,
          0xE0,
          0x00,
          0x10,
        ]); // JPEG magic bytes
        await req.response.close();
      } else if (req.uri.path == '/video.mp4') {
        req.response.headers.contentType = io.ContentType('video', 'mp4');
        final bytes = utf8.encode('DUMMY_MP4_VIDEO_STREAMING_CONTENT_BYTES');
        req.response.headers.set('content-length', bytes.length.toString());
        req.response.add(bytes);
        await req.response.close();
      } else {
        req.response.statusCode = io.HttpStatus.notFound;
        await req.response.close();
      }
    });

    final dio = Dio(
      BaseOptions(
        connectTimeout: const Duration(seconds: 5),
        receiveTimeout: const Duration(seconds: 5),
      ),
    );
    apiClient = ApiClient(dio: dio);

    downloadService = DownloadService(db, apiClient, settingsService);
    await Future.delayed(const Duration(milliseconds: 50));
  });

  tearDown(() async {
    downloadService.dispose();
    settingsService.dispose();
    await db.close();
    await server.close(force: true);
    if (tempDir.existsSync()) {
      tempDir.deleteSync(recursive: true);
    }
  });

  group('DownloadService State & Validation', () {
    test(
      'initial state is empty and canDownload is true on desktop/mobile',
      () {
        expect(downloadService.downloads, isEmpty);
        expect(downloadService.completed, isEmpty);
        expect(downloadService.active, isEmpty);
        expect(downloadService.canDownload, isTrue);
        expect(downloadService.isLoading, isFalse);
        expect(downloadService.lastError, isNull);
      },
    );

    test('downloadContent rejects HLS streams with Ukrainian error', () async {
      final item = MediaItem(
        id: 'hls_1',
        providerId: 'src_a',
        title: 'HLS Movie',
        type: ContentType.movie,
      );
      final source = StreamSource(
        url: 'https://example.com/playlist.m3u8',
        quality: StreamQuality.q1080p,
        type: StreamType.hls,
      );

      final success = await downloadService.downloadContent(
        item: item,
        source: source,
      );
      expect(success, isFalse);
      expect(downloadService.lastError, contains('m3u8'));
      expect(downloadService.downloads, isEmpty);
    });

    test('downloadContent rejects invalid URL schemes', () async {
      final item = MediaItem(
        id: 'bad_url',
        providerId: 'src_a',
        title: 'Bad URL Movie',
        type: ContentType.movie,
      );
      final source = StreamSource(
        url: 'ftp://not-supported.com/video.mp4',
        quality: StreamQuality.q720p,
      );

      final success = await downloadService.downloadContent(
        item: item,
        source: source,
      );
      expect(success, isFalse);
      expect(downloadService.lastError, contains('Некоректне посилання'));
    });

    test('formatSize correctly formats bytes', () {
      expect(downloadService.formatSize(500), '500 B');
      expect(downloadService.formatSize(1024 * 1024 * 5), '5.0 MB');
      expect(downloadService.formatSize(1024 * 1024 * 1024 * 2), '2.00 GB');
    });
  });

  group('DownloadService Full Download Lifecycle', () {
    test(
      'downloads content, saves file, caches poster and marks completed',
      () async {
        final item = MediaItem(
          id: 'movie_42',
          providerId: 'src_a',
          title: 'Тестовий Фільм',
          posterUrl: 'http://127.0.0.1:${server.port}/poster.jpg',
          year: 2024,
          type: ContentType.movie,
        );
        final source = StreamSource(
          url: 'http://127.0.0.1:${server.port}/video.mp4',
          quality: StreamQuality.q1080p,
          voiceover: 'Оригінал',
        );

        final started = await downloadService.downloadContent(
          item: item,
          source: source,
          duration: 3600,
        );
        expect(started, isTrue);

        // Wait for download stream and DB updates to finish
        await Future.delayed(const Duration(milliseconds: 300));

        final downloads = downloadService.downloads;
        expect(downloads.length, 1);
        final download = downloads.first;
        expect(download.title, 'Тестовий Фільм');
        expect(download.status, DownloadStatus.completed);
        expect(download.progress, 1.0);
        expect(download.quality, StreamQuality.q1080p.displayName);
        expect(download.voiceover, 'Оригінал');

        // Verify file exists on disk
        final videoFile = io.File(download.localPath);
        expect(videoFile.existsSync(), isTrue);
        expect(
          videoFile.readAsStringSync(),
          'DUMMY_MP4_VIDEO_STREAMING_CONTENT_BYTES',
        );

        // Verify query getters
        expect(
          downloadService.isAvailableOffline('movie_42', 'src_a'),
          isTrue,
        );
        expect(
          downloadService.getLocalPath('movie_42', 'src_a'),
          download.localPath,
        );
        expect(
          downloadService.getStatus('movie_42', 'src_a'),
          DownloadStatus.completed,
        );
        expect(downloadService.completed.length, 1);

        final totalSize = await downloadService.getTotalSize();
        expect(totalSize, greaterThan(0));
      },
    );

    test(
      'prevents duplicate downloads for already downloading or completed media',
      () async {
        final item = MediaItem(
          id: 'movie_dup',
          providerId: 'src_a',
          title: 'Dup Movie',
          type: ContentType.movie,
        );
        final source = StreamSource(
          url: 'http://127.0.0.1:${server.port}/video.mp4',
          quality: StreamQuality.q720p,
        );

        final first = await downloadService.downloadContent(
          item: item,
          source: source,
        );
        expect(first, isTrue);
        await Future.delayed(const Duration(milliseconds: 200));

        // Attempt downloading same media again
        final second = await downloadService.downloadContent(
          item: item,
          source: source,
        );
        expect(second, isFalse);
        expect(
          downloadService.lastError,
          contains('Вже завантажується або завантажено'),
        );
      },
    );

    test('deleteDownload removes file from disk and record from DB', () async {
      final item = MediaItem(
        id: 'movie_to_delete',
        providerId: 'src_a',
        title: 'Delete Me',
        type: ContentType.movie,
      );
      final source = StreamSource(
        url: 'http://127.0.0.1:${server.port}/video.mp4',
        quality: StreamQuality.q720p,
      );

      await downloadService.downloadContent(item: item, source: source);
      await Future.delayed(const Duration(milliseconds: 200));

      expect(downloadService.downloads.length, 1);
      final id = downloadService.downloads.first.id;
      final path = downloadService.downloads.first.localPath;
      expect(io.File(path).existsSync(), isTrue);

      await downloadService.deleteDownload(id);
      await Future.delayed(const Duration(milliseconds: 100));

      expect(downloadService.downloads, isEmpty);
      expect(io.File(path).existsSync(), isFalse);
    });

    test('clearAll cleans up all downloads and posters', () async {
      final item = MediaItem(
        id: 'movie_clear',
        providerId: 'src_a',
        title: 'Clear Me',
        posterUrl: 'http://127.0.0.1:${server.port}/poster.jpg',
        type: ContentType.movie,
      );
      final source = StreamSource(
        url: 'http://127.0.0.1:${server.port}/video.mp4',
        quality: StreamQuality.q720p,
      );

      await downloadService.downloadContent(item: item, source: source);
      await Future.delayed(const Duration(milliseconds: 200));

      await downloadService.clearAll();
      await Future.delayed(const Duration(milliseconds: 100));

      expect(downloadService.downloads, isEmpty);
      expect(await downloadService.getTotalSize(), 0);
    });
  });
}
