import 'package:flutter/foundation.dart';
import '../services/settings_service.dart';
import '../../core/network/api_client.dart';
import '../../core/utils/logger.dart';

/// TMDb API service for movie/TV metadata enrichment
///
/// Provides ratings, descriptions, trailers, and posters from TMDb
class TMDbService {
  static const String _tag = 'TMDb';
  static const String _baseUrl = 'https://api.themoviedb.org/3';
  static const String _imageBase = 'https://image.tmdb.org/t/p';

  // Default API key - users can override in settings
  static const String _defaultApiKey = 'YOUR_TMDB_API_KEY';

  final ApiClient _client;
  final SettingsService _settings;

  TMDbService(this._client, this._settings);

  String get _apiKey => _settings.state.tmdbApiKey ?? _defaultApiKey;

  /// Search for movies
  Future<List<TMDbSearchResult>> searchMovies(
    String query, {
    String language = 'uk-UA',
  }) async {
    try {
      final response = await _client.getJson(
        '$_baseUrl/search/movie',
        queryParameters: {
          'api_key': _apiKey,
          'query': query,
          'language': language,
          'include_adult': 'false',
        },
      );

      return _parseSearchResults(response, isMovie: true);
    } catch (e) {
      Logger.w('TMDb movie search failed: $e', tag: _tag);
      return [];
    }
  }

  /// Search for TV shows
  Future<List<TMDbSearchResult>> searchTV(
    String query, {
    String language = 'uk-UA',
  }) async {
    try {
      final response = await _client.getJson(
        '$_baseUrl/search/tv',
        queryParameters: {
          'api_key': _apiKey,
          'query': query,
          'language': language,
        },
      );

      return _parseSearchResults(response, isMovie: false);
    } catch (e) {
      Logger.w('TMDb TV search failed: $e', tag: _tag);
      return [];
    }
  }

  /// Get movie details including trailers
  Future<TMDbDetails?> getMovieDetails(
    int id, {
    String language = 'uk-UA',
  }) async {
    try {
      final response = await _client.getJson(
        '$_baseUrl/movie/$id',
        queryParameters: {
          'api_key': _apiKey,
          'language': language,
          'append_to_response': 'videos,credits',
        },
      );

      return _parseDetails(response, isMovie: true);
    } catch (e) {
      Logger.w('TMDb movie details failed: $e', tag: _tag);
      return null;
    }
  }

  /// Get TV show details including trailers
  Future<TMDbDetails?> getTVDetails(int id, {String language = 'uk-UA'}) async {
    try {
      final response = await _client.getJson(
        '$_baseUrl/tv/$id',
        queryParameters: {
          'api_key': _apiKey,
          'language': language,
          'append_to_response': 'videos,credits',
        },
      );

      return _parseDetails(response, isMovie: false);
    } catch (e) {
      Logger.w('TMDb TV details failed: $e', tag: _tag);
      return null;
    }
  }

  /// Find TMDb match for a local media item
  Future<TMDbSearchResult?> findMatch({
    required String title,
    String? originalTitle,
    int? year,
    bool isMovie = true,
  }) async {
    // Try original title first if available
    final searchQuery = originalTitle ?? title;
    final results = isMovie
        ? await searchMovies(searchQuery)
        : await searchTV(searchQuery);

    if (results.isEmpty) {
      // Fallback to localized title
      if (originalTitle != null) {
        final fallback = isMovie
            ? await searchMovies(title)
            : await searchTV(title);
        if (fallback.isNotEmpty) {
          return _bestMatch(fallback, title, year);
        }
      }
      return null;
    }

    return _bestMatch(results, title, year);
  }

  TMDbSearchResult? _bestMatch(
    List<TMDbSearchResult> results,
    String title,
    int? year,
  ) {
    if (results.isEmpty) return null;

    // Prefer exact year match
    if (year != null) {
      final yearMatch = results.where((r) => r.year == year).toList();
      if (yearMatch.isNotEmpty) return yearMatch.first;
    }

    // Return first result as fallback
    return results.first;
  }

  List<TMDbSearchResult> _parseSearchResults(
    Map<String, dynamic> response, {
    required bool isMovie,
  }) {
    final results = <TMDbSearchResult>[];
    final items = response['results'] as List? ?? [];

    for (final item in items) {
      try {
        final titleKey = isMovie ? 'title' : 'name';
        final dateKey = isMovie ? 'release_date' : 'first_air_date';

        results.add(
          TMDbSearchResult(
            id: item['id'] as int,
            title: item[titleKey]?.toString() ?? '',
            originalTitle: item[isMovie ? 'original_title' : 'original_name']
                ?.toString(),
            overview: item['overview']?.toString(),
            posterPath: item['poster_path']?.toString(),
            backdropPath: item['backdrop_path']?.toString(),
            rating: (item['vote_average'] as num?)?.toDouble(),
            voteCount: item['vote_count'] as int? ?? 0,
            year: _parseYear(item[dateKey]?.toString()),
            isMovie: isMovie,
          ),
        );
      } catch (e) {
        debugPrint('Failed to parse TMDb result: $e');
      }
    }

    return results;
  }

  TMDbDetails? _parseDetails(
    Map<String, dynamic> response, {
    required bool isMovie,
  }) {
    try {
      final titleKey = isMovie ? 'title' : 'name';
      final dateKey = isMovie ? 'release_date' : 'first_air_date';

      // Extract trailer
      String? trailerUrl;
      final videos = response['videos']?['results'] as List? ?? [];
      for (final video in videos) {
        if (video['site'] == 'YouTube' &&
            (video['type'] == 'Trailer' || video['type'] == 'Teaser')) {
          trailerUrl = 'https://www.youtube.com/watch?v=${video['key']}';
          break;
        }
      }

      // Extract genres
      final genres = <String>[];
      for (final genre in response['genres'] as List? ?? []) {
        genres.add(genre['name']?.toString() ?? '');
      }

      // Extract cast
      final cast = <TMDbCastMember>[];
      final credits = response['credits']?['cast'] as List? ?? [];
      for (var i = 0; i < credits.length && i < 10; i++) {
        final member = credits[i];
        cast.add(
          TMDbCastMember(
            id: member['id'] as int,
            name: member['name']?.toString() ?? '',
            character: member['character']?.toString(),
            profilePath: member['profile_path']?.toString(),
          ),
        );
      }

      return TMDbDetails(
        id: response['id'] as int,
        title: response[titleKey]?.toString() ?? '',
        originalTitle: response[isMovie ? 'original_title' : 'original_name']
            ?.toString(),
        overview: response['overview']?.toString(),
        posterPath: response['poster_path']?.toString(),
        backdropPath: response['backdrop_path']?.toString(),
        rating: (response['vote_average'] as num?)?.toDouble(),
        voteCount: response['vote_count'] as int? ?? 0,
        year: _parseYear(response[dateKey]?.toString()),
        runtime: isMovie ? response['runtime'] as int? : null,
        genres: genres,
        trailerUrl: trailerUrl,
        cast: cast,
        isMovie: isMovie,
      );
    } catch (e) {
      Logger.w('Failed to parse TMDb details: $e', tag: _tag);
      return null;
    }
  }

  int? _parseYear(String? date) {
    if (date == null || date.isEmpty) return null;
    return int.tryParse(date.split('-').first);
  }

  /// Get full poster URL
  String? getPosterUrl(String? path, {String size = 'w500'}) {
    if (path == null) return null;
    return '$_imageBase/$size$path';
  }

  /// Get full backdrop URL
  String? getBackdropUrl(String? path, {String size = 'w1280'}) {
    if (path == null) return null;
    return '$_imageBase/$size$path';
  }
}

/// TMDb search result
class TMDbSearchResult {
  final int id;
  final String title;
  final String? originalTitle;
  final String? overview;
  final String? posterPath;
  final String? backdropPath;
  final double? rating;
  final int voteCount;
  final int? year;
  final bool isMovie;

  const TMDbSearchResult({
    required this.id,
    required this.title,
    this.originalTitle,
    this.overview,
    this.posterPath,
    this.backdropPath,
    this.rating,
    this.voteCount = 0,
    this.year,
    this.isMovie = true,
  });
}

/// TMDb detailed information
class TMDbDetails extends TMDbSearchResult {
  final int? runtime;
  final List<String> genres;
  final String? trailerUrl;
  final List<TMDbCastMember> cast;

  const TMDbDetails({
    required super.id,
    required super.title,
    super.originalTitle,
    super.overview,
    super.posterPath,
    super.backdropPath,
    super.rating,
    super.voteCount,
    super.year,
    super.isMovie,
    this.runtime,
    this.genres = const [],
    this.trailerUrl,
    this.cast = const [],
  });
}

/// TMDb cast member
class TMDbCastMember {
  final int id;
  final String name;
  final String? character;
  final String? profilePath;

  const TMDbCastMember({
    required this.id,
    required this.name,
    this.character,
    this.profilePath,
  });
}
