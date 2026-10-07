import 'dart:convert';
// Prefixed because Kadrbox has its own ContentType (movie/series/cartoon/...)
// and an unprefixed dart:io ContentType would silently resolve to the wrong one.
import 'dart:io' as io;

import 'package:flutter_test/flutter_test.dart';
import 'package:kadrbox/data/models/provider_catalog.dart';
import 'package:kadrbox/data/providers/server_backed_provider.dart';
import 'package:kadrbox/domain/entities/entities.dart';

/// The provider used to build every content URL as
/// `${AppConfig.serverApiUrl}/content/...`, i.e. our own backend. Those
/// endpoints were deleted when the parsers left this repository, so each read
/// answered 404 with Go's `404 page not found` -- while the catalog the user had
/// just registered was never asked anything.
///
/// These tests run against a real loopback server, because the failure was a
/// disagreement between two processes about where content lives. A mocked HTTP
/// client would have agreed with whatever the client wrote.
void main() {
  late io.HttpServer server;
  late ServerBackedProvider provider;
  late List<String> paths;
  late List<Map<String, String>> queries;

  /// A minimal conforming SearchResponse, per contracts/openapi.yaml.
  Map<String, dynamic> searchResponse() => {
    'query': 'dune',
    'canonical': 'dune',
    'took_ms': 12,
    'segments': [
      {'id': 'ex', 'label': 'Example', 'provider': 'ex', 'item_count': 1},
    ],
    'items': [
      {
        'id': 'ex:dune-2021',
        'segment': 'ex',
        'provider': 'ex',
        'kind': 'movie',
        'title': 'Example Feature',
        'year': 2021,
        'rating': 7.8,
        'poster_url': 'https://cdn.example/p.jpg',
      },
    ],
    'filtered_out': 0,
    'next_page': null,
    'has_more': false,
  };

  setUp(() async {
    paths = [];
    queries = [];

    server = await io.HttpServer.bind(io.InternetAddress.loopbackIPv4, 0);
    server.listen((io.HttpRequest req) async {
      paths.add(req.uri.path);
      queries.add(req.uri.queryParameters);

      req.response.headers.contentType = io.ContentType.json;
      switch (req.uri.path) {
        case '/status':
          req.response.write(
            jsonEncode({
              'protocol_version': 1,
              'app': 'Example',
              'capabilities': ['search', 'details', 'streams'],
            }),
          );
        case '/search':
          req.response.write(jsonEncode(searchResponse()));
        case '/details':
          req.response.write(
            jsonEncode({
              'item': {
                'id': 'ex:dune-2021',
                'segment': 'ex',
                'provider': 'ex',
                'kind': 'series',
                'title': 'Example Show',
              },
              'voiceovers': [
                {'id': 'uk', 'label': 'Ukrainian', 'lang': 'uk', 'default': true},
              ],
              'seasons': [
                {'number': 1, 'label': 'Season 1', 'episode_count': 2},
              ],
              'episodes': [
                {
                  'id': 'ex:show:1:1',
                  'season': 1,
                  'number': 1,
                  'title': 'One',
                  'duration_ms': 1500000,
                },
                {'id': 'ex:show:1:2', 'season': 1, 'number': 2, 'title': 'Two'},
              ],
            }),
          );
        case '/streams':
          req.response.write(
            jsonEncode({
              'streams': [
                {
                  'url': 'https://cdn.example/hls/master.m3u8',
                  'quality': '1080p',
                  'voiceover': 'uk',
                  'headers': {'Referer': 'https://catalog.example/'},
                },
              ],
              'subtitles': [
                {
                  'url': 'https://cdn.example/subs/en.vtt',
                  'label': 'English',
                  'lang': 'en',
                  'format': 'vtt',
                },
              ],
            }),
          );
        default:
          req.response.statusCode = io.HttpStatus.notFound;
          req.response.write('404 page not found');
      }
      await req.response.close();
    });

    provider = ServerBackedProvider(
      ProviderCatalogEntry(
        id: '127.0.0.1-test',
        name: 'Local',
        baseUrl: 'http://127.0.0.1:${server.port}',
        showOnHome: true,
        hasFixedStreams: false,
        contentTypes: const ['movie', 'series'],
        searchEnabledDefault: true,
        enabled: true,
        healthy: true,
      ),
    );
    addTearDown(provider.dispose);
    addTearDown(() => server.close(force: true));
  });

  group('content comes from the registered catalog', () {
    test('the handshake registers, then search hits /search on that server',
        () async {
      final items = await provider.search('dune');

      expect(items, hasLength(1));
      expect(items.first.title, 'Example Feature');
      expect(paths, ['/search']);
      expect(queries.first['q'], 'dune');
    });

    test('no request ever goes to our own backend', () async {
      await provider.search('dune');
      await provider.getDetails('ex:dune-2021');
      await provider.getStreams('ex:dune-2021');

      // Nothing under /api/v1/content, which is what used to 404 forever.
      expect(paths.where((p) => p.contains('/api/v1')), isEmpty);
      expect(paths, ['/search', '/details', '/streams']);
    });

    test('details reads the nested item and groups the flat episodes',
        () async {
      final details = await provider.getDetails('ex:dune-2021');

      expect(details.item.title, 'Example Show');
      expect(details.item.type, ContentType.series);
      expect(queries[0]['id'], 'ex:dune-2021');

      final seasons = details.seasons!;
      expect(seasons.length, 1);
      expect(seasons.first.title, 'Season 1');
      expect(seasons.first.episodes.length, 2);
      expect(seasons.first.episodes.first.streamRef, 'ex:show:1:1');
      expect(seasons.first.episodes.first.duration,
          const Duration(milliseconds: 1500000));

      expect(details.voiceovers.single.name, 'Ukrainian');
    });

    test('streams carry their voiceover, headers and subtitles', () async {
      final streams = await provider.getStreams('ex:show:1:1',
          season: 1, episode: 1);

      expect(streams, hasLength(1));
      expect(streams.first.url, 'https://cdn.example/hls/master.m3u8');
      expect(streams.first.type, StreamType.hls);
      expect(streams.first.quality, StreamQuality.q1080p);
      expect(streams.first.voiceover, 'uk');
      expect(streams.first.headers, {'Referer': 'https://catalog.example/'});

      // Subtitles are a sibling array on the protocol, so they are attached
      // rather than read off the stream.
      expect(streams.first.subtitles, hasLength(1));
      expect(streams.first.subtitles!.first.language, 'en');
      expect(streams.first.subtitles!.first.format, SubtitleFormat.vtt);

      expect(queries[0]['id'], 'ex:show:1:1');
      expect(queries[0]['season'], '1');
      expect(queries[0]['episode'], '1');
    });

    test('browsing uses /search with no query, narrowed by kind', () async {
      final popular = await provider.getPopular();
      expect(popular, hasLength(1));
      expect(queries.last.containsKey('q'), isFalse);

      await provider.getPopular(type: ContentType.series);
      expect(queries.last['kind'], 'series');

      await provider.getByCategory('movie');
      expect(queries.last['kind'], 'movie');

      await provider.getByCategory('anime');
      expect(queries.last['kind'], 'anime');

      // An unrecognised slug browses everything rather than showing nothing.
      await provider.getByCategory('nonsense');
      expect(queries.last.containsKey('kind'), isFalse);
    });

    test('cartoon maps onto the protocol catch-all, not onto movie', () async {
      // The protocol's kind enum has no cartoon. Sending `cartoon` would be
      // rejected, and sending `movie` would silently return films.
      await provider.getPopular(type: ContentType.cartoon);
      expect(queries.last['kind'], 'other');
    });

    test('a local catalog keeps its own loopback stream urls', () async {
      // The provider is pointed at 127.0.0.1 in this suite, so the LAN rule
      // that protects against a hostile remote catalog must not fire on the
      // user's own server.
      final streams = await provider.getStreams('ex:dune-2021');
      expect(streams, hasLength(1));
    });
  });
}