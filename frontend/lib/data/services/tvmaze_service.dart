import '../../core/network/api_client.dart';
import '../../core/utils/logger.dart';

/// TVMaze API service for TV show schedules
///
/// Free API, no authentication required
/// API docs: https://www.tvmaze.com/api
class TVMazeService {
  static const String _tag = 'TVMaze';
  static const String _baseUrl = 'https://api.tvmaze.com';

  final ApiClient _client;

  TVMazeService(this._client);

  /// Search for TV shows
  Future<List<TVMazeShow>> searchShows(String query) async {
    try {
      final response = await _client.dio.get<List<dynamic>>(
        '$_baseUrl/search/shows',
        queryParameters: {'q': query},
      );

      final results = <TVMazeShow>[];
      for (final item in response.data ?? []) {
        final show = item['show'] as Map<String, dynamic>?;
        if (show != null) {
          results.add(_parseShow(show));
        }
      }

      return results;
    } catch (e) {
      Logger.w('TVMaze search failed: $e', tag: _tag);
      return [];
    }
  }

  /// Get show details
  Future<TVMazeShow?> getShowDetails(int id) async {
    try {
      final response = await _client.getJson('$_baseUrl/shows/$id');
      return _parseShow(response);
    } catch (e) {
      Logger.w('TVMaze show details failed: $e', tag: _tag);
      return null;
    }
  }

  /// Get episodes for a show
  Future<List<TVMazeEpisode>> getEpisodes(int showId) async {
    try {
      final response = await _client.dio.get<List<dynamic>>(
        '$_baseUrl/shows/$showId/episodes',
      );

      final episodes = <TVMazeEpisode>[];
      for (final item in response.data ?? []) {
        episodes.add(_parseEpisode(item as Map<String, dynamic>));
      }

      return episodes;
    } catch (e) {
      Logger.w('TVMaze episodes failed: $e', tag: _tag);
      return [];
    }
  }

  /// Get today's schedule (shows airing today)
  Future<List<TVMazeScheduleItem>> getTodaySchedule({
    String country = 'US',
  }) async {
    try {
      final response = await _client.dio.get<List<dynamic>>(
        '$_baseUrl/schedule',
        queryParameters: {'country': country},
      );

      final schedule = <TVMazeScheduleItem>[];
      for (final item in response.data ?? []) {
        schedule.add(_parseScheduleItem(item as Map<String, dynamic>));
      }

      return schedule;
    } catch (e) {
      Logger.w('TVMaze schedule failed: $e', tag: _tag);
      return [];
    }
  }

  /// Get full schedule for a date range
  Future<List<TVMazeScheduleItem>> getSchedule(DateTime date) async {
    try {
      final dateStr =
          '${date.year}-${date.month.toString().padLeft(2, '0')}-${date.day.toString().padLeft(2, '0')}';

      final response = await _client.dio.get<List<dynamic>>(
        '$_baseUrl/schedule',
        queryParameters: {'date': dateStr},
      );

      final schedule = <TVMazeScheduleItem>[];
      for (final item in response.data ?? []) {
        schedule.add(_parseScheduleItem(item as Map<String, dynamic>));
      }

      return schedule;
    } catch (e) {
      Logger.w('TVMaze schedule failed: $e', tag: _tag);
      return [];
    }
  }

  /// Get upcoming episodes for a specific show
  Future<TVMazeEpisode?> getNextEpisode(int showId) async {
    try {
      final show = await getShowDetails(showId);
      if (show == null) return null;

      final episodes = await getEpisodes(showId);
      final now = DateTime.now();

      // Find next upcoming episode
      for (final ep in episodes) {
        if (ep.airdate != null) {
          final airDate = DateTime.tryParse(ep.airdate!);
          if (airDate != null && airDate.isAfter(now)) {
            return ep;
          }
        }
      }

      return null;
    } catch (e) {
      Logger.w('TVMaze next episode failed: $e', tag: _tag);
      return null;
    }
  }

  /// Find TVMaze match for a local show
  Future<TVMazeShow?> findMatch({
    required String title,
    String? originalTitle,
    int? year,
  }) async {
    final searchQuery = originalTitle ?? title;
    final results = await searchShows(searchQuery);

    if (results.isEmpty && originalTitle != null) {
      final fallback = await searchShows(title);
      if (fallback.isNotEmpty) {
        return _bestMatch(fallback, title, year);
      }
      return null;
    }

    return _bestMatch(results, title, year);
  }

  TVMazeShow? _bestMatch(List<TVMazeShow> results, String title, int? year) {
    if (results.isEmpty) return null;

    if (year != null) {
      final yearMatch = results
          .where((r) => r.premiered?.startsWith('$year') ?? false)
          .toList();
      if (yearMatch.isNotEmpty) return yearMatch.first;
    }

    return results.first;
  }

  TVMazeShow _parseShow(Map<String, dynamic> data) {
    final image = data['image'] as Map<String, dynamic>? ?? {};
    final network = data['network'] as Map<String, dynamic>?;
    final schedule = data['schedule'] as Map<String, dynamic>? ?? {};

    // Parse genres
    final genres = <String>[];
    for (final genre in data['genres'] as List? ?? []) {
      genres.add(genre.toString());
    }

    return TVMazeShow(
      id: data['id'] as int? ?? 0,
      name: data['name']?.toString() ?? '',
      summary: _cleanHtml(data['summary']?.toString()),
      imageUrl: image['medium']?.toString() ?? image['original']?.toString(),
      rating: (data['rating']?['average'] as num?)?.toDouble(),
      premiered: data['premiered']?.toString(),
      ended: data['ended']?.toString(),
      status: data['status']?.toString(),
      type: data['type']?.toString(),
      language: data['language']?.toString(),
      genres: genres,
      network: network?['name']?.toString(),
      scheduleTime: schedule['time']?.toString(),
      scheduleDays:
          (schedule['days'] as List?)?.map((e) => e.toString()).toList() ?? [],
      officialSite: data['officialSite']?.toString(),
    );
  }

  TVMazeEpisode _parseEpisode(Map<String, dynamic> data) {
    final image = data['image'] as Map<String, dynamic>? ?? {};

    return TVMazeEpisode(
      id: data['id'] as int? ?? 0,
      name: data['name']?.toString() ?? '',
      season: data['season'] as int? ?? 0,
      number: data['number'] as int? ?? 0,
      airdate: data['airdate']?.toString(),
      airtime: data['airtime']?.toString(),
      runtime: data['runtime'] as int?,
      summary: _cleanHtml(data['summary']?.toString()),
      imageUrl: image['medium']?.toString() ?? image['original']?.toString(),
    );
  }

  TVMazeScheduleItem _parseScheduleItem(Map<String, dynamic> data) {
    final show = data['show'] as Map<String, dynamic>? ?? {};

    return TVMazeScheduleItem(
      episode: _parseEpisode(data),
      show: _parseShow(show),
    );
  }

  String? _cleanHtml(String? html) {
    if (html == null) return null;
    return html.replaceAll(RegExp(r'<[^>]*>'), '').trim();
  }
}

/// TVMaze show
class TVMazeShow {
  final int id;
  final String name;
  final String? summary;
  final String? imageUrl;
  final double? rating;
  final String? premiered;
  final String? ended;
  final String? status;
  final String? type;
  final String? language;
  final List<String> genres;
  final String? network;
  final String? scheduleTime;
  final List<String> scheduleDays;
  final String? officialSite;

  const TVMazeShow({
    required this.id,
    required this.name,
    this.summary,
    this.imageUrl,
    this.rating,
    this.premiered,
    this.ended,
    this.status,
    this.type,
    this.language,
    this.genres = const [],
    this.network,
    this.scheduleTime,
    this.scheduleDays = const [],
    this.officialSite,
  });
}

/// TVMaze episode
class TVMazeEpisode {
  final int id;
  final String name;
  final int season;
  final int number;
  final String? airdate;
  final String? airtime;
  final int? runtime;
  final String? summary;
  final String? imageUrl;

  const TVMazeEpisode({
    required this.id,
    required this.name,
    required this.season,
    required this.number,
    this.airdate,
    this.airtime,
    this.runtime,
    this.summary,
    this.imageUrl,
  });

  /// Get formatted episode number (S01E05)
  String get episodeCode =>
      'S${season.toString().padLeft(2, '0')}E${number.toString().padLeft(2, '0')}';
}

/// TVMaze schedule item
class TVMazeScheduleItem {
  final TVMazeEpisode episode;
  final TVMazeShow show;

  const TVMazeScheduleItem({required this.episode, required this.show});
}
