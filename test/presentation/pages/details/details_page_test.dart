import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:get_it/get_it.dart';
import 'package:go_router/go_router.dart';
import 'package:dio/dio.dart';
import 'package:shared_preferences/shared_preferences.dart';

import 'package:oxide_film/core/network/api_client.dart';
import 'package:oxide_film/data/database/app_database.dart';
import 'package:oxide_film/data/providers/provider_registry.dart';
import 'package:oxide_film/data/services/auth_service.dart';
import 'package:oxide_film/data/services/download_service.dart';
import 'package:oxide_film/data/services/favorites_service.dart';
import 'package:oxide_film/data/services/oxide_server_service.dart';
import 'package:oxide_film/data/services/settings_service.dart';
import 'package:oxide_film/domain/entities/entities.dart';
import 'package:oxide_film/domain/repositories/content_provider.dart';
import 'package:oxide_film/presentation/pages/details/details_page.dart';

import '../../../helpers/in_memory_db.dart';

class _TestContentProvider extends ContentProvider {
  @override
  final String id = 'test_provider';
  @override
  final String name = 'Test Provider';
  @override
  final String baseUrl = 'https://test.example';
  @override
  final bool isEnabled = true;
  @override
  String? get iconUrl => null;
  @override
  final List<ContentType> supportedTypes = [
    ContentType.movie,
    ContentType.series,
  ];

  bool shouldThrow = false;
  MediaDetails? customDetails;
  List<StreamSource>? customStreams;

  @override
  Future<MediaDetails> getDetails(String id) async {
    if (shouldThrow) throw Exception('Помилка завантаження деталей');
    return customDetails ??
        MediaDetails(
          item: MediaItem(
            id: id,
            providerId: this.id,
            title: 'Інтерстеллар',
            originalTitle: 'Interstellar',
            year: 2014,
            posterUrl: 'https://example.com/poster.jpg',
            type: ContentType.movie,
            rating: 8.7,
            genres: ['Фантастика', 'Драма'],
          ),
          genres: const ['Фантастика', 'Драма'],
          fullDescription:
              'Команда дослідників вирушає крізь червоточину в космосі.',
          countries: ['США', 'Велика Британія'],
          director: 'Крістофер Нолан',
          actors: ['Меттью МакКонахі', 'Енн Гетевей'],
          duration: const Duration(minutes: 169),
        );
  }

  @override
  Future<List<StreamSource>> getStreams(
    String id, {
    int? season,
    int? episode,
  }) async {
    if (shouldThrow) throw Exception('Помилка завантаження стрімів');
    return customStreams ??
        [
          StreamSource(
            url: 'https://example.com/stream_1080.mp4',
            quality: StreamQuality.q1080p,
            voiceover: 'Оригінал (дубляж)',
          ),
          StreamSource(
            url: 'https://example.com/stream_720.mp4',
            quality: StreamQuality.q720p,
            voiceover: 'Оригінал (дубляж)',
          ),
        ];
  }

  @override
  Future<List<MediaItem>> getByCategory(
    String category, {
    ContentType? type,
    int page = 1,
  }) async => [];
  @override
  Future<List<MediaItem>> getNew({ContentType? type, int page = 1}) async => [];
  @override
  Future<List<MediaItem>> getPopular({ContentType? type, int page = 1}) async =>
      [];
  @override
  Future<List<MediaItem>> search(
    String query, {
    ContentType? type,
    int page = 1,
  }) async => [];
}

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();

  late AppDatabase db;
  late ProviderRegistry registry;
  late _TestContentProvider provider;
  late SettingsService settingsService;
  late OxideServerService serverService;
  late AuthService authService;
  late FavoritesService favoritesService;
  late DownloadService downloadService;
  late SharedPreferences prefs;

  setUp(() async {
    SharedPreferences.setMockInitialValues({});
    prefs = await SharedPreferences.getInstance();

    db = createTestAppDatabase();
    registry = ProviderRegistry();
    provider = _TestContentProvider();
    registry.register(provider);

    final dio = Dio();
    final apiClient = ApiClient(prefs: prefs, dio: dio);

    settingsService = SettingsService(db);
    serverService = OxideServerService(prefs, apiClient);
    authService = AuthService(serverService);
    favoritesService = FavoritesService(
      database: db,
      server: serverService,
      authService: authService,
    );
    downloadService = DownloadService(db, apiClient, settingsService);

    // Register singletons in GetIt
    GetIt.I.registerSingleton<ProviderRegistry>(registry);
    GetIt.I.registerSingleton<FavoritesService>(favoritesService);
    GetIt.I.registerSingleton<DownloadService>(downloadService);

    await Future.delayed(const Duration(milliseconds: 50));
  });

  tearDown(() async {
    GetIt.I.reset();
    downloadService.dispose();
    settingsService.dispose();
    await db.close();
  });

  Widget buildTestWidget({
    String mediaId = 'movie_1',
    String providerId = 'test_provider',
    void Function(String route, dynamic extra)? onNavigate,
  }) {
    final router = GoRouter(
      routes: [
        GoRoute(
          path: '/',
          builder: (context, state) => Scaffold(
            body: DetailsPage(mediaId: mediaId, providerId: providerId),
          ),
        ),
        GoRoute(
          path: '/player',
          builder: (context, state) {
            onNavigate?.call('/player', state.extra);
            return const Scaffold(body: Text('PlayerScreen'));
          },
        ),
        GoRoute(
          path: '/watch-party',
          builder: (context, state) {
            onNavigate?.call('/watch-party', state.extra);
            return const Scaffold(body: Text('WatchPartyScreen'));
          },
        ),
      ],
    );

    return MaterialApp.router(routerConfig: router);
  }

  testWidgets(
    'DetailsPage displays movie details, metadata, genres, and cast',
    (tester) async {
      tester.view.physicalSize = const Size(1920, 1080);
      tester.view.devicePixelRatio = 1.0;
      addTearDown(tester.view.resetPhysicalSize);
      addTearDown(tester.view.resetDevicePixelRatio);

      await tester.pumpWidget(buildTestWidget());
      await tester.pumpAndSettle();

      expect(find.text('Інтерстеллар'), findsOneWidget);
      expect(find.textContaining('2014'), findsWidgets);
      expect(find.text('Фантастика'), findsWidgets);
      expect(find.text('Драма'), findsWidgets);
      expect(find.textContaining('Крістофер Нолан'), findsWidgets);
      expect(find.textContaining('Меттью МакКонахі'), findsWidgets);
      expect(
        find.textContaining('Команда дослідників вирушає крізь червоточину'),
        findsOneWidget,
      );
    },
  );

  testWidgets('DetailsPage adds item to favorites when button is tapped', (
    tester,
  ) async {
    tester.view.physicalSize = const Size(1920, 1080);
    tester.view.devicePixelRatio = 1.0;
    addTearDown(tester.view.resetPhysicalSize);
    addTearDown(tester.view.resetDevicePixelRatio);

    await tester.pumpWidget(buildTestWidget(mediaId: 'fav_movie_1'));
    await tester.pumpAndSettle();

    // Verify initial state is not favorite
    final addFavButton = find.widgetWithIcon(IconButton, Icons.favorite_border);
    expect(addFavButton, findsOneWidget);

    // Tap to add to favorites
    await tester.tap(addFavButton);
    await tester.pumpAndSettle();

    // Icon should now be filled favorite
    expect(find.byIcon(Icons.favorite), findsOneWidget);
  });

  testWidgets('DetailsPage removes item from favorites when already favorite', (
    tester,
  ) async {
    tester.view.physicalSize = const Size(1920, 1080);
    tester.view.devicePixelRatio = 1.0;
    addTearDown(tester.view.resetPhysicalSize);
    addTearDown(tester.view.resetDevicePixelRatio);

    // Pre-populate favorites
    await favoritesService.add(
      MediaItem(
        id: 'fav_movie_2',
        providerId: 'test_provider',
        title: 'Інтерстеллар',
        type: ContentType.movie,
      ),
    );
    // Allow stream to emit
    await tester.runAsync(
      () => Future.delayed(const Duration(milliseconds: 100)),
    );

    await tester.pumpWidget(buildTestWidget(mediaId: 'fav_movie_2'));
    await tester.pumpAndSettle();

    // Verify initial state is favorite
    final removeFavButton = find.widgetWithIcon(IconButton, Icons.favorite);
    expect(removeFavButton, findsOneWidget);

    // Tap to remove from favorites
    await tester.tap(removeFavButton);
    await tester.pumpAndSettle();

    // Icon should now be border
    expect(find.byIcon(Icons.favorite_border), findsOneWidget);
  });

  testWidgets(
    'DetailsPage play button navigates to /player with extra streams metadata',
    (tester) async {
      tester.view.physicalSize = const Size(1920, 1080);
      tester.view.devicePixelRatio = 1.0;
      addTearDown(tester.view.resetPhysicalSize);
      addTearDown(tester.view.resetDevicePixelRatio);

      dynamic capturedExtra;
      await tester.pumpWidget(
        buildTestWidget(
          onNavigate: (route, extra) {
            if (route == '/player') capturedExtra = extra;
          },
        ),
      );
      await tester.pumpAndSettle();

      // Find "Дивитися" button
      final playBtn = find.text('Дивитися');
      expect(playBtn, findsOneWidget);
      await tester.tap(playBtn);
      await tester.pumpAndSettle();

      expect(find.text('PlayerScreen'), findsOneWidget);
      expect(capturedExtra, isNotNull);
      expect(capturedExtra['title'], 'Інтерстеллар');
      expect(capturedExtra['url'], 'https://example.com/stream_1080.mp4');
      expect(capturedExtra['streams'], isA<List<StreamSource>>());
    },
  );

  testWidgets('DetailsPage renders series with seasons and episode buttons', (
    tester,
  ) async {
    tester.view.physicalSize = const Size(1920, 1080);
    tester.view.devicePixelRatio = 1.0;
    addTearDown(tester.view.resetPhysicalSize);
    addTearDown(tester.view.resetDevicePixelRatio);

    provider.customDetails = MediaDetails(
      item: MediaItem(
        id: 'series_1',
        providerId: 'test_provider',
        title: 'Чорнобиль',
        year: 2019,
        type: ContentType.series,
        rating: 9.4,
      ),
      fullDescription: 'Історія аварії на ЧАЕС.',
      seasons: [
        Season(
          number: 1,
          title: 'Сезон 1',
          episodes: [
            Episode(number: 1, title: '1:23:45'),
            Episode(number: 2, title: 'Будь ласка, зберігайте спокій'),
          ],
        ),
      ],
    );

    await tester.pumpWidget(buildTestWidget(mediaId: 'series_1'));
    await tester.pumpAndSettle();

    expect(find.text('Чорнобиль'), findsOneWidget);
    expect(find.textContaining('Сезон 1'), findsWidgets);
    expect(find.text('1'), findsWidgets);
    expect(find.text('2'), findsWidgets);
  });

  testWidgets(
    'DetailsPage shows error UI and allows retry when provider fails',
    (tester) async {
      tester.view.physicalSize = const Size(1920, 1080);
      tester.view.devicePixelRatio = 1.0;
      addTearDown(tester.view.resetPhysicalSize);
      addTearDown(tester.view.resetDevicePixelRatio);

      provider.shouldThrow = true;

      await tester.pumpWidget(buildTestWidget(mediaId: 'error_item'));
      await tester.pumpAndSettle();

      expect(find.textContaining('Помилка'), findsWidgets);
      final retryBtn = find.text('Спробувати знову');
      expect(retryBtn, findsOneWidget);

      // Fix provider and tap retry
      provider.shouldThrow = false;
      await tester.tap(retryBtn);
      await tester.pumpAndSettle();

      expect(find.text('Інтерстеллар'), findsOneWidget);
    },
  );
}
