import 'package:flutter/foundation.dart';
import 'package:flutter/material.dart';
import 'package:get_it/get_it.dart';
import 'package:go_router/go_router.dart';

import '../../../core/utils/logger.dart';
import '../../../data/providers/provider_registry.dart';
import '../../../data/services/settings_service.dart';
import '../../../data/services/provider_catalog_service.dart';
import '../../../domain/entities/entities.dart';
import '../../../domain/repositories/content_provider.dart';
import '../../widgets/media_card.dart';
import '../../widgets/custom_titlebar.dart';
import '../../widgets/filter_sheet.dart';
import '../../widgets/common/skeleton.dart';

const _tag = 'CategoryPage';

/// Category/Browse page for viewing content by type
class CategoryPage extends StatefulWidget {
  final ContentType? initialType;

  const CategoryPage({super.key, this.initialType});

  @override
  State<CategoryPage> createState() => _CategoryPageState();
}

class _CategoryPageState extends State<CategoryPage>
    with SingleTickerProviderStateMixin {
  final _registry = GetIt.instance<ProviderRegistry>();
  final _settings = GetIt.instance<SettingsService>();
  final _scrollController = ScrollController();
  late TabController _tabController;

  final _tabs = [
    ContentType.movie,
    ContentType.series,
    ContentType.cartoon,
    ContentType.anime,
    ContentType.dorama,
  ];

  final Map<ContentType, List<MediaItem>> _contentByType = {};
  final Map<ContentType, bool> _loadingByType = {};
  final Map<ContentType, String?> _errorByType = {};
  final Map<ContentType, ContentFilter> _filterByType = {};
  final Map<ContentType, int> _pageByType = {};

  UISettings get _ui => _settings.uiSettings;

  @override
  void initState() {
    super.initState();

    // Initialize state for each type
    for (final type in _tabs) {
      _contentByType[type] = [];
      _loadingByType[type] = false;
      _errorByType[type] = null;
      _filterByType[type] = ContentFilter(type: type);
      _pageByType[type] = 1;
    }

    // Find initial tab index
    final initialIndex = widget.initialType != null
        ? _tabs.indexOf(widget.initialType!)
        : 0;

    _tabController = TabController(
      length: _tabs.length,
      vsync: this,
      initialIndex: initialIndex.clamp(0, _tabs.length - 1),
    );

    _tabController.addListener(_onTabChanged);
    _scrollController.addListener(_onScroll);
    try {
      GetIt.instance<ProviderCatalogService>().addListener(_onCatalogChanged);
    } catch (_) {}

    // Load initial content
    _loadContent(_tabs[_tabController.index]);
  }

  @override
  void dispose() {
    _tabController.removeListener(_onTabChanged);
    _tabController.dispose();
    _scrollController.removeListener(_onScroll);
    _scrollController.dispose();
    try {
      GetIt.instance<ProviderCatalogService>().removeListener(
        _onCatalogChanged,
      );
    } catch (_) {}
    super.dispose();
  }

  void _onCatalogChanged() {
    if (mounted) {
      final currentType = _tabs[_tabController.index];
      _contentByType[currentType]?.clear();
      _pageByType[currentType] = 1;
      _loadContent(currentType);
    }
  }

  void _onTabChanged() {
    if (_tabController.indexIsChanging) return;
    final type = _tabs[_tabController.index];
    if (_contentByType[type]!.isEmpty && !_loadingByType[type]!) {
      _loadContent(type);
    }
  }

  void _onScroll() {
    if (_scrollController.position.pixels >=
        _scrollController.position.maxScrollExtent - 500) {
      _loadMoreContent();
    }
  }

  Future<void> _loadContent(ContentType type) async {
    if (_loadingByType[type]!) return;

    setState(() {
      _loadingByType[type] = true;
      _errorByType[type] = null;
    });

    try {
      // Ensure catalog is synced
      try {
        await GetIt.instance<ProviderCatalogService>().sync();
      } catch (_) {}

      // Get only home providers (excludes providers with dedicated pages)
      final providers = _registry.getHomeProvidersByContentType(type);
      Logger.d(
        'Loading $type content from ${providers.length} home providers: '
        '${providers.map((p) => p.name).join(", ")}',
        tag: _tag,
      );
      final allItems = <MediaItem>[];
      final filter = _filterByType[type]!;
      final selectedGenre = filter.genres.isNotEmpty
          ? filter.genres.first
          : null;

      // Parallel fetch from all providers to avoid N sequential RTTs.
      final results = await Future.wait(
        providers.map((provider) async {
          try {
            if (selectedGenre != null) {
              return await provider.getByCategory(
                selectedGenre,
                type: type,
                page: 1,
              );
            }
            return await provider.getPopular(type: type, page: 1);
          } catch (e, stack) {
            Logger.e(
              'Failed to load $type from ${provider.name}',
              tag: _tag,
              error: e,
              stackTrace: stack,
            );
            return <MediaItem>[];
          }
        }),
      );
      for (final items in results) {
        allItems.addAll(items);
      }

      if (mounted) {
        final filtered = filter.apply(allItems);
        Logger.d(
          'Total: ${allItems.length} items, after filter: ${filtered.length}',
          tag: _tag,
        );
        setState(() {
          _contentByType[type] = filtered;
          _loadingByType[type] = false;
          _pageByType[type] = 1;
        });
      }
    } catch (e, stack) {
      Logger.e('Load content failed', tag: _tag, error: e, stackTrace: stack);
      if (mounted) {
        setState(() {
          _errorByType[type] = e.toString();
          _loadingByType[type] = false;
        });
      }
    }
  }

  Future<void> _loadMoreContent() async {
    final type = _tabs[_tabController.index];
    if (_loadingByType[type]!) return;

    final nextPage = _pageByType[type]! + 1;
    final filter = _filterByType[type]!;
    final selectedGenre = filter.genres.isNotEmpty ? filter.genres.first : null;

    Logger.d('Loading more $type content, page $nextPage', tag: _tag);
    setState(() => _loadingByType[type] = true);

    try {
      // Get only home providers for pagination (parallel fetch)
      final providers = _registry.getHomeProvidersByContentType(type);

      final results = await Future.wait(
        providers.map((provider) async {
          try {
            if (selectedGenre != null) {
              return await provider.getByCategory(
                selectedGenre,
                type: type,
                page: nextPage,
              );
            }
            return await provider.getPopular(type: type, page: nextPage);
          } catch (e, stack) {
            Logger.e(
              'Failed to load more $type from ${provider.name}',
              tag: _tag,
              error: e,
              stackTrace: stack,
            );
            return <MediaItem>[];
          }
        }),
      );
      final newItems = results.expand((items) => items).toList();

      if (mounted && newItems.isNotEmpty) {
        final filtered = filter.apply(newItems);
        Logger.d(
          'Added ${filtered.length} items for page $nextPage',
          tag: _tag,
        );
        setState(() {
          _contentByType[type] = [..._contentByType[type]!, ...filtered];
          _pageByType[type] = nextPage;
          _loadingByType[type] = false;
        });
      } else {
        Logger.d('No more items for page $nextPage', tag: _tag);
        setState(() => _loadingByType[type] = false);
      }
    } catch (e) {
      if (mounted) {
        setState(() => _loadingByType[type] = false);
      }
    }
  }

  Future<void> _showFilterSheet(ContentType type) async {
    final currentFilter = _filterByType[type]!;
    final newFilter = await FilterSheet.show(
      context,
      initialFilter: currentFilter,
      showTypeFilter: false, // Type is already selected via tab
    );

    if (newFilter != null && mounted) {
      setState(() {
        _filterByType[type] = newFilter.copyWith(type: type);
        _contentByType[type] = [];
      });
      _loadContent(type);
    }
  }

  bool get _isDesktop =>
      !kIsWeb &&
      (defaultTargetPlatform == TargetPlatform.windows ||
          defaultTargetPlatform == TargetPlatform.linux ||
          defaultTargetPlatform == TargetPlatform.macOS);

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      body: Column(
        children: [
          if (_isDesktop) const CustomTitleBar(),
          _buildAppBar(),
          _buildTabBar(),
          Expanded(child: _buildTabContent()),
        ],
      ),
    );
  }

  Widget _buildAppBar() {
    final currentType = _tabs[_tabController.index];
    final currentFilter = _filterByType[currentType]!;

    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 4),
      decoration: BoxDecoration(
        color: Theme.of(context).colorScheme.surface,
        border: Border(
          bottom: BorderSide(color: Theme.of(context).dividerColor, width: 1),
        ),
      ),
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
            tooltip: 'Р В Р’В Р РЋРЎС™Р В Р’В Р вЂ™Р’В°Р В Р’В Р вЂ™Р’В·Р В Р’В Р вЂ™Р’В°Р В Р’В Р СћРІР‚В',
          ),
          const SizedBox(width: 8),
          const Icon(Icons.category),
          const SizedBox(width: 8),
          const Text(
            'Р В Р’В Р РЋРІвЂћСћР В Р’В Р вЂ™Р’В°Р В Р Р‹Р Р†Р вЂљРЎв„ўР В Р’В Р вЂ™Р’В°Р В Р’В Р вЂ™Р’В»Р В Р’В Р РЋРІР‚СћР В Р’В Р РЋРІР‚вЂњ',
            style: TextStyle(fontSize: 20, fontWeight: FontWeight.bold),
          ),
          const Spacer(),
          // Separate provider buttons
          ..._buildSeparateProviderButtons(),
          const SizedBox(width: 8),
          FilterButton(
            filter: currentFilter,
            onTap: () => _showFilterSheet(currentType),
          ),
          SortButton(
            currentSort: currentFilter.sortBy,
            direction: currentFilter.sortDirection,
            onSortChanged: (sort) {
              setState(() {
                _filterByType[currentType] = currentFilter.copyWith(
                  sortBy: sort,
                );
                _contentByType[currentType] = [];
              });
              _loadContent(currentType);
            },
            onDirectionChanged: (dir) {
              setState(() {
                _filterByType[currentType] = currentFilter.copyWith(
                  sortDirection: dir,
                );
                _contentByType[currentType] = [];
              });
              _loadContent(currentType);
            },
          ),
        ],
      ),
    );
  }

  /// Build buttons for separate providers
  List<Widget> _buildSeparateProviderButtons() {
    final separateProviders = _registry.separateProviders;
    if (separateProviders.isEmpty) return [];

    return separateProviders.map((provider) {
      return Padding(
        padding: const EdgeInsets.symmetric(horizontal: 2),
        child: _SeparateProviderButton(
          provider: provider,
          onTap: () => _openSeparateProvider(provider),
        ),
      );
    }).toList();
  }

  void _openSeparateProvider(ContentProvider provider) {
    context.push('/provider/${provider.id}');
  }

  Widget _buildTabBar() {
    return Container(
      color: Theme.of(context).colorScheme.surface,
      child: TabBar(
        controller: _tabController,
        tabs: _tabs.map((type) {
          return Tab(
            child: Row(
              mainAxisSize: MainAxisSize.min,
              children: [
                Icon(_getTypeIcon(type), size: 18),
                const SizedBox(width: 4),
                Flexible(
                  child: Text(
                    type.pluralName,
                    overflow: TextOverflow.ellipsis,
                    maxLines: 1,
                  ),
                ),
              ],
            ),
          );
        }).toList(),
        isScrollable: true, // Always scrollable to prevent overflow
        indicatorColor: Theme.of(context).colorScheme.primary,
        labelColor: Theme.of(context).colorScheme.primary,
        unselectedLabelColor: Theme.of(context).textTheme.bodySmall?.color,
        tabAlignment: TabAlignment.start,
      ),
    );
  }

  IconData _getTypeIcon(ContentType type) {
    switch (type) {
      case ContentType.movie:
        return Icons.movie;
      case ContentType.series:
        return Icons.tv;
      case ContentType.cartoon:
        return Icons.animation;
      case ContentType.anime:
        return Icons.auto_awesome;
      case ContentType.dorama:
        return Icons.theater_comedy; // Assuming a suitable IconData for dorama
      case ContentType.unknown:
        return Icons.help_outline;
    }
  }

  Widget _buildTabContent() {
    return TabBarView(
      controller: _tabController,
      children: _tabs.map((type) => _buildContentGrid(type)).toList(),
    );
  }

  Widget _buildContentGrid(ContentType type) {
    final items = _contentByType[type]!;
    final isLoading = _loadingByType[type]!;
    final error = _errorByType[type];
    final filter = _filterByType[type]!;

    if (isLoading && items.isEmpty) {
      return _buildSkeletonGrid();
    }

    if (error != null && items.isEmpty) {
      return Center(
        child: Column(
          mainAxisAlignment: MainAxisAlignment.center,
          children: [
            const Icon(Icons.error_outline, size: 64, color: Colors.red),
            const SizedBox(height: 16),
            Text(
              'Р В Р’В Р РЋРЎСџР В Р’В Р РЋРІР‚СћР В Р’В Р РЋР’ВР В Р’В Р РЋРІР‚ВР В Р’В Р вЂ™Р’В»Р В Р’В Р РЋРІР‚СњР В Р’В Р вЂ™Р’В° Р В Р’В Р вЂ™Р’В·Р В Р’В Р вЂ™Р’В°Р В Р’В Р В РІР‚В Р В Р’В Р вЂ™Р’В°Р В Р’В Р В РІР‚В¦Р В Р Р‹Р Р†Р вЂљРЎв„ўР В Р’В Р вЂ™Р’В°Р В Р’В Р вЂ™Р’В¶Р В Р’В Р вЂ™Р’ВµР В Р’В Р В РІР‚В¦Р В Р’В Р В РІР‚В¦Р В Р Р‹Р В Р РЏ',
              style: Theme.of(context).textTheme.headlineSmall,
            ),
            const SizedBox(height: 8),
            Text(error, style: Theme.of(context).textTheme.bodyMedium),
            const SizedBox(height: 24),
            ElevatedButton(
              onPressed: () => _loadContent(type),
              child: const Text('Р В Р’В Р В Р вЂ№Р В Р’В Р РЋРІР‚вЂќР В Р Р‹Р В РІР‚С™Р В Р’В Р РЋРІР‚СћР В Р’В Р вЂ™Р’В±Р В Р Р‹Р РЋРІР‚СљР В Р’В Р В РІР‚В Р В Р’В Р вЂ™Р’В°Р В Р Р‹Р Р†Р вЂљРЎв„ўР В Р’В Р РЋРІР‚В Р В Р’В Р вЂ™Р’В·Р В Р’В Р В РІР‚В¦Р В Р’В Р РЋРІР‚СћР В Р’В Р В РІР‚В Р В Р Р‹Р РЋРІР‚Сљ'),
            ),
          ],
        ),
      );
    }

    if (items.isEmpty) {
      return Center(
        child: Column(
          mainAxisAlignment: MainAxisAlignment.center,
          children: [
            Icon(_getTypeIcon(type), size: 64, color: Colors.grey),
            const SizedBox(height: 16),
            Text(
              'Р В Р’В Р РЋРЎС™Р В Р Р‹Р Р†Р вЂљРІР‚СљР В Р Р‹Р Р†Р вЂљР Р‹Р В Р’В Р РЋРІР‚СћР В Р’В Р РЋРІР‚вЂњР В Р’В Р РЋРІР‚Сћ Р В Р’В Р В РІР‚В¦Р В Р’В Р вЂ™Р’Вµ Р В Р’В Р вЂ™Р’В·Р В Р’В Р В РІР‚В¦Р В Р’В Р вЂ™Р’В°Р В Р’В Р Р†РІР‚С›РІР‚вЂњР В Р’В Р СћРІР‚ВР В Р’В Р вЂ™Р’ВµР В Р’В Р В РІР‚В¦Р В Р’В Р РЋРІР‚Сћ',
              style: Theme.of(context).textTheme.headlineSmall,
            ),
            const SizedBox(height: 8),
            Text(
              'Р В Р’В Р В Р вЂ№Р В Р’В Р РЋРІР‚вЂќР В Р Р‹Р В РІР‚С™Р В Р’В Р РЋРІР‚СћР В Р’В Р вЂ™Р’В±Р В Р Р‹Р РЋРІР‚СљР В Р’В Р Р†РІР‚С›РІР‚вЂњР В Р Р‹Р Р†Р вЂљРЎв„ўР В Р’В Р вЂ™Р’Вµ Р В Р’В Р вЂ™Р’В·Р В Р’В Р РЋР’ВР В Р Р‹Р Р†Р вЂљРІР‚СљР В Р’В Р В РІР‚В¦Р В Р’В Р РЋРІР‚ВР В Р Р‹Р Р†Р вЂљРЎв„ўР В Р’В Р РЋРІР‚В Р В Р Р‹Р Р†Р вЂљРЎвЂєР В Р Р‹Р Р†Р вЂљРІР‚СљР В Р’В Р вЂ™Р’В»Р В Р Р‹Р В Р вЂ°Р В Р Р‹Р Р†Р вЂљРЎв„ўР В Р Р‹Р В РІР‚С™Р В Р’В Р РЋРІР‚В',
              style: Theme.of(context).textTheme.bodyMedium,
            ),
          ],
        ),
      );
    }

    return RefreshIndicator(
      onRefresh: () async {
        setState(() {
          _contentByType[type] = [];
          _pageByType[type] = 1;
        });
        await _loadContent(type);
      },
      child: CustomScrollView(
        controller: _tabController.index == _tabs.indexOf(type)
            ? _scrollController
            : null,
        slivers: [
          // Quick filter chips
          if (filter.hasActiveFilters)
            SliverToBoxAdapter(
              child: QuickFilterChips(
                filter: filter,
                onFilterChanged: (newFilter) {
                  setState(() {
                    _filterByType[type] = newFilter.copyWith(type: type);
                    _contentByType[type] = [];
                  });
                  _loadContent(type);
                },
              ),
            ),

          // Grid
          SliverPadding(
            padding: EdgeInsets.all(_ui.gridSpacing.padding),
            sliver: SliverGrid(
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
              delegate: SliverChildBuilderDelegate((context, index) {
                final item = items[index];
                return MediaCard(item: item, onTap: () => _openDetails(item));
              }, childCount: items.length),
            ),
          ),

          // Loading indicator
          if (isLoading)
            SliverPadding(
              padding: const EdgeInsets.symmetric(vertical: 24),
              sliver: SliverToBoxAdapter(
                child: Center(
                  child: SizedBox(
                    width: 50,
                    height: 50,
                    child: Skeleton(borderRadius: _ui.posterSize.borderRadius),
                  ),
                ),
              ),
            ),

          // Bottom padding
          const SliverPadding(padding: EdgeInsets.only(bottom: 32)),
        ],
      ),
    );
  }

  void _openDetails(MediaItem item) {
    final encodedId = Uri.encodeComponent(item.id);
    context.push('/details/${item.providerId}/$encodedId');
  }

  double _getMaxCrossAxisExtent() {
    switch (_ui.posterSize) {
      case PosterSize.small:
        return _isDesktop ? 130 : 100;
      case PosterSize.medium:
        return _isDesktop ? 180 : 140;
      case PosterSize.large:
        return _isDesktop ? 250 : 180;
    }
  }

  Widget _buildSkeletonGrid() {
    return GridView.builder(
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
      itemCount: 12,
      itemBuilder: (context, index) =>
          Skeleton(borderRadius: _ui.posterSize.borderRadius),
    );
  }
}

/// Button for separate providers
class _SeparateProviderButton extends StatelessWidget {
  final ContentProvider provider;
  final VoidCallback onTap;

  const _SeparateProviderButton({required this.provider, required this.onTap});

  @override
  Widget build(BuildContext context) {
    final color = _getProviderColor(context, provider.id);

    return Material(
      color: Colors.transparent,
      child: InkWell(
        onTap: onTap,
        borderRadius: BorderRadius.circular(8),
        child: Container(
          padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 6),
          decoration: BoxDecoration(
            color: color.withValues(alpha: 0.15),
            borderRadius: BorderRadius.circular(8),
            border: Border.all(color: color.withValues(alpha: 0.3)),
          ),
          child: Row(
            mainAxisSize: MainAxisSize.min,
            children: [
              Icon(_getProviderIcon(provider.id), size: 18, color: color),
              const SizedBox(width: 6),
              Text(
                provider.name.toUpperCase(),
                style: TextStyle(
                  color: color,
                  fontWeight: FontWeight.bold,
                  fontSize: 12,
                  letterSpacing: 0.5,
                ),
              ),
            ],
          ),
        ),
      ),
    );
  }

  IconData _getProviderIcon(String providerId) {
    switch (providerId) {
      default:
        return Icons.video_library;
    }
  }

  Color _getProviderColor(BuildContext context, String providerId) {
    switch (providerId) {
      default:
        return Theme.of(context).colorScheme.primary;
    }
  }
}
