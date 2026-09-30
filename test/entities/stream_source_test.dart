import 'package:flutter_test/flutter_test.dart';

import 'package:oxide_film/domain/entities/stream_source.dart';

void main() {
  group('StreamQuality', () {
    test('displayName should return correct strings', () {
      expect(StreamQuality.q360p.displayName, '360p');
      expect(StreamQuality.q480p.displayName, '480p');
      expect(StreamQuality.q720p.displayName, '720p');
      expect(StreamQuality.q1080p.displayName, '1080p');
      expect(StreamQuality.q1440p.displayName, '1440p');
      expect(StreamQuality.q4k.displayName, '4K');
      expect(StreamQuality.unknown.displayName, 'Авто');
    });

    test('sortOrder should be in ascending quality order', () {
      expect(StreamQuality.unknown.sortOrder, 0);
      expect(StreamQuality.q360p.sortOrder, 1);
      expect(StreamQuality.q480p.sortOrder, 2);
      expect(StreamQuality.q720p.sortOrder, 3);
      expect(StreamQuality.q1080p.sortOrder, 4);
      expect(StreamQuality.q1440p.sortOrder, 5);
      expect(StreamQuality.q4k.sortOrder, 6);
    });

    test('qualities should be sortable by sortOrder', () {
      final qualities = [
        StreamQuality.q1080p,
        StreamQuality.q360p,
        StreamQuality.q720p,
        StreamQuality.unknown,
      ];

      qualities.sort((a, b) => a.sortOrder.compareTo(b.sortOrder));

      expect(qualities[0], StreamQuality.unknown);
      expect(qualities[1], StreamQuality.q360p);
      expect(qualities[2], StreamQuality.q720p);
      expect(qualities[3], StreamQuality.q1080p);
    });
  });

  group('StreamSource', () {
    test('should create with required parameters', () {
      const source = StreamSource(url: 'https://example.com/video.mp4');

      expect(source.url, 'https://example.com/video.mp4');
      expect(source.quality, StreamQuality.unknown);
      expect(source.type, StreamType.direct);
    });

    test('should create with all parameters', () {
      const source = StreamSource(
        url: 'https://example.com/video.m3u8',
        quality: StreamQuality.q1080p,
        type: StreamType.hls,
        language: 'uk',
        voiceover: 'Студія Дубляж',
        sourceName: 'CDN1',
        season: 1,
        episode: 5,
        episodeTitle: 'Перша серія',
      );

      expect(source.url, 'https://example.com/video.m3u8');
      expect(source.quality, StreamQuality.q1080p);
      expect(source.type, StreamType.hls);
      expect(source.language, 'uk');
      expect(source.voiceover, 'Студія Дубляж');
      expect(source.season, 1);
      expect(source.episode, 5);
      expect(source.episodeTitle, 'Перша серія');
    });

    test('copyWith should create new instance with changed values', () {
      const original = StreamSource(
        url: 'https://example.com/video.mp4',
        quality: StreamQuality.q720p,
        voiceover: 'Original',
      );

      final modified = original.copyWith(
        quality: StreamQuality.q1080p,
        voiceover: 'Modified',
      );

      // Original unchanged
      expect(original.quality, StreamQuality.q720p);
      expect(original.voiceover, 'Original');

      // Modified has new values
      expect(modified.quality, StreamQuality.q1080p);
      expect(modified.voiceover, 'Modified');
      expect(modified.url, original.url); // URL unchanged
    });

    test('equality should work correctly', () {
      const source1 = StreamSource(
        url: 'https://example.com/video.mp4',
        quality: StreamQuality.q720p,
      );

      const source2 = StreamSource(
        url: 'https://example.com/video.mp4',
        quality: StreamQuality.q720p,
      );

      const source3 = StreamSource(
        url: 'https://example.com/different.mp4',
        quality: StreamQuality.q720p,
      );

      expect(source1, equals(source2));
      expect(source1, isNot(equals(source3)));
    });
  });

  group('StreamType', () {
    test('should have correct enum values', () {
      expect(StreamType.values, contains(StreamType.direct));
      expect(StreamType.values, contains(StreamType.hls));
      expect(StreamType.values, contains(StreamType.dash));
      expect(StreamType.values, contains(StreamType.torrent));
      expect(StreamType.values, contains(StreamType.iframe));
    });

    test('iframe marks a source as an unplayable HTML page', () {
      // `iframe` is a distinct member rather than a flag on `direct` so a
      // player page can never be mistaken for media by a switch statement.
      const page = StreamSource(
        url: 'https://example.com/embed/abc',
        type: StreamType.iframe,
      );
      expect(page.type, StreamType.iframe);
      expect(page.type, isNot(StreamType.direct));
      expect(page.type, isNot(StreamType.hls));
    });

    test('iframe participates in copyWith and equality', () {
      const original = StreamSource(
        url: 'https://example.com/watch/1',
        type: StreamType.direct,
      );
      final asIframe = original.copyWith(type: StreamType.iframe);

      expect(original.type, StreamType.direct);
      expect(asIframe.type, StreamType.iframe);
      expect(asIframe, isNot(equals(original)));
      expect(asIframe, equals(original.copyWith(type: StreamType.iframe)));
    });
  });
}
