import 'dart:async';

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
import '../../../core/utils/responsive_utils.dart';
import '../../../core/utils/stream_url.dart';

import '../../../data/providers/provider_registry.dart';
import '../../../data/services/settings_service.dart';
import '../../../data/services/episode_update_service.dart';
import '../../../data/services/provider_catalog_service.dart';
import '../../../domain/entities/entities.dart';
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
import '../../widgets/home/hero_banner.dart';

/// What the caller wants to do with the URL typed into the stream dialog.
///
/// A record rather than a bare string: two outcomes now come out of one dialog,
/// and a bare String would have needed a sentinel value like 'party' in the
/// same field as a URL.
enum _StreamDialogAction { play, watchParty }

/// Intent for Ctrl+K search shortcut
class SearchIntent extends Intent {
  const SearchIntent();
}

/// Home page with content browsing
class HomePage extends StatefulWidget {
  const HomePage({super.key});

  @override
  State<HomePage> createState() => _HomePageState();
}

class _HomePageState extends State<HomePage> {
  final _registry = GetIt.instance<ProviderRegistry>();
  final _settings = GetIt.instance<SettingsService>();
  final _episodeUpdateService = GetIt.instance<EpisodeUpdateService>();
  final _scrollController = ScrollController();
  final _connectivity = Connectivity();
  StreamSubscription<List<ConnectivityResult>>? _connectivitySub;

  UISettings get _ui => _settings.uiSettings;

  List<MediaItem> _allItems = [];
  List<MediaItem> _filteredItems = [];
  bool _isLoading = true;
  String? _error;
  ContentFilter _filter = const ContentFilter();
  String? _selectedProviderId; // null = all home providers

  bool _isOffline = false;

  @override
  void initState() {
    super.initState();
    _settings.addListener(_onSettingsChanged);
    try {
      GetIt.instance<ProviderCatalogService>().addListener(_onCatalogChanged);
    } catch (_) {}
    _checkConnectivity();
    _connectivitySub = _connectivity.onConnectivityChanged.listen(
      _updateConnectivity,
    );
    _loadContent();

    // Check for new episodes on startup
    _episodeUpdateService.checkForUpdates();
  }

  Future<void> _checkConnectivity() async {
    final result = await _connectivity.checkConnectivity();
    if (!mounted) return;
    _updateConnectivity(result);
  }

  void _updateConnectivity(List<ConnectivityResult> results) {
    if (!mounted) return;
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
    try {
      GetIt.instance<ProviderCatalogService>().removeListener(
        _onCatalogChanged,
      );
    } catch (_) {}
    _scrollController.dispose();
    // dispose() cannot await, but cancel() detaches the platform-channel
    // listener synchronously enough that no further callback can fire.
    unawaited(_connectivitySub?.cancel());
    _connectivitySub = null;
    super.dispose();
  }

  void _onSettingsChanged() {
    if (mounted) setState(() {});
  }

  void _onCatalogChanged() {
    if (mounted) {
      setState(() {});
      _loadContent(refresh: true);
    }
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

      // Ensure catalog is synced from backend (or fallback cache)
      try {
        await GetIt.instance<ProviderCatalogService>().sync();
      } catch (_) {}

      final homeProviders = _registry.homeProviders;
      if (homeProviders.isEmpty) {
        if (mounted) {
          setState(() {
            _allItems = [];
            _filteredItems = [];
            _isLoading = false;
            _error = null;
          });
        }
        return;
      }

      final providers = _selectedProviderId != null
          ? homeProviders.where((p) => p.id == _selectedProviderId).toList()
          : homeProviders;

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
          if (allItems.isEmpty &&
              errors.length == providers.length &&
              providers.isNotEmpty) {
            _error =
                'Не вдалося завантажити контент. Перевірте з\'єднання з мережею.';
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

  Future<void> _openNetworkStream() async {
  final controller = TextEditingController();
  String? error;

  final result = await showDialog<({_StreamDialogAction action, String url})>(
    context: context,
    builder: (ctx) => StatefulBuilder(
      builder: (ctx, setState) {
        void submit(_StreamDialogAction a) {
          final problem = validateMediaUrl(controller.text);
          if (problem != null) {
            setState(() => error = problem);
            return;
          }
          Navigator.pop(
            ctx,
            (action: a, url: controller.text.trim()),
          );
        }

        return AlertDialog(
          title: const Text('Відкрити мережевий потік'),
          content: TextField(
            controller: controller,
            decoration: InputDecoration(
              hintText: 'https://example.com/stream.m3u8',
              labelText: 'URL відео або HLS потоку',
              prefixIcon: const Icon(Icons.link),
              errorText: error,
            ),
            autofocus: true,
            onChanged: (_) {
              // Clear the complaint as soon as the user starts fixing it.
              if (error != null) setState(() => error = null);
            },
            onSubmitted: (_) => submit(_StreamDialogAction.play),
          ),
          actions: [
            TextButton(
              onPressed: () => Navigator.pop(ctx),
              child: const Text('Скасувати'),
            ),
            OutlinedButton.icon(
              onPressed: () => submit(_StreamDialogAction.watchParty),
              icon: const Icon(Icons.groups, size: 20),
              label: const Text('Спільний перегляд'),
            ),
            FilledButton(
              onPressed: () => submit(_StreamDialogAction.play),
              child: const Text('Відтворити'),
            ),
          ],
        );
      },
    ),
  );

  controller.dispose();
  if (result == null || !mounted) return;

  if (result.action == _StreamDialogAction.watchParty) {
    // Seed the room with this stream, so the host never has to retype it.
    context.push(
      '/watch-party',
      extra: {'mediaUrl': result.url, 'mediaTitle': 'Мережевий потік'},
    );
    return;
  }

  context.push(
    '/player',
    extra: {
      'url': result.url,
      'title': 'Мережевий потік',
      'subtitle': result.url,
      'isOffline': false,
    },
  );
}

  @override
  Widget build(BuildContext context) {
    final isDesktopPlatform =
        !kIsWeb &&
        (defaultTargetPlatform == TargetPlatform.windows ||
            defaultTargetPlatform == TargetPlatform.linux ||
            defaultTargetPlatform == TargetPlatform.macOS);

    final showRail = context.isDesktop;

    final bodyWidget = Row(
      children: [
        if (showRail) _buildNavigationRail(),
        if (showRail) const VerticalDivider(thickness: 1, width: 1),
        Expanded(
          child: Center(
            child: ConstrainedBox(
              constraints: const BoxConstraints(maxWidth: 1400),
              child: _buildBody(),
            ),
          ),
        ),
      ],
    );

    return Shortcuts(
      shortcuts: <ShortcutActivator, Intent>{
        LogicalKeySet(LogicalKeyboardKey.control, LogicalKeyboardKey.keyK):
            const SearchIntent(),
        LogicalKeySet(LogicalKeyboardKey.meta, LogicalKeyboardKey.keyK):
            const SearchIntent(),
      },
      child: Actions(
        actions: <Type, Action<Intent>>{
          SearchIntent: CallbackAction<SearchIntent>(
            onInvoke: (intent) {
              context.push('/search');
              return null;
            },
          ),
        },
        child: Focus(
          autofocus: true,
          child: Scaffold(
            body: Column(
              children: [
                // Custom titlebar for desktop
                if (isDesktopPlatform) const CustomTitleBar(),

                // Main content
                Expanded(child: bodyWidget),
              ],
            ),
            bottomNavigationBar: showRail ? null : _buildBottomNav(),
          ),
        ),
      ),
    );
  }

  Widget _buildNavigationRail() {
    final location = GoRouterState.of(context).uri.toString();
    final currentIndex = location.startsWith('/search')
        ? 1
        : location.startsWith('/plugins')
        ? 2
        : location.startsWith('/favorites')
        ? 3
        : location.startsWith('/history')
        ? 4
        : location.startsWith('/downloads')
        ? 5
        : 0;

    return NavigationRail(
      selectedIndex: currentIndex,
      labelType: NavigationRailLabelType.all,
      leading: Padding(
        padding: const EdgeInsets.symmetric(vertical: 8.0),
        child: IconButton(
          icon: const Icon(Icons.search),
          tooltip: 'Пошук (Ctrl+K)',
          onPressed: () => context.push('/search'),
        ),
      ),
      trailing: Expanded(
        child: Align(
          alignment: Alignment.bottomCenter,
          child: Padding(
            padding: const EdgeInsets.only(bottom: 16.0),
            child: IconButton(
              icon: const Icon(Icons.settings_outlined),
              tooltip: 'Налаштування',
              onPressed: () => context.push('/settings'),
            ),
          ),
        ),
      ),
      destinations: const [
        NavigationRailDestination(
          icon: Icon(Icons.home_outlined),
          selectedIcon: Icon(Icons.home),
          label: Text('Головна'),
        ),
        NavigationRailDestination(
          icon: Icon(Icons.search_outlined),
          selectedIcon: Icon(Icons.search),
          label: Text('Пошук'),
        ),
        NavigationRailDestination(
          icon: Icon(Icons.extension_outlined),
          selectedIcon: Icon(Icons.extension),
          label: Text('Плагіни'),
        ),
        NavigationRailDestination(
          icon: Icon(Icons.favorite_outline),
          selectedIcon: Icon(Icons.favorite),
          label: Text('Улюблене'),
        ),
        NavigationRailDestination(
          icon: Icon(Icons.history_outlined),
          selectedIcon: Icon(Icons.history),
          label: Text('Історія'),
        ),
        NavigationRailDestination(
          icon: Icon(Icons.download_outlined),
          selectedIcon: Icon(Icons.download),
          label: Text('Завантаження'),
        ),
      ],
      onDestinationSelected: (index) {
        if (index == currentIndex) return;
        switch (index) {
          case 0:
            context.go('/');
            break;
          case 1:
            context.go('/search');
            break;
          case 2:
            context.go('/plugins');
            break;
          case 3:
            context.go('/favorites');
            break;
          case 4:
            context.go('/history');
            break;
          case 5:
            context.go('/downloads');
            break;
        }
      },
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
      if (_filter.hasActiveFilters) {
        return Center(
          child: Column(
            mainAxisAlignment: MainAxisAlignment.center,
            children: [
              const Icon(Icons.filter_alt_off, size: 64, color: Colors.grey),
              const SizedBox(height: 16),
              Text(
                'Нічого не знайдено',
                style: Theme.of(context).textTheme.headlineSmall,
              ),
              const SizedBox(height: 8),
              const Text('Спробуйте змінити фільтри'),
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
          ),
        );
      }

      // Чистий медіаплеєр за замовчуванням
      return _buildCleanPlayerHome();
    }

    // Top recommended / popular item for Hero Banner
    MediaItem? heroItem;
    if (!_filter.hasActiveFilters && _filteredItems.isNotEmpty) {
      final recService = GetIt.instance<RecommendationService>();
      if (recService.recommendations.isNotEmpty) {
        heroItem = recService.recommendations.first;
      } else {
        heroItem = _filteredItems.first;
      }
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
            title: const Text('Kadrbox'),
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
                tooltip: 'Пошук (Ctrl+K)',
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

          // Categories & Source filter pills directly under the top header
          SliverToBoxAdapter(child: _buildCategoryPillsBar()),

          // Active quick filter chips
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

          // Hero Banner (Top featured item)
          if (heroItem != null)
            SliverToBoxAdapter(child: HeroBanner(item: heroItem)),

          // Continue Watching Section
          if (!_filter.hasActiveFilters)
            const SliverToBoxAdapter(child: ContinueWatchingSection()),

          // Recommendations Section
          if (!_filter.hasActiveFilters)
            const SliverToBoxAdapter(child: RecommendationsSection()),

          // Section title: "Популярне" with inline "Всі →" button
          SliverPadding(
            padding: const EdgeInsets.fromLTRB(16, 20, 16, 12),
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
                  TextButton.icon(
                    onPressed: () {
                      final typeParam = _filter.type != null
                          ? '?type=${_filter.type!.name}'
                          : '';
                      context.push('/category$typeParam');
                    },
                    icon: const Icon(Icons.arrow_forward_rounded, size: 16),
                    label: const Text('Всі →'),
                  ),
                ],
              ),
            ),
          ),

          // Media grid/list
          _buildMediaSection(),

          // Bottom padding
          const SliverPadding(padding: EdgeInsets.only(bottom: 80)),
        ],
      ),
    );
  }

  Widget _buildCategoryPillsBar() {
    final categories = [
      (null, 'Всі', Icons.grid_view_rounded),
      (ContentType.movie, ContentType.movie.displayName, Icons.movie_outlined),
      (ContentType.series, ContentType.series.displayName, Icons.tv_outlined),
      (
        ContentType.cartoon,
        ContentType.cartoon.displayName,
        Icons.animation_outlined,
      ),
      (
        ContentType.anime,
        ContentType.anime.displayName,
        Icons.auto_awesome_outlined,
      ),
      (
        ContentType.dorama,
        ContentType.dorama.displayName,
        Icons.filter_vintage_outlined,
      ),
    ];

    final homeProviders = _registry.homeProviders;

    return SingleChildScrollView(
      scrollDirection: Axis.horizontal,
      padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 8),
      child: Row(
        children: [
          // Source Selector Popup/Pill
          if (homeProviders.length > 1) ...[
            PopupMenuButton<String?>(
              tooltip: 'Джерело контенту',
              initialValue: _selectedProviderId,
              onSelected: (providerId) {
                setState(() {
                  _selectedProviderId = providerId;
                });
                _loadContent();
              },
              itemBuilder: (context) => [
                const PopupMenuItem(value: null, child: Text('Всі джерела')),
                ...homeProviders.map(
                  (p) => PopupMenuItem(value: p.id, child: Text(p.name)),
                ),
              ],
              child: FilterChip(
                selected: _selectedProviderId != null,
                label: Row(
                  mainAxisSize: MainAxisSize.min,
                  children: [
                    Icon(
                      Icons.source_outlined,
                      size: 16,
                      color: _selectedProviderId != null
                          ? AppTheme.primaryColor
                          : Colors.white70,
                    ),
                    const SizedBox(width: 6),
                    Text(
                      _selectedProviderId == null
                          ? 'Джерело ▾'
                          : '${_registry.getById(_selectedProviderId!)?.name ?? _selectedProviderId} ▾',
                      style: TextStyle(
                        fontSize: 12,
                        fontWeight: FontWeight.w600,
                        color: _selectedProviderId != null
                            ? AppTheme.primaryColor
                            : Colors.white,
                      ),
                    ),
                  ],
                ),
                onSelected: null,
                backgroundColor: AppTheme.darkCard,
                selectedColor: AppTheme.primaryColor.withValues(alpha: 0.2),
                shape: RoundedRectangleBorder(
                  borderRadius: BorderRadius.circular(20),
                  side: BorderSide(
                    color: _selectedProviderId != null
                        ? AppTheme.primaryColor
                        : AppTheme.darkBorder,
                  ),
                ),
              ),
            ),
            const SizedBox(width: 8),
          ],

          // Category Pills
          ...categories.map((cat) {
            final (type, label, icon) = cat;
            final isSelected = _filter.type == type;

            return Padding(
              padding: const EdgeInsets.only(right: 8),
              child: FilterChip(
                selected: isSelected,
                avatar: Icon(
                  icon,
                  size: 16,
                  color: isSelected ? Colors.white : Colors.white70,
                ),
                label: Text(label),
                labelStyle: TextStyle(
                  fontSize: 12,
                  fontWeight: isSelected ? FontWeight.bold : FontWeight.w500,
                  color: isSelected ? Colors.white : Colors.white70,
                ),
                backgroundColor: AppTheme.darkCard,
                selectedColor: AppTheme.primaryColor,
                checkmarkColor: Colors.white,
                shape: RoundedRectangleBorder(
                  borderRadius: BorderRadius.circular(20),
                  side: BorderSide(
                    color: isSelected
                        ? AppTheme.primaryColor
                        : AppTheme.darkBorder,
                  ),
                ),
                onSelected: (selected) {
                  final newType = selected ? type : null;
                  setState(() {
                    _filter = _filter.copyWith(
                      type: newType,
                      clearType: newType == null,
                    );
                  });
                  _loadContent();
                },
              ),
            );
          }),
        ],
      ),
    );
  }

  Widget _buildBottomNav() {
    final location = GoRouterState.of(context).uri.toString();
    final currentIndex = location.startsWith('/search')
        ? 1
        : location.startsWith('/plugins')
        ? 2
        : location.startsWith('/favorites')
        ? 3
        : location.startsWith('/history')
        ? 4
        : location.startsWith('/downloads')
        ? 5
        : 0;
    return BottomNavigationBar(
      currentIndex: currentIndex,
      type: BottomNavigationBarType.fixed,
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
          icon: Icon(Icons.extension_outlined),
          activeIcon: Icon(Icons.extension),
          label: 'Плагіни',
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
        if (index == currentIndex) return;
        switch (index) {
          case 0:
            context.go('/');
            break;
          case 1:
            context.go('/search');
            break;
          case 2:
            context.go('/plugins');
            break;
          case 3:
            context.go('/favorites');
            break;
          case 4:
            context.go('/history');
            break;
          case 5:
            context.go('/downloads');
            break;
        }
      },
    );
  }

  int _calculateColumns(BuildContext context) {
    if (_ui.gridColumns > 0) return _ui.gridColumns;

    final width = MediaQuery.of(context).size.width;
    final isMobile = width < ResponsiveUtils.phoneMaxWidth;

    if (isMobile) {
      return width < ResponsiveUtils.compactWidth ? 2 : 3;
    }

    final double effectiveWidth = width > 1400 ? 1400 : width;
    final cols = (effectiveWidth / 180).floor();
    return cols.clamp(3, 7);
  }

  Widget _buildMediaSection() {
    // When no filters are active, limit "Популярне" to 2 rows
    final columns = _calculateColumns(context);
    final isPopularSection = !_filter.hasActiveFilters;
    final maxItems = isPopularSection ? columns * 2 : _filteredItems.length;
    final itemsToShow = _filteredItems.take(maxItems).toList();

    switch (_ui.listStyle) {
      case ListStyle.list:
        return SliverPadding(
          padding: EdgeInsets.symmetric(horizontal: _ui.gridSpacing.padding),
          sliver: SliverList(
            delegate: SliverChildBuilderDelegate((context, index) {
              final item = itemsToShow[index];
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
            }, childCount: itemsToShow.length),
          ),
        );
      case ListStyle.compact:
        return SliverPadding(
          padding: EdgeInsets.symmetric(horizontal: _ui.gridSpacing.padding),
          sliver: SliverList(
            delegate: SliverChildBuilderDelegate((context, index) {
              final item = itemsToShow[index];
              return FocusableCard(
                onTap: () => _onItemTap(item),
                borderRadius: 8,
                child: _MediaCompactTile(
                  item: item,
                  onTap: () => _onItemTap(item),
                ),
              );
            }, childCount: itemsToShow.length),
          ),
        );
      case ListStyle.grid:
        return SliverPadding(
          padding: EdgeInsets.symmetric(horizontal: _ui.gridSpacing.padding),
          sliver: SliverGrid(
            gridDelegate: SliverGridDelegateWithFixedCrossAxisCount(
              crossAxisCount: columns,
              childAspectRatio: 2 / 3,
              crossAxisSpacing: _ui.gridSpacing.crossAxisSpacing,
              mainAxisSpacing: _ui.gridSpacing.mainAxisSpacing,
            ),
            delegate: SliverChildBuilderDelegate((context, index) {
              final item = itemsToShow[index];
              return MediaCard(item: item, onTap: () => _onItemTap(item));
            }, childCount: itemsToShow.length),
          ),
        );
    }
  }

  void _onItemTap(MediaItem item) {
    context.push('/details/${item.providerId}/${Uri.encodeComponent(item.id)}');
  }

  Widget _buildSkeletonGrid() {
    final columns = _calculateColumns(context);

    return CustomScrollView(
      slivers: [
        // App bar skeleton
        const SliverAppBar(floating: true, title: Text('Kadrbox')),

        // Categories pill bar skeleton
        SliverToBoxAdapter(
          child: Padding(
            padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 12),
            child: Row(
              children: List.generate(
                5,
                (index) => const Padding(
                  padding: EdgeInsets.only(right: 8),
                  child: Skeleton(width: 80, height: 32, borderRadius: 16),
                ),
              ),
            ),
          ),
        ),

        // Hero banner skeleton
        const SliverToBoxAdapter(
          child: Padding(
            padding: EdgeInsets.fromLTRB(16, 8, 16, 16),
            child: Skeleton(
              width: double.infinity,
              height: 340,
              borderRadius: 20,
            ),
          ),
        ),

        // Title skeleton
        const SliverPadding(
          padding: EdgeInsets.fromLTRB(16, 16, 16, 8),
          sliver: SliverToBoxAdapter(child: Skeleton(width: 150, height: 28)),
        ),

        // Grid skeleton (2 rows)
        SliverPadding(
          padding: EdgeInsets.symmetric(horizontal: _ui.gridSpacing.padding),
          sliver: SliverGrid(
            gridDelegate: SliverGridDelegateWithFixedCrossAxisCount(
              crossAxisCount: columns,
              childAspectRatio: 2 / 3,
              crossAxisSpacing: _ui.gridSpacing.crossAxisSpacing,
              mainAxisSpacing: _ui.gridSpacing.mainAxisSpacing,
            ),
            delegate: SliverChildBuilderDelegate(
              (context, index) =>
                  Skeleton(borderRadius: _ui.posterSize.borderRadius),
              childCount: columns * 2,
            ),
          ),
        ),
      ],
    );
  }

  Widget _buildCleanPlayerHome() {
    return SingleChildScrollView(
      padding: const EdgeInsets.symmetric(horizontal: 24, vertical: 32),
      child: Center(
        child: ConstrainedBox(
          constraints: const BoxConstraints(maxWidth: 520),
          child: Column(
            mainAxisAlignment: MainAxisAlignment.center,
            children: [
              Container(
                width: 72,
                height: 72,
                decoration: BoxDecoration(
                  color: AppTheme.primaryColor.withValues(alpha: 0.15),
                  shape: BoxShape.circle,
                ),
                child: const Icon(
                  Icons.play_circle_filled_rounded,
                  size: 44,
                  color: AppTheme.primaryColor,
                ),
              ),
              const SizedBox(height: 20),
              Text(
                'Kadrbox Медіаплеєр',
                style: Theme.of(context).textTheme.headlineMedium?.copyWith(
                  fontWeight: FontWeight.bold,
                ),
                textAlign: TextAlign.center,
              ),
              const SizedBox(height: 8),
              Text(
                'Відтворюйте відеофайли з диска, мережеві потоки або дивіться разом з друзями через Watch Party.',
                style: Theme.of(
                  context,
                ).textTheme.bodyMedium?.copyWith(color: Colors.white70),
                textAlign: TextAlign.center,
              ),
              const SizedBox(height: 28),
              SizedBox(
                width: double.infinity,
                child: FilledButton.icon(
                  onPressed: _openLocalFile,
                  icon: const Icon(Icons.folder_open_rounded),
                  label: const Text('Відкрити локальний файл'),
                  style: FilledButton.styleFrom(
                    padding: const EdgeInsets.symmetric(vertical: 14),
                  ),
                ),
              ),
              const SizedBox(height: 10),
              SizedBox(
                width: double.infinity,
                child: OutlinedButton.icon(
                  onPressed: _openNetworkStream,
                  icon: const Icon(Icons.link_rounded),
                  label: const Text('Відкрити потік / URL'),
                  style: OutlinedButton.styleFrom(
                    padding: const EdgeInsets.symmetric(vertical: 14),
                  ),
                ),
              ),
              const SizedBox(height: 10),
              SizedBox(
                width: double.infinity,
                child: OutlinedButton.icon(
                  onPressed: () => context.push('/watch-party'),
                  icon: const Icon(Icons.group_work_rounded),
                  label: const Text('Спільний перегляд (Watch Party)'),
                  style: OutlinedButton.styleFrom(
                    padding: const EdgeInsets.symmetric(vertical: 14),
                  ),
                ),
              ),
              const SizedBox(height: 32),
              Container(
                padding: const EdgeInsets.all(14),
                decoration: BoxDecoration(
                  color: AppTheme.darkCard,
                  borderRadius: BorderRadius.circular(14),
                  border: Border.all(color: AppTheme.darkBorder),
                ),
                child: Row(
                  children: [
                    const Icon(
                      Icons.extension_outlined,
                      color: Colors.white70,
                      size: 24,
                    ),
                    const SizedBox(width: 12),
                    Expanded(
                      child: Column(
                        crossAxisAlignment: CrossAxisAlignment.start,
                        children: [
                          const Text(
                            'Підключення плагінів',
                            style: TextStyle(
                              fontWeight: FontWeight.w600,
                              fontSize: 13,
                            ),
                          ),
                          const SizedBox(height: 2),
                          Text(
                            'Підключіть сумісний плагін у налаштуваннях для онлайн-каталогів.',
                            style: TextStyle(
                              color: Colors.white.withValues(alpha: 0.6),
                              fontSize: 11,
                            ),
                          ),
                        ],
                      ),
                    ),
                    const SizedBox(width: 8),
                    TextButton(
                      onPressed: () => context.push('/plugins'),
                      child: const Text('Налаштувати'),
                    ),
                  ],
                ),
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
