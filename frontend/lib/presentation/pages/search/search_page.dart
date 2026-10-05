import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter/foundation.dart';
import 'package:get_it/get_it.dart';
import 'package:go_router/go_router.dart';

import '../../../core/utils/logger.dart';
import '../../../data/providers/provider_registry.dart';
import '../../../data/services/history_service.dart';
import '../../../data/services/settings_service.dart';
import '../../../data/services/search/search_envelope.dart';
import '../../../data/services/smart_search/smart_search_service.dart';
import '../../../data/services/provider_catalog_service.dart';
import '../../../domain/entities/entities.dart';
import '../../widgets/media_card.dart';
import '../../widgets/custom_titlebar.dart';
import '../../widgets/tv/focusable_card.dart';
import 'package:cached_network_image/cached_network_image.dart';
import '../../widgets/common/skeleton.dart';
import '../../widgets/common/app_error_widget.dart';

/// Search page with autocomplete suggestions
class SearchPage extends StatefulWidget {
  final String? initialQuery;

  const SearchPage({super.key, this.initialQuery});

  @override
  State<SearchPage> createState() => _SearchPageState();
}

class _SearchPageState extends State<SearchPage> {
  static const String _tag = 'SearchPage';

  final _registry = GetIt.instance<ProviderRegistry>();
  final _smartSearchService = GetIt.instance<SmartSearchService>();
  final _historyService = GetIt.instance<HistoryService>();
  final _settings = GetIt.instance<SettingsService>();
  final _searchController = TextEditingController();
  final _focusNode = FocusNode();

  List<MediaItem> _results = [];
  List<SearchSuggestion> _suggestions = [];
  SmartSearchResult? _searchResult;
  bool _isLoading = false;
  String? _error;
  bool _hasSearched = false;
  bool _showSuggestions = false;
  Timer? _debounceTimer;

  /// Source filter. Null means "all sources" — the clustered view the server
  /// produced. Selecting a source narrows to the raw entries it contributed.
  String? _selectedSourceId;

  // Recent searches from smart search
  List<String> _recentSearches = [];

  // Spell correction suggestion
  String? _suggestedQuery;

  UISettings get _ui => _settings.uiSettings;

  @override
  void initState() {
    super.initState();
    _loadRecentSearches();
    _focusNode.addListener(_onFocusChanged);
    try {
      GetIt.instance<ProviderCatalogService>().addListener(_onCatalogChanged);
    } catch (_) {}

    if (widget.initialQuery != null) {
      _searchController.text = widget.initialQuery!;
      _performSearch();
    }
  }

  @override
  void dispose() {
    _debounceTimer?.cancel();
    _searchSubscription?.cancel();
    _searchController.dispose();
    _focusNode.removeListener(_onFocusChanged);
    _focusNode.dispose();
    try {
      GetIt.instance<ProviderCatalogService>().removeListener(
        _onCatalogChanged,
      );
    } catch (_) {}
    super.dispose();
  }

  void _onCatalogChanged() {
    if (mounted) {
      setState(() {});
      if (_searchController.text.isNotEmpty && _hasSearched) {
        _performSearch();
      }
    }
  }

  void _onFocusChanged() {
    if (_focusNode.hasFocus && _searchController.text.isEmpty) {
      setState(() => _showSuggestions = true);
    }
  }

  Future<void> _loadRecentSearches() async {
    // Get recent searches from SmartSearchService (from database)
    final recentQueries = await _smartSearchService.getRecentSearches(limit: 5);
    if (!mounted) return;

    // Fallback to history titles if no search history yet
    if (recentQueries.isEmpty) {
      final history = _historyService.history.take(10).toList();
      setState(() {
        _recentSearches = history.map((h) => h.title).toSet().take(5).toList();
      });
    } else {
      setState(() {
        _recentSearches = recentQueries;
      });
    }
  }

  void _onSearchChanged(String query) {
    _debounceTimer?.cancel();

    if (query.isEmpty) {
      setState(() {
        _suggestions = [];
      });
      return;
    }

    // Debounce for 300ms before fetching suggestions
    _debounceTimer = Timer(const Duration(milliseconds: 300), () {
      _fetchSuggestions(query);
    });
  }

  Future<void> _fetchSuggestions(String query) async {
    if (query.length < 2) return;

    // Loading state for suggestions is no longer used explicitly in UI

    try {
      // Use SmartSearchService for smart suggestions with fuzzy matching
      final suggestions = await _smartSearchService.getSuggestions(
        query,
        limit: 10,
      );

      if (mounted && _searchController.text == query) {
        setState(() {
          _suggestions = suggestions;
          _showSuggestions = true;
        });
      }
    } catch (e) {
      // Suggestions are best-effort; the search itself does not depend on them.
      Logger.w('Failed to fetch suggestions for "$query": $e', tag: _tag);
    }
  }

  StreamSubscription<SmartSearchResult>? _searchSubscription;

  Future<void> _performSearch() async {
    final query = _searchController.text.trim();
    if (query.isEmpty) return;

    // Cancel previous search
    await _searchSubscription?.cancel();
    _searchSubscription = null;
    if (!mounted) return;

    setState(() {
      _isLoading = true;
      _error = null;
      _hasSearched = true;
      _showSuggestions = false;
      _suggestedQuery = null;
      // Don't clear results immediately if we want to show loading indicator over old results?
      // Or clear them? Standard is clear or show skeleton.
      _results = [];
      _selectedSourceId = null;
      _searchResult = null;
    });

    // Add to recent searches (will be handled by SmartSearchService)
    if (!_recentSearches.contains(query)) {
      _recentSearches.insert(0, query);
      if (_recentSearches.length > 5) {
        _recentSearches = _recentSearches.take(5).toList();
      }
    }

    try {
      // One request. Ranking, deduplication and relevance cutoff are all
      // server-side concerns now.
      final stream = _smartSearchService.search(query);

      _searchSubscription = stream.listen(
        (result) async {
          if (!mounted) return;

          // A backend failure must not masquerade as "nothing found".
          if (result.hasError) {
            setState(() {
              _error = result.error;
              _isLoading = false;
              _searchResult = result;
              _results = const [];
            });
            return;
          }

          final items = _itemsForCurrentFilter(result);

          // Only offer a spelling correction when the backend genuinely
          // completed and found nothing — not while sources are still failing.
          String? suggestion;
          if (items.isEmpty) {
            suggestion = await _smartSearchService.suggestCorrection(query);
          }
          if (!mounted) return;

          setState(() {
            _searchResult = result;
            _results = items;
            _isLoading = false;
            _suggestedQuery = suggestion;
          });

          _loadRecentSearches();
        },
        onError: (e) {
          if (mounted) {
            setState(() {
              _error = e.toString();
              _isLoading = false;
            });
          }
        },
        onDone: () {
          if (mounted) {
            setState(() => _isLoading = false);
          }
        },
      );
    } catch (e) {
      if (mounted) {
        setState(() {
          _error = e.toString();
          _isLoading = false;
        });
      }
    }
  }

  void _selectSuggestion(MediaItem item) {
    setState(() => _showSuggestions = false);
    context.push('/details/${item.providerId}/${Uri.encodeComponent(item.id)}');
  }

  void _useRecentSearch(String query) {
    _searchController.text = query;
    _performSearch();
  }

  @override
  Widget build(BuildContext context) {
    final isDesktop =
        !kIsWeb &&
        (defaultTargetPlatform == TargetPlatform.windows ||
            defaultTargetPlatform == TargetPlatform.linux ||
            defaultTargetPlatform == TargetPlatform.macOS);

    return Scaffold(
      body: Column(
        children: [
          if (isDesktop) const CustomTitleBar(),
          _buildSearchBar(),
          if (_hasSearched && !_showSuggestions) _buildProviderFilter(),
          if (_showSuggestions) _buildSuggestions(),
          Expanded(child: _buildResults()),
        ],
      ),
    );
  }

  /// Build provider filter chips
  Widget _buildProviderFilter() {
    final providers = _registry.enabled;
    if (providers.length <= 1) return const SizedBox.shrink();

    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 8),
      color: Theme.of(context).colorScheme.surface,
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          SingleChildScrollView(
            scrollDirection: Axis.horizontal,
            child: Row(
              children: [
                // "All sources" chip
                _buildFilterChip(
                  label: 'Усі джерела',
                  isSelected: _selectedSourceId == null,
                  count: _searchResult?.totalCount,
                  onSelected: () => _onProviderFilterChanged(null),
                ),

                // One chip per source that actually contributed results.
                //
                // Sources that returned nothing, failed, or timed out are not
                // listed as selectable filters — a chip reading "0" next to an
                // orange warning icon is noise. Failures are surfaced once in
                // the banner below instead.
                ..._visibleSources.map((source) {
                  return Padding(
                    padding: const EdgeInsets.only(right: 8),
                    child: _buildFilterChip(
                      label: source.label,
                      isSelected: _selectedSourceId == source.key,
                      count: source.count,
                      onSelected: () => _onProviderFilterChanged(source.key),
                    ),
                  );
                }),
              ],
            ),
          ),

          // Partial-failure banner: the search worked, but not everywhere.
          if (_hasSourceFailures)
            Padding(
              padding: const EdgeInsets.only(top: 8),
              child: _buildFailureBanner(),
            ),

          // Search stats
          if (_searchResult != null && !_isLoading)
            Padding(
              padding: const EdgeInsets.only(top: 8),
              child: Text(
                _buildSearchStats(),
                style: TextStyle(
                  color: Theme.of(context).textTheme.bodySmall?.color,
                  fontSize: 12,
                ),
              ),
            ),
        ],
      ),
    );
  }

  /// Sources worth offering as a filter: anything with at least one result.
  List<_SourceChip> get _visibleSources {
    return [
      for (final s
          in _searchResult?.envelope.allSources ?? const <SearchSourceStatus>[])
        if (s.hasResults)
          _SourceChip(
            key: s.key,
            label: _displayNameFor(s.key),
            count: s.count,
          ),
    ]..sort((a, b) => b.count.compareTo(a.count));
  }

  /// Sources that were asked and did not deliver.
  List<SearchSourceStatus> get _failedSources =>
      _searchResult?.envelope.failedSources ?? const <SearchSourceStatus>[];

  bool get _hasSourceFailures => _failedSources.isNotEmpty;

  /// Prefer the registered provider's display name; fall back to the raw key
  /// so an unregistered source is still identifiable to the user.
  String _displayNameFor(String sourceKey) {
    for (final p in _registry.enabled) {
      if (p.id == sourceKey) return p.name;
    }
    return sourceKey;
  }

  Widget _buildFailureBanner() {
    final failed = _failedSources;
    final names = failed.map((s) => _displayNameFor(s.key)).join(', ');

    return Container(
      width: double.infinity,
      padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 8),
      decoration: BoxDecoration(
        color: Colors.orange.withValues(alpha: 0.12),
        borderRadius: BorderRadius.circular(8),
        border: Border.all(color: Colors.orange.withValues(alpha: 0.4)),
      ),
      child: Row(
        children: [
          const Icon(
            Icons.warning_amber_rounded,
            size: 18,
            color: Colors.orange,
          ),
          const SizedBox(width: 8),
          Expanded(
            child: Text(
              'Недоступні джерела: $names',
              style: TextStyle(fontSize: 12, color: Colors.orange.shade800),
            ),
          ),
          TextButton(onPressed: _performSearch, child: const Text('Повторити')),
        ],
      ),
    );
  }

  Widget _buildFilterChip({
    required String label,
    required bool isSelected,
    int? count,
    required VoidCallback onSelected,
  }) {
    return FilterChip(
      label: Row(
        mainAxisSize: MainAxisSize.min,
        children: [
          Text(label),
          if (count != null) ...[
            const SizedBox(width: 4),
            Container(
              padding: const EdgeInsets.symmetric(horizontal: 6, vertical: 2),
              decoration: BoxDecoration(
                color: isSelected
                    ? Colors.white.withValues(alpha: 0.2)
                    : Theme.of(
                        context,
                      ).textTheme.bodySmall?.color?.withValues(alpha: 0.2),
                borderRadius: BorderRadius.circular(10),
              ),
              child: Text('$count', style: const TextStyle(fontSize: 11)),
            ),
          ],
        ],
      ),
      selected: isSelected,
      onSelected: (_) => onSelected(),
      selectedColor: Theme.of(
        context,
      ).colorScheme.primary.withValues(alpha: 0.3),
      checkmarkColor: Theme.of(context).colorScheme.primary,
    );
  }

  void _onProviderFilterChanged(String? sourceId) {
    setState(() {
      _selectedSourceId = sourceId;
      final result = _searchResult;
      _results = result == null ? const [] : _itemsForCurrentFilter(result);
    });
  }

  /// Project the server's clustered items through the current source filter.
  ///
  /// With no filter selected we show the clustered list as-is. With a source
  /// selected we keep only the clusters that source contributed to.
  ///
  /// Note this filters clusters, it does not re-expand them into per-source
  /// raw entries: `SearchItemSource` carries only identity
  /// (`source_key`/`item_id`/`url`), not a full title. Expanding would mean a
  /// second round trip per source, which is exactly the fan-out we removed.
  List<MediaItem> _itemsForCurrentFilter(SmartSearchResult result) {
    final source = _selectedSourceId;
    if (source == null) return result.rankedItems;

    return [
      for (final scored in result.scoredItems)
        if (scored.sources.any((s) => s.sourceKey == source)) scored.item,
    ];
  }

  String _buildSearchStats() {
    if (_searchResult == null) return '';

    final sr = _searchResult!;
    final envelope = sr.envelope;
    final duration = sr.totalDuration.inMilliseconds;

    // Count sources that actually answered, not the number registered.
    final answered = envelope.answeredCount;
    final asked = envelope.askedCount;
    final clusterText = sr.totalCount == 1
        ? '1 результат'
        : '${sr.totalCount} результатів';
    final buffer = StringBuffer(clusterText);

    // Only show the source fraction when the backend actually reported
    // sources. A legacy or partial response that says nothing about sources
    // must not render as "0 of 0", which reads as a total outage.
    if (asked > 0) {
      buffer.write(' з $answered/$asked джерел');
    }

    if (envelope.tookMs > 0) {
      buffer.write(' за $duration мс (сервер: ${envelope.tookMs} мс)');
    } else {
      buffer.write(' за $duration мс');
    }

    // Make heavy filtering visible: without this, a search that matched 400
    // candidates but kept 6 looks identical to a source with 6 titles total.
    if (envelope.filteredOut > 0) {
      buffer.write(' · відсічено ${envelope.filteredOut} нерелевантних');
    }

    return buffer.toString();
  }

  Widget _buildSearchBar() {
    return Container(
      padding: const EdgeInsets.all(16),
      color: Theme.of(context).colorScheme.surface,
      child: Row(
        children: [
          IconButton(
            icon: const Icon(Icons.arrow_back),
            onPressed: () {
              if (context.canPop()) {
                context.pop();
              } else {
                context.go('/');
              }
            },
          ),
          const SizedBox(width: 8),
          Expanded(
            child: TextField(
              controller: _searchController,
              focusNode: _focusNode,
              autofocus: true,
              textInputAction: TextInputAction.search,
              decoration: InputDecoration(
                hintText: 'Пошук фільмів, серіалів...',
                prefixIcon: const Icon(Icons.search),
                suffixIcon: Row(
                  mainAxisSize: MainAxisSize.min,
                  children: [
                    if (_searchController.text.isNotEmpty)
                      IconButton(
                        icon: const Icon(Icons.clear),
                        onPressed: () {
                          _searchController.clear();
                          setState(() {
                            _results = [];
                            _suggestions = [];
                            _hasSearched = false;
                            _showSuggestions = true;
                          });
                        },
                      ),
                  ],
                ),
                border: OutlineInputBorder(
                  borderRadius: BorderRadius.circular(12),
                  borderSide: BorderSide.none,
                ),
                filled: true,
                fillColor: Theme.of(context).cardColor,
              ),
              onSubmitted: (_) => _performSearch(),
              onChanged: _onSearchChanged,
              onTap: () => setState(() => _showSuggestions = true),
            ),
          ),
          const SizedBox(width: 8),
          ElevatedButton(
            onPressed: _performSearch,
            child: const Text('Знайти'),
          ),
        ],
      ),
    );
  }

  Widget _buildSuggestions() {
    return Container(
      color: Theme.of(context).colorScheme.surface,
      constraints: const BoxConstraints(maxHeight: 300),
      child: ListView(
        shrinkWrap: true,
        children: [
          // Recent searches
          if (_searchController.text.isEmpty && _recentSearches.isNotEmpty) ...[
            Padding(
              padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 8),
              child: Text(
                'Нещодавні пошуки',
                style: TextStyle(
                  color: Theme.of(context).textTheme.bodySmall?.color,
                  fontSize: 12,
                  fontWeight: FontWeight.bold,
                ),
              ),
            ),
            ..._recentSearches.map(
              (query) => Padding(
                padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 2),
                child: FocusableCard(
                  onTap: () => _useRecentSearch(query),
                  borderRadius: 8,
                  child: ListTile(
                    leading: Icon(
                      Icons.history,
                      color: Theme.of(context).textTheme.bodySmall?.color,
                    ),
                    title: Text(query),
                    dense: true,
                  ),
                ),
              ),
            ),
          ],

          // Search suggestions
          if (_suggestions.isNotEmpty) ...[
            Padding(
              padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 8),
              child: Text(
                'Пропозиції',
                style: TextStyle(
                  color: Theme.of(context).textTheme.bodySmall?.color,
                  fontSize: 12,
                  fontWeight: FontWeight.bold,
                ),
              ),
            ),
            ..._suggestions.map((suggestion) {
              // Get icon based on suggestion type
              final icon = switch (suggestion.type) {
                SuggestionType.history => Icons.history,
                SuggestionType.cached => Icons.cached,
                SuggestionType.live => Icons.search,
              };

              final mediaItem = suggestion.mediaItem;

              if (mediaItem != null) {
                // Show media item with poster
                return Padding(
                  padding: const EdgeInsets.symmetric(
                    horizontal: 8,
                    vertical: 2,
                  ),
                  child: FocusableCard(
                    onTap: () => _selectSuggestion(mediaItem),
                    borderRadius: 8,
                    child: ListTile(
                      leading: mediaItem.posterUrl != null
                          ? ClipRRect(
                              borderRadius: BorderRadius.circular(4),
                              child: CachedNetworkImage(
                                imageUrl: mediaItem.posterUrl!,
                                width: 40,
                                height: 56,
                                fit: BoxFit.cover,
                                memCacheHeight: 200,
                                placeholder: (context, url) => const Skeleton(
                                  width: 40,
                                  height: 56,
                                  borderRadius: 0,
                                ),
                                errorWidget: (context, url, error) =>
                                    const Icon(Icons.movie, size: 40),
                              ),
                            )
                          : const Icon(Icons.movie, size: 40),
                      title: Text(mediaItem.title),
                      subtitle: Text(
                        '${mediaItem.type.displayName}${mediaItem.year != null ? ' • ${mediaItem.year}' : ''}',
                        style: TextStyle(
                          color: Theme.of(context).textTheme.bodySmall?.color,
                          fontSize: 12,
                        ),
                      ),
                      dense: true,
                    ),
                  ),
                );
              } else {
                // Show text-only suggestion
                return Padding(
                  padding: const EdgeInsets.symmetric(
                    horizontal: 8,
                    vertical: 2,
                  ),
                  child: FocusableCard(
                    onTap: () => _useRecentSearch(suggestion.text),
                    borderRadius: 8,
                    child: ListTile(
                      leading: Icon(icon),
                      title: Text(suggestion.text),
                      trailing:
                          suggestion.searchCount != null &&
                              suggestion.searchCount! > 1
                          ? Text(
                              '${suggestion.searchCount}x',
                              style: TextStyle(
                                color: Theme.of(
                                  context,
                                ).textTheme.bodySmall?.color,
                                fontSize: 12,
                              ),
                            )
                          : null,
                      dense: true,
                    ),
                  ),
                );
              }
            }),
          ],
        ],
      ),
    );
  }

  Widget _buildResults() {
    if (_isLoading && _results.isEmpty) {
      return _buildSkeletonResults();
    }

    if (_error != null) {
      return AppErrorWidget.loading(message: _error, onRetry: _performSearch);
    }

    if (!_hasSearched) {
      return Center(
        child: Column(
          mainAxisAlignment: MainAxisAlignment.center,
          children: [
            Icon(
              Icons.search,
              size: 80,
              color: Theme.of(context).textTheme.bodySmall?.color,
            ),
            const SizedBox(height: 16),
            Text(
              'Введіть запит для пошуку',
              style: TextStyle(
                color: Theme.of(context).textTheme.bodySmall?.color,
                fontSize: 16,
              ),
            ),
          ],
        ),
      );
    }

    if (_results.isEmpty) {
      return Center(
        child: Column(
          mainAxisAlignment: MainAxisAlignment.center,
          children: [
            Icon(
              Icons.movie_filter,
              size: 80,
              color: Theme.of(context).textTheme.bodySmall?.color,
            ),
            const SizedBox(height: 16),
            Text(
              'Нічого не знайдено',
              style: TextStyle(
                color: Theme.of(context).textTheme.bodySmall?.color,
                fontSize: 16,
              ),
            ),
            const SizedBox(height: 8),
            Text(
              'Спробуйте інший запит',
              style: TextStyle(
                color: Theme.of(context).textTheme.bodySmall?.color,
              ),
            ),
            // Show spell correction suggestion
            if (_suggestedQuery != null) ...[
              const SizedBox(height: 16),
              InkWell(
                onTap: () {
                  _searchController.text = _suggestedQuery!;
                  _performSearch();
                },
                child: Container(
                  padding: const EdgeInsets.symmetric(
                    horizontal: 16,
                    vertical: 10,
                  ),
                  decoration: BoxDecoration(
                    color: Theme.of(
                      context,
                    ).colorScheme.primary.withValues(alpha: 0.15),
                    borderRadius: BorderRadius.circular(8),
                    border: Border.all(
                      color: Theme.of(
                        context,
                      ).colorScheme.primary.withValues(alpha: 0.3),
                    ),
                  ),
                  child: Row(
                    mainAxisSize: MainAxisSize.min,
                    children: [
                      Icon(
                        Icons.lightbulb_outline,
                        color: Theme.of(context).colorScheme.primary,
                        size: 20,
                      ),
                      const SizedBox(width: 8),
                      Text(
                        'Можливо, ви мали на увазі: ',
                        style: TextStyle(
                          color: Theme.of(context).textTheme.bodySmall?.color,
                        ),
                      ),
                      Text(
                        _suggestedQuery!,
                        style: TextStyle(
                          color: Theme.of(context).colorScheme.primary,
                          fontWeight: FontWeight.bold,
                        ),
                      ),
                    ],
                  ),
                ),
              ),
            ],
          ],
        ),
      );
    }

    return RefreshIndicator(
      onRefresh: () async {
        if (_searchController.text.isNotEmpty) {
          await _performSearch();
        }
      },
      child: GridView.builder(
        cacheExtent: 1000.0,
        padding: EdgeInsets.all(_ui.gridSpacing.padding),
        gridDelegate: _ui.gridColumns > 0
            ? SliverGridDelegateWithFixedCrossAxisCount(
                crossAxisCount: _ui.gridColumns,
                childAspectRatio: _ui.posterSize.aspectRatio,
                crossAxisSpacing: _ui.gridSpacing.crossAxisSpacing,
                mainAxisSpacing: _ui.gridSpacing.mainAxisSpacing,
              )
            : SliverGridDelegateWithMaxCrossAxisExtent(
                maxCrossAxisExtent: _getMaxCrossAxisExtent(),
                childAspectRatio: _ui.posterSize.aspectRatio,
                crossAxisSpacing: _ui.gridSpacing.crossAxisSpacing,
                mainAxisSpacing: _ui.gridSpacing.mainAxisSpacing,
              ),
        itemCount: _results.length,
        itemBuilder: (context, index) {
          final item = _results[index];
          return MediaCard(
            item: item,
            onTap: () => context.push(
              '/details/${item.providerId}/${Uri.encodeComponent(item.id)}',
            ),
          );
        },
      ),
    );
  }

  double _getMaxCrossAxisExtent() {
    switch (_ui.posterSize) {
      case PosterSize.small:
        return 130;
      case PosterSize.medium:
        return 180;
      case PosterSize.large:
        return 250;
    }
  }

  /// Placeholder grid shown while the first search is in flight.
  ///
  /// It uses the SAME grid delegate as the real results, because a skeleton that
  /// does not match the layout it stands in for makes the page jump when results
  /// arrive. The previous version delegated to `SkeletonWrappers.grid`, which
  /// hardcoded `Colors.white` cards, ignored the user's poster size and spacing
  /// settings, and used a fixed count of 6 with `NeverScrollableScrollPhysics`,
  /// which rendered a single short row floating at the top of an otherwise
  /// empty page.
  Widget _buildSkeletonResults() {
    return LayoutBuilder(
      builder: (context, constraints) {
        // Enough rows to fill the viewport, so the page does not visibly
        // "grow" into itself when the real results replace the placeholders.
        final columns = _columnsFor(constraints.biggest.width);
        final rowHeight =
            constraints.maxWidth / columns / _ui.posterSize.aspectRatio +
            _ui.gridSpacing.mainAxisSpacing;
        final rows = (constraints.maxHeight / rowHeight).ceil().clamp(2, 6);

        return GridView.builder(
          padding: EdgeInsets.all(_ui.gridSpacing.padding),
          physics: const NeverScrollableScrollPhysics(),
          gridDelegate: _ui.gridColumns > 0
              ? SliverGridDelegateWithFixedCrossAxisCount(
                  crossAxisCount: _ui.gridColumns,
                  childAspectRatio: _ui.posterSize.aspectRatio,
                  crossAxisSpacing: _ui.gridSpacing.crossAxisSpacing,
                  mainAxisSpacing: _ui.gridSpacing.mainAxisSpacing,
                )
              : SliverGridDelegateWithMaxCrossAxisExtent(
                  maxCrossAxisExtent: _getMaxCrossAxisExtent(),
                  childAspectRatio: _ui.posterSize.aspectRatio,
                  crossAxisSpacing: _ui.gridSpacing.crossAxisSpacing,
                  mainAxisSpacing: _ui.gridSpacing.mainAxisSpacing,
                ),
          itemCount: columns * rows,
          itemBuilder: (context, index) =>
              Skeleton(borderRadius: _ui.posterSize.borderRadius),
        );
      },
    );
  }

  /// Column count for [width], mirroring what the results grid will lay out.
  int _columnsFor(double width) {
    if (_ui.gridColumns > 0) return _ui.gridColumns;
    final maxExtent = _getMaxCrossAxisExtent();
    if (width <= 0) return 1;
    return (width / (maxExtent + _ui.gridSpacing.crossAxisSpacing))
        .floor()
        .clamp(1, 20);
  }
}

/// A selectable source filter chip.
///
/// Only sources that returned at least one result get one of these. Sources
/// that failed or returned nothing are represented by the failure banner
/// instead — a chip labelled "0" invites the user to tap into an empty list.
class _SourceChip {
  const _SourceChip({
    required this.key,
    required this.label,
    required this.count,
  });

  /// The backend's source key, e.g. `uakino`.
  final String key;

  /// Human-readable name resolved from the provider registry.
  final String label;

  /// Results this source contributed.
  final int count;
}
