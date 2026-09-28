import 'package:get_it/get_it.dart';
import 'package:injectable/injectable.dart';
import 'package:shared_preferences/shared_preferences.dart';
import '../network/api_client.dart';
import '../../data/providers/provider_registry.dart';
import '../../data/providers/uakino_provider.dart';
import '../../data/providers/hdrezka_provider.dart';
import '../../data/providers/youtube_provider.dart';
import '../../data/providers/eneyida_provider.dart';
// import '../../data/providers/yummyanime_provider.dart';  // Disabled - not working properly
import '../../data/providers/uaflix_provider.dart';
import '../../data/providers/uaserials_provider.dart';
import '../../data/database/app_database.dart';
import '../../data/services/settings_service.dart';
import '../../data/services/favorites_service.dart';
import '../../data/services/history_service.dart';
import '../../data/services/sync_service.dart';
import '../../data/services/download_service.dart';
import '../../data/services/stats_service.dart';
import '../../data/services/episode_update_service.dart';
import '../../data/services/video_player_service.dart';
import '../../data/services/watch_party_service.dart';
import '../../data/services/oxide_server_service.dart';
import '../../data/services/auth_service.dart';
import '../../data/services/pocketbase_service.dart';
import '../../data/services/search_service.dart';
import '../../data/services/url_resolver_service.dart';
import '../../data/services/recommendation_service.dart';
import '../../data/services/smart_search/smart_search_service.dart';
import '../../data/services/data_transfer_service.dart';
import '../../data/services/user_agent_service.dart';
import '../../data/services/update_service.dart';
import '../../data/database/dao/search_history_dao.dart';
import '../../data/database/dao/history_dao.dart';
import '../../data/database/dao/favorites_dao.dart';
import '../../data/database/dao/media_items_dao.dart';
import '../../data/repositories/unified_content_repository_impl.dart';
import '../../domain/repositories/unified_content_repository.dart';
// External API services
import '../../data/services/tmdb_service.dart';
import '../../data/services/jikan_service.dart';
import '../../data/services/tvmaze_service.dart';

/// Service locator instance
final getIt = GetIt.instance;

/// Initialize all dependencies
@InjectableInit()
Future<void> configureDependencies() async {
  // SharedPreferences (must be first - async)
  final prefs = await SharedPreferences.getInstance();
  getIt.registerSingleton<SharedPreferences>(prefs);

  // Core services - register UserAgentService and ApiClient
  final uaService = UserAgentService(prefs: prefs);
  await uaService.initialize();
  getIt.registerSingleton<UserAgentService>(uaService);

  final apiClient = ApiClient(uaService: uaService);
  getIt.registerSingleton<ApiClient>(apiClient);

  // Oxide Go Server backend service (Chi + PostgreSQL 16 + Redis)
  final serverService = OxideServerService(prefs, apiClient);
  getIt.registerSingleton<OxideServerService>(serverService);

  // PocketBase backend service (legacy fallback)
  getIt.registerSingleton<PocketBaseService>(PocketBaseService(prefs));

  // Database (must be early)
  final database = AppDatabase();
  getIt.registerSingleton<AppDatabase>(database);

  // URL Resolver for auto-detecting domain changes
  getIt.registerLazySingleton<UrlResolverService>(
    () => UrlResolverService(getIt<SharedPreferences>()),
  );

  // Auth service (supports both Oxide Server and PocketBase fallback)
  getIt.registerLazySingleton<AuthService>(
    () => AuthService(
      getIt<OxideServerService>(),
      getIt<PocketBaseService>(),
    ),
  );

  // Services
  getIt.registerLazySingleton<SettingsService>(() => SettingsService(database));
  getIt.registerLazySingleton<UpdateService>(
    () => UpdateService(getIt<SettingsService>()),
  );
  getIt.registerLazySingleton<FavoritesService>(
    () => FavoritesService(
      database: database,
      server: getIt<OxideServerService>(),
      pocketBase: getIt<PocketBaseService>(),
      authService: getIt<AuthService>(),
    ),
  );
  getIt.registerLazySingleton<HistoryService>(
    () => HistoryService(
      database: database,
      server: getIt<OxideServerService>(),
      pocketBase: getIt<PocketBaseService>(),
      authService: getIt<AuthService>(),
    ),
  );
  getIt.registerLazySingleton<SyncService>(() => SyncService(database));
  getIt.registerLazySingleton<DownloadService>(
    () =>
        DownloadService(database, getIt<ApiClient>(), getIt<SettingsService>()),
  );
  getIt.registerLazySingleton<VideoPlayerService>(() => VideoPlayerService());
  getIt.registerLazySingleton<StatsService>(() => StatsService(database));
  getIt.registerLazySingleton<WatchPartyService>(
    () => WatchPartyService(
      server: getIt<OxideServerService>(),
      pocketBase: getIt<PocketBaseService>(),
      settings: getIt<SettingsService>(),
    ),
  );

  getIt.registerLazySingleton<DataTransferService>(
    () =>
        DataTransferService(getIt<HistoryService>(), getIt<FavoritesService>()),
  );

  // External API services
  getIt.registerLazySingleton<TMDbService>(
    () => TMDbService(getIt<ApiClient>(), getIt<SettingsService>()),
  );
  getIt.registerLazySingleton<JikanService>(
    () => JikanService(getIt<ApiClient>()),
  );
  getIt.registerLazySingleton<TVMazeService>(
    () => TVMazeService(getIt<ApiClient>()),
  );

  // Provider registry
  getIt.registerLazySingleton<ProviderRegistry>(() => ProviderRegistry());

  // Register content providers
  _registerProviders();

  // Search service (depends on registry)
  getIt.registerLazySingleton<SearchService>(
    () => SearchService(getIt<ProviderRegistry>()),
  );

  // Smart search service (depends on SearchService and database)
  getIt.registerLazySingleton<SearchHistoryDao>(
    () => SearchHistoryDao(database),
  );
  getIt.registerLazySingleton<SmartSearchService>(
    () => SmartSearchService(
      getIt<SearchService>(),
      getIt<ProviderRegistry>(),
      getIt<SearchHistoryDao>(),
    ),
  );

  // Episode update service (depends on registry)
  getIt.registerLazySingleton<EpisodeUpdateService>(
    () => EpisodeUpdateService(database, getIt<ProviderRegistry>()),
  );

  // Recommendation service (depends on history, favorites, registry)
  getIt.registerLazySingleton<RecommendationService>(
    () => RecommendationService(
      historyService: getIt<HistoryService>(),
      favoritesService: getIt<FavoritesService>(),
      providerRegistry: getIt<ProviderRegistry>(),
      mediaItemsDao: MediaItemsDao(database),
    ),
  );

  // Unified Repository
  getIt.registerLazySingleton<UnifiedContentRepository>(
    () => UnifiedContentRepositoryImpl(
      getIt<ProviderRegistry>().all,
      HistoryDao(database),
      FavoritesDao(database),
      MediaItemsDao(database),
    ),
  );
}

void _registerProviders() {
  final registry = getIt<ProviderRegistry>();
  final apiClient = getIt<ApiClient>();

  // Ukrainian providers (shown on home page)
  registry.register(UakinoProvider(apiClient));
  registry.register(EneyidaProvider(apiClient));
  registry.register(UaflixProvider(apiClient));
  registry.register(UaserialsProvider(apiClient));

  // Anime providers (shown on home page)
  // registry.register(YummyAnimeProvider(apiClient));  // Disabled - not working properly

  // Separate providers (NOT shown on home page - have dedicated buttons)
  // HDRezka: Multi-language, fixed streams (can't change quality/voiceover after start)
  registry.register(HdrezkaProvider(apiClient, getIt<UserAgentService>()));
  // YouTube: Embed-only (ToS compliant, no direct stream extraction)
  registry.register(YouTubeProvider(apiClient));
}
