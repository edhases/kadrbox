import 'dart:async';

import 'package:fuzzywuzzy/fuzzywuzzy.dart';

import '../../../core/utils/logger.dart';
import '../../../domain/entities/entities.dart';
import '../../database/dao/search_history_dao.dart';
import '../../providers/provider_registry.dart';
import '../../providers/server_backed_provider.dart';
import '../search/search_envelope.dart';
import 'transliteration_service.dart';

/// Smart search service with fuzzy matching, transliteration, and history
///
/// Features:
/// - Automatic Latin ↔ Cyrillic transliteration
/// - Fuzzy matching for typo tolerance
/// - Search history for autocomplete
/// - Memory cache for fast suggestions
/// - Result ranking by relevance
/// Smart search service.
///
/// Search itself is a thin client: one request to the server-side search
/// pipeline, which normalises the query, scores relevance with a hard cutoff
/// and clusters cross-provider duplicates. This service keeps the local
/// concerns — history, autocomplete, and spelling suggestions.
class SmartSearchService {
  static const String _tag = 'SmartSearch';

  /// Fuzzy matching threshold (0-100) used only for local spelling
  /// suggestions. Search relevance is computed server-side.
  static const int _fuzzyThreshold = 75;

  final ProviderRegistry _registry;
  final TransliterationService _transliteration;
  final SearchHistoryDao _historyDao;

  /// Memory cache for search results (cleared on app restart)
  /// In-memory cache of server envelopes, used to power autocomplete.
  ///
  /// Search itself is not served from here: the backend already caches by
  /// query-plan hash, so a second client-side cache would only add staleness.
  final Map<String, SearchEnvelope> _memoryCache = {};

  /// Cache TTL (10 minutes)
  static const Duration _cacheTtl = Duration(minutes: 10);
  final Map<String, DateTime> _cacheTimestamps = {};

  SmartSearchService(this._registry, this._historyDao)
    : _transliteration = TransliterationService();

  /// Perform a search.
  ///
  /// The whole pipeline now lives on the server: one request per segment,
  /// query normalisation, relevance scoring with a hard cutoff, and
  /// cross-provider clustering. This method issues a SINGLE request and
  /// renders what comes back — it no longer generates transliteration
  /// variants, no longer fans out per provider, no longer dedupes and no
  /// longer runs an isolate for Levenshtein scoring.
  ///
  /// That collapse is deliberate. The old client-side pipeline cost up to
  /// 24 HTTP requests per keystroke-submit, had no relevance floor at all
  /// (`fuzzyThreshold` was passed into the isolate and never read), and
  /// deduplicated with a title normaliser whose `[^\w\s]` reduced every
  /// Cyrillic title to an empty string — because Dart's `\w` is ASCII-only.
  /// So duplicate merging silently never ran for Ukrainian content.
  Stream<SmartSearchResult> search(
    String query, {
    ContentType? type,
    int page = 1,
  }) {
    final trimmed = query.trim();
    if (trimmed.isEmpty) {
      return Stream.value(SmartSearchResult.empty(query));
    }

    return Stream.fromFuture(_runSearch(query, type: type, page: page));
  }

  Future<SmartSearchResult> _runSearch(
    String query, {
    ContentType? type,
    int page = 1,
  }) async {
    final stopwatch = Stopwatch()..start();
    final provider = _resolveProvider();
    if (provider == null) {
      Logger.w('No server-backed provider available; cannot search', tag: _tag);
      return SmartSearchResult.empty(query);
    }

    try {
      final envelope = await provider.searchEnvelope(
        query,
        type: type,
        page: page,
      );
      stopwatch.stop();

      Logger.i(
        'Search "${envelope.canonical}": ${envelope.items.length} items, '
        '${envelope.filteredOut} filtered, ${envelope.tookMs}ms server-side',
        tag: _tag,
      );

      final key = _cacheKeyFor(query);
      _putCache(key, envelope);
      await _saveToHistory(
        query,
        envelope.canonical.isEmpty ? query : envelope.canonical,
        envelope.items.length,
      );

      return SmartSearchResult.fromEnvelope(
        originalQuery: query,
        envelope: envelope,
        totalDuration: stopwatch.elapsed,
      );
    } catch (e, st) {
      stopwatch.stop();
      Logger.e(
        'Search failed for "$query"',
        tag: _tag,
        error: e,
        stackTrace: st,
      );
      // Surfaced as an error rather than an empty result so the UI can tell a
      // dead backend from a genuinely empty catalogue.
      return SmartSearchResult.error(
        query,
        totalDuration: stopwatch.elapsed,
        error: e.toString(),
      );
    }
  }

  /// The provider that can serve the server-side envelope.
  ///
  /// Bandera is the one provider that fronts an aggregator, so it is the
  /// only sensible target. Falling back to "any server-backed provider" would
  /// reintroduce per-provider fan-out against scrapers, which is exactly the
  /// ban surface we closed.
  ServerBackedProvider? _resolveProvider() {
    final bandera = _registry.getById('bandera');
    if (bandera is ServerBackedProvider) return bandera;
    return null;
  }

  String _cacheKeyFor(String query) => query.toLowerCase().trim();

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

    // Live provider suggestions are intentionally gone. They fanned out to the
    // DLE scrapers on every keystroke, which is the exact request pattern that
    // gets a client IP blocked. Autocomplete is served from local history and
    // the envelopes this device has already fetched; the backend, not the
    // phone, is the right place to add remote type-ahead if it is ever needed.

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

  /// Store an entry, enforcing both the TTL and a hard cardinality bound.
  ///
  /// Without this the cache only ever grew for the life of the process: every
  /// distinct query added an entry that nothing removed, and each entry holds a
  /// whole [SearchEnvelope]. [_cacheTtl] was declared to expire entries but was
  /// never consulted, so the intent was documented without being implemented.
  void _putCache(String key, SearchEnvelope envelope) {
    final now = DateTime.now();

    _cacheTimestamps.removeWhere(
      (_, storedAt) => now.difference(storedAt) > _cacheTtl,
    );
    _memoryCache.removeWhere((key, _) => !_cacheTimestamps.containsKey(key));

    // Evict oldest-first once the cache exceeds the bound.
    while (_memoryCache.length >= maxCacheEntries) {
      final oldest = _cacheTimestamps.keys.first;
      _memoryCache.remove(oldest);
      _cacheTimestamps.remove(oldest);
    }

    _memoryCache[key] = envelope;
    _cacheTimestamps[key] = now;
  }

  /// Hard cap on distinct cached queries. A 500-item envelope is a few hundred
  /// KB, so an unbounded map is tens of MB of unreachable data per session.
  static const int maxCacheEntries = 100;

  // =========================================================================
  // Private methods
  // =========================================================================

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
}

/// Result of smart search
class SmartSearchResult {
  final String originalQuery;
  final String normalizedQuery;
  final SearchEnvelope envelope;
  final Duration totalDuration;
  final bool fromCache;
  final String? error;

  const SmartSearchResult({
    required this.originalQuery,
    required this.normalizedQuery,
    required this.envelope,
    required this.totalDuration,
    this.fromCache = false,
    this.error,
  });

  /// Build from the server envelope. Ranking and dedup are already done
  /// server-side, so [rankedItems] is a straight projection of
  /// [envelope.items] — the client never reorders or re-merges.
  factory SmartSearchResult.fromEnvelope({
    required String originalQuery,
    required SearchEnvelope envelope,
    required Duration totalDuration,
    bool fromCache = false,
  }) {
    return SmartSearchResult(
      originalQuery: originalQuery,
      normalizedQuery: envelope.canonical.isEmpty
          ? originalQuery
          : envelope.canonical,
      envelope: envelope,
      totalDuration: totalDuration,
      fromCache: fromCache,
    );
  }

  factory SmartSearchResult.empty(String query) {
    return SmartSearchResult(
      originalQuery: query,
      normalizedQuery: query,
      envelope: SearchEnvelope.empty(query),
      totalDuration: Duration.zero,
    );
  }

  /// A failure that the UI must show as a failure, not as "no results".
  factory SmartSearchResult.error(
    String query, {
    required Duration totalDuration,
    required String error,
  }) {
    return SmartSearchResult(
      originalQuery: query,
      normalizedQuery: query,
      envelope: SearchEnvelope.empty(query),
      totalDuration: totalDuration,
      error: error,
    );
  }

  /// Scored items, already ranked and clustered by the server.
  List<ScoredMediaItem> get scoredItems => envelope.items;

  /// Plain items for the card grid.
  List<MediaItem> get rankedItems => envelope.items.map((s) => s.item).toList();

  bool get isEmpty => envelope.items.isEmpty;

  /// True when the search failed rather than finding nothing.
  bool get hasError => error != null;

  int get totalCount => envelope.items.length;

  /// Candidates the server dropped for low relevance. Shown in the UI so a
  /// heavily filtered search is visibly different from an empty catalogue.
  int get filteredOut => envelope.filteredOut;

  /// Sources that could not be reached, for the "N sources unavailable" banner.
  List<SearchSourceStatus> get failedSources => envelope.failedSources;
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
