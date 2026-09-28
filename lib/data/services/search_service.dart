import 'dart:async';

import 'package:equatable/equatable.dart';

import '../../core/utils/logger.dart';
import '../../domain/entities/entities.dart';
import '../../domain/repositories/content_provider.dart';
import '../providers/provider_registry.dart';

/// Result from a single provider search
class ProviderSearchResult {
  final String providerId;
  final String providerName;
  final List<MediaItem> items;
  final String? error;
  final Duration searchDuration;

  const ProviderSearchResult({
    required this.providerId,
    required this.providerName,
    required this.items,
    this.error,
    required this.searchDuration,
  });

  bool get isSuccess => error == null;
  bool get isEmpty => items.isEmpty;
}

/// Aggregated search results from all providers
class AggregatedSearchResult extends Equatable {
  final String query;
  final List<ProviderSearchResult> providerResults;
  final Duration totalDuration;
  final bool isComplete;

  const AggregatedSearchResult({
    required this.query,
    required this.providerResults,
    required this.totalDuration,
    this.isComplete = true,
  });

  /// All items from all providers (flat list)
  List<MediaItem> get allItems =>
      providerResults.where((r) => r.isSuccess).expand((r) => r.items).toList();

  /// Items grouped by provider
  Map<String, List<MediaItem>> get itemsByProvider => {
    for (final result in providerResults.where((r) => r.isSuccess))
      result.providerId: result.items,
  };

  /// Total number of results
  int get totalCount =>
      providerResults.fold(0, (sum, r) => sum + r.items.length);

  /// Number of successful provider searches
  int get successCount => providerResults.where((r) => r.isSuccess).length;

  /// Number of failed provider searches
  int get failureCount => providerResults.where((r) => !r.isSuccess).length;

  /// Get items filtered by provider ID (null = all providers)
  List<MediaItem> getItemsForProvider(String? providerId) {
    if (providerId == null) return allItems;
    return itemsByProvider[providerId] ?? [];
  }

  @override
  List<Object?> get props => [
    query,
    providerResults,
    totalDuration,
    isComplete,
  ];
}

/// Search service that aggregates results from multiple providers
///
/// Features:
/// - Parallel search across all enabled providers
/// - Progressive results (stream results as they arrive)
/// - Timeout handling per provider
/// - Error isolation (one provider failing doesn't break others)
/// - Deduplication (optional)
class SearchService {
  static const String _tag = 'SearchService';

  /// Timeout for individual provider searches
  static const Duration _providerTimeout = Duration(seconds: 10);

  /// Timeout for suggestions (shorter for UX)
  static const Duration _suggestionTimeout = Duration(seconds: 3);

  final ProviderRegistry _registry;

  SearchService(this._registry);

  /// Perform parallel search across all enabled providers
  ///
  /// Returns aggregated results from all providers.
  /// Each provider search is isolated - failures don't affect others.
  Future<AggregatedSearchResult> search(
    String query, {
    ContentType? type,
    int page = 1,
    String? onlyProviderId,
  }) async {
    final stopwatch = Stopwatch()..start();
    final results = <ProviderSearchResult>[];

    // Get providers to search
    List<ContentProvider> providers;
    if (onlyProviderId != null) {
      final provider = _registry.getById(onlyProviderId);
      providers = provider != null ? [provider] : [];
    } else {
      providers = _registry.enabled;
    }

    if (providers.isEmpty) {
      Logger.w('No enabled providers for search', tag: _tag);
      return AggregatedSearchResult(
        query: query,
        providerResults: [],
        totalDuration: stopwatch.elapsed,
      );
    }

    Logger.i(
      'Searching "$query" across ${providers.length} providers',
      tag: _tag,
    );

    // Search all providers in parallel
    final futures = providers.map(
      (provider) => _searchProvider(provider, query, type: type, page: page),
    );

    final providerResults = await Future.wait(futures);
    results.addAll(providerResults);

    stopwatch.stop();

    final aggregated = AggregatedSearchResult(
      query: query,
      providerResults: results,
      totalDuration: stopwatch.elapsed,
    );

    Logger.i(
      'Search completed: ${aggregated.totalCount} results from '
      '${aggregated.successCount}/${providers.length} providers '
      'in ${stopwatch.elapsedMilliseconds}ms',
      tag: _tag,
    );

    return aggregated;
  }

  /// Stream search results as they arrive from each provider
  ///
  /// Useful for progressive UI updates - shows results immediately
  /// without waiting for all providers to complete.
  Stream<AggregatedSearchResult> searchStream(
    String query, {
    ContentType? type,
    int page = 1,
  }) async* {
    final stopwatch = Stopwatch()..start();
    final results = <ProviderSearchResult>[];
    final providers = _registry.enabled;

    if (providers.isEmpty) {
      yield AggregatedSearchResult(
        query: query,
        providerResults: [],
        totalDuration: stopwatch.elapsed,
      );
      return;
    }

    Logger.i(
      'Stream searching "$query" across ${providers.length} providers',
      tag: _tag,
    );

    int completedCount = 0;
    final controller = StreamController<AggregatedSearchResult>();

    for (final provider in providers) {
      _searchProvider(provider, query, type: type, page: page).then((result) {
        results.add(result);
        completedCount++;
        if (!controller.isClosed) {
          controller.add(
            AggregatedSearchResult(
              query: query,
              providerResults: List.from(results),
              totalDuration: stopwatch.elapsed,
              isComplete: completedCount == providers.length,
            ),
          );
        }
        if (completedCount == providers.length && !controller.isClosed) {
          controller.close();
        }
      }).catchError((e) {
        Logger.w('Stream search error for ${provider.name}: $e', tag: _tag);
        completedCount++;
        if (!controller.isClosed) {
          controller.add(
            AggregatedSearchResult(
              query: query,
              providerResults: List.from(results),
              totalDuration: stopwatch.elapsed,
              isComplete: completedCount == providers.length,
            ),
          );
        }
        if (completedCount == providers.length && !controller.isClosed) {
          controller.close();
        }
      });
    }

    yield* controller.stream;
  }

  /// Get quick suggestions from fastest providers
  ///
  /// Uses shorter timeout and limits results per provider for speed.
  Future<List<MediaItem>> getSuggestions(
    String query, {
    int maxPerProvider = 3,
    int maxTotal = 10,
  }) async {
    if (query.length < 2) return [];

    final providers = _registry.enabled;
    if (providers.isEmpty) return [];

    Logger.d('Getting suggestions for "$query"', tag: _tag);

    // Search all providers in parallel with short timeout
    final futures = providers.map(
      (provider) =>
          _searchProvider(provider, query, timeout: _suggestionTimeout)
              .then((result) => result.items.take(maxPerProvider).toList())
              .catchError((_) => <MediaItem>[]),
    );

    final allResults = await Future.wait(futures);

    // Flatten and limit total
    final suggestions = allResults
        .expand((items) => items)
        .take(maxTotal)
        .toList();

    Logger.d('Got ${suggestions.length} suggestions', tag: _tag);
    return suggestions;
  }

  /// Search a single provider with timeout and error handling
  Future<ProviderSearchResult> _searchProvider(
    ContentProvider provider,
    String query, {
    ContentType? type,
    int page = 1,
    Duration? timeout,
  }) async {
    final stopwatch = Stopwatch()..start();

    try {
      final results = await provider
          .search(query, type: type, page: page)
          .timeout(timeout ?? _providerTimeout);

      stopwatch.stop();

      return ProviderSearchResult(
        providerId: provider.id,
        providerName: provider.name,
        items: results,
        searchDuration: stopwatch.elapsed,
      );
    } on TimeoutException {
      stopwatch.stop();
      Logger.w('Search timeout for ${provider.name}', tag: _tag);

      return ProviderSearchResult(
        providerId: provider.id,
        providerName: provider.name,
        items: [],
        error: 'Timeout',
        searchDuration: stopwatch.elapsed,
      );
    } catch (e) {
      stopwatch.stop();
      Logger.w('Search failed for ${provider.name}: $e', tag: _tag);

      return ProviderSearchResult(
        providerId: provider.id,
        providerName: provider.name,
        items: [],
        error: e.toString(),
        searchDuration: stopwatch.elapsed,
      );
    }
  }

  /// Deduplicate results based on title similarity
  ///
  /// Keeps the result from the highest priority provider when duplicates found.
  List<MediaItem> deduplicateResults(
    List<MediaItem> items, {
    double similarityThreshold = 0.85,
  }) {
    if (items.length <= 1) return items;

    final unique = <MediaItem>[];
    final seenTitles = <String>{};

    for (final item in items) {
      final normalizedTitle = _normalizeTitle(item.title);

      // Check if we've seen a similar title
      bool isDuplicate = false;
      for (final seen in seenTitles) {
        if (_calculateSimilarity(normalizedTitle, seen) >=
            similarityThreshold) {
          isDuplicate = true;
          break;
        }
      }

      if (!isDuplicate) {
        unique.add(item);
        seenTitles.add(normalizedTitle);
      }
    }

    return unique;
  }

  /// Normalize title for comparison
  String _normalizeTitle(String title) {
    return title
        .toLowerCase()
        .replaceAll(RegExp(r'[^\w\s]'), '') // Remove punctuation
        .replaceAll(RegExp(r'\s+'), ' ') // Normalize whitespace
        .trim();
  }

  /// Calculate similarity between two strings (Jaccard similarity)
  double _calculateSimilarity(String a, String b) {
    if (a == b) return 1.0;
    if (a.isEmpty || b.isEmpty) return 0.0;

    final setA = a.split(' ').toSet();
    final setB = b.split(' ').toSet();

    final intersection = setA.intersection(setB).length;
    final union = setA.union(setB).length;

    return union > 0 ? intersection / union : 0.0;
  }
}
