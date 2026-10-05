import 'package:flutter/foundation.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:oxide_film/data/services/recommendation_service.dart';
import 'package:oxide_film/data/services/history_service.dart';
import 'package:oxide_film/data/providers/provider_registry.dart';
import 'package:oxide_film/domain/repositories/content_provider.dart';
import 'package:oxide_film/domain/entities/entities.dart';
import 'package:oxide_film/data/services/favorites_service.dart';
import 'package:oxide_film/data/database/dao/media_items_dao.dart';
import 'package:oxide_film/data/database/app_database.dart'; // For WatchHistoryData

// --- Manual Mocks ---

class MockHistoryService extends ChangeNotifier implements HistoryService {
  final List<WatchHistoryData> _mockHistory;

  MockHistoryService({List<WatchHistoryData> mockHistory = const []})
    : _mockHistory = mockHistory;

  @override
  List<WatchHistoryData> get history => _mockHistory;

  @override
  List<WatchHistoryData> get continueWatching => [];

  @override
  bool get isLoading => false;

  @override
  bool get isSyncing => false;

  @override
  Future<void> clearAll() async {}

  @override
  Future<int> get count async => _mockHistory.length;

  @override
  String formatRemaining(WatchHistoryData item) => '';

  @override
  Future<WatchHistoryData?> getForMedia(
    String mediaId,
    String providerId, {
    int? season,
    int? episode,
  }) async => null;

  @override
  Future<Duration?> getLastPosition(
    String mediaId,
    String providerId, {
    int? season,
    int? episode,
  }) async => null;

  @override
  double getProgress(WatchHistoryData item) => 0;

  @override
  Future<void> remove(String mediaId, String providerId) async {}

  @override
  Future<void> saveProgress({
    required String mediaId,
    required String providerId,
    required String title,
    String? posterUrl,
    int? year,
    required String mediaType,
    required Duration position,
    required Duration duration,
    int? season,
    int? episode,
    String? episodeTitle,
    String? lastStreamUrl,
    String? voiceover,
  }) async {}

  @override
  Future<void> syncNow() async {}
}

class MockFavoritesService extends ChangeNotifier implements FavoritesService {
  final List<Favorite> _mockFavorites;

  MockFavoritesService({List<Favorite> mockFavorites = const []})
    : _mockFavorites = mockFavorites;

  @override
  List<Favorite> get favorites => _mockFavorites;

  @override
  bool get isLoading => false;

  @override
  bool get isSyncing => false;

  @override
  Future<void> syncNow() async {}

  @override
  Future<int> get count async => _mockFavorites.length;

  @override
  bool isFavorite(String mediaId, String providerId) {
    return _mockFavorites.any(
      (f) => f.mediaId == mediaId && f.providerId == providerId,
    );
  }

  @override
  Future<bool> toggle(MediaItem item) async => false;

  @override
  Future<void> add(MediaItem item) async {}

  @override
  Future<void> remove(String mediaId, String providerId) async {}

  @override
  Future<void> clearAll() async {}

  @override
  Future<List<Favorite>> getByType(ContentType type) async => [];

  @override
  Stream<bool> watchIsFavorite(String mediaId, String providerId) =>
      Stream.value(false);
}

class MockMediaItemsDao implements MediaItemsDao {
  final Map<String, MediaItem> _cache = {};

  @override
  Future<MediaItem?> get(String id, String providerId) async =>
      _cache['$providerId:$id'];

  @override
  Future<void> upsert(MediaItem item) async {
    _cache['${item.providerId}:${item.id}'] = item;
  }
}

class MockProviderRegistry extends ProviderRegistry {
  final List<ContentProvider> _mockProviders;

  MockProviderRegistry(this._mockProviders);

  @override
  List<ContentProvider> get homeProviders => _mockProviders;

  @override
  List<ContentProvider> get enabled => _mockProviders;

  @override
  ContentProvider? getById(String id) {
    try {
      return _mockProviders.firstWhere((p) => p.id == id);
    } catch (_) {
      return null;
    }
  }
}

class MockContentProvider extends ContentProvider {
  final String _id;
  final List<MediaItem> _mockPopular;
  final List<MediaItem> _mockCategoryItems;

  MockContentProvider(
    this._id, {
    List<MediaItem> mockPopular = const [],
    List<MediaItem> mockCategoryItems = const [],
  }) : _mockPopular = mockPopular,
       _mockCategoryItems = mockCategoryItems;

  @override
  String get id => _id;

  @override
  String get name => 'MockProvider $_id';

  @override
  bool get isEnabled => true; // Default to enabled for testing

  @override
  Future<List<MediaItem>> getPopular({ContentType? type, int page = 1}) async =>
      _mockPopular;

  @override
  Future<List<MediaItem>> getByCategory(
    String category, {
    ContentType? type,
    int page = 1,
  }) async => _mockCategoryItems;

  // Stubs for other required methods
  @override
  String get baseUrl => '';
  @override
  Future<List<String>> getCategories() async => [];
  @override
  Future<MediaDetails> getDetails(String id) async =>
      throw UnimplementedError();
  @override
  Future<List<MediaItem>> getNew({ContentType? type, int page = 1}) async => [];
  @override
  Future<List<StreamSource>> getStreams(
    String id, {
    int? season,
    int? episode,
  }) async => [];
  @override
  Future<List<MediaItem>> search(
    String query, {
    ContentType? type,
    int page = 1,
  }) async => [];
  @override
  Future<List<MediaItem>> getSimilar(String id, MediaDetails details) async =>
      [];
  @override
  String get effectiveBaseUrl => '';
  @override
  String? get iconUrl => '';
  @override
  List<ContentType> get supportedTypes => [ContentType.movie];
}

void main() {
  group('RecommendationService', () {
    late RecommendationService service;
    late MockHistoryService mockHistoryService;
    late MockFavoritesService mockFavoritesService;
    late MockProviderRegistry mockProviderRegistry;
    late MockContentProvider mockProvider;
    late MockMediaItemsDao mockMediaItemsDao;

    final testItem = MediaItem(
      id: '1',
      providerId: 'prov1',
      title: 'Test Movie',
      type: ContentType.movie,
      posterUrl: 'url',
    );

    setUp(() {
      // Default setup
      mockProvider = MockContentProvider('prov1', mockPopular: [testItem]);
      mockProviderRegistry = MockProviderRegistry([mockProvider]);
      mockHistoryService = MockHistoryService(mockHistory: []);
      mockFavoritesService = MockFavoritesService(mockFavorites: []);
      mockMediaItemsDao = MockMediaItemsDao();

      service = RecommendationService(
        historyService: mockHistoryService,
        favoritesService: mockFavoritesService,
        providerRegistry: mockProviderRegistry,
        mediaItemsDao: mockMediaItemsDao,
      );
    });

    test('init should load popular content when history is empty', () async {
      await service.init();

      expect(service.isLoading, false);
      expect(service.recommendations.isNotEmpty, true);
      expect(service.recommendations.first.title, 'Test Movie');
    });

    // Note: Testing "history-based" recommendations effectively requires mocking
    // MediaDetails fetching which happens internally in RecommendationService via
    // provider.getDetails(). Since we can't easily inject a mock provider specifically
    // for the `providerRegistry.getById()` call without a more complex mock registry,
    // we'll focus on the fallback logic for now, or we need a smarter MockProviderRegistry.
  });
}
