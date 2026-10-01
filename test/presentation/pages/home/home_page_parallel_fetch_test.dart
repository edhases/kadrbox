// Tests for HomePage parallel fetch and deduplication

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:get_it/get_it.dart';
import 'package:go_router/go_router.dart';

import 'package:oxide_film/data/providers/provider_registry.dart';
import 'package:oxide_film/domain/entities/entities.dart';
import 'package:oxide_film/presentation/pages/home/home_page.dart';
import 'package:oxide_film/presentation/widgets/media_card.dart';
import 'package:oxide_film/data/services/settings_service.dart';
import 'package:oxide_film/domain/repositories/content_provider.dart';
import 'package:oxide_film/data/services/recommendation_service.dart';
import 'package:oxide_film/data/services/episode_update_service.dart';
import 'package:oxide_film/data/services/history_service.dart';
import 'package:oxide_film/data/services/favorites_service.dart';
import 'package:oxide_film/data/services/download_service.dart';

import '../../../helpers/mock_services.dart';
import 'package:mocktail/mocktail.dart';

class _FakeProvider extends ContentProvider {
  @override
  final String id;

  @override
  final String name;

  final List<MediaItem> _items;

  _FakeProvider(this.id, this.name, this._items);

  @override
  String get baseUrl => 'https://$id.example';

  @override
  bool get isEnabled => true;

  @override
  String get effectiveBaseUrl => baseUrl;

  @override
  String? get iconUrl => null;

  @override
  List<ContentType> get supportedTypes => [ContentType.movie];

  @override
  Future<List<MediaItem>> getByCategory(
    String category, {
    ContentType? type,
    int page = 1,
  }) {
    return Future.value(_items);
  }

  @override
  Future<List<MediaItem>> getPopular({ContentType? type, int page = 1}) {
    return Future.value(_items);
  }

  @override
  Future<List<MediaItem>> search(
    String query, {
    ContentType? type,
    int page = 1,
  }) {
    return Future.value(_items);
  }

  @override
  Future<List<MediaItem>> getSimilar(String id, MediaDetails details) async =>
      [];

  @override
  Future<List<StreamSource>> getStreams(
    String id, {
    int? season,
    int? episode,
  }) {
    return Future.value([]);
  }

  @override
  Future<MediaDetails> getDetails(String id) {
    throw UnimplementedError();
  }

  @override
  Future<List<MediaItem>> getNew({ContentType? type, int page = 1}) {
    return Future.value(_items);
  }

  @override
  Future<List<String>> getCategories() {
    return Future.value(['Category1', 'Category2']);
  }
}

class FakeSettingsService extends Fake implements SettingsService {
  @override
  SettingsState get state => const SettingsState();

  @override
  UISettings get uiSettings => const UISettings();

  @override
  bool isProviderEnabled(String providerId) => true;

  @override
  void addListener(VoidCallback listener) {}

  @override
  void removeListener(VoidCallback listener) {}
}

void main() {
  setUp(() {
    // Register a fake settings service
    final startSettings = FakeSettingsService();

    if (GetIt.I.isRegistered<SettingsService>()) {
      GetIt.I.unregister<SettingsService>();
    }
    GetIt.I.registerSingleton<SettingsService>(startSettings);

    final mockRecommendationService = MockRecommendationService();
    when(() => mockRecommendationService.recommendations).thenReturn([]);
    when(() => mockRecommendationService.isLoading).thenReturn(false);
    when(() => mockRecommendationService.init()).thenAnswer((_) async {});
    when(() => mockRecommendationService.addListener(any())).thenReturn(null);
    when(
      () => mockRecommendationService.removeListener(any()),
    ).thenReturn(null);
    GetIt.I.registerSingleton<RecommendationService>(mockRecommendationService);

    final mockEpisodeUpdateService = MockEpisodeUpdateService();
    when(
      () => mockEpisodeUpdateService.checkForUpdates(),
    ).thenAnswer((_) async {});
    when(() => mockEpisodeUpdateService.hasNewEpisodes).thenReturn(false);
    when(() => mockEpisodeUpdateService.newEpisodesCount).thenReturn(0);
    when(() => mockEpisodeUpdateService.isChecking).thenReturn(false);
    when(() => mockEpisodeUpdateService.addListener(any())).thenReturn(null);
    when(() => mockEpisodeUpdateService.removeListener(any())).thenReturn(null);
    GetIt.I.registerSingleton<EpisodeUpdateService>(mockEpisodeUpdateService);

    final mockHistoryService = MockHistoryService();
    when(() => mockHistoryService.continueWatching).thenReturn([]);
    when(() => mockHistoryService.addListener(any())).thenReturn(null);
    when(() => mockHistoryService.removeListener(any())).thenReturn(null);
    GetIt.I.registerSingleton<HistoryService>(mockHistoryService);

    final mockFavoritesService = MockFavoritesService();
    when(() => mockFavoritesService.isFavorite(any(), any())).thenReturn(false);
    when(() => mockFavoritesService.favorites).thenReturn([]);
    when(() => mockFavoritesService.addListener(any())).thenReturn(null);
    when(() => mockFavoritesService.removeListener(any())).thenReturn(null);
    GetIt.I.registerSingleton<FavoritesService>(mockFavoritesService);

    final mockDownloadService = MockDownloadService();
    when(() => mockDownloadService.getLocalPosterPath(any(), any())).thenReturn(null);
    GetIt.I.registerSingleton<DownloadService>(mockDownloadService);
  });

  tearDown(() {
    GetIt.I.reset();
  });

  testWidgets('HomePage loads providers in parallel and deduplicates items', (
    tester,
  ) async {
    final registry = ProviderRegistry();

    final item1 = MediaItem(
      id: '1',
      providerId: 'provA',
      title: 'Title1',
      type: ContentType.movie,
    );
    final item2 = MediaItem(
      id: '2',
      providerId: 'provA',
      title: 'Title2',
      type: ContentType.movie,
    );
    final item3 = MediaItem(
      id: '3',
      providerId: 'provB',
      title: 'Title3',
      type: ContentType.movie,
    );

    // Provider A returns a duplicate of item1 inside its list
    final providerA = _FakeProvider('provA', 'Prov A', [item1, item2, item1]);
    final providerB = _FakeProvider('provB', 'Prov B', [item3]);

    registry.register(providerA);
    registry.register(providerB);

    GetIt.I.registerSingleton<ProviderRegistry>(registry);

    // Set a large enough screen size to ensure grid items are rendered and no overflow
    tester.view.physicalSize = const Size(2000, 2000);
    tester.view.devicePixelRatio = 1.0;
    addTearDown(tester.view.resetPhysicalSize);
    addTearDown(tester.view.resetDevicePixelRatio);

    final router = GoRouter(
      routes: [
        GoRoute(
          path: '/',
          builder: (context, state) => const Scaffold(body: HomePage()),
        ),
      ],
    );

    await tester.pumpWidget(MaterialApp.router(routerConfig: router));

    // Allow async content to load
    await tester.pumpAndSettle(const Duration(seconds: 1));

    // MediaCard widgets should be present for unique items (3 unique entries: 1,2,3)
    final cards = find.byType(MediaCard);
    expect(cards, findsNWidgets(3));

    // Cleanup provider registry
    GetIt.I.unregister<ProviderRegistry>();
  });
}
