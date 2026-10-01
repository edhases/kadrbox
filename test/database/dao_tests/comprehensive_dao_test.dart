import 'package:flutter_test/flutter_test.dart';
import 'package:oxide_film/data/database/app_database.dart';
import 'package:oxide_film/data/database/dao/downloads_dao.dart';
import 'package:oxide_film/data/database/dao/favorites_dao.dart';
import 'package:oxide_film/data/database/dao/history_dao.dart';
import 'package:oxide_film/data/database/dao/media_items_dao.dart';
import 'package:oxide_film/data/database/dao/search_history_dao.dart';
import 'package:oxide_film/data/database/dao/settings_dao.dart';
import 'package:oxide_film/domain/entities/media_item.dart';

import '../../helpers/in_memory_db.dart';

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();

  late AppDatabase db;
  late DownloadsDao downloadsDao;
  late FavoritesDao favoritesDao;
  late HistoryDao historyDao;
  late MediaItemsDao mediaItemsDao;
  late SearchHistoryDao searchHistoryDao;
  late SettingsDao settingsDao;

  setUp(() {
    db = createTestAppDatabase();
    downloadsDao = DownloadsDao(db);
    favoritesDao = FavoritesDao(db);
    historyDao = HistoryDao(db);
    mediaItemsDao = MediaItemsDao(db);
    searchHistoryDao = SearchHistoryDao(db);
    settingsDao = SettingsDao(db);
  });

  tearDown(() async {
    await db.close();
  });

  group('DownloadsDao In-Memory DB Tests', () {
    test('add, getByMediaId, isDownloaded, and getAll', () async {
      final id = await downloadsDao.add(
        mediaId: 'm100',
        providerId: 'uakino',
        title: 'Інтерстеллар',
        posterUrl: 'https://cdn/poster.jpg',
        year: 2014,
        mediaType: 'movie',
        streamUrl: 'https://cdn/1080.m3u8',
        localPath: '/storage/100.mp4',
        quality: '1080p',
        voiceover: 'Дубляж',
        headers: {'User-Agent': 'TestUA', 'Referer': 'https://uakino.club/'},
        localPosterPath: '/storage/poster.jpg',
        duration: 10140,
      );

      expect(id, isPositive);

      final dl = await downloadsDao.getByMediaId('m100', 'uakino');
      expect(dl, isNotNull);
      expect(dl!.title, 'Інтерстеллар');
      expect(dl.quality, '1080p');
      expect(dl.duration, 10140);
      expect(dl.headers, contains('TestUA'));

      // Initial status is pending, not completed
      final downloadedBefore = await downloadsDao.isDownloaded('m100', 'uakino');
      expect(downloadedBefore, isFalse);

      final all = await downloadsDao.getAll();
      expect(all.length, 1);
    });

    test('updateStatus and updateProgress', () async {
      final id = await downloadsDao.add(
        mediaId: 's1',
        providerId: 'lavakino',
        title: 'Гра престолів',
        mediaType: 'series',
        season: 1,
        episode: 1,
        episodeTitle: 'Зима близько',
        streamUrl: 'https://cdn/s1e1.m3u8',
        localPath: '/storage/s1e1.mp4',
        quality: '720p',
      );

      // Update progress
      await downloadsDao.updateProgress(
        id,
        progress: 0.5,
        downloadedBytes: 500000,
        fileSizeBytes: 1000000,
      );

      var dl = await downloadsDao.getByMediaId('s1', 'lavakino', season: 1, episode: 1);
      expect(dl!.progress, 0.5);
      expect(dl.downloadedBytes, 500000);
      expect(dl.fileSizeBytes, 1000000);

      // Check active downloads
      var active = await downloadsDao.getActive();
      expect(active.length, 1);

      // Update status to completed
      await downloadsDao.updateStatus(id, DownloadStatus.completed);
      dl = await downloadsDao.getByMediaId('s1', 'lavakino', season: 1, episode: 1);
      expect(dl!.status, DownloadStatus.completed);
      expect(dl.completedAt, isNotNull);

      final isDl = await downloadsDao.isDownloaded('s1', 'lavakino', season: 1, episode: 1);
      expect(isDl, isTrue);

      final completed = await downloadsDao.getCompleted();
      expect(completed.length, 1);

      final totalSize = await downloadsDao.getTotalSize();
      expect(totalSize, 1000000);
    });

    test('delete, deleteByMediaId, and clearAll', () async {
      await downloadsDao.add(
        mediaId: 'd1',
        providerId: 'p1',
        title: 'Title 1',
        mediaType: 'movie',
        streamUrl: 'url1',
        localPath: 'path1',
        quality: '1080p',
      );
      await downloadsDao.add(
        mediaId: 'd2',
        providerId: 'p1',
        title: 'Title 2',
        mediaType: 'movie',
        streamUrl: 'url2',
        localPath: 'path2',
        quality: '1080p',
      );

      final all = await downloadsDao.getAll();
      expect(all.length, 2);

      await downloadsDao.delete(all[0].id);
      expect((await downloadsDao.getAll()).length, 1);

      await downloadsDao.deleteByMediaId('d2', 'p1');
      expect((await downloadsDao.getAll()).isEmpty, isTrue);

      // Re-add and clear all
      await downloadsDao.add(
        mediaId: 'd3',
        providerId: 'p1',
        title: 'Title 3',
        mediaType: 'movie',
        streamUrl: 'url3',
        localPath: 'path3',
        quality: '1080p',
      );
      await downloadsDao.clearAll();
      expect((await downloadsDao.getAll()).isEmpty, isTrue);
    });

    test('watchAll and watchActive streams emit updates', () async {
      final watchFuture = downloadsDao.watchAll().first;
      await downloadsDao.add(
        mediaId: 'w1',
        providerId: 'p1',
        title: 'Watched Item',
        mediaType: 'movie',
        streamUrl: 'u',
        localPath: 'p',
        quality: '720p',
      );
      final emitted = await watchFuture;
      expect(emitted.length, 1);
      expect(emitted.first.mediaId, 'w1');

      final activeStream = downloadsDao.watchActive();
      expect(await activeStream.first, isNotEmpty);
    });
  });

  group('FavoritesDao In-Memory DB Tests', () {
    test('add, get, isFavorite, getByType, and count', () async {
      await favoritesDao.add(
        mediaId: 'fav1',
        providerId: 'uakino',
        title: 'Дюна',
        posterUrl: 'https://cdn/dune.jpg',
        year: 2021,
        rating: 8.2,
        ratingSource: 'IMDb',
        mediaType: 'movie',
      );
      await favoritesDao.add(
        mediaId: 'fav2',
        providerId: 'uakino',
        title: 'Аркейн',
        year: 2021,
        rating: 9.0,
        mediaType: 'series',
      );

      expect(await favoritesDao.count(), 2);
      expect(await favoritesDao.isFavorite('fav1', 'uakino'), isTrue);
      expect(await favoritesDao.isFavorite('nonexistent', 'uakino'), isFalse);

      final fav1 = await favoritesDao.get('fav1', 'uakino');
      expect(fav1, isNotNull);
      expect(fav1!.title, 'Дюна');
      expect(fav1.rating, 8.2);
      expect(fav1.ratingSource, 'IMDb');

      final movies = await favoritesDao.getByType('movie');
      expect(movies.length, 1);
      expect(movies.first.mediaId, 'fav1');

      final series = await favoritesDao.getByType('series');
      expect(series.length, 1);
      expect(series.first.mediaId, 'fav2');
    });

    test('toggle favorites seamlessly', () async {
      // Toggle ON
      final added = await favoritesDao.toggle(
        mediaId: 'tog1',
        providerId: 'lavakino',
        title: 'Оппенгеймер',
        mediaType: 'movie',
      );
      expect(added, isTrue);
      expect(await favoritesDao.isFavorite('tog1', 'lavakino'), isTrue);

      // Toggle OFF
      final removed = await favoritesDao.toggle(
        mediaId: 'tog1',
        providerId: 'lavakino',
        title: 'Оппенгеймер',
        mediaType: 'movie',
      );
      expect(removed, isFalse);
      expect(await favoritesDao.isFavorite('tog1', 'lavakino'), isFalse);
    });

    test('remove and clearAll', () async {
      await favoritesDao.add(
        mediaId: 'r1',
        providerId: 'p1',
        title: 'Test',
        mediaType: 'movie',
      );
      expect(await favoritesDao.count(), 1);

      await favoritesDao.remove('r1', 'p1');
      expect(await favoritesDao.count(), 0);

      await favoritesDao.add(
        mediaId: 'r2',
        providerId: 'p1',
        title: 'Test 2',
        mediaType: 'movie',
      );
      await favoritesDao.clearAll();
      expect(await favoritesDao.count(), 0);
    });

    test('watchAll and watchIsFavorite reactive streams', () async {
      final isFavStream = favoritesDao.watchIsFavorite('react1', 'prov');
      final initial = await isFavStream.first;
      expect(initial, isFalse);

      await favoritesDao.add(
        mediaId: 'react1',
        providerId: 'prov',
        title: 'Reactive Test',
        mediaType: 'movie',
      );

      final updated = await favoritesDao.watchIsFavorite('react1', 'prov').first;
      expect(updated, isTrue);
    });
  });

  group('MediaItemsDao & StoredMediaItems Tests', () {
    test('upsert and get media item cache', () async {
      const item = MediaItem(
        id: 'meta1',
        providerId: 'bandera',
        title: 'Володар Перснів',
        originalTitle: 'The Lord of the Rings',
        posterUrl: 'https://cdn/lotr.jpg',
        year: 2001,
        rating: 8.8,
        type: ContentType.movie,
        description: 'Фентезі епопея',
        genres: ['Фентезі', 'Пригоди'],
        country: 'Нова Зеландія',
      );

      await mediaItemsDao.upsert(item);

      final retrieved = await mediaItemsDao.get('meta1', 'bandera');
      expect(retrieved, isNotNull);
      expect(retrieved!.id, 'meta1');
      expect(retrieved.originalTitle, 'The Lord of the Rings');
      expect(retrieved.genres, contains('Фентезі'));
      expect(retrieved.country, 'Нова Зеландія');
      expect(retrieved.type, ContentType.movie);

      // Non-existent returns null
      expect(await mediaItemsDao.get('404', 'bandera'), isNull);
    });
  });

  group('SearchHistoryDao In-Memory DB Tests', () {
    test('addSearch, getRecent, getSuccessful, and searchByPrefix autocomplete', () async {
      await searchHistoryDao.addSearch(
        query: 'Матриця',
        normalizedQuery: 'матриця',
        resultCount: 5,
      );
      await searchHistoryDao.addSearch(
        query: 'Мандалорець',
        normalizedQuery: 'мандалорець',
        resultCount: 10,
      );
      await searchHistoryDao.addSearch(
        query: 'Неіснуючий фільм',
        normalizedQuery: 'неіснуючий фільм',
        resultCount: 0,
      );

      final all = await searchHistoryDao.getAll();
      expect(all.length, 3);

      final recent = await searchHistoryDao.getRecent(limit: 2);
      expect(recent.length, 2);

      // getSuccessful should exclude resultCount == 0
      final successful = await searchHistoryDao.getSuccessful();
      expect(successful.length, 2);
      expect(successful.any((s) => s.normalizedQuery == 'неіснуючий фільм'), isFalse);

      // Autocomplete by prefix
      final prefixMatches = await searchHistoryDao.searchByPrefix('ма');
      expect(prefixMatches.length, 2);
      final titles = prefixMatches.map((m) => m.query).toList();
      expect(titles, containsAll(['Матриця', 'Мандалорець']));

      // Updating existing search increments count
      await searchHistoryDao.addSearch(
        query: 'Матриця HD',
        normalizedQuery: 'матриця',
        resultCount: 8,
      );

      final updated = await searchHistoryDao.getAll();
      final matrix = updated.firstWhere((s) => s.normalizedQuery == 'матриця');
      expect(matrix.searchCount, 2);
      expect(matrix.resultCount, 8);
      expect(matrix.query, 'Матриця HD');

      // Failed searches
      final failed = await searchHistoryDao.getFailedSearches();
      expect(failed.length, 1);
      expect(failed.first.normalizedQuery, 'неіснуючий фільм');

      // Watch recent stream
      final streamFuture = searchHistoryDao.watchRecent().first;
      expect((await streamFuture).length, 3);

      // Delete and clear
      await searchHistoryDao.deleteById(matrix.id);
      expect((await searchHistoryDao.getAll()).length, 2);

      await searchHistoryDao.clearAll();
      expect((await searchHistoryDao.getAll()).isEmpty, isTrue);
    });
  });

  group('SettingsDao In-Memory DB Tests', () {
    test('getSetting, setSetting, getAllSettings, and watchSetting', () async {
      await settingsDao.setSetting('custom_key', 'custom_value');
      final val = await settingsDao.getSetting('custom_key');
      expect(val, 'custom_value');

      // Update existing setting
      await settingsDao.setSetting('custom_key', 'updated_value');
      expect(await settingsDao.getSetting('custom_key'), 'updated_value');

      final all = await settingsDao.getAllSettings();
      expect(all['custom_key'], 'updated_value');

      // Watch setting
      final streamVal = await settingsDao.watchSetting('custom_key').first;
      expect(streamVal, 'updated_value');
    });

    test('provider settings and priorities', () async {
      await settingsDao.setProviderEnabled('uakino', true);
      await settingsDao.setProviderPriority('uakino', 1);

      await settingsDao.setProviderEnabled('lavakino', false);
      await settingsDao.setProviderPriority('lavakino', 2);

      expect(await settingsDao.isProviderEnabled('uakino'), isTrue);
      expect(await settingsDao.isProviderEnabled('lavakino'), isFalse);
      expect(await settingsDao.isProviderEnabled('unknown'), isTrue); // default

      final providers = await settingsDao.getEnabledProviders();
      expect(providers.length, 2);
      expect(providers[0].providerId, 'uakino');
      expect(providers[0].priority, 1);
      expect(providers[1].providerId, 'lavakino');
      expect(providers[1].priority, 2);

      // Watch provider states map
      final states = await settingsDao.watchProviderStates().first;
      expect(states['uakino'], isTrue);
      expect(states['lavakino'], isFalse);
    });
  });

  group('DownloadStatus Enum and Converter Tests', () {
    test('DownloadStatus conversion', () {
      expect(DownloadStatus.fromValue(0), DownloadStatus.pending);
      expect(DownloadStatus.fromValue(1), DownloadStatus.downloading);
      expect(DownloadStatus.fromValue(2), DownloadStatus.paused);
      expect(DownloadStatus.fromValue(3), DownloadStatus.completed);
      expect(DownloadStatus.fromValue(4), DownloadStatus.failed);
      expect(DownloadStatus.fromValue(999), DownloadStatus.pending); // fallback

      const conv = DownloadStatusConverter();
      expect(conv.toSql(DownloadStatus.completed), 3);
      expect(conv.fromSql(3), DownloadStatus.completed);
    });
  });
}
