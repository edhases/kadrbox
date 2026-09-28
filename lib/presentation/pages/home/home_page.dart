import '../../../data/services/recommendation_service.dart';
import 'package:flutter/material.dart';
import 'package:flutter/foundation.dart';
import 'package:flutter/services.dart';
import 'package:cached_network_image/cached_network_image.dart';
import 'package:get_it/get_it.dart';
import 'package:go_router/go_router.dart';
import 'package:file_picker/file_picker.dart';
import 'package:connectivity_plus/connectivity_plus.dart';

import '../../../core/l10n/app_strings.dart';

import '../../../data/providers/provider_registry.dart';
import '../../../data/services/settings_service.dart';
import '../../../data/services/episode_update_service.dart';
import '../../../domain/entities/entities.dart';
import '../../../domain/repositories/content_provider.dart';
import '../../theme/app_theme.dart';
import '../../widgets/media_card.dart';
import '../../widgets/custom_titlebar.dart';
import '../../widgets/filter_sheet.dart';
import '../../widgets/new_episodes_widget.dart';
import '../../widgets/common/skeleton.dart';
import '../../widgets/common/app_error_widget.dart';
import '../../widgets/tv/focusable_card.dart';
import '../../widgets/home/continue_watching_section.dart';
import '../../widgets/home/recommendations_section.dart';

/// Home page with content browsing
class HomePage extends StatefulWidget {
  const HomePage({super.key});

  @override
  State<HomePage> createState() => _HomePageState();
}

class _HomePageState extends State<HomePage> {
  final _registry = GetIt.instance<ProviderRegistry>();
  final _settings = GetIt.instance<SettingsService>();
  final _categoriesController = ScrollController();
  final _providersController = ScrollController();
  final _episodeUpdateService = GetIt.instance<EpisodeUpdateService>();
  final _scrollController = ScrollController();

  UISettings get _ui => _settings.uiSettings;

  List<MediaItem> _allItems = [];
  List<MediaItem> _filteredItems = [];
  bool _isLoading = true;
  String? _error;
  ContentFilter _filter = const ContentFilter();

  bool _isOffline = false;

  @override
  void initState() {
    super.initState();
    _settings.addListener(_onSettingsChanged);
    _checkConnectivity();
    Connectivity().onConnectivityChanged.listen(_updateConnectivity);
    _loadContent();

    // Check for new episodes on startup
    _episodeUpdateService.checkForUpdates();
  }

  Future<void> _checkConnectivity() async {
    final result = await Connectivity().checkConnectivity();
    _updateConnectivity(result);
  }

  void _updateConnectivity(List<ConnectivityResult> results) {
    final isOffline = results.contains(ConnectivityResult.none);
    if (_isOffline != isOffline) {
      setState(() => _isOffline = isOffline);
      if (!isOffline) {
        _loadContent(refresh: true);
      }
    }
  }

  @override
  void dispose() {
    _settings.removeListener(_onSettingsChanged);
    _scrollController.dispose();
    _categoriesController.dispose();
    _providersController.dispose();
    super.dispose();
  }

  void _onSettingsChanged() {
    if (mounted) setState(() {});
  }

  Future<void> _loadContent({bool refresh = false}) async {
    if (!mounted) return;

    if (_isOffline) {
      setState(() {
        _isLoading = false;
        _error = null;
      });
      return;
    }

    if (!refresh) {
      setState(() {
        _isLoading = true;
        _error = null;
      });
    }

    try {
      // Also refresh recommendations on pull-to-refresh
      if (refresh) {
        GetIt.instance<RecommendationService>().refresh();
      }

      final providers = _registry.homeProviders;
      final allItems = <MediaItem>[];
      final seenIds = <String>{};

      // Check if genre filter is active
      final selectedGenre = _filter.genres.isNotEmpty
          ? _filter.genres.first
          : null;
      final selectedType = _filter.type;

      final errors = <String>[];
      final results = await Future.wait(
        providers.map((provider) async {
          try {
            if (selectedGenre != null) {
              return await provider.getByCategory(
                selectedGenre,
                type: selectedType,
                page: 1,
              );
            } else {
              return await provider.getPopular(type: selectedType, page: 1);
            }
          } catch (e) {
            debugPrint('Failed to load from ${provider.name}: $e');
            errors.add('${provider.name}: $e');
            return <MediaItem>[];
          }
        }),
      );

      for (final items in results) {
        for (final item in items) {
          if (!seenIds.contains(item.uniqueId)) {
            seenIds.add(item.uniqueId);
            allItems.add(item);
          }
        }
      }

      if (mounted) {
        setState(() {
          _allItems = allItems;
          _applyFilter();
          _isLoading = false;
          if (allItems.isEmpty && errors.length == providers.length && providers.isNotEmpty) {
            _error = 'Не вдалося завантажити контент. Перевірте з\'єднання з мережею.';
          }
        });
      }
    } catch (e) {
      if (mounted) {
        setState(() {
          _error = e.toString();
          _isLoading = false;
        });
      }
    }
  }

  void _applyFilter() {
    _filteredItems = _filter.apply(_allItems);
  }

  Future<void> _showFilterSheet() async {
    final oldFilter = _filter;
    final newFilter = await FilterSheet.show(context, initialFilter: _filter);
    if (newFilter != null && mounted) {
      // Check if genre or type changed - need to reload from server
      final genresChanged = !_setEquals(oldFilter.genres, newFilter.genres);
      final typeChanged = oldFilter.type != newFilter.type;

      setState(() {
        _filter = newFilter;
      });

      if (genresChanged || typeChanged) {
        // Reload content with new filter from server
        _loadContent();
      } else {
        // Only client-side filtering needed
        _applyFilter();
      }
    }
  }

  bool _setEquals<T>(Set<T> a, Set<T> b) {
    if (a.length != b.length) return false;
    return a.every((item) => b.contains(item));
  }

  Future<void> _openLocalFile() async {
    try {
      // Check if platform supports file picker
      if (kIsWeb) {
        if (mounted) {
          ScaffoldMessenger.of(context).showSnackBar(
            const SnackBar(
              content: Text('Локальні файли не підтримуються у веб-версії'),
            ),
          );
        }
        return;
      }

      final result = await FilePicker.platform.pickFiles(
        type: FileType.custom,
        allowedExtensions: ['mp4', 'mkv', 'avi', 'mov', 'webm'],
        dialogTitle: AppStrings.of(context).selectVideoFile,
      );

      if (result != null && result.files.single.path != null) {
        final filePath = result.files.single.path!;
        final fileName = result.files.single.name.replaceAll(
          RegExp(r'\.[^.]+$'),
          '',
        );

        if (mounted) {
          context.push(
            '/player',
            extra: {
              'url': 'file://$filePath',
              'title': fileName,
              'subtitle': AppStrings.of(context).localVideo,
              'isOffline': true,
            },
          );
        }
      }
    } catch (e) {
      if (mounted) {
        ScaffoldMessenger.of(
          context,
        ).showSnackBar(SnackBar(content: Text('Помилка: $e')));
      }
    }
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
          // Custom titlebar for desktop
          if (isDesktop) const CustomTitleBar(),

          // Main content
          Expanded(child: _buildBody()),
        ],
      ),
      bottomNavigationBar: isDesktop ? null : _buildBottomNav(),
    );
  }

  Widget _buildBody() {
    if (_isOffline) {
      return Center(
        child: Column(
          mainAxisAlignment: MainAxisAlignment.center,
          children: [
            const Icon(Icons.wifi_off, size: 64, color: Colors.grey),
            const SizedBox(height: 16),
            Text(
              'Відсутнє інтернет-з\'єднання',
              style: Theme.of(context).textTheme.headlineSmall,
            ),
            const SizedBox(height: 8),
            Text(
              'Перевірте підключення або перегляньте завантажений контент',
              style: Theme.of(context).textTheme.bodyMedium,
              textAlign: TextAlign.center,
            ),
            const SizedBox(height: 24),
            FilledButton.icon(
              onPressed: () => context.push('/downloads'),
              icon: const Icon(Icons.download),
              label: const Text('Перейти до завантажень'),
            ),
          ],
        ),
      );
    }

    if (_isLoading) {
      return _buildSkeletonGrid();
    }

    if (_error != null) {
      return AppErrorWidget.loading(message: _error, onRetry: _loadContent);
    }

    if (_filteredItems.isEmpty) {
      return Center(
        child: Column(
          mainAxisAlignment: MainAxisAlignment.center,
          children: [
            Icon(
              _filter.hasActiveFilters
                  ? Icons.filter_alt_off
                  : Icons.movie_outlined,
              size: 64,
              color: Colors.grey,
            ),
            const SizedBox(height: 16),
            Text(
              _filter.hasActiveFilters
                  ? 'Нічого не знайдено'
                  : 'Немає контенту',
              style: Theme.of(context).textTheme.headlineSmall,
            ),
            const SizedBox(height: 8),
            Text(
              _filter.hasActiveFilters
                  ? 'Спробуйте змінити фільтри'
                  : 'Спробуйте пошукати щось',
              style: Theme.of(context).textTheme.bodyMedium,
            ),
            if (_filter.hasActiveFilters) ...[
              const SizedBox(height: 16),
              TextButton.icon(
                onPressed: () {
                  final hadGenresOrType =
                      _filter.genres.isNotEmpty || _filter.type != null;
                  setState(() {
                    _filter = const ContentFilter();
                  });
                  if (hadGenresOrType) {
                    _loadContent();
                  } else {
                    _applyFilter();
                  }
                },
                icon: const Icon(Icons.clear_all),
                label: const Text('Скинути фільтри'),
              ),
            ],
          ],
        ),
      );
    }

    return RefreshIndicator(
      onRefresh: () => _loadContent(refresh: true),
      child: CustomScrollView(
        cacheExtent: 1000.0,
        controller: _scrollController,
        slivers: [
          // App bar
          SliverAppBar(
            floating: true,
            title: const Text('Oxide Film'),
            actions: [
              // Filter button
              FilterButton(filter: _filter, onTap: _showFilterSheet),
              // Sort button
              SortButton(
                currentSort: _filter.sortBy,
                direction: _filter.sortDirection,
                onSortChanged: (sort) => setState(() {
                  _filter = _filter.copyWith(sortBy: sort);
                  _applyFilter();
                }),
                onDirectionChanged: (dir) => setState(() {
                  _filter = _filter.copyWith(sortDirection: dir);
                  _applyFilter();
                }),
              ),
              // New episodes notification
              const NewEpisodesButton(),
              IconButton(
                icon: const Icon(Icons.search),
                onPressed: () => context.push('/search'),
                tooltip: 'Пошук',
              ),
              PopupMenuButton<String>(
                icon: const Icon(Icons.more_vert),
                tooltip: 'Більше',
                onSelected: (value) {
                  switch (value) {
                    case 'favorites':
                      context.push('/favorites');
                      break;
                    case 'history':
                      context.push('/history');
                      break;
                    case 'local_file':
                      _openLocalFile();
                      break;
                    case 'watch_party':
                      context.push('/watch-party');
                      break;
                    case 'downloads':
                      context.push('/downloads');
                      break;
                    case 'settings':
                      context.push('/settings');
                      break;
                  }
                },
                itemBuilder: (context) => [
                  const PopupMenuItem(
                    value: 'favorites',
                    child: Row(
                      children: [
                        Icon(Icons.favorite_border, size: 20),
                        SizedBox(width: 8),
                        Text('Обране'),
                      ],
                    ),
                  ),
                  const PopupMenuItem(
                    value: 'history',
                    child: Row(
                      children: [
                        Icon(Icons.history, size: 20),
                        SizedBox(width: 8),
                        Text('Історія'),
                      ],
                    ),
                  ),
                  PopupMenuItem(
                    value: 'local_file',
                    child: Row(
                      children: [
                        const Icon(Icons.folder_open, size: 20),
                        const SizedBox(width: 8),
                        Text(AppStrings.of(context).openLocalFile),
                      ],
                    ),
                  ),
                  const PopupMenuItem(
                    value: 'watch_party',
                    child: Row(
                      children: [
                        Icon(Icons.group_work, size: 20),
                        SizedBox(width: 8),
                        Text('Спільний перегляд'),
                      ],
                    ),
                  ),
                  const PopupMenuItem(
                    value: 'downloads',
                    child: Row(
                      children: [
                        Icon(Icons.download, size: 20),
                        SizedBox(width: 8),
                        Text('Завантаження'),
                      ],
                    ),
                  ),
                  const PopupMenuItem(
                    value: 'settings',
                    child: Row(
                      children: [
                        Icon(Icons.settings, size: 20),
                        SizedBox(width: 8),
                        Text('Налаштування'),
                      ],
                    ),
                  ),
                ],
              ),
            ],
          ),

          // Quick filter chips
          if (_filter.hasActiveFilters)
            SliverToBoxAdapter(
              child: QuickFilterChips(
                filter: _filter,
                onFilterChanged: (newFilter) {
                  final oldFilter = _filter;
                  final genresChanged = !_setEquals(
                    oldFilter.genres,
                    newFilter.genres,
                  );
                  final typeChanged = oldFilter.type != newFilter.type;

                  setState(() {
                    _filter = newFilter;
                  });

                  if (genresChanged || typeChanged) {
                    _loadContent();
                  } else {
                    _applyFilter();
                  }
                },
              ),
            ),

          // Continue Watching Section
          // Only show if no active filters (don't clutter filtered results)
          if (!_filter.hasActiveFilters)
            const SliverToBoxAdapter(child: ContinueWatchingSection()),

          // Recommendations Section
          // Only show if no active filters
          if (!_filter.hasActiveFilters)
            const SliverToBoxAdapter(child: RecommendationsSection()),

          // Categories section
          SliverToBoxAdapter(child: _buildCategoriesSection()),

          // Providers section
          SliverToBoxAdapter(child: _buildProvidersSection()),

          // Section title
          SliverPadding(
            padding: const EdgeInsets.fromLTRB(16, 16, 16, 8),
            sliver: SliverToBoxAdapter(
              child: Row(
                children: [
                  Expanded(
                    child: Text(
                      _filter.hasActiveFilters
                          ? 'Результати (${_filteredItems.length})'
                          : 'Популярне',
                      style: Theme.of(context).textTheme.headlineSmall
                          ?.copyWith(fontWeight: FontWeight.bold),
                    ),
                  ),
                ],
              ),
            ),
          ),

          // Media grid/list with Focus support for D-Pad
          _buildMediaSection(),

          // Bottom padding
          const SliverPadding(padding: EdgeInsets.only(bottom: 80)),
        ],
      ),
    );
  }

  Widget _buildBottomNav() {
    return BottomNavigationBar(
      currentIndex: 0,
      items: const [
        BottomNavigationBarItem(
          icon: Icon(Icons.home_outlined),
          activeIcon: Icon(Icons.home),
          label: 'Головна',
        ),
        BottomNavigationBarItem(
          icon: Icon(Icons.search_outlined),
          activeIcon: Icon(Icons.search),
          label: 'Пошук',
        ),
        BottomNavigationBarItem(
          icon: Icon(Icons.favorite_outline),
          activeIcon: Icon(Icons.favorite),
          label: 'Улюблене',
        ),
        BottomNavigationBarItem(
          icon: Icon(Icons.history_outlined),
          activeIcon: Icon(Icons.history),
          label: 'Історія',
        ),
        BottomNavigationBarItem(
          icon: Icon(Icons.download_outlined),
          activeIcon: Icon(Icons.download),
          label: 'Завантаження',
        ),
      ],
      onTap: (index) {
        switch (index) {
          case 1:
            context.push('/search');
            break;
          case 2:
            context.push('/favorites');
            break;
          case 3:
            context.push('/history');
            break;
          case 4:
            context.push('/downloads');
            break;
        }
      },
    );
  }

  Widget _buildCategoriesSection() {
    final categories = [
      (ContentType.movie, Icons.movie, Colors.blue),
      (ContentType.series, Icons.tv, Colors.green),
      (ContentType.cartoon, Icons.animation, Colors.orange),
      (ContentType.anime, Icons.auto_awesome, Colors.pink),
      (ContentType.dorama, Icons.filter_vintage, Colors.deepPurple),
    ];

    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Padding(
          padding: const EdgeInsets.fromLTRB(16, 16, 16, 12),
          child: Row(
            children: [
              Text(
                'Категорії',
                style: Theme.of(
                  context,
                ).textTheme.titleMedium?.copyWith(fontWeight: FontWeight.bold),
              ),
              const Spacer(),
              TextButton(
                onPressed: () => context.push('/category'),
                child: const Text('Всі →'),
              ),
            ],
          ),
        ),
        SizedBox(
          height: 100,
          child: Scrollbar(
            controller: _categoriesController,
            child: ListView.builder(
              controller: _categoriesController,
              scrollDirection: Axis.horizontal,
              padding: const EdgeInsets.symmetric(horizontal: 12),
              itemCount: categories.length + 1, // +1 for HDRezka
              itemBuilder: (context, index) {
                if (index < categories.length) {
                  final (type, icon, color) = categories[index];
                  return Padding(
                    padding: const EdgeInsets.symmetric(horizontal: 4),
                    child: _HomeSquareTile(
                      label: type.pluralName,
                      icon: icon,
                      color: color,
                      onTap: () => context.push('/category?type=${type.name}'),
                    ),
                  );
                } else {
                  // HDRezka button - styled to match categories
                  return Padding(
                    padding: const EdgeInsets.symmetric(horizontal: 4),
                    child: _HomeSquareTile(
                      label: 'HDREZKA',
                      icon: Icons.play_circle_filled,
                      color: Colors.orange,
                      onTap: () => context.push('/provider/hdrezka'),
                    ),
                  );
                }
              },
            ),
          ),
        ),
      ],
    );
  }

  Widget _buildProvidersSection() {
    final separateProviders = _registry.separateProviders
        .where((p) => p.id != 'youtube' && p.id != 'hdrezka')
        .toList();
    if (separateProviders.isEmpty) return const SizedBox.shrink();

    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Padding(
          padding: const EdgeInsets.fromLTRB(16, 12, 16, 12),
          child: Row(
            children: [
              Text(
                'Провайдери',
                style: Theme.of(
                  context,
                ).textTheme.titleMedium?.copyWith(fontWeight: FontWeight.bold),
              ),
            ],
          ),
        ),
        SizedBox(
          height: 56,
          child: Scrollbar(
            controller: _providersController,
            child: ListView.builder(
              controller: _providersController,
              scrollDirection: Axis.horizontal,
              padding: const EdgeInsets.symmetric(horizontal: 12),
              itemCount: separateProviders.length,
              itemBuilder: (context, index) {
                final provider = separateProviders[index];
                return Padding(
                  padding: const EdgeInsets.symmetric(horizontal: 4),
                  child: _ProviderCard(
                    provider: provider,
                    onTap: () => context.push('/provider/${provider.id}'),
                  ),
                );
              },
            ),
          ),
        ),
      ],
    );
  }

  Widget _buildMediaSection() {
    switch (_ui.listStyle) {
      case ListStyle.list:
        return SliverPadding(
          padding: EdgeInsets.symmetric(horizontal: _ui.gridSpacing.padding),
          sliver: SliverList(
            delegate: SliverChildBuilderDelegate((context, index) {
              final item = _filteredItems[index];
              return Padding(
                padding: const EdgeInsets.only(bottom: 8),
                child: FocusableCard(
                  onTap: () => _onItemTap(item),
                  borderRadius: 12,
                  child: _MediaListTile(
                    item: item,
                    onTap: () => _onItemTap(item),
                  ),
                ),
              );
            }, childCount: _filteredItems.length),
          ),
        );
      case ListStyle.compact:
        return SliverPadding(
          padding: EdgeInsets.symmetric(horizontal: _ui.gridSpacing.padding),
          sliver: SliverList(
            delegate: SliverChildBuilderDelegate((context, index) {
              final item = _filteredItems[index];
              return FocusableCard(
                onTap: () => _onItemTap(item),
                borderRadius: 8,
                child: _MediaCompactTile(
                  item: item,
                  onTap: () => _onItemTap(item),
                ),
              );
            }, childCount: _filteredItems.length),
          ),
        );
      case ListStyle.grid:
        return SliverPadding(
          padding: EdgeInsets.symmetric(horizontal: _ui.gridSpacing.padding),
          sliver: SliverGrid(
            gridDelegate: SliverGridDelegateWithMaxCrossAxisExtent(
              maxCrossAxisExtent: _getMaxCrossAxisExtent(),
              childAspectRatio: _ui.posterSize.aspectRatio,
              crossAxisSpacing: _ui.gridSpacing.crossAxisSpacing,
              mainAxisSpacing: _ui.gridSpacing.mainAxisSpacing,
            ),
            delegate: SliverChildBuilderDelegate((context, index) {
              final item = _filteredItems[index];
              return MediaCard(item: item, onTap: () => _onItemTap(item));
            }, childCount: _filteredItems.length),
          ),
        );
    }
  }

  void _onItemTap(MediaItem item) {
    context.push('/details/${item.providerId}/${Uri.encodeComponent(item.id)}');
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

  Widget _buildSkeletonGrid() {
    return CustomScrollView(
      slivers: [
        // App bar skeleton
        const SliverAppBar(floating: true, title: Text('Oxide Film')),

        // Categories skeleton
        SliverToBoxAdapter(
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              const Padding(
                padding: EdgeInsets.fromLTRB(16, 16, 16, 12),
                child: Skeleton(width: 100, height: 24),
              ),
              SizedBox(
                height: 100,
                child: ListView.builder(
                  scrollDirection: Axis.horizontal,
                  padding: const EdgeInsets.symmetric(horizontal: 12),
                  itemCount: 4,
                  itemBuilder: (context, index) {
                    return Padding(
                      padding: const EdgeInsets.symmetric(horizontal: 4),
                      child: Skeleton(
                        width: 100,
                        height: 100,
                        borderRadius: 12,
                      ),
                    );
                  },
                ),
              ),
            ],
          ),
        ),

        // Title skeleton
        const SliverPadding(
          padding: EdgeInsets.fromLTRB(16, 16, 16, 8),
          sliver: SliverToBoxAdapter(child: Skeleton(width: 150, height: 28)),
        ),

        // Grid skeleton
        SliverPadding(
          padding: EdgeInsets.symmetric(horizontal: _ui.gridSpacing.padding),
          sliver: SliverGrid(
            gridDelegate: SliverGridDelegateWithMaxCrossAxisExtent(
              maxCrossAxisExtent: _getMaxCrossAxisExtent(),
              childAspectRatio: _ui.posterSize.aspectRatio,
              crossAxisSpacing: _ui.gridSpacing.crossAxisSpacing,
              mainAxisSpacing: _ui.gridSpacing.mainAxisSpacing,
            ),
            delegate: SliverChildBuilderDelegate(
              (context, index) =>
                  Skeleton(borderRadius: _ui.posterSize.borderRadius),
              childCount: 12,
            ),
          ),
        ),
      ],
    );
  }
}

/// Quick action square tile (Category or Provider)
class _HomeSquareTile extends StatelessWidget {
  final String label;
  final IconData icon;
  final Color color;
  final VoidCallback onTap;

  const _HomeSquareTile({
    required this.label,
    required this.icon,
    required this.color,
    required this.onTap,
  });

  @override
  Widget build(BuildContext context) {
    return SizedBox(
      width: 100,
      child: Card(
        color: color.withValues(alpha: 0.1),
        shape: RoundedRectangleBorder(
          borderRadius: BorderRadius.circular(12),
          side: BorderSide(color: color.withValues(alpha: 0.2)),
        ),
        child: InkWell(
          onTap: () {
            HapticFeedback.lightImpact();
            onTap();
          },
          borderRadius: BorderRadius.circular(12),
          child: Column(
            mainAxisAlignment: MainAxisAlignment.center,
            children: [
              Icon(icon, size: 32, color: color),
              const SizedBox(height: 8),
              Text(
                label,
                style: TextStyle(
                  color: color,
                  fontWeight: FontWeight.bold,
                  fontSize: 12,
                ),
                maxLines: 1,
                overflow: TextOverflow.ellipsis,
              ),
            ],
          ),
        ),
      ),
    );
  }
}

/// Media list tile for list view
class _MediaListTile extends StatelessWidget {
  final MediaItem item;
  final VoidCallback onTap;

  const _MediaListTile({required this.item, required this.onTap});

  @override
  Widget build(BuildContext context) {
    return Card(
      clipBehavior: Clip.antiAlias,
      child: InkWell(
        onTap: onTap,
        child: Padding(
          padding: const EdgeInsets.all(12),
          child: Row(
            children: [
              // Poster
              ClipRRect(
                borderRadius: BorderRadius.circular(6),
                child: SizedBox(
                  width: 60,
                  height: 90,
                  child: item.posterUrl != null
                      ? CachedNetworkImage(
                          imageUrl: item.posterUrl!,
                          fit: BoxFit.cover,
                          memCacheHeight: 400,
                          placeholder: (context, url) => const Skeleton(
                            width: 60,
                            height: 90,
                            borderRadius: 0,
                          ),
                          errorWidget: (context, url, error) =>
                              _posterPlaceholder(),
                        )
                      : _posterPlaceholder(),
                ),
              ),
              const SizedBox(width: 12),
              // Info
              Expanded(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Text(
                      item.title,
                      maxLines: 2,
                      overflow: TextOverflow.ellipsis,
                      style: const TextStyle(
                        fontWeight: FontWeight.bold,
                        fontSize: 15,
                      ),
                    ),
                    const SizedBox(height: 4),
                    Wrap(
                      spacing: 8,
                      runSpacing: 4,
                      children: [
                        if (item.year != null)
                          _buildChip(Icons.calendar_today, '${item.year}'),
                        if (item.rating != null)
                          _buildChip(
                            Icons.star,
                            item.rating!.toStringAsFixed(1),
                            color: item.rating! >= 7.0
                                ? Colors.green
                                : item.rating! >= 5.0
                                ? Colors.orange
                                : Colors.red,
                          ),
                        _buildChip(
                          _getTypeIcon(item.type),
                          item.type.displayName,
                          color: _getTypeColor(item.type),
                        ),
                      ],
                    ),
                  ],
                ),
              ),
              const Icon(Icons.chevron_right),
            ],
          ),
        ),
      ),
    );
  }

  Widget _posterPlaceholder() {
    return Container(
      color: Colors.grey[800],
      child: const Icon(Icons.movie, color: Colors.grey),
    );
  }

  Widget _buildChip(IconData icon, String text, {Color? color}) {
    return Row(
      mainAxisSize: MainAxisSize.min,
      children: [
        Icon(icon, size: 14, color: color ?? Colors.grey),
        const SizedBox(width: 4),
        Text(text, style: TextStyle(fontSize: 12, color: color ?? Colors.grey)),
      ],
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
      default:
        return Icons.video_library;
    }
  }

  Color _getTypeColor(ContentType type) {
    switch (type) {
      case ContentType.movie:
        return Colors.blue;
      case ContentType.series:
        return Colors.purple;
      case ContentType.cartoon:
        return Colors.orange;
      case ContentType.anime:
        return Colors.pink;
      case ContentType.dorama:
        return Colors.deepPurple;
      default:
        return Colors.grey;
    }
  }
}

/// Compact media tile for compact list view
class _MediaCompactTile extends StatelessWidget {
  final MediaItem item;
  final VoidCallback onTap;

  const _MediaCompactTile({required this.item, required this.onTap});

  @override
  Widget build(BuildContext context) {
    return ListTile(
      contentPadding: const EdgeInsets.symmetric(horizontal: 12, vertical: 2),
      leading: ClipRRect(
        borderRadius: BorderRadius.circular(4),
        child: SizedBox(
          width: 36,
          height: 54,
          child: item.posterUrl != null
              ? CachedNetworkImage(
                  imageUrl: item.posterUrl!,
                  fit: BoxFit.cover,
                  memCacheHeight: 200,
                  placeholder: (context, url) =>
                      const Skeleton(width: 36, height: 54, borderRadius: 0),
                  errorWidget: (context, url, error) =>
                      Container(color: Colors.grey[800]),
                )
              : Container(color: Colors.grey[800]),
        ),
      ),
      title: Text(
        item.title,
        maxLines: 1,
        overflow: TextOverflow.ellipsis,
        style: const TextStyle(fontSize: 14),
      ),
      subtitle: Text(
        [
          if (item.year != null) '${item.year}',
          item.type.displayName,
          if (item.rating != null) '★${item.rating!.toStringAsFixed(1)}',
        ].join(' • '),
        style: const TextStyle(fontSize: 12),
      ),
      trailing: const Icon(Icons.chevron_right, size: 20),
      onTap: onTap,
    );
  }
}

/// Card for provider (HDREZKA, YouTube)
class _ProviderCard extends StatelessWidget {
  final ContentProvider provider;
  final VoidCallback onTap;

  const _ProviderCard({required this.provider, required this.onTap});

  @override
  Widget build(BuildContext context) {
    final color = _getProviderColor(provider.id);

    return Material(
      color: Colors.transparent,
      child: InkWell(
        onTap: onTap,
        borderRadius: BorderRadius.circular(12),
        child: Container(
          width: 160,
          padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 12),
          decoration: BoxDecoration(
            color: color.withValues(alpha: 0.15),
            borderRadius: BorderRadius.circular(12),
            border: Border.all(color: color.withValues(alpha: 0.3)),
          ),
          child: Row(
            mainAxisSize: MainAxisSize.min,
            children: [
              Icon(_getProviderIcon(provider.id), size: 24, color: color),
              const SizedBox(width: 8),
              Expanded(
                child: Text(
                  provider.name.toUpperCase(),
                  style: TextStyle(
                    color: color,
                    fontWeight: FontWeight.bold,
                    fontSize: 14,
                    letterSpacing: 0.5,
                  ),
                  overflow: TextOverflow.ellipsis,
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
      case 'hdrezka':
        return Icons.play_circle_filled;
      case 'youtube':
        return Icons.play_arrow;
      default:
        return Icons.video_library;
    }
  }

  Color _getProviderColor(String providerId) {
    switch (providerId) {
      case 'hdrezka':
        return Colors.orange;
      case 'youtube':
        return Colors.red;
      default:
        return AppTheme.primaryColor;
    }
  }
}
