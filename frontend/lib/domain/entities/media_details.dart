import 'package:equatable/equatable.dart';
import 'media_item.dart';

/// Detailed media information including seasons/episodes
class MediaDetails extends Equatable {
  final MediaItem item;
  final String? fullDescription;
  final List<String>? genres;
  final List<String>? countries;
  final String? director;
  final List<String>? actors;
  final Duration? duration;
  final String? trailerUrl;
  final List<Season>? seasons; // For series/anime
  final double? imdbRating;

  /// Dubbing studios reported by the backend, each with its own season tree.
  ///
  /// Never null: backends that predate this field simply report nothing, and
  /// an empty list keeps every call site free of null handling.
  final List<Voiceover> voiceovers;

  const MediaDetails({
    required this.item,
    this.fullDescription,
    this.genres,
    this.countries,
    this.director,
    this.actors,
    this.duration,
    this.trailerUrl,
    this.seasons,
    this.imdbRating,
    this.voiceovers = const [],
  });

  /// True when the item has more than one playable part.
  ///
  /// A non-empty [seasons] list is NOT enough. DLE-style providers describe a
  /// movie as a single season holding a single "episode", and treating that
  /// as a series renders an episode picker over a film. A series is therefore
  /// either several seasons, or one season with several episodes.
  ///
  /// A genuine one-part miniseries is the rare exception the declared content
  /// type covers — but only as a fallback, because DLE providers mislabel
  /// their series as `movie` and are exactly the case this must not trust.
  bool get isSeries {
    final list = seasons;
    if (list == null || list.isEmpty) return false;
    if (list.length > 1) return true;
    if (list.first.episodes.length > 1) return true;
    return item.type == ContentType.series ||
        item.type == ContentType.dorama ||
        item.type == ContentType.cartoon ||
        item.type == ContentType.anime;
  }

  /// True when any season carries at least one episode.
  bool get hasEpisodes {
    final list = seasons;
    if (list == null) return false;
    for (final season in list) {
      if (season.episodes.isNotEmpty) return true;
    }
    return false;
  }

  /// True when the backend reported dubbing studios with their own seasons.
  bool get hasVoiceovers => voiceovers.isNotEmpty;

  /// First voiceover, or null when the backend reported none.
  ///
  /// The backend sorts voiceovers by dub weight, so the first entry is the
  /// preferred studio.
  Voiceover? get primaryVoiceover {
    if (voiceovers.isEmpty) return null;
    for (final voiceover in voiceovers) {
      if (voiceover.seasons.isNotEmpty) return voiceover;
    }
    return voiceovers.first;
  }

  @override
  List<Object?> get props => [
    item,
    fullDescription,
    genres,
    countries,
    director,
    actors,
    duration,
    trailerUrl,
    seasons,
    imdbRating,
    voiceovers,
  ];

  MediaDetails copyWith({
    MediaItem? item,
    String? fullDescription,
    List<String>? genres,
    List<String>? countries,
    String? director,
    List<String>? actors,
    Duration? duration,
    String? trailerUrl,
    List<Season>? seasons,
    double? imdbRating,
    List<Voiceover>? voiceovers,
  }) {
    return MediaDetails(
      item: item ?? this.item,
      fullDescription: fullDescription ?? this.fullDescription,
      genres: genres ?? this.genres,
      countries: countries ?? this.countries,
      director: director ?? this.director,
      actors: actors ?? this.actors,
      duration: duration ?? this.duration,
      trailerUrl: trailerUrl ?? this.trailerUrl,
      seasons: seasons ?? this.seasons,
      imdbRating: imdbRating ?? this.imdbRating,
      voiceovers: voiceovers ?? this.voiceovers,
    );
  }
}

/// A dubbing studio ("1+1", "Postmodern") and the seasons it provides.
///
/// Distinct from the CDN/player that actually serves the bytes (a host
/// name): [id] is the studio identifier the backend uses to resolve
/// playback, [name] is what the user reads.
class Voiceover extends Equatable {
  final String id;
  final String name;
  final List<Season> seasons;

  const Voiceover({
    required this.id,
    required this.name,
    this.seasons = const [],
  });

  /// Total episodes across every season of this voiceover.
  int get episodeCount {
    var total = 0;
    for (final season in seasons) {
      total += season.episodes.length;
    }
    return total;
  }

  /// Season with [number], or null when this voiceover does not have it.
  Season? seasonByNumber(int number) {
    for (final season in seasons) {
      if (season.number == number) return season;
    }
    return null;
  }

  @override
  List<Object?> get props => [id, name, seasons];
}

/// Season of a series
class Season extends Equatable {
  final int number;
  final String? title;
  final List<Episode> episodes;

  const Season({required this.number, this.title, required this.episodes});

  @override
  List<Object?> get props => [number, title, episodes];
}

/// Episode of a season
class Episode extends Equatable {
  final int number;
  final String? title;
  final String? thumbnailUrl;
  final Duration? duration;
  final String? streamRef;

  const Episode({
    required this.number,
    this.title,
    this.thumbnailUrl,
    this.duration,
    this.streamRef,
  });

  @override
  List<Object?> get props => [number, title, thumbnailUrl, duration, streamRef];
}
