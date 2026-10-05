import 'package:flutter_test/flutter_test.dart';
import 'package:kadrbox/data/database/app_database.dart';
import 'package:kadrbox/data/database/dao/search_history_dao.dart';
import 'package:kadrbox/data/providers/provider_registry.dart';
import 'package:kadrbox/data/providers/server_backed_provider.dart';
import 'package:kadrbox/data/services/search/search_envelope.dart';
import 'package:kadrbox/data/services/smart_search/smart_search_service.dart';
import 'package:kadrbox/domain/entities/entities.dart';
import 'package:kadrbox/domain/repositories/content_provider.dart';

/// Stands in for the server-backed source the search path talks to.
///
/// Records how many times the client asked it to search, because the central
/// invariant of this refactor is that one user-initiated search produces
/// exactly one backend request — the old implementation fanned out across
/// transliteration variants and every scraper simultaneously.
class FakeServerProvider extends Fake implements ServerBackedProvider {
  FakeServerProvider(this.id, this._searches);
  @override
  final String id;

  final List<String> _searches;
  Object? Function()? onSearch;

  int get searchCount => _searches.length;
  List<String> get queries => List.unmodifiable(_searches);

  @override
  Future<SearchEnvelope> searchEnvelope(
    String query, {
    ContentType? type,
    int page = 1,
  }) async {
    _searches.add(query);
    final hook = onSearch;
    if (hook != null) return hook() as SearchEnvelope;
    return SearchEnvelope.empty(query);
  }
}

class FakeProviderRegistry extends Fake implements ProviderRegistry {
  FakeProviderRegistry(this._providers);

  final List<ContentProvider> _providers;

  /// Resolves a source id to the registered provider, falling back to the one
  /// server-backed provider when nothing matches.
  ///
  /// The fallback is deliberate: the app must not know which catalogue server
  /// it talks to, so the search path resolves "a server-backed source" instead
  /// of a hardcoded id. Pinning a brand here would make these tests assert an
  /// id the app is not allowed to carry.
  @override
  ContentProvider? getById(String providerId) {
    for (final p in _providers) {
      if (p.id == providerId) return p;
    }
    for (final p in _providers) {
      if (p is ServerBackedProvider) return p;
    }
    return null;
  }

  /// Every registered provider counts as enabled in these tests.
  ///
  /// Added alongside the change that made SmartSearchService resolve its
  /// server-backed provider by capability over `enabled` instead of by a
  /// hardcoded id. The fake modelled only the seam the old implementation
  /// happened to use, so a name-agnostic lookup found nothing here -- which is
  /// the fake being incomplete, not the lookup being wrong. Every provider in
  /// this suite is registered precisely because it should be used.
  @override
  List<ContentProvider> get enabled => List.unmodifiable(_providers);
}

class FakeSearchHistoryDao extends Fake implements SearchHistoryDao {
  final added = <String>[];

  @override
  Future<void> addSearch({
    required String query,
    required String normalizedQuery,
    required int resultCount,
  }) async {
    added.add(query);
  }

  @override
  Future<List<SearchHistoryTableData>> searchByPrefix(
    String prefix, {
    int limit = 10,
  }) async => [];

  @override
  Future<List<SearchHistoryTableData>> getSuccessful({int limit = 20}) async =>
      [];

  @override
  Future<List<SearchHistoryTableData>> getRecent({int limit = 100}) async => [];
}

/// Minimal [SearchEnvelope] with one ok source and one failed source.
SearchEnvelope envelopeWith({
  required String query,
  List<SearchItemSource> itemSources = const [],
  List<ScoredMediaItem> items = const [],
  int filteredOut = 0,
  bool withFailure = false,
}) {
  final sources = <String, SearchSourceStatus>{
    'src_a': SearchSourceStatus(
      key: 'src_a',
      status: SourceStatus.ok,
      count: items.isEmpty ? 0 : 1,
      elapsedMs: 12,
    ),
  };
  if (withFailure) {
    sources['src_b'] = SearchSourceStatus(
      key: 'src_b',
      status: SourceStatus.timeout,
      count: 0,
      elapsedMs: 3000,
    );
  }

  final segment = SearchSegment(
    id: 'main',
    status: 'ok',
    count: items.length,
    sources: sources,
  );

  return SearchEnvelope(
    query: query,
    canonical: query,
    tookMs: 42,
    segments: [segment],
    items: items,
    filteredOut: filteredOut,
  );
}

void main() {
  group('SmartSearchService', () {
    late List<String> searches;
    late FakeServerProvider provider;
    late SmartSearchService service;

    setUp(() {
      searches = [];
      provider = FakeServerProvider('src_d', searches);
      service = SmartSearchService(
        FakeProviderRegistry([provider]),
        FakeSearchHistoryDao(),
      );
    });

    test('issues exactly one backend request per search', () async {
      provider.onSearch = () => envelopeWith(query: 'Matrix');

      await service.search('Matrix').toList();

      expect(
        provider.searchCount,
        1,
        reason: 'one search, one request — no variant or provider fan-out',
      );
      expect(provider.queries, ['Matrix']);
    });

    test('does not transliterate the query before searching', () async {
      provider.onSearch = () => envelopeWith(query: 'Матриця');

      await service.search('Матриця').toList();

      expect(
        provider.queries,
        ['Матриця'],
        reason: 'normalisation is server-side; the client sends what was typed',
      );
    });

    test('returns ranked items without reordering them', () async {
      final items = [
        ScoredMediaItem(
          item: MediaItem(
            id: 'a',
            title: 'A',
            type: ContentType.movie,
            providerId: 'src_d',
          ),
          score: 0.91,
          matchedBy: 'title_exact',
          clusterKey: 'a|movie',
          sources: const [],
        ),
        ScoredMediaItem(
          item: MediaItem(
            id: 'b',
            title: 'B',
            type: ContentType.movie,
            providerId: 'src_d',
          ),
          score: 0.42,
          matchedBy: 'partial_tokens',
          clusterKey: 'b|movie',
          sources: const [],
        ),
      ];
      provider.onSearch = () => envelopeWith(query: 'q', items: items);

      final result = await service.search('q').first;

      expect(
        result.rankedItems.map((i) => i.id),
        ['a', 'b'],
        reason: 'server order is authoritative — the client must not re-rank',
      );
    });

    test('surfaces filtered-out count for display', () async {
      provider.onSearch = () => envelopeWith(query: 'q', filteredOut: 394);

      final result = await service.search('q').first;

      expect(result.filteredOut, 394);
    });

    test('exposes failed sources so the UI can warn', () async {
      provider.onSearch = () => envelopeWith(query: 'q', withFailure: true);

      final result = await service.search('q').first;

      expect(result.hasError, isFalse);
      expect(result.failedSources.map((s) => s.key), ['src_b']);
    });

    test('reports a backend failure instead of an empty result', () async {
      provider.onSearch = () => throw Exception('backend down');

      final result = await service.search('q').first;

      expect(
        result.hasError,
        isTrue,
        reason: 'a dead backend must not look like "nothing found"',
      );
      expect(result.isEmpty, isTrue);
    });

    test('saves the search to history', () async {
      final dao = FakeSearchHistoryDao();
      final svc = SmartSearchService(FakeProviderRegistry([provider]), dao);
      provider.onSearch = () => envelopeWith(query: 'Inception');

      await svc.search('Inception').first;

      expect(dao.added, ['Inception']);
    });

    test('short-circuits a blank query without hitting the network', () async {
      final result = await service.search('   ').first;

      expect(result.isEmpty, isTrue);
      expect(provider.searchCount, 0);
    });
  });
}
