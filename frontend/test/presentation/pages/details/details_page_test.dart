import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:get_it/get_it.dart';
import 'package:go_router/go_router.dart';
import 'package:dio/dio.dart';
import 'package:shared_preferences/shared_preferences.dart';

import 'package:kadrbox/core/network/api_client.dart';
import 'package:kadrbox/data/database/app_database.dart';
import 'package:kadrbox/data/providers/provider_registry.dart';
import 'package:kadrbox/data/services/auth_service.dart';
import 'package:kadrbox/data/services/download_service.dart';
import 'package:kadrbox/data/services/favorites_service.dart';
import 'package:kadrbox/data/services/kadrbox_server_service.dart';
import 'package:kadrbox/data/services/settings_service.dart';
import 'package:kadrbox/domain/entities/entities.dart';
import 'package:kadrbox/domain/repositories/content_provider.dart';
import 'package:kadrbox/presentation/pages/details/details_page.dart';

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

  /// Per-episode-ref sources, keyed by the id the page asks for.
  final Map<String, List<StreamSource>> episodeStreams = {};

  /// Every id `getStreams` was called with, in order.
  final List<String> streamRequests = [];

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
    streamRequests.add(id);
    final byRef = episodeStreams[id];
    if (byRef != null) return byRef;
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
  late KadrboxServerService serverService;
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
    serverService = KadrboxServerService(prefs, apiClient);
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
    // FocusableCard resolves SettingsService for its focus behaviour.
    GetIt.I.registerSingleton<SettingsService>(settingsService);

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
    'DetailsPage prefers details.voiceover studios over stream voiceovers',
    (tester) async {
      tester.view.physicalSize = const Size(1920, 1080);
      tester.view.devicePixelRatio = 1.0;
      addTearDown(tester.view.resetPhysicalSize);
      addTearDown(tester.view.resetDevicePixelRatio);

      // Two DLE streams whose only distinguishing label is the CDN. The old
      // selector grouped by `stream.voiceover`, so this rendered "CDN Alpha" and
      // "CDN Beta" as the dubbing options.
      provider.customStreams = const [
        StreamSource(
          url: 'https://cdn.tv/alpha/master.m3u8',
          sourceName: 'CDN Alpha',
          type: StreamType.hls,
        ),
        StreamSource(
          url: 'https://cdn.tv/beta/master.m3u8',
          sourceName: 'CDN Beta',
          type: StreamType.hls,
        ),
      ];

      provider.customDetails = MediaDetails(
        item: MediaItem(
          id: 'series_vo',
          providerId: 'test_provider',
          title: 'Візит Президента',
          type: ContentType.series,
        ),
        voiceovers: const [
          Voiceover(
            id: '1plus1',
            name: '1+1',
            seasons: [
              Season(
                number: 1,
                episodes: [
                  Episode(number: 1, title: 'Початок'),
                  Episode(number: 2, title: 'Далі'),
                ],
              ),
            ],
          ),
          Voiceover(
            id: 'postmodern',
            name: 'Postmodern',
            seasons: [
              Season(
                number: 1,
                episodes: [Episode(number: 1, title: 'Початок')],
              ),
            ],
          ),
        ],
      );

      await tester.pumpWidget(buildTestWidget(mediaId: 'series_vo'));
      await tester.pumpAndSettle();

      // Studio names from the details payload.
      expect(find.text('1+1'), findsOneWidget);
      expect(find.text('Postmodern'), findsOneWidget);

      // The CDN names must not have been offered as dubbing studios.
      expect(find.text('CDN Alpha'), findsNothing);
      expect(find.text('CDN Beta'), findsNothing);
    },
  );

  testWidgets('DetailsPage renders season and episode pickers per voiceover', (
    tester,
  ) async {
    tester.view.physicalSize = const Size(1920, 1080);
    tester.view.devicePixelRatio = 1.0;
    addTearDown(tester.view.resetPhysicalSize);
    addTearDown(tester.view.resetDevicePixelRatio);

    provider.customStreams = const [
      StreamSource(url: 'https://cdn.tv/a.m3u8', sourceName: 'CDN Alpha'),
    ];

    provider.customDetails = MediaDetails(
      item: MediaItem(
        id: 'series_4seasons',
        providerId: 'test_provider',
        title: 'Довгий серіал',
        type: ContentType.series,
      ),
      voiceovers: const [
        Voiceover(
          id: 'a',
          name: 'Студія A',
          seasons: [
            Season(
              number: 1,
              episodes: [Episode(number: 1), Episode(number: 2)],
            ),
            Season(number: 2, episodes: [Episode(number: 1)]),
            Season(number: 3, episodes: [Episode(number: 1)]),
            Season(number: 4, episodes: [Episode(number: 1)]),
          ],
        ),
        Voiceover(
          id: 'b',
          name: 'Студія B',
          seasons: [
            Season(
              number: 1,
              episodes: [Episode(number: 1), Episode(number: 2)],
            ),
          ],
        ),
      ],
    );

    await tester.pumpWidget(buildTestWidget(mediaId: 'series_4seasons'));
    await tester.pumpAndSettle();

    // Both studios listed.
    expect(find.text('Студія A'), findsOneWidget);
    expect(find.text('Студія B'), findsOneWidget);

    // Four seasons for the active studio, but only two episodes in season 1.
    expect(find.text('Сезон 1'), findsOneWidget);
    expect(find.text('Сезон 2'), findsOneWidget);
    expect(find.text('Сезон 3'), findsOneWidget);
    expect(find.text('Сезон 4'), findsOneWidget);
    expect(find.text('Серії (2)'), findsOneWidget);

    // Exactly one episode section, not the old duplicate seasons section.
    expect(find.text('Сезони та серії'), findsNothing);

    // Switching studio must rebuild season and episode rows from THAT
    // studio's tree. Студія B has a single season of two episodes, so the
    // four season chips collapse into one episode row.
    await tester.tap(find.text('Студія B'));
    await tester.pumpAndSettle();

    expect(find.text('Сезон 1'), findsNothing);
    expect(find.text('Сезон 2'), findsNothing);
    expect(find.text('Сезон 3'), findsNothing);
    expect(find.text('Сезон 4'), findsNothing);
    expect(find.text('Серії (2)'), findsOneWidget);
    expect(find.text('Сезони та серії'), findsNothing);
  });

  testWidgets(
    'DetailsPage does not render a series selector for a single-episode movie',
    (tester) async {
      tester.view.physicalSize = const Size(1920, 1080);
      tester.view.devicePixelRatio = 1.0;
      addTearDown(tester.view.resetPhysicalSize);
      addTearDown(tester.view.resetDevicePixelRatio);

      // The exact shape a DLE backend sends for a film: one season, one
      // "episode", type movie.
      provider.customDetails = MediaDetails(
        item: MediaItem(
          id: 'movie_single',
          providerId: 'test_provider',
          title: 'Одинокий фільм',
          type: ContentType.movie,
        ),
        seasons: const [
          Season(number: 1, episodes: [Episode(number: 1, title: 'Фільм')]),
        ],
        voiceovers: const [
          Voiceover(
            id: 'one',
            name: 'Основна',
            seasons: [
              Season(number: 1, episodes: [Episode(number: 1)]),
            ],
          ),
        ],
      );

      expect(provider.customDetails!.isSeries, isFalse);

      await tester.pumpWidget(buildTestWidget(mediaId: 'movie_single'));
      await tester.pumpAndSettle();

      expect(find.text('Одинокий фільм'), findsOneWidget);
      expect(find.text('Сезони та серії'), findsNothing);
      expect(find.text('Сезон 1'), findsNothing);
      expect(find.text('Серії (1)'), findsNothing);
      // The studio is still named, so the user knows which dub will play.
      expect(find.text('Основна'), findsOneWidget);
    },
  );

  testWidgets(
    'DetailsPage keeps the stream-based selector when no voiceovers are reported',
    (tester) async {
      tester.view.physicalSize = const Size(1920, 1080);
      tester.view.devicePixelRatio = 1.0;
      addTearDown(tester.view.resetPhysicalSize);
      addTearDown(tester.view.resetDevicePixelRatio);

      provider.customStreams = const [
        StreamSource(
          url: 'https://cdn.tv/a.m3u8',
          voiceover: 'Українська',
          quality: StreamQuality.q1080p,
        ),
        StreamSource(
          url: 'https://cdn.tv/b.m3u8',
          voiceover: 'Російська',
          quality: StreamQuality.q720p,
        ),
      ];

      await tester.pumpWidget(buildTestWidget(mediaId: 'movie_legacy'));
      await tester.pumpAndSettle();

      // Backwards-compatible path: dropdowns built from the streams themselves.
      expect(find.byType(DropdownButtonFormField<String>), findsOneWidget);
      expect(find.text('Українська'), findsOneWidget);
      expect(find.text('Оригінал'), findsNothing);
    },
  );

  testWidgets(
    'DetailsPage plays the episode selected in the voiceover selector',
    (tester) async {
      tester.view.physicalSize = const Size(1920, 1080);
      tester.view.devicePixelRatio = 1.0;
      addTearDown(tester.view.resetPhysicalSize);
      addTearDown(tester.view.resetDevicePixelRatio);

      provider.customStreams = const [
        StreamSource(
          url: 'https://cdn.tv/default.m3u8',
          sourceName: 'CDN Alpha',
        ),
      ];
      provider.customDetails = MediaDetails(
        item: MediaItem(
          id: 'series_play',
          providerId: 'test_provider',
          title: 'Серіал',
          type: ContentType.series,
        ),
        voiceovers: const [
          Voiceover(
            id: 'a',
            name: '1+1',
            seasons: [
              Season(
                number: 1,
                episodes: [
                  Episode(number: 1, streamRef: 'ref-e1'),
                  Episode(number: 2, streamRef: 'ref-e2'),
                ],
              ),
            ],
          ),
        ],
      );

      // Return a distinct stream per requested episode ref.
      provider.episodeStreams['ref-e2'] = [
        StreamSource(
          url: 'https://cdn.tv/episode2.m3u8',
          sourceName: 'CDN Beta',
          type: StreamType.hls,
        ),
      ];

      dynamic capturedExtra;
      await tester.pumpWidget(
        buildTestWidget(
          mediaId: 'series_play',
          onNavigate: (route, extra) {
            if (route == '/player') capturedExtra = extra;
          },
        ),
      );
      await tester.pumpAndSettle();

      await tester.tap(find.text('2'));
      await tester.pumpAndSettle();

      expect(capturedExtra, isNotNull);
      expect(capturedExtra['url'], 'https://cdn.tv/episode2.m3u8');
      expect(capturedExtra['episode'], 2);
      expect(capturedExtra['season'], 1);
      // The subtitle is the studio the user picked, not the CDN.
      expect(capturedExtra['subtitle'], '1+1');

      // The episode's own stream ref resolved playback, not the series page url.
      expect(provider.streamRequests, contains('ref-e2'));
    },
  );

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
