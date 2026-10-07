import 'package:flutter_test/flutter_test.dart';

import 'package:kadrbox/data/models/provider_catalog.dart';
import 'package:kadrbox/data/providers/server_backed_provider.dart';
import 'package:kadrbox/domain/entities/entities.dart';

void main() {
  group('ServerBackedProvider', () {
    late ServerBackedProvider provider;

    setUp(() {
      provider = ServerBackedProvider(
        const ProviderCatalogEntry(
          id: 'src_a',
          name: 'Source A',
          baseUrl: 'https://src-a.example',
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
      expect(provider.id, 'src_a');
      expect(provider.name, 'Source A');
      expect(provider.baseUrl, 'https://src-a.example');
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
          id: 'src_a',
          name: 'Source A',
          baseUrl: 'https://src-a.example',
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
          id: 'src_a',
          name: 'Source A',
          baseUrl: 'https://src-a.example',
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

    test('keeps only the headers the protocol allows, and drops the rest', () {
      // contracts/openapi.yaml states the allow-list as a client obligation:
      // "allow-list is exactly Referer, User-Agent, Origin; any other header is
      // dropped without being sent" and "never send Authorization, Cookie or any
      // other client credential, even when the server asks for it by name".
      // A catalog server is a third party, so this is enforced here rather than
      // trusted.
      final source = provider.mapStreamForTest({
        'url': 'https://cdn.tv/a.mp4',
        'headers': {
          'Referer': 'https://site.tv',
          'User-Agent': 'Kadrbox/1.0',
          'Origin': 'https://catalog.example',
          'X-Token': 123,
          'Authorization': 'Bearer stolen',
          'Cookie': 'session=abc',
        },
      });
      expect(source.headers, {
        'Referer': 'https://site.tv',
        'User-Agent': 'Kadrbox/1.0',
        'Origin': 'https://catalog.example',
      });
    });

    test('header names are matched case-insensitively', () {
      final source = provider.mapStreamForTest({
        'url': 'https://cdn.tv/a.mp4',
        'headers': {'referer': 'https://site.tv', 'X-Token': '1'},
      });
      expect(source.headers, {'referer': 'https://site.tv'});
    });

    test('headers are null when absent or empty after filtering', () {
      expect(
        provider.mapStreamForTest({'url': 'https://cdn.tv/a.mp4'}).headers,
        isNull,
      );
      expect(
        provider
            .mapStreamForTest({
              'url': 'https://cdn.tv/a.mp4',
              'headers': {'X-Token': '1'},
            })
            .headers,
        isNull,
      );
      expect(
        provider.mapStreamForTest({'url': 'https://cdn.tv/a.mp4', 'headers': 7})
            .headers,
        isNull,
      );
    });

    test('populates voiceover so alternative chips are distinguishable', () {
      // The protocol's `Stream.voiceover` is the track id, which is also what
      // `/streams?voiceover=` and `Episode.voiceovers` use. There is no
      // `language` member on a protocol Stream, so nothing is invented for it.
      final source = provider.mapStreamForTest({
        'url': 'https://cdn.tv/a.mp4',
        'voiceover': 'uk',
      });
      expect(source.voiceover, 'uk');
    });

    test('voiceover is null-safe against non-String values', () {
      final source = provider.mapStreamForTest({
        'url': 'https://cdn.tv/a.mp4',
        'voiceover': 7,
      });
      expect(source.voiceover, '7');
    });

    test('groups the flat protocol episode list into seasons', () {
      // The protocol ships episodes flat across all seasons so a details screen
      // needs no request per episode; MediaDetails nests them, so the grouping
      // happens in the mapper.
      final details = provider.mapDetailsForTest({
        'item': {'id': 'ex:show', 'provider': 'ex', 'kind': 'series', 'title': 'Example'},
        'seasons': [
          {'number': 1, 'label': 'Season 1', 'episode_count': 2},
          {'number': 2, 'label': 'Season 2', 'episode_count': 1},
        ],
        'episodes': [
          {'id': 'ex:show:1:1', 'season': 1, 'number': 1, 'title': 'One'},
          {'id': 'ex:show:1:2', 'season': 1, 'number': 2, 'title': 'Two'},
          {'id': 'ex:show:2:1', 'season': 2, 'number': 1, 'title': 'Three'},
        ],
      });

      final seasons = details.seasons;
      expect(seasons, isNotNull);
      expect(seasons!.map((s) => s.number), [1, 2]);
      expect(seasons[0].title, 'Season 1');
      expect(seasons[0].episodes.length, 2);
      expect(seasons[1].episodes.length, 1);

      // The opaque episode id is what `/streams?id=` is called with, with season
      // and episode passed alongside it.
      expect(seasons[0].episodes[0].streamRef, 'ex:show:1:1');
      expect(seasons[0].episodes[1].streamRef, 'ex:show:1:2');
      expect(seasons[1].episodes[0].streamRef, 'ex:show:2:1');
    });

    test('a season the server summarised but never populated still appears', () {
      final details = provider.mapDetailsForTest({
        'item': {'id': 'ex:show', 'provider': 'ex', 'kind': 'series', 'title': 'Example'},
        'seasons': [
          {'number': 1, 'label': 'Season 1', 'episode_count': 2},
          {'number': 2, 'label': 'Season 2', 'episode_count': 0},
        ],
        'episodes': [
          {'id': 'ex:show:1:1', 'season': 1, 'number': 1, 'title': 'One'},
        ],
      });

      // Only seasons that actually have episodes are rendered, because an empty
      // one is a dead end the user can click into.
      expect(details.seasons!.map((s) => s.number), [1]);
    });

    test('an episode without a season is dropped rather than filed under 0', () {
      final details = provider.mapDetailsForTest({
        'item': {'id': 'ex:show', 'provider': 'ex', 'kind': 'series', 'title': 'Example'},
        'seasons': const [],
        'episodes': [
          {'id': 'ex:show:x:1', 'number': 1, 'title': 'No season'},
        ],
      });
      expect(details.seasons, isNull);
    });

    test('a movie has no seasons at all, which is what null means', () {
      // MediaDetails reads null as "this is a movie" and renders no episode
      // picker. An empty list would put a picker over a film.
      final details = provider.mapDetailsForTest({
        'item': {
          'id': 'ex:film',
          'provider': 'ex',
          'kind': 'movie',
          'title': 'Example Feature',
          'year': 2021,
          'rating': 7.8,
          'genres': ['science fiction'],
        },
        'voiceovers': const [],
        'seasons': const [],
        'episodes': const [],
      });

      expect(details.seasons, isNull);
      expect(details.item.title, 'Example Feature');
      expect(details.item.type, ContentType.movie);
      expect(details.item.year, 2021);
      expect(details.genres, ['science fiction']);
    });

    test('reads voiceovers by their protocol label, not a legacy name', () {
      final details = provider.mapDetailsForTest({
        'item': {'id': 'ex:film', 'provider': 'ex', 'kind': 'movie', 'title': 'F'},
        'voiceovers': [
          {'id': 'en', 'label': 'English', 'lang': 'en', 'default': true},
          {'id': 'uk', 'label': 'Ukrainian', 'lang': 'uk', 'default': false},
        ],
        'seasons': const [],
        'episodes': const [],
      });

      expect(details.voiceovers.map((v) => v.id), ['en', 'uk']);
      expect(details.voiceovers.first.name, 'English');
    });

    test('maps the protocol kind onto a content type', () {
      ContentType typeOf(String? kind) => provider.mapItemForTest({
        'id': 'x',
        'title': 't',
        'kind': kind,
      }).type;

      expect(typeOf('movie'), ContentType.movie);
      expect(typeOf('series'), ContentType.series);
      expect(typeOf('episode'), ContentType.series);
      expect(typeOf('anime'), ContentType.anime);
      expect(typeOf('tv_show'), ContentType.dorama);
      expect(typeOf('documentary'), ContentType.movie);
      // `other` and anything unknown must not be guessed at.
      expect(typeOf('other'), ContentType.unknown);
      expect(typeOf('nonsense'), ContentType.unknown);
      expect(typeOf(null), ContentType.unknown);
    });

    test('an item id is used verbatim, with no url smuggled into it', () {
      final item = provider.mapItemForTest({
        'id': 'ex:film-2021',
        'provider': 'ex',
        'kind': 'movie',
        'title': 'Example',
        'external_id': 'film-2021',
      });
      expect(item.id, 'ex:film-2021');
      expect(item.providerId, 'ex');
      // external_id is diagnostic only; the protocol forbids building URLs from
      // it, so it must not surface as the item's url.
      expect(item.url, isNull);
    });
  });
}
