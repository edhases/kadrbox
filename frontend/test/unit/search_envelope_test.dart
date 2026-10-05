import 'package:flutter_test/flutter_test.dart';
import 'package:kadrbox/data/models/provider_catalog.dart';
import 'package:kadrbox/data/providers/server_backed_provider.dart';
import 'package:kadrbox/data/services/search/search_envelope.dart';
import 'package:kadrbox/domain/entities/entities.dart';

/// Wire format captured from `GET /content/search` on the Go backend.
///
/// The client is now a pure renderer of this envelope, so parsing is the one
/// place where a backend contract change silently becomes an empty screen.
/// These tests pin the exact shapes the server emits, including the awkward
/// cases: scalar-or-object coercion and the `content` field that is a string
/// for one source and a list for another.
///
/// The "ServerBackedProvider real mapping" group below uses
/// [ServerBackedProvider.mapItemForTest] — the production mapping — rather than
/// a local builder. A hand-rolled mapper is what let the `providerId` defect
/// survive: the test read `provider_id` correctly while the real mapper threw it
/// away, so both were green.

/// The envelope shape tests below still use a local mapper, because they are
/// about `SearchEnvelope.fromJson` in isolation and the mapper is a parameter.
/// They deliberately assert the envelope's own fields rather than MediaItem
/// attributes, so a divergence between this stub and the production mapper
/// cannot make them pass or fail for the wrong reason. The production mapping is
/// pinned separately, in the "ServerBackedProvider real mapping" group below.
MediaItem buildItem(Map<String, dynamic> json) {
  return MediaItem(
    id: json['id']?.toString() ?? '',
    title: json['title']?.toString() ?? '',
    type: ContentType.movie,
    providerId: json['provider_id']?.toString() ?? '',
    year: (json['year'] as num?)?.toInt(),
  );
}

void main() {
  group('SearchEnvelope.fromJson', () {
    test('parses a full response with segments, items and counters', () {
      final envelope = SearchEnvelope.fromJson({
        'query': 'Matrix',
        'canonical': 'Matrix',
        'took_ms': 314,
        'filtered_out': 394,
        'count': 12,
        'has_more': true,
        'next_page': 2,
        'segments': [
          {
            'id': 'main',
            'status': 'ok',
            'count': 12,
            'sources': {
              'src_a': {'status': 'ok', 'count': 8, 'elapsed_ms': 120},
              'src_b': {'status': 'timeout', 'count': 0, 'elapsed_ms': 3000},
            },
          },
        ],
        'items': [
          {
            'id': 'x1',
            'title': 'The Matrix',
            'score': 1.0,
            'matched_by': 'title_exact',
            'cluster': 'the matrix|1999|movie',
            'sources': [
              {
                'provider_id': 'src_d',
                'source_key': 'src_a',
                'item_id': 'tt0133093',
              },
            ],
          },
        ],
      }, buildItem);

      expect(envelope.query, 'Matrix');
      expect(envelope.canonical, 'Matrix');
      expect(envelope.tookMs, 314);
      expect(envelope.filteredOut, 394);
      expect(envelope.hasMore, isTrue);
      expect(envelope.nextPage, 2);

      expect(envelope.segments, hasLength(1));
      final segment = envelope.segments.first;
      expect(segment.id, 'main');
      expect(segment.sources.keys, containsAll(['src_a', 'src_b']));

      expect(envelope.items, hasLength(1));
      final scored = envelope.items.first;
      expect(scored.item.title, 'The Matrix');
      expect(scored.score, 1.0);
      expect(scored.matchedBy, 'title_exact');
      expect(scored.clusterKey, 'the matrix|1999|movie');
      expect(scored.sources.single.sourceKey, 'src_a');
      expect(scored.sources.single.itemId, 'tt0133093');
    });

    test('reports failures and empties separately', () {
      final envelope = SearchEnvelope.fromJson({
        'query': 'q',
        'segments': [
          {
            'id': 'main',
            'status': 'partial',
            'count': 0,
            'sources': {
              'src_a': {'status': 'ok', 'count': 0},
              'src_b': {'status': 'timeout', 'count': 0},
              'src_c': {'status': 'error', 'count': 0},
            },
          },
        ],
        'items': [],
      }, buildItem);

      final segment = envelope.segments.first;
      expect(
        segment.failures.map((s) => s.key),
        containsAll(['src_b', 'src_c']),
      );
      expect(
        segment.emptySources.map((s) => s.key),
        ['src_a'],
        reason: 'a source that answered with nothing is "empty", not "failed"',
      );
    });

    test('counts only sources that actually answered', () {
      final envelope = SearchEnvelope.fromJson({
        'query': 'q',
        'segments': [
          {
            'id': 'main',
            'status': 'partial',
            'count': 3,
            'sources': {
              'src_a': {'status': 'ok', 'count': 3},
              'src_b': {'status': 'timeout', 'count': 0},
            },
          },
        ],
        'items': [],
      }, buildItem);

      expect(envelope.answeredCount, 1);
      expect(envelope.askedCount, 2);
    });

    test('survives a response with no segments at all', () {
      final envelope = SearchEnvelope.fromJson({'query': 'q'}, buildItem);

      expect(envelope.items, isEmpty);
      expect(envelope.askedCount, 0);
      expect(envelope.failedSources, isEmpty);
      expect(envelope.answeredCount, 0);
    });

    test('treats an unrecognised status string as "answered"', () {
      // The status string comes from Source D's `meta.statuses` and is passed
      // through by the backend without validation. A value this client version
      // has never seen must not be misread as a failure — that would show a
      // scary "sources unavailable" banner over a perfectly good search.
      final envelope = SearchEnvelope.fromJson({
        'query': 'q',
        'segments': [
          {
            'id': 'main',
            'status': 'whatever-they-invent-next',
            'count': 4,
            'sources': {
              'src_a': {'status': 'brand_new_status', 'count': 4},
            },
          },
        ],
        'items': [],
      }, buildItem);

      expect(envelope.failedSources, isEmpty);
      expect(envelope.answeredCount, 1);
      expect(envelope.allSources.single.status, SourceStatus.unknown);
      expect(envelope.allSources.single.hasResults, isTrue);
    });

    test('derives "empty" from a zero count, not from the status string', () {
      final envelope = SearchEnvelope.fromJson({
        'query': 'q',
        'segments': [
          {
            'id': 'main',
            'status': 'ok',
            'count': 0,
            'sources': {
              'src_a': {'status': 'ok', 'count': 0},
            },
          },
        ],
        'items': [],
      }, buildItem);

      expect(envelope.segments.first.emptySources.map((s) => s.key), [
        'src_a',
      ]);
      expect(
        envelope.segments.first.emptySources.single.hasResults,
        isFalse,
        reason: 'a zero-hit source must not become a selectable filter',
      );
    });

    test('tolerates missing and wrongly-typed optional fields', () {
      // `score` arrives as an int from some paths and a float from others.
      // `sources` may be absent. Neither should throw.
      final envelope = SearchEnvelope.fromJson({
        'query': 'q',
        'segments': [
          {'id': 's', 'sources': <String, dynamic>{}},
        ],
        'items': [
          {'id': 'a', 'title': 'A', 'score': 1},
        ],
      }, buildItem);

      expect(envelope.items.single.score, 1.0);
      expect(envelope.items.single.sources, isEmpty);
      expect(envelope.filteredOut, 0);
      expect(envelope.tookMs, 0);
      expect(envelope.hasMore, isFalse);
    });
  });

  group('SearchEnvelope.empty', () {
    test('is empty but not failed', () {
      final envelope = SearchEnvelope.empty('q');

      expect(envelope.isEmpty, isTrue);
      expect(envelope.failedSources, isEmpty);
      expect(envelope.answeredCount, 0);
    });
  });

  group('ServerBackedProvider real mapping', () {
    test(
      'preserves provider_id and id without overwriting them with aggregator defaults',
      () {
        final entry = ProviderCatalogEntry.fromJson({
          'id': 'src_d',
          'name': 'Source D',
          'baseUrl': 'https://example.com',
        });
        final provider = ServerBackedProvider(entry);

        final item = provider.mapItemForTest({
          'provider_id': 'src_a',
          'id': 'abc',
          'url': 'https://src-a.example/item-123.html',
          'title': 'Sample Title',
          'type': 'movie',
        });

        expect(item.providerId, 'src_a');
        expect(item.id, 'https://src-a.example/item-123.html');
        expect(item.url, 'https://src-a.example/item-123.html');
        expect(item.uniqueId, 'src_a:https://src-a.example/item-123.html');
      },
    );

    test(
      'SearchEnvelope.fromJson with real mapping maps multi-provider items correctly',
      () {
        final entry = ProviderCatalogEntry.fromJson({
          'id': 'src_d',
          'name': 'Source D',
          'baseUrl': 'https://example.com',
        });
        final provider = ServerBackedProvider(entry);

        final envelope = SearchEnvelope.fromJson({
          'query': 'Dune',
          'items': [
            {
              'provider_id': 'src_a',
              'id': 'src-a-dune-1',
              'url': 'https://src-a.example/dune.html',
              'title': 'Dune',
              'score': 1.0,
            },
            {
              'provider_id': 'src_b',
              'id': 'src-b-dune-2',
              'url': 'https://src-b.example/dune.html',
              'title': 'Dune',
              'score': 0.9,
            },
          ],
        }, provider.mapItemForTest);

        // NOTE: two separate items both titled "Dune" with no year is NOT a
        // shape the server can produce. The Go clusterer
        // (search.GenerateClusterKey) keys on title+year+type, so it would
        // collapse these into ONE item carrying two sources. This test pins the
        // Dart mapping of a multi-provider payload, which is what the bug was
        // about; the clustering contract itself is asserted server-side.
        expect(envelope.items.length, 2);
        expect(envelope.items[0].item.providerId, 'src_a');
        expect(envelope.items[0].item.id, 'https://src-a.example/dune.html');
        expect(
          envelope.items[0].item.uniqueId,
          'src_a:https://src-a.example/dune.html',
        );

        expect(envelope.items[1].item.providerId, 'src_b');
        expect(envelope.items[1].item.id, 'https://src-b.example/dune.html');
        expect(
          envelope.items[1].item.uniqueId,
          'src_b:https://src-b.example/dune.html',
        );
      },
    );

    test('falls back to the aggregator id when provider_id is absent', () {
      // A per-provider endpoint that omits provider_id must still label items
      // with this provider, not leave them unattributed. This is the only
      // remaining path that could mislabel an item, so it needs its own case.
      final provider = ServerBackedProvider(
        ProviderCatalogEntry.fromJson({
          'id': 'src_a',
          'name': 'Source A',
          'baseUrl': 'https://example.com',
        }),
      );

      final item = provider.mapItemForTest({
        'id': 'https://src-a.example/dune.html',
        'title': 'Dune',
        'type': 'movie',
      });

      expect(item.providerId, 'src_a');
      expect(item.uniqueId, 'src_a:https://src-a.example/dune.html');
    });

    test('an empty provider_id string also falls back, not blank', () {
      // The server emits provider_id without omitempty, so it is always present
      // — but "" is what an unregistered provider looks like. Treating that as
      // a real id would put a nameless provider into the DB key.
      final provider = ServerBackedProvider(
        ProviderCatalogEntry.fromJson({
          'id': 'src_a',
          'name': 'Source A',
          'baseUrl': 'https://example.com',
        }),
      );

      final item = provider.mapItemForTest({
        'provider_id': '',
        'id': 'https://src-a.example/dune.html',
        'title': 'Dune',
        'type': 'movie',
      });

      expect(item.providerId, 'src_a');
    });
  });
}
