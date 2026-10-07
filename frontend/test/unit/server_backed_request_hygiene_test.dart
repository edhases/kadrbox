import 'dart:async';
import 'dart:convert';
// Prefixed because Kadrbox has its own ContentType (movie/series/cartoon/...).
import 'dart:io' as io;

import 'package:flutter_test/flutter_test.dart';
import 'package:kadrbox/data/models/provider_catalog.dart';
import 'package:kadrbox/data/providers/server_backed_provider.dart';
import 'package:kadrbox/data/services/catalog_client.dart';

/// Request hygiene: coalescing, caching and failure suppression.
///
/// This used to stub `ApiClient` in GetIt, which was correct while the provider
/// talked to our own backend. It now builds its own [CatalogClient] per
/// registered catalog, so a GetIt fake is never consulted -- the fake silently
/// agreed with whatever the client wrote, which is exactly the class of test that
/// cannot catch a wrong URL.
///
/// So the fake is a real loopback server. It counts requests, serves canned
/// bodies per path, can be told to fail, and can hold a request open to force
/// concurrency.
class FakeCatalog {
  FakeCatalog._(this._server);

  static Future<FakeCatalog> start() async {
    final server = await io.HttpServer.bind(io.InternetAddress.loopbackIPv4, 0);
    final fake = FakeCatalog._(server);
    unawaited(fake._serve());
    return fake;
  }

  final io.HttpServer _server;

  /// Requests seen, as `path?query`, in order.
  final List<String> requests = [];

  /// Canned bodies by request path.
  final Map<String, Object?> bodies = {};

  /// Paths that should answer 500 instead of a body.
  final Set<String> failing = {};

  /// Paths that should answer 404.
  final Set<String> missing = {};

  /// Held open while set, so concurrent callers genuinely overlap.
  Completer<void>? gate;

  int get totalCalls => requests.length;

  String get baseUrl => 'http://127.0.0.1:${_server.port}';

  Future<void> _serve() async {
    await for (final req in _server) {
      final path = req.uri.path;
      requests.add(
        req.uri.queryParameters.isEmpty
            ? path
            : '$path?${req.uri.queryParameters.entries.map((e) => '${e.key}=${e.value}').join('&')}',
      );

      final held = gate;
      if (held != null) await held.future;

      req.response.headers.contentType = io.ContentType.json;
      if (failing.contains(path)) {
        req.response.statusCode = io.HttpStatus.internalServerError;
        req.response.write(jsonEncode({'error': 'boom'}));
      } else if (missing.contains(path)) {
        req.response.statusCode = io.HttpStatus.notFound;
        req.response.write(jsonEncode({'error': 'no such thing'}));
      } else {
        req.response.write(jsonEncode(bodies[path] ?? const {}));
      }
      await req.response.close();
    }
  }

  int callsTo(String path) => requests.where((r) => r.startsWith(path)).length;

  Future<void> close() => _server.close(force: true);
}

/// A conforming SearchResponse, so `getPopular` has something real to parse.
Map<String, dynamic> _browseBody(List<Map<String, Object?>> items) => {
  'query': '',
  'canonical': '',
  'took_ms': 1,
  'segments': [
    {'id': 'ex', 'label': 'Example', 'provider': 'ex', 'item_count': items.length},
  ],
  'items': items,
  'filtered_out': 0,
  'next_page': null,
  'has_more': false,
};

void main() {
  late FakeCatalog catalog;
  late DateTime now;
  late ServerBackedProvider provider;

  setUp(() async {
    catalog = await FakeCatalog.start();
    now = DateTime(2026, 10, 1, 12);
    provider = ServerBackedProvider(
      ProviderCatalogEntry(
        id: 'ex',
        name: 'Example',
        baseUrl: catalog.baseUrl,
        showOnHome: false,
        hasFixedStreams: false,
        contentTypes: const ['movie', 'series'],
        searchEnabledDefault: true,
        enabled: true,
        healthy: true,
      ),
      clock: () => now,
    );
  });

  tearDown(() async {
    provider.dispose();
    await catalog.close();
  });

  group('concurrent identical reads share one request', () {
    test('browse', () async {
      catalog.bodies['/search'] = _browseBody([
        {'id': 'ex:a', 'provider': 'ex', 'kind': 'movie', 'title': 'A'},
      ]);

      final results = await Future.wait([
        provider.getPopular(),
        provider.getPopular(),
        provider.getPopular(),
        provider.getPopular(),
      ]);

      expect(catalog.totalCalls, 1);
      for (final r in results) {
        expect(r.map((i) => i.title), ['A']);
      }
      // Each waiter owns its list: mutating one must not affect the others.
      results.first.add(results.first.first);
      expect(results[1], hasLength(1));
    });

    test('streams are coalesced but never cached', () async {
      catalog.bodies['/streams'] = {
        'streams': [
          {'url': 'https://cdn.example/x.m3u8', 'quality': 'auto', 'voiceover': ''},
        ],
        'subtitles': <Object>[],
      };

      final first = await Future.wait([
        provider.getStreams('ref', season: 1, episode: 2),
        provider.getStreams('ref', season: 1, episode: 2),
      ]);
      expect(first.every((s) => s.length == 1), isTrue);

      // Sequential calls go to the network again: signed urls may rotate.
      await provider.getStreams('ref', season: 1, episode: 2);

      expect(
        catalog.callsTo('/streams'),
        2,
        reason: 'one shared call for the concurrent pair, one for the rerun',
      );
    });

    test('details are served from the short cache afterwards', () async {
      catalog.bodies['/details'] = {
        'item': {
          'id': 'ex:d',
          'provider': 'ex',
          'kind': 'movie',
          'title': 'D',
        },
        'voiceovers': <Object>[],
        'seasons': <Object>[],
        'episodes': <Object>[],
      };

      final first = await provider.getDetails('ref');
      final second = await provider.getDetails('ref');

      expect(first.item.title, 'D');
      expect(second.item.title, 'D');
      expect(
        catalog.totalCalls,
        1,
        reason: 'the same large body must not be refetched a minute later',
      );
    });

    test('details cache expires', () async {
      catalog.bodies['/details'] = {
        'item': {
          'id': 'ex:d',
          'provider': 'ex',
          'kind': 'movie',
          'title': 'D',
        },
        'voiceovers': <Object>[],
        'seasons': <Object>[],
        'episodes': <Object>[],
      };

      await provider.getDetails('ref');
      now = now.add(const Duration(seconds: 121));
      await provider.getDetails('ref');

      expect(catalog.totalCalls, 2);
    });
  });

  group('recent failures are not hammered', () {
    test('a 500 suppresses the identical read for a short while', () async {
      catalog.failing.add('/search');
      final p = provider;

      await expectLater(p.getPopular(), throwsA(isA<CatalogException>()));
      await expectLater(p.getPopular(), throwsA(isA<CatalogException>()));

      expect(catalog.callsTo('/search'), 1);

      now = now.add(const Duration(seconds: 31));
      await expectLater(p.getPopular(), throwsA(isA<CatalogException>()));
      expect(catalog.callsTo('/search'), 2);
    });

    test('a 404 is never suppressed', () async {
      catalog.missing.add('/search');
      final p = provider;

      await expectLater(p.getPopular(), throwsA(isA<CatalogException>()));
      await expectLater(p.getPopular(), throwsA(isA<CatalogException>()));

      expect(
        catalog.callsTo('/search'),
        2,
        reason: "client errors are the caller's fault, not the upstream's",
      );
    });

    test('a dead catalog is suppressed, then retried after expiry', () async {
      // No server is listening on this port at all: a transport failure rather
      // than an answered request.
      final dead = ServerBackedProvider(
        ProviderCatalogEntry(
          id: 'dead',
          name: 'Dead',
          // Port 1 on loopback: nothing listens there.
          baseUrl: 'http://127.0.0.1:1',
          showOnHome: false,
          hasFixedStreams: false,
          contentTypes: const ['movie'],
          searchEnabledDefault: true,
          enabled: true,
          healthy: true,
        ),
        clock: () => now,
      );
      addTearDown(dead.dispose);

      await expectLater(dead.getPopular(), throwsA(isA<CatalogException>()));
      await expectLater(dead.getPopular(), throwsA(isA<CatalogException>()));

      now = now.add(const Duration(seconds: 31));
      await expectLater(dead.getPopular(), throwsA(isA<CatalogException>()));
    });
  });

  group('empty catalogues stay empty', () {
    test('an empty browse body becomes an empty list', () async {
      catalog.bodies['/search'] = _browseBody(const []);
      expect(await provider.getPopular(), isEmpty);
    });
  });
}