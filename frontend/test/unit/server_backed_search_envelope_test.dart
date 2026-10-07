import 'dart:async';
import 'dart:convert';
// Prefixed because Kadrbox has its own ContentType (movie/series/cartoon/...).
import 'dart:io' as io;

import 'package:flutter_test/flutter_test.dart';
import 'package:kadrbox/data/models/provider_catalog.dart';
import 'package:kadrbox/data/providers/server_backed_provider.dart';
import 'package:kadrbox/data/services/catalog_client.dart';
import 'package:kadrbox/domain/entities/entities.dart';

/// Serves one canned body for every `/search`, so response-shape handling can be
/// exercised against a real socket.
///
/// It used to fake `ApiClient` in GetIt, which worked only while the provider
/// talked to our own backend. The provider now builds its own `CatalogClient`,
/// so the GetIt fake was never consulted -- it agreed with whatever the client
/// wrote, which is precisely the kind of test that cannot catch a wrong URL.
class FakeCatalog {
  FakeCatalog._(this._server);

  static Future<FakeCatalog> start() async {
    final server = await io.HttpServer.bind(io.InternetAddress.loopbackIPv4, 0);
    final fake = FakeCatalog._(server);
    unawaited(fake._serve());
    return fake;
  }

  final io.HttpServer _server;

  /// What `/search` answers with, verbatim -- including a non-JSON body.
  Object? searchBody;

  /// Raw text to send instead of encoding [searchBody]. Used to simulate a body
  /// that is not JSON at all.
  String? rawSearchBody;

  /// Query string of the last `/search` request.
  Map<String, String>? lastQuery;

  int calls = 0;

  String get baseUrl => 'http://127.0.0.1:${_server.port}';

  Future<void> _serve() async {
    await for (final req in _server) {
      if (req.uri.path == '/search') {
        calls++;
        lastQuery = req.uri.queryParameters;
      }
      final raw = rawSearchBody;
      if (raw != null) {
        req.response.headers.contentType = io.ContentType.text;
        req.response.write(raw);
      } else if (searchBody == null) {
        req.response.write('null');
      } else {
        req.response.headers.contentType = io.ContentType.json;
        req.response.write(jsonEncode(searchBody));
      }
      await req.response.close();
    }
  }

  Future<void> close() => _server.close(force: true);
}

void main() {
  late FakeCatalog catalog;

  setUp(() async {
    catalog = await FakeCatalog.start();
  });

  tearDown(() => catalog.close());

  /// A provider pointed at the fake, with [body] as its `/search` response.
  ServerBackedProvider providerReturning(Object? body) {
    catalog.searchBody = body;
    catalog.rawSearchBody = null;
    final provider = ServerBackedProvider(
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
    );
    addTearDown(provider.dispose);
    return provider;
  }

  group('searchEnvelope: the catalog contract', () {
    test('parses the envelope and passes the query through', () async {
      final provider = providerReturning({
        'query': 'Matrix',
        'canonical': 'Matrix',
        'took_ms': 120,
        'filtered_out': 7,
        'segments': [
          {
            'id': 'ex',
            'label': 'Example',
            'provider': 'ex',
            'status': 'ok',
            'item_count': 1,
          },
        ],
        'items': [
          {
            'id': 'ex:matrix',
            'segment': 'ex',
            'provider': 'ex',
            'kind': 'movie',
            'title': 'The Matrix',
          },
        ],
        'next_page': null,
        'has_more': false,
      });

      final envelope = await provider.searchEnvelope('Matrix');

      expect(envelope.items, hasLength(1));
      expect(envelope.items.first.item.title, 'The Matrix');
      expect(envelope.items.first.item.id, 'ex:matrix');
      expect(envelope.filteredOut, 7);
      expect(catalog.lastQuery, {'q': 'Matrix'});
    });

    test("reads the segment count from item_count, not count", () async {
      // The protocol names it item_count. Reading `count` reported zero items
      // for every catalog, and the UI shows this as a per-source count.
      final provider = providerReturning({
        'query': 'q',
        'segments': [
          {'id': 'ex', 'status': 'ok', 'item_count': 4},
        ],
        'items': <Object>[],
      });

      final envelope = await provider.searchEnvelope('q');

      expect(envelope.segments.single.count, 4);
    });

    test('an errored segment is reported as such, and its items are kept',
        () async {
      // The protocol makes `status` required precisely so one broken source
      // does not make the whole catalogue look empty.
      final provider = providerReturning({
        'query': 'q',
        'segments': [
          {'id': 'ok_one', 'status': 'ok', 'item_count': 1},
          {'id': 'broken', 'status': 'error', 'error': 'upstream_timeout'},
        ],
        'items': [
          {
            'id': 'ok_one:1',
            'segment': 'ok_one',
            'provider': 'ok_one',
            'kind': 'movie',
            'title': 'Kept',
          },
        ],
      });

      final envelope = await provider.searchEnvelope('q');

      expect(envelope.segments.map((s) => s.id), ['ok_one', 'broken']);
      expect(envelope.segments.last.status, 'error');
      expect(envelope.items.single.item.title, 'Kept');
    });

    test('never synthesises a page number for an opaque token', () async {
      final provider = providerReturning({
        'query': 'q',
        'segments': <Object>[],
        'items': <Object>[],
      });

      await provider.searchEnvelope('q', page: 3);

      expect(
        catalog.lastQuery!.containsKey('page'),
        isFalse,
        reason:
            'page is an opaque continuation token; sending "3" would return '
            'page one again forever',
      );
    });
  });

  group('searchEnvelope: a broken body must not look like "found nothing"', () {
    // The whole point of surfacing failures: a malformed response must not
    // render as an empty result set.
    test('a non-JSON body is reported', () async {
      final provider = providerReturning(null);
      catalog.rawSearchBody = '<html>502 Bad Gateway</html>';

      await expectLater(
        provider.searchEnvelope('q'),
        throwsA(isA<CatalogException>()),
      );
    });

    test('a JSON null body is reported', () async {
      final provider = providerReturning(null);

      await expectLater(
        provider.searchEnvelope('q'),
        throwsA(isA<CatalogException>()),
      );
    });

    test('a bare list is reported rather than silently wrapped', () async {
      // The old backend could answer a flat list and the client wrapped it into
      // an envelope. That shim is gone: the catalog protocol has exactly one
      // shape, and inventing a compatibility path for a shape the contract does
      // not describe is how a wrong response becomes an empty screen.
      final provider = providerReturning([
        {'id': 'ex:1', 'provider': 'ex', 'kind': 'movie', 'title': 'A'},
      ]);

      await expectLater(
        provider.searchEnvelope('q'),
        throwsA(isA<CatalogException>()),
      );
    });
  });

  group('searchEnvelope: type narrowing', () {
    test('filters by kind and keeps the rest of the envelope', () async {
      final provider = providerReturning({
        'query': 'q',
        'took_ms': 50,
        'filtered_out': 3,
        'segments': [
          {'id': 'ex', 'status': 'ok', 'item_count': 2},
        ],
        'items': [
          {
            'id': 'ex:1',
            'provider': 'ex',
            'kind': 'movie',
            'title': 'Movie',
          },
          {
            'id': 'ex:2',
            'provider': 'ex',
            'kind': 'series',
            'title': 'Show',
          },
        ],
      });

      final envelope = await provider.searchEnvelope(
        'q',
        type: ContentType.series,
      );

      expect(envelope.items, hasLength(1));
      expect(envelope.items.single.item.title, 'Show');
      // Narrowing must not rewrite what the server said about the whole search.
      expect(envelope.filteredOut, 3);
      expect(envelope.segments.single.count, 2);
    });
  });
}