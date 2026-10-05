import '../../core/network/api_client.dart';
import '../../core/utils/logger.dart';

/// Jikan API service for anime metadata from MyAnimeList
///
/// Free API, no authentication required
/// API docs: https://docs.api.jikan.moe/
class JikanService {
  static const String _tag = 'Jikan';
  static const String _baseUrl = 'https://api.jikan.moe/v4';

  final ApiClient _client;

  JikanService(this._client);

  /// Search for anime
  Future<List<JikanAnime>> searchAnime(String query, {int page = 1}) async {
    try {
      final response = await _client.getJson(
        '$_baseUrl/anime',
        queryParameters: {'q': query, 'page': page.toString(), 'sfw': 'true'},
      );

      return _parseAnimeList(response);
    } catch (e) {
      Logger.w('Jikan anime search failed: $e', tag: _tag);
      return [];
    }
  }

  /// Get anime details by MAL ID
  Future<JikanAnimeDetails?> getAnimeDetails(int malId) async {
    try {
      final response = await _client.getJson('$_baseUrl/anime/$malId/full');
      final data = response['data'] as Map<String, dynamic>?;

      if (data == null) return null;
      return _parseAnimeDetails(data);
    } catch (e) {
      Logger.w('Jikan anime details failed: $e', tag: _tag);
      return null;
    }
  }

  /// Get top anime
  Future<List<JikanAnime>> getTopAnime({
    String filter = 'airing',
    int page = 1,
  }) async {
    try {
      final response = await _client.getJson(
        '$_baseUrl/top/anime',
        queryParameters: {
          'filter': filter,
          'page': page.toString(),
          'sfw': 'true',
        },
      );

      return _parseAnimeList(response);
    } catch (e) {
      Logger.w('Jikan top anime failed: $e', tag: _tag);
      return [];
    }
  }

  /// Get seasonal anime
  Future<List<JikanAnime>> getSeasonalAnime({int? year, String? season}) async {
    try {
      final now = DateTime.now();
      final y = year ?? now.year;
      final s = season ?? _getCurrentSeason();

      final response = await _client.getJson(
        '$_baseUrl/seasons/$y/$s',
        queryParameters: {'sfw': 'true'},
      );

      return _parseAnimeList(response);
    } catch (e) {
      Logger.w('Jikan seasonal anime failed: $e', tag: _tag);
      return [];
    }
  }

  /// Get anime recommendations based on a MAL ID
  Future<List<JikanAnime>> getRecommendations(int malId) async {
    try {
      final response = await _client.getJson(
        '$_baseUrl/anime/$malId/recommendations',
      );
      final data = response['data'] as List? ?? [];

      final results = <JikanAnime>[];
      for (final item in data) {
        final entry = item['entry'] as Map<String, dynamic>?;
        if (entry != null) {
          results.add(_parseAnime(entry));
        }
      }

      return results;
    } catch (e) {
      Logger.w('Jikan recommendations failed: $e', tag: _tag);
      return [];
    }
  }

  /// Find MAL match for a local anime
  Future<JikanAnime?> findMatch({
    required String title,
    String? originalTitle,
    int? year,
  }) async {
    // Try original title first
    final searchQuery = originalTitle ?? title;
    final results = await searchAnime(searchQuery);

    if (results.isEmpty && originalTitle != null) {
      // Fallback to localized title
      final fallback = await searchAnime(title);
      if (fallback.isNotEmpty) {
        return _bestMatch(fallback, title, year);
      }
      return null;
    }

    return _bestMatch(results, title, year);
  }

  JikanAnime? _bestMatch(List<JikanAnime> results, String title, int? year) {
    if (results.isEmpty) return null;

    // Prefer exact year match
    if (year != null) {
      final yearMatch = results.where((r) => r.year == year).toList();
      if (yearMatch.isNotEmpty) return yearMatch.first;
    }

    return results.first;
  }

  List<JikanAnime> _parseAnimeList(Map<String, dynamic> response) {
    final data = response['data'] as List? ?? [];
    return data
        .map((item) => _parseAnime(item as Map<String, dynamic>))
        .toList();
  }

  JikanAnime _parseAnime(Map<String, dynamic> data) {
    final images = data['images'] as Map<String, dynamic>? ?? {};
    final jpg = images['jpg'] as Map<String, dynamic>? ?? {};

    final aired = data['aired'] as Map<String, dynamic>? ?? {};
    final from = aired['from']?.toString();

    return JikanAnime(
      malId: data['mal_id'] as int? ?? 0,
      title: data['title']?.toString() ?? '',
      titleEnglish: data['title_english']?.toString(),
      titleJapanese: data['title_japanese']?.toString(),
      synopsis: data['synopsis']?.toString(),
      imageUrl:
          jpg['large_image_url']?.toString() ?? jpg['image_url']?.toString(),
      score: (data['score'] as num?)?.toDouble(),
      scoredBy: data['scored_by'] as int? ?? 0,
      rank: data['rank'] as int?,
      popularity: data['popularity'] as int?,
      episodes: data['episodes'] as int?,
      status: data['status']?.toString(),
      type: data['type']?.toString(),
      year: from != null ? int.tryParse(from.split('-').first) : null,
    );
  }

  JikanAnimeDetails _parseAnimeDetails(Map<String, dynamic> data) {
    final base = _parseAnime(data);

    // Parse genres
    final genres = <String>[];
    for (final genre in data['genres'] as List? ?? []) {
      genres.add(genre['name']?.toString() ?? '');
    }

    // Parse studios
    final studios = <String>[];
    for (final studio in data['studios'] as List? ?? []) {
      studios.add(studio['name']?.toString() ?? '');
    }

    // Parse themes
    final themes = <String>[];
    for (final theme in data['themes'] as List? ?? []) {
      themes.add(theme['name']?.toString() ?? '');
    }

    // Get trailer
    final trailer = data['trailer'] as Map<String, dynamic>? ?? {};
    final trailerUrl = trailer['url']?.toString();

    return JikanAnimeDetails(
      malId: base.malId,
      title: base.title,
      titleEnglish: base.titleEnglish,
      titleJapanese: base.titleJapanese,
      synopsis: base.synopsis,
      imageUrl: base.imageUrl,
      score: base.score,
      scoredBy: base.scoredBy,
      rank: base.rank,
      popularity: base.popularity,
      episodes: base.episodes,
      status: base.status,
      type: base.type,
      year: base.year,
      genres: genres,
      studios: studios,
      themes: themes,
      trailerUrl: trailerUrl,
      duration: data['duration']?.toString(),
      rating: data['rating']?.toString(),
      source: data['source']?.toString(),
      season: data['season']?.toString(),
      airing: data['airing'] as bool? ?? false,
    );
  }

  String _getCurrentSeason() {
    final month = DateTime.now().month;
    if (month <= 3) return 'winter';
    if (month <= 6) return 'spring';
    if (month <= 9) return 'summer';
    return 'fall';
  }
}

/// Jikan anime basic info
class JikanAnime {
  final int malId;
  final String title;
  final String? titleEnglish;
  final String? titleJapanese;
  final String? synopsis;
  final String? imageUrl;
  final double? score;
  final int scoredBy;
  final int? rank;
  final int? popularity;
  final int? episodes;
  final String? status;
  final String? type;
  final int? year;

  const JikanAnime({
    required this.malId,
    required this.title,
    this.titleEnglish,
    this.titleJapanese,
    this.synopsis,
    this.imageUrl,
    this.score,
    this.scoredBy = 0,
    this.rank,
    this.popularity,
    this.episodes,
    this.status,
    this.type,
    this.year,
  });
}

/// Jikan anime detailed info
class JikanAnimeDetails extends JikanAnime {
  final List<String> genres;
  final List<String> studios;
  final List<String> themes;
  final String? trailerUrl;
  final String? duration;
  final String? rating;
  final String? source;
  final String? season;
  final bool airing;

  const JikanAnimeDetails({
    required super.malId,
    required super.title,
    super.titleEnglish,
    super.titleJapanese,
    super.synopsis,
    super.imageUrl,
    super.score,
    super.scoredBy,
    super.rank,
    super.popularity,
    super.episodes,
    super.status,
    super.type,
    super.year,
    this.genres = const [],
    this.studios = const [],
    this.themes = const [],
    this.trailerUrl,
    this.duration,
    this.rating,
    this.source,
    this.season,
    this.airing = false,
  });
}
