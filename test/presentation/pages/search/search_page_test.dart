import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:get_it/get_it.dart';
import 'package:go_router/go_router.dart';
import 'package:dio/dio.dart';
import 'package:shared_preferences/shared_preferences.dart';

import 'package:oxide_film/core/network/api_client.dart';
import 'package:oxide_film/data/database/app_database.dart';
import 'package:oxide_film/data/database/dao/search_history_dao.dart';
import 'package:oxide_film/data/models/provider_catalog.dart';
import 'package:oxide_film/data/providers/provider_registry.dart';
import 'package:oxide_film/data/providers/server_backed_provider.dart';
import 'package:oxide_film/data/services/auth_service.dart';
import 'package:oxide_film/data/services/download_service.dart';
import 'package:oxide_film/data/services/favorites_service.dart';
import 'package:oxide_film/data/services/history_service.dart';
import 'package:oxide_film/data/services/oxide_server_service.dart';
import 'package:oxide_film/data/services/provider_catalog_service.dart';
import 'package:oxide_film/data/services/search/search_envelope.dart';
import 'package:oxide_film/data/services/settings_service.dart';
import 'package:oxide_film/data/services/smart_search/smart_search_service.dart';
import 'package:oxide_film/domain/entities/entities.dart';
import 'package:oxide_film/domain/repositories/content_provider.dart';
import 'package:oxide_film/presentation/pages/search/search_page.dart';

import '../../../helpers/in_memory_db.dart';

class _FakeBanderaProvider extends ServerBackedProvider {
  _FakeBanderaProvider({this.mockEnvelope, this.throwError = false})
    : super(
        const ProviderCatalogEntry(
          id: 'bandera',
          name: 'Bandera Online',
          baseUrl: 'https://test.bandera',
          enabled: true,
          healthy: true,
          showOnHome: true,
          hasFixedStreams: false,
          contentTypes: ['movie', 'series'],
          searchEnabledDefault: true,
        ),
      );

  SearchEnvelope? mockEnvelope;
  bool throwError;

  /// Number of searchEnvelope calls, so a test can assert that a retry really
  /// re-issues the request instead of replaying a cached failure.
  int searchCalls = 0;

  @override
  Future<SearchEnvelope> searchEnvelope(
    String query, {
    ContentType? type,
    int page = 1,
  }) async {
    searchCalls++;
    if (throwError) {
      throw Exception('Мережева помилка пошуку');
    }
    return mockEnvelope ??
        SearchEnvelope(
          query: query,
          canonical: query,
          tookMs: 42,
          segments: [
            SearchSegment(
              id: 'seg_1',
              status: 'ok',
              count: 2,
              sources: {
                'uakino': const SearchSourceStatus(
                  key: 'uakino',
                  status: SourceStatus.ok,
                  count: 1,
                ),
                'eneyida': const SearchSourceStatus(
                  key: 'eneyida',
                  status: SourceStatus.ok,
                  count: 1,
                ),
              },
            ),
          ],
          items: [
            ScoredMediaItem(
              clusterKey: 'cluster_dune_2',
              item: MediaItem(
                id: 'dune_2',
                providerId: 'bandera',
                title: 'Дюна: Частина друга',
                originalTitle: 'Dune: Part Two',
                year: 2024,
                type: ContentType.movie,
                rating: 8.6,
              ),
              score: 0.95,
              matchedBy: 'title',
              sources: const [
                SearchItemSource(
                  providerId: 'bandera',
                  sourceKey: 'uakino',
                  itemId: 'dune_2_ua',
                ),
              ],
            ),
            ScoredMediaItem(
              clusterKey: 'cluster_oppenheimer',
              item: MediaItem(
                id: 'oppenheimer',
                providerId: 'bandera',
                title: 'Оппенгеймер',
                originalTitle: 'Oppenheimer',
                year: 2023,
                type: ContentType.movie,
                rating: 8.9,
              ),
              score: 0.88,
              matchedBy: 'title',
              sources: const [
                SearchItemSource(
                  providerId: 'bandera',
                  sourceKey: 'eneyida',
                  itemId: 'oppenheimer_en',
                ),
              ],
            ),
          ],
          filteredOut: 0,
        );
  }
}

class _DummyProvider extends ContentProvider {
  @override
  String get id => 'other_provider';
  @override
  String get name => 'Other Provider';
  @override
  String get baseUrl => 'https://other.example';
  @override
  bool get isEnabled => true;
  @override
  String? get iconUrl => null;
  @override
  List<ContentType> get supportedTypes => [ContentType.movie];
  @override
  Future<MediaDetails> getDetails(String id) async =>
      throw UnimplementedError();
  @override
  Future<List<StreamSource>> getStreams(
    String id, {
    int? season,
    int? episode,
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
  late _FakeBanderaProvider banderaProvider;
  late SettingsService settingsService;
  late OxideServerService serverService;
  late AuthService authService;
  late FavoritesService favoritesService;
  late DownloadService downloadService;
  late HistoryService historyService;
  late SmartSearchService smartSearchService;
  late ProviderCatalogService catalogService;
  late SharedPreferences prefs;

  setUp(() async {
    SharedPreferences.setMockInitialValues({});
    prefs = await SharedPreferences.getInstance();

    db = createTestAppDatabase();
    registry = ProviderRegistry();
    banderaProvider = _FakeBanderaProvider();
    registry.register(banderaProvider);
    registry.register(_DummyProvider());

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
    historyService = HistoryService(
      database: db,
      server: serverService,
      authService: authService,
    );
    final historyDao = SearchHistoryDao(db);
    smartSearchService = SmartSearchService(registry, historyDao);
    catalogService = ProviderCatalogService(
      registry: registry,
      prefs: prefs,
      api: apiClient,
    );

    // Register dependencies in GetIt
    GetIt.I.registerSingleton<ProviderRegistry>(registry);
    GetIt.I.registerSingleton<SmartSearchService>(smartSearchService);
    GetIt.I.registerSingleton<HistoryService>(historyService);
    GetIt.I.registerSingleton<FavoritesService>(favoritesService);
    GetIt.I.registerSingleton<DownloadService>(downloadService);
    GetIt.I.registerSingleton<SettingsService>(settingsService);
    GetIt.I.registerSingleton<ProviderCatalogService>(catalogService);
    GetIt.I.registerSingleton<ApiClient>(apiClient);
    GetIt.I.registerSingleton<SharedPreferences>(prefs);

    await Future.delayed(const Duration(milliseconds: 50));
  });

  tearDown(() async {
    GetIt.I.reset();
    downloadService.dispose();
    settingsService.dispose();
    historyService.dispose();
    await db.close();
  });

  Widget buildTestWidget({
    String? initialQuery,
    void Function(String route)? onNavigate,
  }) {
    final router = GoRouter(
      routes: [
        GoRoute(
          path: '/',
          builder: (context, state) => SearchPage(initialQuery: initialQuery),
        ),
        GoRoute(
          path: '/details/:providerId/:id',
          builder: (context, state) {
            final pId = state.pathParameters['providerId'];
            final id = state.pathParameters['id'];
            onNavigate?.call('/details/$pId/$id');
            return Scaffold(body: Text('Details: $pId / $id'));
          },
        ),
      ],
    );

    return MaterialApp.router(routerConfig: router, theme: ThemeData.dark());
  }

  testWidgets('SearchPage renders empty search bar and placeholder initially', (
    tester,
  ) async {
    tester.view.physicalSize = const Size(1920, 1080);
    tester.view.devicePixelRatio = 1.0;
    addTearDown(tester.view.resetPhysicalSize);
    addTearDown(tester.view.resetDevicePixelRatio);

    await tester.pumpWidget(buildTestWidget());
    await tester.pumpAndSettle();

    // Verify search bar exists with placeholder
    expect(find.byType(TextField), findsOneWidget);
    expect(find.text('Пошук фільмів, серіалів...'), findsOneWidget);

    // Verify empty prompt
    expect(find.text('Введіть запит для пошуку'), findsOneWidget);
  });

  testWidgets(
    'SearchPage auto-searches and displays results when initialQuery is provided',
    (tester) async {
      tester.view.physicalSize = const Size(1920, 1080);
      tester.view.devicePixelRatio = 1.0;
      addTearDown(tester.view.resetPhysicalSize);
      addTearDown(tester.view.resetDevicePixelRatio);

      await tester.pumpWidget(buildTestWidget(initialQuery: 'Дюна'));
      await tester.pumpAndSettle();

      // Results from _FakeBanderaProvider should be displayed
      expect(find.text('Дюна: Частина друга'), findsWidgets);
      expect(find.text('Оппенгеймер'), findsWidgets);
      expect(find.textContaining('2024'), findsWidgets);
      expect(find.textContaining('2023'), findsWidgets);

      // Verify provider filter chips are rendered
      expect(find.text('Усі джерела'), findsOneWidget);
      expect(find.text('uakino'), findsOneWidget);
      expect(find.text('eneyida'), findsOneWidget);
    },
  );

  testWidgets(
    'SearchPage filters results when specific source chip is tapped',
    (tester) async {
      tester.view.physicalSize = const Size(1920, 1080);
      tester.view.devicePixelRatio = 1.0;
      addTearDown(tester.view.resetPhysicalSize);
      addTearDown(tester.view.resetDevicePixelRatio);

      await tester.pumpWidget(buildTestWidget(initialQuery: 'Фільми'));
      await tester.pumpAndSettle();

      expect(find.text('Дюна: Частина друга'), findsWidgets);
      expect(find.text('Оппенгеймер'), findsWidgets);

      // Filter by 'uakino'
      final uakinoChip = find.text('uakino');
      expect(uakinoChip, findsOneWidget);
      await tester.tap(uakinoChip);
      await tester.pumpAndSettle();

      // Only Dune should remain as it was sourced from uakino
      expect(find.text('Дюна: Частина друга'), findsWidgets);
      expect(find.text('Оппенгеймер'), findsNothing);

      // Tap 'Усі джерела' to restore all results
      final allSourcesChip = find.text('Усі джерела');
      await tester.tap(allSourcesChip);
      await tester.pumpAndSettle();

      expect(find.text('Дюна: Частина друга'), findsWidgets);
      expect(find.text('Оппенгеймер'), findsWidgets);
    },
  );

  testWidgets(
    'SearchPage navigates to details page when a result card is tapped',
    (tester) async {
      tester.view.physicalSize = const Size(1920, 1080);
      tester.view.devicePixelRatio = 1.0;
      addTearDown(tester.view.resetPhysicalSize);
      addTearDown(tester.view.resetDevicePixelRatio);

      String? navigatedRoute;
      await tester.pumpWidget(
        buildTestWidget(
          initialQuery: 'Дюна',
          onNavigate: (route) => navigatedRoute = route,
        ),
      );
      await tester.pumpAndSettle();

      final itemCard = find.text('Дюна: Частина друга').first;
      expect(itemCard, findsOneWidget);
      await tester.tap(itemCard);
      await tester.pumpAndSettle();

      expect(navigatedRoute, '/details/bandera/dune_2');
      expect(find.text('Details: bandera / dune_2'), findsOneWidget);
    },
  );

  testWidgets(
    'SearchPage displays failure banner when sources fail partially',
    (tester) async {
      tester.view.physicalSize = const Size(1920, 1080);
      tester.view.devicePixelRatio = 1.0;
      addTearDown(tester.view.resetPhysicalSize);
      addTearDown(tester.view.resetDevicePixelRatio);

      banderaProvider.mockEnvelope = SearchEnvelope(
        query: 'Бетмен',
        canonical: 'Бетмен',
        tookMs: 50,
        segments: [
          SearchSegment(
            id: 'seg_1',
            status: 'partial',
            count: 1,
            sources: {
              'uakino': const SearchSourceStatus(
                key: 'uakino',
                status: SourceStatus.ok,
                count: 1,
              ),
              'lavakino': const SearchSourceStatus(
                key: 'lavakino',
                status: SourceStatus.error,
                count: 0,
              ),
            },
          ),
        ],
        items: [
          ScoredMediaItem(
            clusterKey: 'cluster_batman',
            item: MediaItem(
              id: 'batman',
              providerId: 'bandera',
              title: 'Бетмен',
              year: 2022,
              type: ContentType.movie,
            ),
            score: 0.9,
            matchedBy: 'title',
            sources: const [
              SearchItemSource(
                providerId: 'bandera',
                sourceKey: 'uakino',
                itemId: 'batman_ua',
              ),
            ],
          ),
        ],
        filteredOut: 0,
      );

      await tester.pumpWidget(buildTestWidget(initialQuery: 'Бетмен'));
      await tester.pumpAndSettle();

      expect(find.text('Бетмен'), findsWidgets);
      // Failure banner should mention lavakino
      expect(
        find.textContaining('Недоступні джерела: lavakino'),
        findsOneWidget,
      );
      expect(find.text('Повторити'), findsOneWidget);
    },
  );

  testWidgets(
    'SearchPage displays error state and retry button when search fails completely',
    (tester) async {
      tester.view.physicalSize = const Size(1920, 1080);
      tester.view.devicePixelRatio = 1.0;
      addTearDown(tester.view.resetPhysicalSize);
      addTearDown(tester.view.resetDevicePixelRatio);

      banderaProvider.throwError = true;
      await tester.pumpWidget(buildTestWidget(initialQuery: 'Помилка'));
      await tester.pumpAndSettle();

      expect(
        banderaProvider.searchCalls,
        1,
        reason: 'the initial query should issue exactly one request',
      );

      // Verify error UI is displayed
      expect(find.textContaining('Мережева помилка'), findsWidgets);
      // `ElevatedButton.icon` builds an `_ElevatedButtonWithIcon` subclass, and
      // `find.byType` compares the exact runtime type, so neither `byType` nor
      // `widgetWithText(ElevatedButton, ...)` can see this button. Match on the
      // `is ElevatedButton` predicate instead.
      final retryButton = find.ancestor(
        of: find.text('Спробувати знову'),
        matching: find.byWidgetPredicate((w) => w is ElevatedButton),
      );
      expect(retryButton, findsOneWidget);

      // Fix error and retry
      banderaProvider.throwError = false;
      await tester.ensureVisible(retryButton);
      await tester.pumpAndSettle();
      (tester.widget(retryButton) as ElevatedButton).onPressed!();
      // The retry runs a real async chain (controller -> service -> provider ->
      // setState). Frame pumping alone does not drive the Future, so let real
      // async work run and then settle the frames it produces.
      await tester.runAsync(
        () => Future<void>.delayed(const Duration(milliseconds: 50)),
      );
      await tester.pumpAndSettle();

      expect(
        banderaProvider.searchCalls,
        2,
        reason:
            'the retry must re-issue the request, not replay a cached failure',
      );
      expect(find.text('Дюна: Частина друга'), findsWidgets);
    },
  );

  testWidgets('SearchPage clear button clears query and resets state', (
    tester,
  ) async {
    tester.view.physicalSize = const Size(1920, 1080);
    tester.view.devicePixelRatio = 1.0;
    addTearDown(tester.view.resetPhysicalSize);
    addTearDown(tester.view.resetDevicePixelRatio);

    await tester.pumpWidget(buildTestWidget(initialQuery: 'Дюна'));
    await tester.pumpAndSettle();

    expect(find.text('Дюна: Частина друга'), findsWidgets);

    // Tap clear button in TextField
    final clearBtn = find.byIcon(Icons.clear);
    expect(clearBtn, findsOneWidget);
    await tester.tap(clearBtn);
    await tester.pumpAndSettle();

    expect(find.text('Дюна: Частина друга'), findsNothing);
    expect(find.text('Введіть запит для пошуку'), findsOneWidget);
  });

  testWidgets('SearchPage shows suggestions when user types in search bar', (
    tester,
  ) async {
    tester.view.physicalSize = const Size(1920, 1080);
    tester.view.devicePixelRatio = 1.0;
    addTearDown(tester.view.resetPhysicalSize);
    addTearDown(tester.view.resetDevicePixelRatio);

    // Populate search history DAO
    final historyDao = SearchHistoryDao(db);
    await historyDao.addSearch(
      query: 'Матриця',
      normalizedQuery: 'матриця',
      resultCount: 3,
    );

    await tester.pumpWidget(buildTestWidget());
    await tester.pumpAndSettle();

    // Enter text in search field
    final searchInput = find.byType(TextField);
    await tester.enterText(searchInput, 'Мат');
    // Pump past the 300ms debounce timer
    await tester.pump(const Duration(milliseconds: 350));
    await tester.pumpAndSettle();

    // Verify autocomplete suggestion appears
    expect(find.text('Матриця'), findsOneWidget);

    // Tapping suggestion triggers search
    await tester.tap(find.text('Матриця'));
    await tester.pumpAndSettle();

    expect(find.text('Дюна: Частина друга'), findsWidgets);
  });
}
