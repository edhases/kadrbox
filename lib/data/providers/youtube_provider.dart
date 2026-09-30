import 'package:youtube_explode_dart/youtube_explode_dart.dart' as yt;

import '../../core/network/api_client.dart';
import '../../domain/entities/entities.dart';
import '../../domain/repositories/content_provider.dart';

/// YouTube content provider adapter (Embed/Player only, ToS compliant)
class YouTubeProvider implements ContentProvider {
  final ApiClient _client;
  bool _isEnabled = false;

  /// Whether to show this provider on the home page (false for YouTube)
  static const bool showOnHome = false;

  /// Whether browsing requires search
  static const bool requiresSearch = true;

  YouTubeProvider(this._client);

  @override
  String get id => 'youtube';

  @override
  String get name => 'YouTube';

  @override
  String get baseUrl => 'https://www.youtube.com';

  @override
  String get effectiveBaseUrl => baseUrl;

  @override
  String? get iconUrl => 'https://www.youtube.com/favicon.ico';

  @override
  bool get isEnabled => _isEnabled;

  set isEnabled(bool value) => _isEnabled = value;

  @override
  List<ContentType> get supportedTypes => [
    ContentType.movie,
    ContentType.series,
    ContentType.cartoon,
    ContentType.anime,
  ];

  @override
  Future<List<MediaItem>> search(
    String query, {
    ContentType? type,
    int page = 1,
  }) async {
    final youtube = yt.YoutubeExplode();
    try {
      final results = await youtube.search.search(query);
      return results.map((video) {
        return MediaItem(
          id: video.id.value,
          providerId: id,
          title: video.title,
          posterUrl: video.thumbnails.highResUrl,
          year: video.uploadDate?.year,
          type: ContentType.movie,
        );
      }).toList();
    } catch (_) {
      return [];
    } finally {
      youtube.close();
    }
  }

  @override
  Future<List<MediaItem>> getPopular({ContentType? type, int page = 1}) async =>
      [];

  @override
  Future<List<MediaItem>> getNew({ContentType? type, int page = 1}) async => [];

  @override
  Future<List<String>> getCategories() async => [
    'Фільми',
    'Музика',
    'Ігри',
    'Новини',
    'Спорт',
    'Освіта',
    'Розваги',
  ];

  @override
  Future<List<MediaItem>> getByCategory(
    String category, {
    ContentType? type,
    int page = 1,
  }) => search(category, page: page);

  @override
  Future<MediaDetails> getDetails(String id) async {
    final youtube = yt.YoutubeExplode();
    try {
      final video = await youtube.videos.get(id);
      return MediaDetails(
        item: MediaItem(
          id: video.id.value,
          providerId: this.id,
          title: video.title,
          posterUrl: video.thumbnails.highResUrl,
          year: video.uploadDate?.year,
          type: ContentType.movie,
        ),
        fullDescription: video.description,
        director: video.author,
      );
    } finally {
      youtube.close();
    }
  }

  @override
  Future<List<MediaItem>> getSimilar(String id, MediaDetails details) async =>
      [];

  @override
  Future<List<StreamSource>> getStreams(
    String id, {
    int? season,
    int? episode,
  }) async {
    return [
      StreamSource(
        url: id,
        quality: StreamQuality.unknown,
        type: StreamType.youtubeEmbed,
        voiceover: 'YouTube',
        sourceName: 'YouTube',
      ),
    ];
  }
}
