import 'package:flutter_test/flutter_test.dart';
import 'package:get_it/get_it.dart';
import 'package:kadrbox/core/error/exceptions.dart';
import 'package:kadrbox/core/network/api_client.dart';
import 'package:kadrbox/data/models/provider_catalog.dart';
import 'package:kadrbox/data/providers/server_backed_provider.dart';

/// Fake transport with per-route canned behaviour, call counting and an
/// optional gate so concurrent calls can be forced to overlap.
class FakeApiClient extends Fake implements ApiClient {
  final calls = <String, int>{};
  final Map<String, Object? Function()> listBodies = {};
  final Map<String, Object? Function()> mapBodies = {};
  Future<void>? gate;

  void _count(String route, Map<String, dynamic>? params) {
    final key = '$route:${_stable(params)}';
    calls[key] = (calls[key] ?? 0) + 1;
  }

  static String _stable(Map<String, dynamic>? params) {
    if (params == null || params.isEmpty) return '';
    final keys = params.keys.toList()..sort();
    return keys.map((k) => '$k=${params[k]}').join('&');
  }

  int callsFor(String route, Map<String, dynamic> params) =>
      calls['$route:${_stable(params)}'] ?? 0;

  int get totalCalls => calls.values.fold<int>(0, (a, b) => a + b);

  /// Bodies are keyed by route suffix (`/content/popular`) because the
  /// provider always calls with the full server URL.
  Object? _lookup(Map<String, Object? Function()> bodies, String url) {
    for (final entry in bodies.entries) {
      if (url.endsWith(entry.key)) return entry.value();
    }
    return null;
  }

  @override
  Future<List<dynamic>> getJsonList(
    String url, {
    Map<String, dynamic>? queryParameters,
    Map<String, dynamic>? headers,
  }) async {
    _count(url, queryParameters);
    await gate;
    final body = _lookup(listBodies, url);
    if (body is List) return List<dynamic>.from(body);
    return [];
  }

  @override
  Future<Map<String, dynamic>> getJson(
    String url, {
    Map<String, dynamic>? queryParameters,
    Map<String, dynamic>? headers,
  }) async {
    _count(url, queryParameters);
    await gate;
    final body = _lookup(mapBodies, url);
    if (body is Map) return Map<String, dynamic>.from(body);
    return {};
  }

  @override
  Future<dynamic> getRawJson(
    String url, {
    Map<String, dynamic>? queryParameters,
    Map<String, dynamic>? headers,
  }) async {
    _count(url, queryParameters);
    await gate;
    return null;
  }
}

ProviderCatalogEntry entry() => const ProviderCatalogEntry(
  id: 'src_d',
  name: 'Source D',
  baseUrl: 'http://localhost:8080',
  showOnHome: false,
  hasFixedStreams: false,
  contentTypes: ['movie', 'series'],
  searchEnabledDefault: true,
  enabled: true,
  healthy: true,
);

void main() {
  late FakeApiClient api;
  late DateTime now;

  setUp(() {
    api = FakeApiClient();
    now = DateTime(2026, 10, 1, 12);
    GetIt.instance.registerSingleton<ApiClient>(api);
  });

  tearDown(GetIt.instance.reset);

  ServerBackedProvider provider() =>
      ServerBackedProvider(entry(), clock: () => now);

  group('concurrent identical reads share one request', () {
    test('popular', () async {
      api.listBodies['/content/popular'] = () => [
        {'title': 'A', 'url': 'https://x/1'},
      ];
      final p = provider();

      final results = await Future.wait([
        p.getPopular(),
        p.getPopular(),
        p.getPopular(),
        p.getPopular(),
      ]);

      expect(api.totalCalls, 1);
      for (final r in results) {
        expect(r.map((i) => i.title), ['A']);
      }
      // Each waiter owns its list: mutating one must not affect the others.
      results.first.add(results.first.first);
      expect(results[1], hasLength(1));
    });

    test('streams are coalesced but never cached', () async {
      api.mapBodies['/content/streams'] = () => {
        'streams': [
          {'url': 'https://cdn/x.m3u8'},
        ],
      };
      final p = provider();

      final first = await Future.wait([
        p.getStreams('ref', season: 1, episode: 2),
        p.getStreams('ref', season: 1, episode: 2),
      ]);
      expect(first.every((s) => s.length == 1), isTrue);

      // Sequential calls go to the network again: signed URLs may rotate.
      await p.getStreams('ref', season: 1, episode: 2);

      expect(
        api.totalCalls,
        2,
        reason: 'one shared call for the concurrent pair, one for the rerun',
      );
    });

    test('details are served from the short cache afterwards', () async {
      api.mapBodies['/content/details'] = () => {
        'title': 'D',
        'url': 'https://x/d',
      };
      final p = provider();

      final first = await p.getDetails('ref');
      final second = await p.getDetails('ref');

      expect(first.item.title, 'D');
      expect(second.item.title, 'D');
      expect(
        api.totalCalls,
        1,
        reason: 'the same 47KB body must not be refetched a minute later',
      );
    });

    test('details cache expires', () async {
      api.mapBodies['/content/details'] = () => {
        'title': 'D',
        'url': 'https://x/d',
      };
      final p = provider();

      await p.getDetails('ref');
      now = now.add(const Duration(seconds: 121));
      await p.getDetails('ref');

      expect(api.totalCalls, 2);
    });
  });

  group('recent failures are not hammered', () {
    test('a 500 suppresses the identical read for a short while', () async {
      var attempts = 0;
      api.listBodies['/content/popular'] = () {
        attempts++;
        throw const ServerException(message: 'boom', statusCode: 500);
      };
      final p = provider();

      await expectLater(p.getPopular(), throwsA(isA<ServerException>()));
      await expectLater(p.getPopular(), throwsA(isA<ServerException>()));

      expect(attempts, 1);

      now = now.add(const Duration(seconds: 31));
      await expectLater(p.getPopular(), throwsA(isA<ServerException>()));
      expect(attempts, 2);
    });

    test('a 404 is never suppressed', () async {
      var attempts = 0;
      api.listBodies['/content/popular'] = () {
        attempts++;
        throw const ServerException(message: 'nope', statusCode: 404);
      };
      final p = provider();

      await expectLater(p.getPopular(), throwsA(isA<ServerException>()));
      await expectLater(p.getPopular(), throwsA(isA<ServerException>()));

      expect(
        attempts,
        2,
        reason: 'client errors are the caller’s fault, not the upstream’s',
      );
    });

    test('network errors are suppressed, then retried after expiry', () async {
      var attempts = 0;
      api.listBodies['/content/popular'] = () {
        attempts++;
        throw const NetworkException(message: 'down');
      };
      final p = provider();

      await expectLater(p.getPopular(), throwsA(isA<NetworkException>()));
      await expectLater(p.getPopular(), throwsA(isA<NetworkException>()));
      expect(attempts, 1);

      now = now.add(const Duration(seconds: 31));
      await expectLater(p.getPopular(), throwsA(isA<NetworkException>()));
      expect(attempts, 2);
    });
  });

  group('empty catalogues stay empty', () {
    test('a null popular body becomes an empty list', () async {
      api.listBodies['/content/popular'] = () => null;
      final p = provider();

      expect(await p.getPopular(), isEmpty);
    });
  });
}
