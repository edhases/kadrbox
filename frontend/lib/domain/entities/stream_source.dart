import 'package:equatable/equatable.dart';

/// Type of streaming source
enum StreamType {
  direct, // Direct MP4/MKV link
  hls, // HLS .m3u8 playlist
  dash, // DASH .mpd manifest
  torrent, // Magnet link / torrent
  iframe, // HTML player page — NOT playable by libmpv, must never be opened
}

/// Quality of video stream
enum StreamQuality { q360p, q480p, q720p, q1080p, q1440p, q4k, unknown }

extension StreamQualityExtension on StreamQuality {
  /// Display name for UI
  ///
  /// Returns "Авто" for unknown quality (HLS adaptive streams)
  /// which will automatically select the best quality based on bandwidth.
  String get displayName {
    switch (this) {
      case StreamQuality.q360p:
        return '360p';
      case StreamQuality.q480p:
        return '480p';
      case StreamQuality.q720p:
        return '720p';
      case StreamQuality.q1080p:
        return '1080p';
      case StreamQuality.q1440p:
        return '1440p';
      case StreamQuality.q4k:
        return '4K';
      case StreamQuality.unknown:
        return 'Авто'; // HLS adaptive - auto quality selection
    }
  }

  int get sortOrder {
    switch (this) {
      case StreamQuality.q360p:
        return 1;
      case StreamQuality.q480p:
        return 2;
      case StreamQuality.q720p:
        return 3;
      case StreamQuality.q1080p:
        return 4;
      case StreamQuality.q1440p:
        return 5;
      case StreamQuality.q4k:
        return 6;
      case StreamQuality.unknown:
        return 0;
    }
  }
}

/// Streaming source for playback
class StreamSource extends Equatable {
  final String url;
  final StreamQuality quality;
  final StreamType type;
  final String? language;
  final String? voiceover; // Voice dubbing (e.g., "Укр", "Оригінал")
  final String? sourceName;
  final List<Subtitle>? subtitles;
  final Map<String, String>? headers; // Custom headers if needed
  final int? season; // Season number for series
  final int? episode; // Episode number for series
  final String? episodeTitle; // Episode title

  const StreamSource({
    required this.url,
    this.quality = StreamQuality.unknown,
    this.type = StreamType.direct,
    this.language,
    this.voiceover,
    this.sourceName,
    this.subtitles,
    this.headers,
    this.season,
    this.episode,
    this.episodeTitle,
  });

  /// Create a copy with different parameters
  StreamSource copyWith({
    String? url,
    StreamQuality? quality,
    StreamType? type,
    String? language,
    String? voiceover,
    String? sourceName,
    List<Subtitle>? subtitles,
    Map<String, String>? headers,
    int? season,
    int? episode,
    String? episodeTitle,
  }) {
    return StreamSource(
      url: url ?? this.url,
      quality: quality ?? this.quality,
      type: type ?? this.type,
      language: language ?? this.language,
      voiceover: voiceover ?? this.voiceover,
      sourceName: sourceName ?? this.sourceName,
      subtitles: subtitles ?? this.subtitles,
      headers: headers ?? this.headers,
      season: season ?? this.season,
      episode: episode ?? this.episode,
      episodeTitle: episodeTitle ?? this.episodeTitle,
    );
  }

  @override
  List<Object?> get props => [
    url,
    quality,
    type,
    language,
    voiceover,
    sourceName,
    subtitles,
    headers,
    season,
    episode,
    episodeTitle,
  ];
}

/// Subtitle track
class Subtitle extends Equatable {
  final String url;
  final String language;
  final String? label;
  final SubtitleFormat format;

  const Subtitle({
    required this.url,
    required this.language,
    this.label,
    this.format = SubtitleFormat.srt,
  });

  @override
  List<Object?> get props => [url, language, label, format];
}

enum SubtitleFormat { srt, vtt, ass }
