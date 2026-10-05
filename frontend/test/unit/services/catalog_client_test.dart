import 'dart:async';
import 'dart:convert';
import 'dart:io';

import 'package:flutter_test/flutter_test.dart';
import 'package:kadrbox/data/services/catalog_client.dart';

// A real HttpServer on loopback, not a mocked Dio. The point of these tests is
// that the client speaks HTTP correctly and survives whatever comes back; a
// mock would only prove the mock agrees with the client.

void main() {
  late HttpServer server;
  late CatalogClient client;
  late List<HttpRequest> received;

  /// Serves [handler] until the server closes, on the real port it bound to.
  void pump(Future<(int, String)> Function(HttpRequest) handler) {
    unawaited(() async {
      await for (final req in server) {
        received.add(req);
        await utf8.decoder.bind(req).join();
        final (code, payload) = await handler(req);
        req.response.statusCode = code;
        req.response.headers.contentType = ContentType.json;
        req.response.write(payload);
        await req.response.close();
      }
    }());
  }

  /// Binds a server answering every request with [handler], and points a
  /// client at it.
  Future<void> serve(Future<(int, String)> Function(HttpRequest) handler) async {
    server = await HttpServer.bind(InternetAddress.loopbackIPv4, 0);
    received = [];
    client = CatalogClient(baseUrl: 'http://127.0.0.1:${server.port}');
    pump(handler);
  }

  /// A minimal well-formed /status payload, overridable per test.
  (int, String) ok(Map<String, Object?> overrides) => (
        200,
        jsonEncode({
          'protocol_version': 1,
          'catalog_id': 'cat-a',
          'app': 'Example',
          'capabilities': ['search', 'details', 'streams'],
          ...overrides,
        }),
      );

  tearDown(() async {
    client.dispose();
    await server.close(force: true);
  });

  group('handshake', () {
    test('accepts a well-formed server and reports its identity', () async {
      await serve((_) async => ok({}));

      final status = await client.handshake();

      expect(status.protocolVersion, 1);
      expect(status.catalogId, 'cat-a');
      expect(status.supports('search'), isTrue);
      expect(status.supports('nope'), isFalse);
      expect(status.compatibilityWarning, isNull);
    });

    test('a server without protocol_version is rejected as "not a catalog"',
        () async {
      await serve((_) async => (200, jsonEncode({'app': 'Something else'})));

      await expectLater(
        client.handshake(),
        throwsA(
          isA<CatalogException>()
              .having((e) => e.message, 'message', contains('не сервер каталогу')),
        ),
      );
    });

    test('a server that omits catalog_id is accepted and gets a derived id',
        () async {
      // The normative contract (contracts/openapi.yaml) does not require
      // catalog_id. Requiring it anyway rejected conforming servers, which is
      // the same failure the contract warns about from the other side: a server
      // we cannot update must never be able to break a deployed client.
      await serve((_) async =>
          (200, jsonEncode({'protocol_version': 1, 'app': 'Example'})));

      final status = await client.handshake();

      expect(status.protocolVersion, 1);
      expect(status.catalogId, isNotEmpty);
      // The derived id must be usable as a storage namespace component.
      expect(status.catalogId, matches(RegExp(r'^[A-Za-z0-9._:-]+$')));
    });

    test('a blank or whitespace catalog_id falls back to the derived id',
        () async {
      for (final declared in ['', '   ', 42, null, <String>[]]) {
        await serve((_) async => ok({'catalog_id': declared}));

        final status = await client.handshake();

        expect(status.catalogId, isNotEmpty, reason: 'declared=$declared');
        expect(status.catalogId, isNot('   '));
      }
    });

    test('a declared catalog_id always wins over the derived one', () async {
      await serve((_) async => ok({'catalog_id': '  cat-a  '}));

      final status = await client.handshake();

      // Trimmed, and not replaced by anything the client made up.
      expect(status.catalogId, 'cat-a');
    });

    test('a newer protocol warns but is not refused', () async {
      await serve((_) async => ok({'protocol_version': 99, 'catalog_id': 'cat-f'}));

      final status = await client.handshake();

      expect(status.isNewerThanApp, isTrue);
      expect(status.compatibilityWarning, isNotNull);
    });

    test('an older protocol also warns', () async {
      await serve((_) async => ok({'protocol_version': 0, 'catalog_id': 'cat-o'}));

      expect((await client.handshake()).isOlderThanApp, isTrue);
    });

    test('unknown and non-string capabilities are ignored, not fatal', () async {
      await serve((_) async => ok({
            'capabilities': ['search', 'somethingFromTheFuture', 42],
          }));

      final status = await client.handshake();
      expect(status.capabilities, {'search', 'somethingFromTheFuture'});
    });

    test('404 says there is no catalog there, not "network error"', () async {
      await serve((_) async => (404, jsonEncode({'error': 'not found'})));

      await expectLater(
        client.handshake(),
        throwsA(
          isA<CatalogException>()
              .having((e) => e.message, 'message', contains('немає сервера каталогу')),
        ),
      );
    });

    test('429 tells the user to retry rather than showing a raw status', () async {
      await serve((_) async => (429, jsonEncode({'error': 'slow down'})));

      await expectLater(
        client.handshake(),
        throwsA(
          isA<CatalogException>()
              .having((e) => e.message, 'message', contains('обмежив запити')),
        ),
      );
    });

    test('a non-JSON body is reported rather than crashing', () async {
      await serve((_) async => (200, '<html>hello</html>'));

      await expectLater(client.handshake(), throwsA(isA<CatalogException>()));
    });

    test('a dead port becomes an actionable message', () async {
      // Bind, then release, so the port is almost certainly closed.
      final probe = await HttpServer.bind(InternetAddress.loopbackIPv4, 0);
      final port = probe.port;
      await probe.close();

      final dead = CatalogClient(baseUrl: 'http://127.0.0.1:$port');
      addTearDown(dead.dispose);

      await expectLater(
        dead.handshake(),
        throwsA(
          isA<CatalogException>().having(
            (e) => e.message,
            'message',
            anyOf(contains('Не вдалося зєднатися'), contains('не відповідає')),
          ),
        ),
      );
    });
  });

  group('request shape', () {
    test('a base URL with a trailing slash does not double up', () async {
      await serve((_) async => ok({}));

      final withSlash = CatalogClient(
        baseUrl: 'http://127.0.0.1:${server.port}/',
      );
      addTearDown(withSlash.dispose);

      await withSlash.handshake();
      expect(received.single.uri.path, '/status');
    });

    test('the handshake carries no credentials of ours', () async {
      await serve((_) async => ok({}));

      await client.handshake();

      final req = received.single;
      expect(req.uri.path, '/status');
      // A bearer token for our backend, handed to a host the user typed, is
      // the exact thing SECURITY.md rules out.
      expect(req.headers.value('authorization'), isNull);
      expect(req.headers.value('cookie'), isNull);
    });
  });
}