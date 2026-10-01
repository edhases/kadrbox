// Common mock implementations for services used in tests
// Uses mocktail (preferred) or mockito. Register mocks into GetIt or pass to constructors.

import 'package:mocktail/mocktail.dart';
import 'package:media_kit/media_kit.dart';
import 'package:oxide_film/data/services/history_service.dart';
import 'package:oxide_film/data/services/favorites_service.dart';
import 'package:oxide_film/data/services/download_service.dart';
import 'package:oxide_film/data/services/settings_service.dart';
import 'package:oxide_film/data/services/watch_party_service.dart';
import 'package:oxide_film/core/network/api_client.dart';
import 'package:oxide_film/data/services/recommendation_service.dart';
import 'package:oxide_film/data/services/episode_update_service.dart';
import 'package:oxide_film/data/services/user_agent_service.dart';

class MockHistoryService extends Mock implements HistoryService {}

class MockFavoritesService extends Mock implements FavoritesService {}

class MockDownloadService extends Mock implements DownloadService {}

class MockSettingsService extends Mock implements SettingsService {}

class MockWatchPartyService extends Mock implements WatchPartyService {}

class MockApiClient extends Mock implements ApiClient {}

class MockRecommendationService extends Mock implements RecommendationService {}

class MockEpisodeUpdateService extends Mock implements EpisodeUpdateService {}

class MockUserAgentService extends Mock implements UserAgentService {}

// Additional lightweight stubs can be added here as tests expand.

// Mock Player used in PlayerController tests
class MockPlayer extends Mock implements Player {}

class FakePlayerState {
  final bool playing;
  FakePlayerState(this.playing);
}
