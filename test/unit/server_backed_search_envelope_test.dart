import 'package:flutter_test/flutter_test.dart';
import 'package:get_it/get_it.dart';
import 'package:oxide_film/core/network/api_client.dart';
import 'package:oxide_film/data/models/provider_catalog.dart';
import 'package:oxide_film/data/providers/server_backed_provider.dart';
import 'package:oxide_film/domain/entities/entities.dart';

/// Returns a canned body for any GET, so response-shape handling can be
/// exercised without a live backend.
class FakeApiClient extends Fake implements ApiClient {
  FakeApiClient(this.body);

  final Object? body;
  int callCount = 0;
  Map<String, dynamic>? lastQuery;

  @override
  Future<dynamic> getRawJson(
    String url, {
    Map<String, dynamic>? queryParameters,
    Map<String, dynamic>? headers,
  }) async {
    callCount++;
    lastQuery = queryParameters;
    return body;
  }
}

ProviderCatalogEntry entry() => const ProviderCatalogEntry(
  id: 'bandera',
  name: 'Bandera',
  baseUrl: 'http://localhost:8080',
  showOnHome: false,
  hasFixedStreams: false,
  contentTypes: ['movie', 'series'],
  searchEnabledDefault: true,
  enabled: true,
  healthy: true,
);

void main() {
  setUp(() {
    GetIt.instance.registerSingleton<ApiClient>(FakeApiClient(null));
  });

  tearDown(GetIt.instance.reset);

  /// Swap in a fake returning [body] and build a provider around it.
  Future<ServerBackedProvider> providerReturning(Object? body) async {
    final api = FakeApiClient(body);
    await GetIt.instance.unregister<ApiClient>();
    GetIt.instance.registerSingleton<ApiClient>(api);
    return ServerBackedProvider(entry());
  }

  group('searchEnvelope: current contract', () {
    test('parses the envelope and passes the query through', () async {
      final provider = await providerReturning({
        'query': 'Matrix',
        'canonical': 'Matrix',
        'took_ms': 120,
        'filtered_out': 7,
        'segments': [
          {
            'id': 'bandera',
            'status': 'ok',
            'count': 1,
            'sources': {
              'uaflix': {'status': 'ok', 'count': 1, 'elapsed_ms': 40},
            },
          },
        ],
        'items': [
          {'title': 'The Matrix', 'url': 'https://x/1'},
        ],
      });

      final envelope = await provider.searchEnvelope('Matrix');

      expect(envelope.items, hasLength(1));
      expect(envelope.items.first.item.title, 'The Matrix');
      expect(envelope.filteredOut, 7);
      expect(envelope.answeredCount, 1);
      expect(envelope.askedCount, 1);
      expect((GetIt.instance<ApiClient>() as FakeApiClient).lastQuery, {
        'q': 'Matrix',
      });
    });

    test('unwraps server response when wrapped in data property', () async {
      final provider = await providerReturning({
        'data': {
          'query': 'Matrix',
          'canonical': 'Matrix',
          'took_ms': 120,
          'filtered_out': 2,
          'segments': <dynamic>[],
          'items': [
            {'title': 'The Matrix Reloaded', 'url': 'https://x/2'},
          ],
        },
      });

      final envelope = await provider.searchEnvelope('Matrix');

      expect(envelope.items, hasLength(1));
      expect(envelope.items.first.item.title, 'The Matrix Reloaded');
      expect(envelope.filteredOut, 2);
    });

    test('does not send a page parameter the backend ignores', () async {
      final provider = await providerReturning({
        'query': 'q',
        'segments': <dynamic>[],
        'items': <dynamic>[],
      });

      await provider.searchEnvelope('q', page: 3);

      final params = (GetIt.instance<ApiClient>() as FakeApiClient).lastQuery;
      expect(
        params,
        isNot(contains('page')),
        reason: 'sending an ignored page makes every page return page 1',
      );
    });
  });

  group('searchEnvelope: legacy flat list', () {
    test('wraps the list without inventing a score', () async {
      final provider = await providerReturning([
        {'title': 'Вітролом', 'url': 'https://x/1'},
        {'title': 'Matrix', 'url': 'https://x/2'},
      ]);

      final envelope = await provider.searchEnvelope('Вітролом');

      expect(envelope.items, hasLength(2));
      expect(
        envelope.items.every((s) => s.score == 0),
        isTrue,
        reason:
            'the old server never scored; a fabricated 1.0 would make '
            'unranked results look authoritatively relevant',
      );
      expect(envelope.items.every((s) => s.matchedBy == 'legacy'), isTrue);
    });

    test('invents no source attribution', () async {
      final provider = await providerReturning([
        {'title': 'Вітролом', 'url': 'https://x/1'},
      ]);

      final envelope = await provider.searchEnvelope('Вітролом');

      expect(
        envelope.items.first.sources,
        isEmpty,
        reason: 'a flat list carries no source data, so none may be claimed',
      );
      expect(
        envelope.askedCount,
        0,
        reason:
            'reporting a source the server never mentioned would let the UI '
            'render a chip that filters on nothing',
      );
    });

    test('skips non-object entries instead of throwing', () async {
      final provider = await providerReturning([
        {'title': 'Good', 'url': 'https://x/1'},
        'garbage',
        42,
        null,
        {'title': 'Also good', 'url': 'https://x/2'},
      ]);

      final envelope = await provider.searchEnvelope('q');

      expect(envelope.items.map((s) => s.item.title), ['Good', 'Also good']);
    });
  });

  group('searchEnvelope: malformed responses', () {
    // The whole point of surfacing failures is that a broken response must
    // NOT look like "the search found nothing".
    test('throws on a bare string body', () async {
      final provider = await providerReturning('<html>502 Bad Gateway</html>');

      expect(
        () => provider.searchEnvelope('q'),
        throwsA(isA<FormatException>()),
      );
    });

    test('throws on a null body', () async {
      final provider = await providerReturning(null);

      expect(
        () => provider.searchEnvelope('q'),
        throwsA(isA<FormatException>()),
      );
    });

    test('throws on a numeric body', () async {
      final provider = await providerReturning(200);

      expect(
        () => provider.searchEnvelope('q'),
        throwsA(isA<FormatException>()),
      );
    });
  });

  group('searchEnvelope: type narrowing', () {
    test('filters client-side and keeps source stats intact', () async {
      final provider = await providerReturning({
        'query': 'q',
        'took_ms': 50,
        'filtered_out': 3,
        'segments': [
          {
            'id': 'bandera',
            'status': 'ok',
            'count': 2,
            'sources': {
              'uaflix': {'status': 'ok', 'count': 2},
            },
          },
        ],
        'items': [
          {'title': 'Movie', 'type': 'movie', 'url': 'https://x/1'},
          {'title': 'Show', 'type': 'series', 'url': 'https://x/2'},
        ],
      });

      final envelope = await provider.searchEnvelope(
        'q',
        type: ContentType.series,
      );

      expect(envelope.items, hasLength(1));
      expect(envelope.items.first.item.title, 'Show');
      expect(
        envelope.askedCount,
        1,
        reason: 'narrowing type must not drop the source breakdown',
      );
      expect(envelope.filteredOut, 3);
    });
  });
}
