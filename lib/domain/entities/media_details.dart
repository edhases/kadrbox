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
  });

  bool get isSeries => seasons != null && seasons!.isNotEmpty;

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
    );
  }
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
