import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter/foundation.dart';
import 'package:get_it/get_it.dart';
import 'package:go_router/go_router.dart';

import '../../../data/providers/provider_registry.dart';
import '../../../data/services/history_service.dart';
import '../../../data/services/settings_service.dart';
import '../../../data/services/search_service.dart';
import '../../../data/services/smart_search/smart_search_service.dart';
import '../../../data/services/provider_catalog_service.dart';
import '../../../domain/entities/entities.dart';
import '../../widgets/media_card.dart';
import '../../widgets/custom_titlebar.dart';
import '../../widgets/tv/focusable_card.dart';
import 'package:cached_network_image/cached_network_image.dart';
import 'package:skeletonizer/skeletonizer.dart' hide Skeleton;
import '../../widgets/common/skeleton_wrappers.dart';
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
  final _registry = GetIt.instance<ProviderRegistry>();
  final _searchService = GetIt.instance<SearchService>();
  final _smartSearchService = GetIt.instance<SmartSearchService>();
  final _historyService = GetIt.instance<HistoryService>();
  final _settings = GetIt.instance<SettingsService>();
  final _searchController = TextEditingController();
  final _focusNode = FocusNode();

  List<MediaItem> _results = [];
  List<SearchSuggestion> _suggestions = [];
  SmartSearchResult? _searchResult;
  AggregatedSearchResult? _aggregatedResult;
  bool _isLoading = false;
  String? _error;
  bool _hasSearched = false;
  bool _showSuggestions = false;
  Timer? _debounceTimer;

  // Provider filter
  String? _selectedProviderId;

  // Deduplication toggle
  bool _deduplicateResults = false;

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
      GetIt.instance<ProviderCatalogService>().removeListener(_onCatalogChanged);
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
      // Ignore errors for suggestions
    }
  }

  StreamSubscription? _searchSubscription;

  Future<void> _performSearch() async {
    final query = _searchController.text.trim();
    if (query.isEmpty) return;

    // Cancel previous search
    await _searchSubscription?.cancel();
    _searchSubscription = null;

    setState(() {
      _isLoading = true;
      _error = null;
      _hasSearched = true;
      _showSuggestions = false;
      _suggestedQuery = null;
      // Don't clear results immediately if we want to show loading indicator over old results?
      // Or clear them? Standard is clear or show skeleton.
      _results = [];
      _aggregatedResult = null;
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
      // Use SmartSearchService for intelligent multi-provider search
      final stream = _smartSearchService.search(query);

      _searchSubscription = stream.listen(
        (result) async {
          if (!mounted) return;

          // Get filtered results based on selected provider
          var items = result.rankedItems;

          // Filter by provider if selected
          if (_selectedProviderId != null) {
            items = items
                .where((i) => i.providerId == _selectedProviderId)
                .toList();
          }

          // Apply deduplication if enabled
          if (_deduplicateResults && _selectedProviderId == null) {
            // SmartSearchService now handles basic dedup, but this UI toggle forces aggressive dedup?
            // Or maybe we reused SearchService logic.
            // _searchService.deduplicateResults is still available.
            items = _searchService.deduplicateResults(items);
          }

          // Check if we should suggest a correction
          String? suggestion;
          if (items.isEmpty && result.aggregatedResult.isComplete) {
            suggestion = await _smartSearchService.suggestCorrection(query);
          }

          setState(() {
            _searchResult = result;
            _aggregatedResult = result.aggregatedResult;
            _results = items;
            // Only stop loading if complete? Or keep loading true until done?
            // "Loading" usually means "Waiting for first result" or "In progress".
            // If we have results, we can show them.
            // But if we hide loading indicator, user might think search is finished.
            // Better to keep _isLoading = true until stream is done?
            // BUT if stream is progressive, we want to show data.
            // Let's use !isComplete for loading state?
            _isLoading = !result.aggregatedResult.isComplete;
            _suggestedQuery = suggestion;
          });

          // Reload recent searches after search (once we have some results)
          if (result.fromCache || result.aggregatedResult.isComplete) {
            _loadRecentSearches();
          }
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
                // "All providers" chip
                _buildFilterChip(
                  label: 'Усі джерела',
                  isSelected: _selectedProviderId == null,
                  count: _searchResult?.totalCount,
                  onSelected: () => _onProviderFilterChanged(null),
                ),
                const SizedBox(width: 8),

                // Individual provider chips
                ...providers.map((provider) {
                  final result = _aggregatedResult?.providerResults
                      .where((r) => r.providerId == provider.id)
                      .firstOrNull;
                  final count = result?.items.length ?? 0;
                  final hasError = result?.error != null;

                  return Padding(
                    padding: const EdgeInsets.only(right: 8),
                    child: _buildFilterChip(
                      label: provider.name,
                      isSelected: _selectedProviderId == provider.id,
                      count: count,
                      hasError: hasError,
                      onSelected: () => _onProviderFilterChanged(provider.id),
                    ),
                  );
                }),

                // Deduplication toggle (only when "All" selected)
                if (_selectedProviderId == null) ...[
                  const SizedBox(width: 16),
                  FilterChip(
                    label: const Text('Без дублів'),
                    selected: _deduplicateResults,
                    onSelected: (value) {
                      setState(() {
                        _deduplicateResults = value;
                        if (_searchResult != null) {
                          var items = _searchResult!.rankedItems;
                          if (value) {
                            items = _searchService.deduplicateResults(items);
                          }
                          _results = items;
                        }
                      });
                    },
                    avatar: Icon(
                      _deduplicateResults
                          ? Icons.filter_alt
                          : Icons.filter_alt_outlined,
                      size: 18,
                    ),
                  ),
                ],
              ],
            ),
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

  Widget _buildFilterChip({
    required String label,
    required bool isSelected,
    int? count,
    bool hasError = false,
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
              child: Text(
                '$count',
                style: TextStyle(
                  fontSize: 11,
                  color: hasError ? Colors.red : null,
                ),
              ),
            ),
          ],
          if (hasError) ...[
            const SizedBox(width: 4),
            const Icon(Icons.warning_amber, size: 14, color: Colors.orange),
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

  void _onProviderFilterChanged(String? providerId) {
    setState(() {
      _selectedProviderId = providerId;
      if (_searchResult != null) {
        var items = _searchResult!.rankedItems;
        if (providerId != null) {
          items = items.where((i) => i.providerId == providerId).toList();
        }
        if (_deduplicateResults && providerId == null) {
          items = _searchService.deduplicateResults(items);
        }
        _results = items;
      }
    });
  }

  String _buildSearchStats() {
    if (_searchResult == null) return '';

    final sr = _searchResult!;
    final duration = sr.totalDuration.inMilliseconds;
    final total = sr.totalCount;
    final aggr = sr.aggregatedResult;
    final success = aggr.successCount;
    final providers = _registry.enabled.length;

    // Show cache indicator
    final cacheText = sr.fromCache ? ' (кеш)' : '';

    if (aggr.failureCount > 0) {
      return '$total результатів з $success/$providers джерел за $durationмс$cacheText';
    }
    return '$total результатів з $providers джерел за $durationмс$cacheText';
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
      return Skeletonizer(enabled: true, child: _buildSkeletonResults());
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

  Widget _buildSkeletonResults() {
    return SkeletonWrappers.grid(
      maxExtent: _getMaxCrossAxisExtent(),
      spacing: _ui.gridSpacing.padding,
      crossAxisCount: _ui.gridColumns,
    );
  }
}
