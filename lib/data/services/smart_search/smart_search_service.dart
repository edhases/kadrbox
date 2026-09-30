import 'dart:async';

import 'package:flutter/foundation.dart';
import 'package:fuzzywuzzy/fuzzywuzzy.dart';

import '../../../core/utils/logger.dart';
import '../../../domain/entities/entities.dart';
import '../../database/dao/search_history_dao.dart';
import '../../providers/provider_registry.dart';
import '../search_service.dart';
import 'transliteration_service.dart';

/// Smart search service with fuzzy matching, transliteration, and history
///
/// Features:
/// - Automatic Latin ↔ Cyrillic transliteration
/// - Fuzzy matching for typo tolerance
/// - Search history for autocomplete
/// - Memory cache for fast suggestions
/// - Result ranking by relevance
class SmartSearchService {
  static const String _tag = 'SmartSearch';

  /// Fuzzy matching threshold (0-100)
  /// 75 = allows 1-2 typos
  static const int _fuzzyThreshold = 75;

  /// Deduplication threshold (0-100)
  /// 92 = тільки майже ідентичні назви зливаються; 85 було надто агресивно
  /// для серіалів де є "Серія 68", "Серія 112" — вони різні але схожі
  static const int _deduplicationThreshold = 92;

  final SearchService _searchService;
  // ignore: unused_field - reserved for future provider-specific logic
  final ProviderRegistry _registry;
  final TransliterationService _transliteration;
  final SearchHistoryDao _historyDao;

  /// Memory cache for search results (cleared on app restart)
  final Map<String, AggregatedSearchResult> _memoryCache = {};

  /// Cache TTL (10 minutes)
  static const Duration _cacheTtl = Duration(minutes: 10);
  final Map<String, DateTime> _cacheTimestamps = {};

  SmartSearchService(this._searchService, this._registry, this._historyDao)
    : _transliteration = TransliterationService();

  /// Perform smart search with transliteration and fuzzy matching
  ///
  /// 1. Generates search variants (transliterated, translated)
  /// 2. Searches all variants in parallel
  /// 3. Merges and deduplicates results
  /// 4. Ranks by relevance
  /// 5. Saves to history
  Stream<SmartSearchResult> search(
    String query, {
    ContentType? type,
    int page = 1,
  }) {
    final normalized = _transliteration.normalizeQuery(query);
    if (normalized.isEmpty) {
      return Stream.value(SmartSearchResult.empty(query));
    }

    // Check memory cache first (emit immediate value)
    final cacheKey = '$normalized:$type:$page';
    if (_isCacheValid(cacheKey)) {
      // If we have full cache, we can just emit it?
      // But maybe we want to refresh in background?
      // For now, if cache hit, emit it as single value stream.
      // Or we can emit it and THEN search fresh if valid-but-stale?
      // Existing logic used cache as final.
      return Stream.fromFuture(() async {
        Logger.d('Cache hit for "$normalized"', tag: _tag);
        final cached = _memoryCache[cacheKey]!;
        final ranked = await compute(_processResultsCompute, {
          'items': cached.allItems,
          'query': normalized,
          'fuzzyThreshold': _fuzzyThreshold,
          'dedupThreshold': _deduplicationThreshold,
        });
        return SmartSearchResult(
          originalQuery: query,
          normalizedQuery: normalized,
          searchVariants: [normalized],
          aggregatedResult: cached,
          rankedItems: ranked,
          totalDuration: Duration.zero,
          fromCache: true,
        );
      }());
    }

    final controller = StreamController<SmartSearchResult>();
    final stopwatch = Stopwatch()..start();

    // Generate variants
    final variants = _transliteration.generateSearchVariants(query);
    Logger.d('Search variants for "$query": $variants', tag: _tag);

    // Track state per variant
    final variantResults = <String, AggregatedSearchResult>{
      for (var v in variants)
        v: AggregatedSearchResult(
          query: v,
          providerResults: const [],
          totalDuration: Duration.zero,
        ),
    };

    // Throttling Logic
    Timer? throttleTimer;
    bool isDirty = false;
    int completedStreams = 0;

    Future<void> processAndEmit({bool closeAfter = false}) async {
      isDirty = false;

      // Combine all results
      final allProviderResults = variantResults.values
          .expand((r) => r.providerResults)
          .toList();

      final combinedAggregated = AggregatedSearchResult(
        query: query,
        providerResults: allProviderResults,
        totalDuration: stopwatch.elapsed,
        isComplete: completedStreams == variants.length,
      );

      final allItems = combinedAggregated.allItems;

      // Run heavy compute in isolate
      try {
        final ranked = await compute(_processResultsCompute, {
          'items': allItems,
          'query': normalized,
          'fuzzyThreshold': _fuzzyThreshold,
          'dedupThreshold': _deduplicationThreshold,
        });

        if (!controller.isClosed) {
          final result = SmartSearchResult(
            originalQuery: query,
            normalizedQuery: normalized,
            searchVariants: variants,
            aggregatedResult: combinedAggregated,
            rankedItems: ranked,
            totalDuration: stopwatch.elapsed,
            fromCache: false,
          );

          if (completedStreams == variants.length) {
            _memoryCache[cacheKey] = combinedAggregated;
            _cacheTimestamps[cacheKey] = DateTime.now();

            Logger.i(
              'Smart search completed: ${ranked.length} results in ${stopwatch.elapsedMilliseconds}ms',
              tag: _tag,
            );
            await _saveToHistory(query, normalized, ranked.length);
          }

          controller.add(result);
        }
      } catch (e) {
        Logger.e('Error processing search results', tag: _tag, error: e);
      } finally {
        if (closeAfter && !controller.isClosed) {
          controller.close();
          stopwatch.stop();
        }
      }
    }

    void onUpdate() {
      if (throttleTimer?.isActive ?? false) {
        isDirty = true;
      } else {
        processAndEmit();
        throttleTimer = Timer(const Duration(milliseconds: 100), () {
          if (isDirty) processAndEmit();
          throttleTimer = null;
        });
      }
    }

    // Launch streams
    for (final variant in variants) {
      _searchService
          .searchStream(variant, type: type, page: page)
          .listen(
            (event) {
              variantResults[variant] = event;
              onUpdate();
            },
            onError: (e) {
              Logger.w(
                'Search error for variant $variant',
                tag: _tag,
                error: e,
              );
            },
            onDone: () {
              completedStreams++;
              if (completedStreams == variants.length) {
                throttleTimer?.cancel(); // Cancel pending
                processAndEmit(closeAfter: true); // Final emit
              }
            },
          );
    }

    return controller.stream;
  }

  /// Get quick suggestions from history and memory cache
  Future<List<SearchSuggestion>> getSuggestions(
    String query, {
    int limit = 10,
  }) async {
    if (query.length < 2) return [];

    final normalized = _transliteration.normalizeQuery(query);
    final suggestions = <SearchSuggestion>[];

    // 1. Get matching history entries
    try {
      final historyMatches = await _historyDao.searchByPrefix(
        normalized,
        limit: 5,
      );

      for (final entry in historyMatches) {
        suggestions.add(
          SearchSuggestion(
            text: entry.query,
            type: SuggestionType.history,
            searchCount: entry.searchCount,
          ),
        );
      }
    } catch (e) {
      Logger.w('Failed to get history suggestions: $e', tag: _tag);
    }

    // 2. Check memory cache for partial matches
    for (final cacheKey in _memoryCache.keys) {
      if (suggestions.length >= limit) break;

      final cachedQuery = cacheKey.split(':').first;
      if (cachedQuery.startsWith(normalized) && cachedQuery != normalized) {
        // Don't add duplicates
        if (!suggestions.any((s) => s.text == cachedQuery)) {
          suggestions.add(
            SearchSuggestion(text: cachedQuery, type: SuggestionType.cached),
          );
        }
      }
    }

    // 3. Get live suggestions from providers (fast timeout)
    if (suggestions.length < limit) {
      try {
        final liveSuggestions = await _searchService.getSuggestions(
          query,
          maxPerProvider: 2,
          maxTotal: limit - suggestions.length,
        );

        for (final item in liveSuggestions) {
          if (suggestions.length >= limit) break;

          // Add as title suggestion
          if (!suggestions.any(
            (s) => s.text.toLowerCase() == item.title.toLowerCase(),
          )) {
            suggestions.add(
              SearchSuggestion(
                text: item.title,
                type: SuggestionType.live,
                mediaItem: item,
              ),
            );
          }
        }
      } catch (e) {
        Logger.w('Failed to get live suggestions: $e', tag: _tag);
      }
    }

    return suggestions.take(limit).toList();
  }

  /// Check if query might have a typo based on fuzzy matching with history
  Future<String?> suggestCorrection(String query) async {
    final normalized = _transliteration.normalizeQuery(query);
    if (normalized.length < 3) return null;

    try {
      // Get successful searches from history
      final successful = await _historyDao.getSuccessful(limit: 50);

      String? bestMatch;
      int bestScore = 0;

      for (final entry in successful) {
        final score = ratio(normalized, entry.normalizedQuery);

        // If similar but not exact, and better than threshold
        if (score >= _fuzzyThreshold && score < 100 && score > bestScore) {
          bestScore = score;
          bestMatch = entry.query;
        }
      }

      return bestMatch;
    } catch (e) {
      Logger.w('Failed to suggest correction: $e', tag: _tag);
      return null;
    }
  }

  /// Get recent searches for display
  Future<List<String>> getRecentSearches({int limit = 10}) async {
    try {
      final history = await _historyDao.getRecent(limit: limit);
      return history.map((e) => e.query).toList();
    } catch (e) {
      Logger.w('Failed to get recent searches: $e', tag: _tag);
      return [];
    }
  }

  /// Clear search history
  Future<void> clearHistory() async {
    try {
      await _historyDao.clearAll();
      Logger.i('Search history cleared', tag: _tag);
    } catch (e) {
      Logger.e('Failed to clear history', tag: _tag, error: e);
    }
  }

  /// Clear memory cache
  void clearCache() {
    _memoryCache.clear();
    _cacheTimestamps.clear();
    Logger.d('Memory cache cleared', tag: _tag);
  }

  // =========================================================================
  // Private methods
  // =========================================================================

  bool _isCacheValid(String key) {
    if (!_memoryCache.containsKey(key)) return false;

    final timestamp = _cacheTimestamps[key];
    if (timestamp == null) return false;

    return DateTime.now().difference(timestamp) < _cacheTtl;
  }

  Future<void> _saveToHistory(
    String query,
    String normalizedQuery,
    int resultCount,
  ) async {
    try {
      await _historyDao.addSearch(
        query: query,
        normalizedQuery: normalizedQuery,
        resultCount: resultCount,
      );
    } catch (e) {
      Logger.w('Failed to save search history: $e', tag: _tag);
    }
  }

  /// Process results in isolate (deduplication + ranking)
  static List<MediaItem> _processResultsCompute(Map<String, dynamic> args) {
    final items = args['items'] as List<MediaItem>;
    final query = args['query'] as String;
    final fuzzyThreshold = args['fuzzyThreshold'] as int;
    final dedupThreshold = args['dedupThreshold'] as int;

    // 1. Deduplicate
    final unique = <MediaItem>[];
    final seenTitles = <String>[];

    for (final item in items) {
      final normalizedTitle = item.title.toLowerCase().trim();
      bool isDuplicate = false;
      for (final seen in seenTitles) {
        final similarity = ratio(normalizedTitle, seen);
        if (similarity >= dedupThreshold) {
          isDuplicate = true;
          break;
        }
      }
      if (!isDuplicate) {
        unique.add(item);
        seenTitles.add(normalizedTitle);
      }
    }

    // 2. Rank
    final scored = unique.map((item) {
      final score = _calculateRelevanceScoreStatic(item, query, fuzzyThreshold);
      return _ScoredItem(item, score);
    }).toList();

    scored.sort((a, b) => b.score.compareTo(a.score));
    return scored.map((s) => s.item).toList();
  }

  static double _calculateRelevanceScoreStatic(
    MediaItem item,
    String query,
    int fuzzyThreshold,
  ) {
    var score = 0.0;
    final normalizedTitle = item.title.toLowerCase().trim();
    final normalizedQuery = query.toLowerCase().trim();

    // Точний збіг назви — максимальний пріоритет
    if (normalizedTitle == normalizedQuery) {
      score += 150;
    } else if (normalizedTitle.startsWith(normalizedQuery)) {
      score += 80;
    } else if (normalizedTitle.contains(normalizedQuery)) {
      score += 40;
    }

    // Fuzzy similarity
    final similarity = ratio(normalizedTitle, normalizedQuery);
    score += similarity * 0.5;

    // Рейтинг
    if (item.rating != null) {
      score += item.rating! * 2;
    }

    // Бонус за свіжий контент (але слабший ніж раніше)
    if (item.year != null) {
      final yearsOld = DateTime.now().year - item.year!;
      if (yearsOld < 5) {
        score += (5 - yearsOld) * 1.5;
      }
    }

    // Невеликий бонус за наявність постеру
    if (item.posterUrl != null && item.posterUrl!.isNotEmpty) {
      score += 5;
    }

    // Штраф за заголовки-серії типу "Серія 101 Великолепный" — вони менш релевантні
    // як результат пошуку назви серіалу, ніж сам серіал
    if (normalizedTitle.contains('серия') ||
        normalizedTitle.contains('серія') ||
        RegExp(r'^(episode|ep\.?\s*\d+|\d+\s*серия|\d+\s*серія)').hasMatch(normalizedTitle)) {
      score -= 15;
    }

    return score;
  }
}

/// Result of smart search
class SmartSearchResult {
  final String originalQuery;
  final String normalizedQuery;
  final List<String> searchVariants;
  final AggregatedSearchResult aggregatedResult;
  final List<MediaItem> rankedItems;
  final Duration totalDuration;
  final bool fromCache;

  const SmartSearchResult({
    required this.originalQuery,
    required this.normalizedQuery,
    required this.searchVariants,
    required this.aggregatedResult,
    required this.rankedItems,
    required this.totalDuration,
    required this.fromCache,
  });

  factory SmartSearchResult.empty(String query) {
    return SmartSearchResult(
      originalQuery: query,
      normalizedQuery: query,
      searchVariants: [],
      aggregatedResult: AggregatedSearchResult(
        query: query,
        providerResults: [],
        totalDuration: Duration.zero,
      ),
      rankedItems: [],
      totalDuration: Duration.zero,
      fromCache: false,
    );
  }

  bool get isEmpty => rankedItems.isEmpty;
  int get totalCount => rankedItems.length;
}

/// Search suggestion type
enum SuggestionType {
  history, // From search history
  cached, // From memory cache
  live, // From live provider search
}

/// Search suggestion item
class SearchSuggestion {
  final String text;
  final SuggestionType type;
  final int? searchCount;
  final MediaItem? mediaItem;

  const SearchSuggestion({
    required this.text,
    required this.type,
    this.searchCount,
    this.mediaItem,
  });
}

/// Helper class for sorting
class _ScoredItem {
  final MediaItem item;
  final double score;

  _ScoredItem(this.item, this.score);
}
