import 'package:flutter/foundation.dart';
import 'package:flutter/material.dart';
import 'package:get_it/get_it.dart';
import 'package:go_router/go_router.dart';

import '../../../core/utils/logger.dart';
import '../../../data/providers/provider_registry.dart';
import '../../../data/services/settings_service.dart';
import '../../../domain/entities/entities.dart';
import '../../../domain/repositories/content_provider.dart';
import '../../theme/app_theme.dart';
import '../../widgets/media_card.dart';
import '../../widgets/custom_titlebar.dart';
import '../../widgets/filter_sheet.dart';

const _tag = 'ProviderPage';

/// Dedicated page for a single separate provider
class ProviderPage extends StatefulWidget {
  final String providerId;

  const ProviderPage({super.key, required this.providerId});

  @override
  State<ProviderPage> createState() => _ProviderPageState();
}

class _ProviderPageState extends State<ProviderPage>
    with SingleTickerProviderStateMixin {
  final _registry = GetIt.instance<ProviderRegistry>();
  final _settings = GetIt.instance<SettingsService>();
  final _scrollController = ScrollController();
  late TabController _tabController;

  ContentProvider? _provider;

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

    _provider = _registry.getById(widget.providerId);

    // Initialize state for each type
    for (final type in _tabs) {
      _contentByType[type] = [];
      _loadingByType[type] = false;
      _errorByType[type] = null;
      _filterByType[type] = ContentFilter(type: type);
      _pageByType[type] = 1;
    }

    _tabController = TabController(length: _tabs.length, vsync: this);
    _tabController.addListener(_onTabChanged);
    _scrollController.addListener(_onScroll);

    // Load initial content
    if (_provider != null) {
      _loadContent(_tabs[_tabController.index]);
    }
  }

  @override
  void dispose() {
    _tabController.removeListener(_onTabChanged);
    _tabController.dispose();
    _scrollController.removeListener(_onScroll);
    _scrollController.dispose();
    super.dispose();
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
    if (_loadingByType[type]! || _provider == null) return;

    setState(() {
      _loadingByType[type] = true;
      _errorByType[type] = null;
    });

    try {
      final filter = _filterByType[type]!;
      final selectedGenre = filter.genres.isNotEmpty
          ? filter.genres.first
          : null;

      Logger.d('Loading $type content from ${_provider!.name}', tag: _tag);

      List<MediaItem> items;
      if (selectedGenre != null) {
        items = await _provider!.getByCategory(
          selectedGenre,
          type: type,
          page: 1,
        );
      } else {
        items = await _provider!.getPopular(type: type, page: 1);
      }

      Logger.d('Got ${items.length} items from ${_provider!.name}', tag: _tag);

      if (mounted) {
        final filtered = filter.apply(items);
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
    if (_loadingByType[type]! || _provider == null) return;

    final nextPage = _pageByType[type]! + 1;
    final filter = _filterByType[type]!;
    final selectedGenre = filter.genres.isNotEmpty ? filter.genres.first : null;

    Logger.d('Loading more $type content, page $nextPage', tag: _tag);
    setState(() => _loadingByType[type] = true);

    try {
      List<MediaItem> items;
      if (selectedGenre != null) {
        items = await _provider!.getByCategory(
          selectedGenre,
          type: type,
          page: nextPage,
        );
      } else {
        items = await _provider!.getPopular(type: type, page: nextPage);
      }

      if (mounted && items.isNotEmpty) {
        final filtered = filter.apply(items);
        setState(() {
          _contentByType[type] = [..._contentByType[type]!, ...filtered];
          _pageByType[type] = nextPage;
          _loadingByType[type] = false;
        });
      } else {
        setState(() => _loadingByType[type] = false);
      }
    } catch (e) {
      if (mounted) {
        setState(() => _loadingByType[type] = false);
        ScaffoldMessenger.of(context).showSnackBar(
          SnackBar(
            content: Text(
              'РќРµ РІРґР°Р»РѕСЃСЏ Р·Р°РІР°РЅС‚Р°Р¶РёС‚Рё СЃС‚РѕСЂС–РЅРєСѓ: $e',
            ),
            duration: const Duration(seconds: 3),
          ),
        );
      }
    }
  }

  Future<void> _showFilterSheet(ContentType type) async {
    final currentFilter = _filterByType[type]!;
    final newFilter = await FilterSheet.show(
      context,
      initialFilter: currentFilter,
      showTypeFilter: false,
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

  Color get _providerColor {
    switch (widget.providerId) {
      default:
        return AppTheme.primaryColor;
    }
  }

  IconData get _providerIcon {
    switch (widget.providerId) {
      default:
        return Icons.video_library;
    }
  }

  /// Check if provider has fixed streams (can't change quality/voiceover after start)
  bool get _hasFixedStreams {
    if (_provider != null) return _registry.hasFixed(_provider!);
    return false;
  }

  @override
  Widget build(BuildContext context) {
    if (_provider == null) {
      return Scaffold(
        appBar: AppBar(
          title: const Text('РџСЂРѕРІР°Р№РґРµСЂ РЅРµ Р·РЅР°Р№РґРµРЅРѕ'),
        ),
        body: const Center(
          child: Text('РџСЂРѕРІР°Р№РґРµСЂ РЅРµ Р·РЅР°Р№РґРµРЅРѕ'),
        ),
      );
    }

    return Scaffold(
      body: Column(
        children: [
          if (_isDesktop) const CustomTitleBar(),
          _buildAppBar(),
          if (_hasFixedStreams) _buildFixedStreamsWarning(),
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
        color: AppTheme.surfaceColor,
        border: Border(
          bottom: BorderSide(color: AppTheme.borderColor, width: 1),
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
            tooltip: 'РќР°Р·Р°Рґ',
          ),
          const SizedBox(width: 8),
          Icon(_providerIcon, color: _providerColor),
          const SizedBox(width: 8),
          Text(
            _provider!.name.toUpperCase(),
            style: TextStyle(
              fontSize: 20,
              fontWeight: FontWeight.bold,
              color: _providerColor,
              letterSpacing: 1,
            ),
          ),
          const Spacer(),
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

  Widget _buildFixedStreamsWarning() {
    return Container(
      width: double.infinity,
      padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 8),
      color: Colors.orange.withOpacity(0.15),
      child: Row(
        children: [
          Icon(Icons.info_outline, size: 16, color: Colors.orange.shade700),
          const SizedBox(width: 8),
          Expanded(
            child: Text(
              'РЇРєС–СЃС‚СЊ С‚Р° РґСѓР±Р»СЏР¶ С„С–РєСЃСѓСЋС‚СЊСЃСЏ РїСЂРё Р·Р°РїСѓСЃРєСѓ РІС–РґРµРѕ С– РЅРµ РјРѕР¶СѓС‚СЊ Р±СѓС‚Рё Р·РјС–РЅРµРЅС– РїС–Рґ С‡Р°СЃ РїРµСЂРµРіР»СЏРґСѓ',
              style: TextStyle(fontSize: 12, color: Colors.orange.shade700),
            ),
          ),
        ],
      ),
    );
  }

  Widget _buildTabBar() {
    return Container(
      color: AppTheme.surfaceColor,
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
        isScrollable: true,
        indicatorColor: _providerColor,
        labelColor: _providerColor,
        unselectedLabelColor: AppTheme.textMuted,
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
        return Icons.filter_vintage;
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
      return const Center(child: CircularProgressIndicator());
    }

    if (error != null && items.isEmpty) {
      return Center(
        child: Column(
          mainAxisAlignment: MainAxisAlignment.center,
          children: [
            const Icon(Icons.error_outline, size: 64, color: Colors.red),
            const SizedBox(height: 16),
            Text(
              'РџРѕРјРёР»РєР° Р·Р°РІР°РЅС‚Р°Р¶РµРЅРЅСЏ',
              style: Theme.of(context).textTheme.headlineSmall,
            ),
            const SizedBox(height: 8),
            Text(error, style: Theme.of(context).textTheme.bodyMedium),
            const SizedBox(height: 24),
            ElevatedButton(
              onPressed: () => _loadContent(type),
              child: const Text('РЎРїСЂРѕР±СѓРІР°С‚Рё Р·РЅРѕРІСѓ'),
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
              'РќС–С‡РѕРіРѕ РЅРµ Р·РЅР°Р№РґРµРЅРѕ',
              style: Theme.of(context).textTheme.headlineSmall,
            ),
            const SizedBox(height: 8),
            Text(
              'РЎРїСЂРѕР±СѓР№С‚Рµ Р·РјС–РЅРёС‚Рё С„С–Р»СЊС‚СЂРё',
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
              gridDelegate: SliverGridDelegateWithMaxCrossAxisExtent(
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
            const SliverToBoxAdapter(
              child: Padding(
                padding: EdgeInsets.all(32),
                child: Center(child: CircularProgressIndicator()),
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
}
