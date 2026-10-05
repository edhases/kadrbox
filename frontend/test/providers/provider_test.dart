import 'package:flutter_test/flutter_test.dart';

import 'package:oxide_film/data/models/provider_catalog.dart';
import 'package:oxide_film/data/providers/server_backed_provider.dart';
import 'package:oxide_film/domain/entities/entities.dart';

void main() {
  group('ServerBackedProvider', () {
    late ServerBackedProvider provider;

    setUp(() {
      provider = ServerBackedProvider(
        const ProviderCatalogEntry(
          id: 'uakino',
          name: 'UAKino',
          baseUrl: 'https://uakino.biz',
          showOnHome: true,
          hasFixedStreams: false,
          contentTypes: ['movie', 'series', 'cartoon', 'anime'],
          searchEnabledDefault: true,
          enabled: true,
          healthy: true,
        ),
      );
    });

    test('should have correct metadata', () {
      expect(provider.id, 'uakino');
      expect(provider.name, 'UAKino');
      expect(provider.baseUrl, 'https://uakino.biz');
      expect(provider.supportedTypes, contains(ContentType.movie));
      expect(provider.supportedTypes, contains(ContentType.series));
      expect(provider.supportedTypes, contains(ContentType.cartoon));
      expect(provider.supportedTypes, contains(ContentType.anime));
    });

    test('should be enabled by default', () {
      expect(provider.isEnabled, isTrue);
    });
  });

  group('ServerBackedProvider stream type detection', () {
    late ServerBackedProvider provider;

    setUp(() {
      provider = ServerBackedProvider(
        const ProviderCatalogEntry(
          id: 'uakino',
          name: 'UAKino',
          baseUrl: 'https://uakino.biz',
          showOnHome: true,
          hasFixedStreams: false,
          contentTypes: ['movie'],
          searchEnabledDefault: true,
          enabled: true,
          healthy: true,
        ),
      );
    });

    test('detects HLS playlists by path extension', () {
      expect(
        provider.mapStreamTypeForTest('https://cdn.tv/a/master.m3u8'),
        StreamType.hls,
      );
      // Query string must not defeat the path check.
      expect(
        provider.mapStreamTypeForTest('https://cdn.tv/a/master.m3u8?token=abc'),
        StreamType.hls,
      );
      // Uppercase extensions from a sloppy backend still work.
      expect(
        provider.mapStreamTypeForTest('https://cdn.tv/a/MASTER.M3U8'),
        StreamType.hls,
      );
    });

    test('detects DASH manifests by path extension', () {
      expect(
        provider.mapStreamTypeForTest('https://cdn.tv/a/manifest.mpd'),
        StreamType.dash,
      );
      expect(
        provider.mapStreamTypeForTest('https://cdn.tv/a/manifest.mpd?x=1'),
        StreamType.dash,
      );
    });

    test('detects direct media containers', () {
      expect(
        provider.mapStreamTypeForTest('https://cdn.tv/a/video.mp4'),
        StreamType.direct,
      );
      expect(
        provider.mapStreamTypeForTest('https://cdn.tv/a/video.mkv'),
        StreamType.direct,
      );
      expect(
        provider.mapStreamTypeForTest('https://cdn.tv/a/video.webm'),
        StreamType.direct,
      );
    });

    test('detects magnet links as torrent', () {
      expect(
        provider.mapStreamTypeForTest(
          'magnet:?xt=urn:btih:abcdef123456&dn=movie',
        ),
        StreamType.torrent,
      );
      expect(
        provider.mapStreamTypeForTest('MAGNET:?xt=urn:btih:abc'),
        StreamType.torrent,
      );
    });

    test('extension-less URL stays direct and is left to the probe', () {
      expect(
        provider.mapStreamTypeForTest('https://cdn.tv/stream/12345'),
        StreamType.direct,
      );
      // Extension-less HLS is the exact case the probe exists for.
      expect(
        provider.mapStreamTypeForTest('https://cdn.tv/hls/master'),
        StreamType.direct,
      );
    });

    test('iframe-shaped player pages are never playable media', () {
      expect(
        provider.mapStreamTypeForTest('https://site.tv/embed/video/1'),
        StreamType.iframe,
      );
      expect(
        provider.mapStreamTypeForTest('https://site.tv/player/abc'),
        StreamType.iframe,
      );
      expect(
        provider.mapStreamTypeForTest('https://site.tv/iframe/x'),
        StreamType.iframe,
      );
      expect(
        provider.mapStreamTypeForTest('https://site.tv/watch/movie-1'),
        StreamType.iframe,
      );
      // A bare directory path with a trailing slash is an HTML page.
      expect(
        provider.mapStreamTypeForTest('https://site.tv/film/123/'),
        StreamType.iframe,
      );
      expect(
        provider.mapStreamTypeForTest('https://site.tv/'),
        StreamType.iframe,
      );
    });

    test('iframe detection beats a media-looking segment in the path', () {
      // The path *is* a player page even though it mentions "player".
      expect(
        provider.mapStreamTypeForTest('https://site.tv/embed/avideo.mp4'),
        StreamType.iframe,
      );
    });

    test('.m3u8 inside a query string is not a false positive', () {
      // The previous implementation used `contains('.m3u8?')`, so any URL
      // whose query mentioned a playlist flipped the whole source to HLS.
      final redirect = 'https://site.tv/redirect?to=https://other.tv/x.m3u8';
      expect(provider.mapStreamTypeForTest(redirect), isNot(StreamType.hls));
      final download = 'https://site.tv/download?f=a.m3u8';
      expect(provider.mapStreamTypeForTest(download), isNot(StreamType.hls));
      // And a directory-shaped path that merely mentions one is an HTML page.
      expect(
        provider.mapStreamTypeForTest('https://site.tv/list/?next=a.m3u8'),
        StreamType.iframe,
      );
    });

    test('empty and unparsable URLs degrade to direct', () {
      expect(provider.mapStreamTypeForTest(''), StreamType.direct);
      expect(
        provider.mapStreamTypeForTest('not a url at all'),
        StreamType.direct,
      );
    });
  });

  group('ServerBackedProvider stream mapping', () {
    late ServerBackedProvider provider;

    setUp(() {
      provider = ServerBackedProvider(
        const ProviderCatalogEntry(
          id: 'uakino',
          name: 'UAKino',
          baseUrl: 'https://uakino.biz',
          showOnHome: true,
          hasFixedStreams: false,
          contentTypes: ['movie'],
          searchEnabledDefault: true,
          enabled: true,
          healthy: true,
        ),
      );
    });

    test('prefers direct_url over url', () {
      final source = provider.mapStreamForTest({
        'url': 'https://site.tv/embed/1',
        'direct_url': 'https://cdn.tv/a/master.m3u8',
      });
      expect(source.url, 'https://cdn.tv/a/master.m3u8');
      expect(source.type, StreamType.hls);
    });

    test('falls back to url when direct_url is empty', () {
      final source = provider.mapStreamForTest({
        'url': 'https://cdn.tv/a/video.mp4',
        'direct_url': '',
      });
      expect(source.url, 'https://cdn.tv/a/video.mp4');
      expect(source.type, StreamType.direct);
    });

    test('non-String url does not throw a TypeError', () {
      // The old code did `json['url'] as String?` then `.toLowerCase()` on
      // the result, so a numeric or object value crashed the whole mapping.
      final source = provider.mapStreamForTest({'url': 12345});
      expect(source.url, '12345');
      expect(source.type, StreamType.direct);
    });

    test('non-String direct_url falls back to a String url', () {
      final source = provider.mapStreamForTest({
        'url': 'https://cdn.tv/a/video.mp4',
        'direct_url': 42,
      });
      expect(source.url, '42');
    });

    test('missing url yields an empty source instead of throwing', () {
      final source = provider.mapStreamForTest({'quality': '1080'});
      expect(source.url, '');
      expect(source.type, StreamType.direct);
      expect(source.quality, StreamQuality.q1080p);
    });

    test('maps quality strings case-insensitively', () {
      expect(
        provider.mapStreamForTest({'url': 'a', 'quality': '4K'}).quality,
        StreamQuality.q4k,
      );
      expect(
        provider.mapStreamForTest({'url': 'a', 'quality': '720p'}).quality,
        StreamQuality.q720p,
      );
      // A numeric quality (the backend sometimes sends one) is coerced, not
      // dropped by an unguarded `as String?`.
      expect(
        provider.mapStreamForTest({'url': 'a', 'quality': 1080}).quality,
        StreamQuality.q1080p,
      );
      expect(
        provider.mapStreamForTest({'url': 'a', 'quality': 'auto'}).quality,
        StreamQuality.unknown,
      );
      expect(
        provider.mapStreamForTest({'url': 'a'}).quality,
        StreamQuality.unknown,
      );
    });

    test('maps headers when present and null when absent', () {
      final withHeaders = provider.mapStreamForTest({
        'url': 'https://cdn.tv/a.mp4',
        'headers': {'Referer': 'https://site.tv', 'X-Token': 123},
      });
      expect(withHeaders.headers, {
        'Referer': 'https://site.tv',
        'X-Token': '123',
      });

      final withoutHeaders = provider.mapStreamForTest({
        'url': 'https://cdn.tv/a.mp4',
      });
      expect(withoutHeaders.headers, isNull);
    });

    test('populates voiceover so alternative chips are distinguishable', () {
      // Without voiceover every chip rendered as the identical "РђРІС‚Рѕ".
      final source = provider.mapStreamForTest({
        'url': 'https://cdn.tv/a.mp4',
        'voiceover': 'РЎС‚СѓРґС–СЏ Р”СѓР±Р»СЏР¶',
        'language': 'uk',
      });
      expect(source.voiceover, 'РЎС‚СѓРґС–СЏ Р”СѓР±Р»СЏР¶');
      expect(source.language, 'uk');
    });

    test('voiceover is null-safe against non-String values', () {
      final source = provider.mapStreamForTest({
        'url': 'https://cdn.tv/a.mp4',
        'voiceover': 7,
      });
      expect(source.voiceover, '7');
    });

    test('maps Episode streamRef from em["stream_ref"] or em["url"]', () {
      final details = provider.mapDetailsForTest({
        'id': 'bo_test_1',
        'title': 'Серіал',
        'seasons': [
          {
            'number': 1,
            'title': 'Сезон 1',
            'episodes': [
              {
                'number': 1,
                'title': 'Серія 1',
                'url': '{"source":"animeon","ref":{"episode_id":60300}}',
              },
              {
                'number': 2,
                'title': 'Серія 2',
                'stream_ref': {'episode_id': 60301},
              },
              {'number': 3, 'title': 'Серія 3'},
            ],
          },
        ],
      });

      expect(details.seasons, isNotNull);
      expect(details.seasons!.first.episodes.length, 3);

      final ep1 = details.seasons!.first.episodes[0];
      expect(ep1.streamRef, '{"source":"animeon","ref":{"episode_id":60300}}');

      final ep2 = details.seasons!.first.episodes[1];
      expect(ep2.streamRef, contains('"episode_id":60301'));

      final ep3 = details.seasons!.first.episodes[2];
      expect(ep3.streamRef, isNull);
    });
  });
}
