import 'package:get_it/get_it.dart';
import 'package:injectable/injectable.dart';
import 'package:shared_preferences/shared_preferences.dart';
import '../network/api_client.dart';
import '../../data/providers/provider_registry.dart';
import '../../data/providers/youtube_provider.dart';
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
import '../../data/services/search_service.dart';
import '../../data/services/url_resolver_service.dart';
import '../../data/services/recommendation_service.dart';
import '../../data/services/smart_search/smart_search_service.dart';
import '../../data/services/data_transfer_service.dart';
import '../../data/services/user_agent_service.dart';
import '../../data/services/provider_catalog_service.dart';
import '../../data/services/update_service.dart';
import '../../data/database/dao/search_history_dao.dart';
import '../../data/database/dao/media_items_dao.dart';
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

  // Database (must be early)
  final database = AppDatabase();
  getIt.registerSingleton<AppDatabase>(database);

  // URL Resolver for auto-detecting domain changes
  getIt.registerLazySingleton<UrlResolverService>(
    () => UrlResolverService(getIt<SharedPreferences>()),
  );

  // Auth service (Oxide Server)
  getIt.registerLazySingleton<AuthService>(
    () => AuthService(getIt<OxideServerService>()),
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
      authService: getIt<AuthService>(),
    ),
  );
  getIt.registerLazySingleton<HistoryService>(
    () => HistoryService(
      database: database,
      server: getIt<OxideServerService>(),
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

  // Backend provider catalog sync (source of truth for provider list)
  getIt.registerLazySingleton<ProviderCatalogService>(
    () => ProviderCatalogService(),
  );

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
}

void _registerProviders() {
  final registry = getIt<ProviderRegistry>();
  final apiClient = getIt<ApiClient>();

  // YouTube: Embed-only client-side provider (ToS compliant, no direct stream extraction)
  registry.register(YouTubeProvider(apiClient));
}
