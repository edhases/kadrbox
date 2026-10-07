import 'dart:convert';
// Prefixed because Kadrbox has its own ContentType (movie/series/cartoon/...).
import 'dart:io' as io;

import 'package:flutter_test/flutter_test.dart';
import 'package:kadrbox/data/models/provider_catalog.dart';
import 'package:kadrbox/data/providers/server_backed_provider.dart';

/// `/streams` is the most security-sensitive endpoint in the protocol: it tells
/// the player where to open a socket and what headers to send while doing it.
///
/// contracts/openapi.yaml states the constraints as client obligations, not
/// server requests -- "the client applies every rule in PROTOCOL.md#client-
/// security-rules to every url here, and again to every redirect target it
/// follows". A catalog server is a third party we cannot update, so these have
/// to be enforced on this side.
void main() {
  late io.HttpServer server;
  late List<String> servedUrls;

  setUp(() async {
    servedUrls = [];
    server = await io.HttpServer.bind(io.InternetAddress.loopbackIPv4, 0);
    server.listen((io.HttpRequest req) async {
      req.response.headers.contentType = io.ContentType.json;
      if (req.uri.path == '/streams') {
        req.response.write(
          jsonEncode({
            'streams': servedUrls
                .map((u) => {
                      'url': u,
                      'quality': 'auto',
                      'voiceover': '',
                    })
                .toList(),
            'subtitles': <Object>[],
          }),
        );
      } else {
        req.response.write(jsonEncode({'error': 'not found'}));
        req.response.statusCode = io.HttpStatus.notFound;
      }
      await req.response.close();
    });
  });

  tearDown(() => server.close(force: true));

  /// A provider whose catalog is reached through loopback, but whose *identity*
  /// is a public host.
  ///
  /// The transport has to be loopback for the test to be hermetic, while the
  /// security decision is made on `baseUrl`. Since the decision is made on
  /// baseUrl and the request must reach this server, the base is the loopback
  /// one -- so these cases assert the local-catalog exemption. The remote
  /// behaviour is covered by the pure validator tests in stream_url_test.dart.
  ServerBackedProvider localProvider() {
    final provider = ServerBackedProvider(
      ProviderCatalogEntry(
        id: 'ex',
        name: 'Example',
        baseUrl: 'http://127.0.0.1:${server.port}',
        showOnHome: true,
        hasFixedStreams: false,
        contentTypes: const ['movie'],
        searchEnabledDefault: true,
        enabled: true,
        healthy: true,
      ),
    );
    addTearDown(provider.dispose);
    return provider;
  }

  test('a loopback catalog may serve its own loopback stream urls', () async {
    // The user deliberately pointed the app at 127.0.0.1. Refusing their own
    // server's stream urls would break local development and protect nothing,
    // because there is no third party to protect against.
    servedUrls = ['http://127.0.0.1:${server.port}/stream.m3u8'];

    final streams = await localProvider().getStreams('id');

    expect(streams, hasLength(1));
    expect(streams.single.url, contains('stream.m3u8'));
  });

  test('even a loopback catalog cannot smuggle a file url to the player', () async {
    // The local exemption is for the *network* rule, not for the scheme rule.
    // A `file://` url is a local file read whose bytes the catalog then gets
    // back; that is the sharp end of the whole protocol.
    servedUrls = ['file:///C:/Users/someone/secret.txt'];

    final streams = await localProvider().getStreams('id');

    expect(streams, isEmpty);
  });

  test('a loopback catalog cannot smuggle a non-http scheme', () async {
    servedUrls = [
      'ftp://example.com/a.mp4',
      'javascript:alert(1)',
      'blob:http://x/y',
      'data:video/mp4;base64,AAAA',
    ];

    expect(await localProvider().getStreams('id'), isEmpty);
  });

  test('refused streams do not take the good ones down with them', () async {
    servedUrls = [
      'file:///etc/passwd',
      'http://127.0.0.1:${server.port}/good.m3u8',
    ];

    final streams = await localProvider().getStreams('id');

    // Dropping one is the documented behaviour; losing the list would be a bug.
    expect(streams, hasLength(1));
    expect(streams.single.url, endsWith('good.m3u8'));
  });

  test('an empty streams array is not an error', () async {
    servedUrls = [];
    expect(await localProvider().getStreams('id'), isEmpty);
  });
}