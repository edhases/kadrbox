import 'dart:io';
import 'package:flutter/material.dart';
import 'package:flutter/foundation.dart';
import 'package:get_it/get_it.dart';
import 'package:go_router/go_router.dart';
import 'package:cached_network_image/cached_network_image.dart';

import '../../../data/database/app_database.dart';
import '../../../data/services/favorites_service.dart';
import '../../../data/services/settings_service.dart';
import '../../../data/services/download_service.dart';
import '../../../domain/entities/entities.dart';
import '../../widgets/custom_titlebar.dart';
import '../../widgets/tv/focusable_card.dart';

/// Favorites page - displays saved media items
class FavoritesPage extends StatefulWidget {
  const FavoritesPage({super.key});

  @override
  State<FavoritesPage> createState() => _FavoritesPageState();
}

class _FavoritesPageState extends State<FavoritesPage>
    with SingleTickerProviderStateMixin {
  final _favoritesService = GetIt.instance<FavoritesService>();
  final _settings = GetIt.instance<SettingsService>();
  late TabController _tabController;

  final _tabs = const [
    Tab(text: 'Все'),
    Tab(text: 'Фільми'),
    Tab(text: 'Серіали'),
    Tab(text: 'Мультфільми'),
    Tab(text: 'Аніме'),
  ];

  UISettings get _ui => _settings.uiSettings;

  @override
  void initState() {
    super.initState();
    _tabController = TabController(length: _tabs.length, vsync: this);
    _favoritesService.addListener(_onUpdate);
  }

  @override
  void dispose() {
    _tabController.dispose();
    _favoritesService.removeListener(_onUpdate);
    super.dispose();
  }

  void _onUpdate() {
    if (mounted) setState(() {});
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
          TabBar(
            controller: _tabController,
            tabs: _tabs,
            isScrollable: true,
            indicatorColor: Theme.of(context).colorScheme.primary,
            labelColor: Theme.of(context).colorScheme.primary,
            unselectedLabelColor: Theme.of(context).textTheme.bodySmall?.color,
          ),
          Expanded(
            child: TabBarView(
              controller: _tabController,
              children: [
                _buildFavoritesList(null),
                _buildFavoritesList('movie'),
                _buildFavoritesList('series'),
                _buildFavoritesList('cartoon'),
                _buildFavoritesList('anime'),
              ],
            ),
          ),
        ],
      ),
    );
  }

  Widget _buildAppBar() {
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
            tooltip: 'Назад',
          ),
          const SizedBox(width: 8),
          const Icon(Icons.favorite, color: Colors.red),
          const SizedBox(width: 8),
          const Text(
            'Обране',
            style: TextStyle(fontSize: 20, fontWeight: FontWeight.bold),
          ),
          const Spacer(),
          if (_favoritesService.favorites.isNotEmpty)
            IconButton(
              icon: const Icon(Icons.delete_sweep),
              onPressed: _showClearDialog,
              tooltip: 'Очистити все',
            ),
        ],
      ),
    );
  }

  Widget _buildFavoritesList(String? typeFilter) {
    final items = typeFilter == null
        ? _favoritesService.favorites
        : _favoritesService.favorites
              .where((f) => f.mediaType == typeFilter)
              .toList();

    if (items.isEmpty) {
      return _EmptyState(
        icon: Icons.favorite_border,
        title: 'Тут поки порожньо',
        subtitle: typeFilter == null
            ? 'Додайте фільми та серіали в обране'
            : 'Немає елементів у цій категорії',
      );
    }

    // Use listStyle from settings
    switch (_ui.listStyle) {
      case ListStyle.list:
        return _buildListView(items);
      case ListStyle.compact:
        return _buildCompactListView(items);
      case ListStyle.grid:
        return _buildGridView(items);
    }
  }

  Widget _buildGridView(List<Favorite> items) {
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
      itemCount: items.length,
      itemBuilder: (context, index) {
        final item = items[index];
        return FocusableCard(
          onTap: () => _openDetails(item),
          borderRadius: 8,
          child: _FavoriteCard(
            favorite: item,
            onRemove: () => _removeFavorite(item),
          ),
        );
      },
    );
  }

  Widget _buildListView(List<Favorite> items) {
    return ListView.builder(
      padding: EdgeInsets.all(_ui.gridSpacing.padding),
      itemCount: items.length,
      itemBuilder: (context, index) {
        final item = items[index];
        return Padding(
          padding: const EdgeInsets.only(bottom: 8),
          child: FocusableCard(
            onTap: () => _openDetails(item),
            borderRadius: 8,
            child: _FavoriteListTile(
              favorite: item,
              onRemove: () => _removeFavorite(item),
            ),
          ),
        );
      },
    );
  }

  Widget _buildCompactListView(List<Favorite> items) {
    return ListView.builder(
      padding: EdgeInsets.all(_ui.gridSpacing.padding),
      itemCount: items.length,
      itemBuilder: (context, index) {
        final item = items[index];
        return FocusableCard(
          onTap: () => _openDetails(item),
          borderRadius: 4,
          child: _FavoriteCompactTile(
            favorite: item,
            onRemove: () => _removeFavorite(item),
          ),
        );
      },
    );
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

  void _openDetails(Favorite item) {
    final encodedId = Uri.encodeComponent(item.mediaId);
    context.push('/details/${item.providerId}/$encodedId');
  }

  void _removeFavorite(Favorite item) {
    _favoritesService.remove(item.mediaId, item.providerId);
    ScaffoldMessenger.of(context).showSnackBar(
      SnackBar(
        content: Text('${item.title} видалено з обраного'),
        action: SnackBarAction(
          label: 'Відновити',
          onPressed: () {
            _favoritesService.add(
              MediaItem(
                id: item.mediaId,
                providerId: item.providerId,
                title: item.title,
                posterUrl: item.posterUrl,
                year: item.year,
                type: ContentType.values.firstWhere(
                  (t) => t.name == item.mediaType,
                  orElse: () => ContentType.movie,
                ),
              ),
            );
          },
        ),
      ),
    );
  }

  void _showClearDialog() {
    showDialog(
      context: context,
      builder: (context) => AlertDialog(
        backgroundColor: Theme.of(context).colorScheme.surface,
        title: const Text('Очистити обране?'),
        content: const Text(
          'Ви впевнені, що хочете видалити всі елементи з обраного? '
          'Цю дію неможливо відмінити.',
        ),
        actions: [
          TextButton(
            onPressed: () => Navigator.pop(context),
            child: const Text('Скасувати'),
          ),
          TextButton(
            onPressed: () {
              Navigator.pop(context);
              _favoritesService.clearAll();
            },
            child: Text(
              'Очистити',
              style: TextStyle(color: Theme.of(context).colorScheme.error),
            ),
          ),
        ],
      ),
    );
  }
}

// =============================================================================
// WIDGETS
// =============================================================================

class _FavoriteCard extends StatelessWidget {
  final Favorite favorite;
  final VoidCallback onRemove;

  const _FavoriteCard({required this.favorite, required this.onRemove});

  @override
  Widget build(BuildContext context) {
    return Card(
      margin: EdgeInsets.zero,
      clipBehavior: Clip.antiAlias,
      color: Theme.of(context).colorScheme.surface,
      shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(8)),
      child: Stack(
        fit: StackFit.expand,
        children: [
          // Poster
          Builder(
            builder: (context) {
              final localPath = GetIt.I<DownloadService>().getLocalPosterPath(
                favorite.mediaId,
                favorite.providerId,
              );
              return localPath != null
                  ? Image.file(File(localPath), fit: BoxFit.cover)
                  : favorite.posterUrl != null
                  ? CachedNetworkImage(
                      imageUrl: favorite.posterUrl!,
                      fit: BoxFit.cover,
                      placeholder: (_, __) => Container(
                        color: Theme.of(context).colorScheme.surface,
                      ),
                      errorWidget: (_, __, ___) => _PosterPlaceholder(),
                    )
                  : _PosterPlaceholder();
            },
          ),

          // Gradient overlay
          Positioned(
            bottom: 0,
            left: 0,
            right: 0,
            child: Container(
              padding: const EdgeInsets.all(8),
              decoration: BoxDecoration(
                gradient: LinearGradient(
                  begin: Alignment.topCenter,
                  end: Alignment.bottomCenter,
                  colors: [
                    Colors.transparent,
                    Colors.black.withValues(alpha: 0.8),
                  ],
                ),
              ),
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Text(
                    favorite.title,
                    maxLines: 2,
                    overflow: TextOverflow.ellipsis,
                    style: const TextStyle(
                      color: Colors.white,
                      fontSize: 12,
                      fontWeight: FontWeight.bold,
                    ),
                  ),
                  if (favorite.year != null)
                    Text(
                      '${favorite.year}',
                      style: TextStyle(
                        color: Colors.white.withValues(alpha: 0.7),
                        fontSize: 11,
                      ),
                    ),
                ],
              ),
            ),
          ),

          // Remove button
          Positioned(
            top: 4,
            right: 4,
            child: Material(
              color: Colors.black54,
              borderRadius: BorderRadius.circular(16),
              child: InkWell(
                borderRadius: BorderRadius.circular(16),
                onTap: onRemove,
                child: const Padding(
                  padding: EdgeInsets.all(4),
                  child: Icon(Icons.close, size: 16, color: Colors.white),
                ),
              ),
            ),
          ),
        ],
      ),
    );
  }
}

class _PosterPlaceholder extends StatelessWidget {
  @override
  Widget build(BuildContext context) {
    return Container(
      color: Theme.of(context).colorScheme.surface,
      child: Center(
        child: Icon(
          Icons.movie,
          size: 48,
          color: Theme.of(context).textTheme.bodySmall?.color,
        ),
      ),
    );
  }
}

class _FavoriteListTile extends StatelessWidget {
  final Favorite favorite;
  final VoidCallback onRemove;

  const _FavoriteListTile({required this.favorite, required this.onRemove});

  @override
  Widget build(BuildContext context) {
    return Card(
      margin: EdgeInsets.zero,
      color: Theme.of(context).colorScheme.surface,
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
                child: Builder(
                  builder: (context) {
                    final localPath = GetIt.I<DownloadService>()
                        .getLocalPosterPath(
                          favorite.mediaId,
                          favorite.providerId,
                        );
                    return localPath != null
                        ? Image.file(File(localPath), fit: BoxFit.cover)
                        : favorite.posterUrl != null
                        ? CachedNetworkImage(
                            imageUrl: favorite.posterUrl!,
                            fit: BoxFit.cover,
                            placeholder: (_, __) => _PosterPlaceholder(),
                            errorWidget: (_, __, ___) => _PosterPlaceholder(),
                          )
                        : _PosterPlaceholder();
                  },
                ),
              ),
            ),
            const SizedBox(width: 12),
            // Info
            Expanded(
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Text(
                    favorite.title,
                    maxLines: 2,
                    overflow: TextOverflow.ellipsis,
                    style: const TextStyle(
                      fontWeight: FontWeight.bold,
                      fontSize: 15,
                    ),
                  ),
                  const SizedBox(height: 4),
                  Row(
                    children: [
                      if (favorite.year != null) ...[
                        Icon(
                          Icons.calendar_today,
                          size: 14,
                          color: Theme.of(context).textTheme.bodySmall?.color,
                        ),
                        const SizedBox(width: 4),
                        Text(
                          '${favorite.year}',
                          style: TextStyle(
                            color: Theme.of(context).textTheme.bodySmall?.color,
                            fontSize: 13,
                          ),
                        ),
                        const SizedBox(width: 12),
                      ],
                      Icon(
                        Icons.category,
                        size: 14,
                        color: Theme.of(context).textTheme.bodySmall?.color,
                      ),
                      const SizedBox(width: 4),
                      Text(
                        _getTypeName(favorite.mediaType),
                        style: TextStyle(
                          color: Theme.of(context).textTheme.bodySmall?.color,
                          fontSize: 13,
                        ),
                      ),
                    ],
                  ),
                ],
              ),
            ),
            // Remove button
            IconButton(
              icon: Icon(
                Icons.delete_outline,
                color: Theme.of(context).textTheme.bodySmall?.color,
              ),
              onPressed: onRemove,
              tooltip: 'Видалити',
            ),
          ],
        ),
      ),
    );
  }

  String _getTypeName(String? type) {
    switch (type) {
      case 'movie':
        return 'Фільм';
      case 'series':
        return 'Серіал';
      case 'cartoon':
        return 'Мультфільм';
      case 'anime':
        return 'Аніме';
      default:
        return 'Відео';
    }
  }
}

class _FavoriteCompactTile extends StatelessWidget {
  final Favorite favorite;
  final VoidCallback onRemove;

  const _FavoriteCompactTile({required this.favorite, required this.onRemove});

  @override
  Widget build(BuildContext context) {
    return ListTile(
      contentPadding: const EdgeInsets.symmetric(horizontal: 12, vertical: 4),
      leading: ClipRRect(
        borderRadius: BorderRadius.circular(4),
        child: SizedBox(
          width: 40,
          height: 60,
          child: favorite.posterUrl != null
              ? CachedNetworkImage(
                  imageUrl: favorite.posterUrl!,
                  fit: BoxFit.cover,
                  placeholder: (_, __) =>
                      Container(color: Theme.of(context).colorScheme.surface),
                  errorWidget: (_, __, ___) =>
                      Container(color: Theme.of(context).colorScheme.surface),
                )
              : Container(color: Theme.of(context).colorScheme.surface),
        ),
      ),
      title: Text(
        favorite.title,
        maxLines: 1,
        overflow: TextOverflow.ellipsis,
        style: const TextStyle(fontSize: 14),
      ),
      subtitle: favorite.year != null
          ? Text('${favorite.year}', style: TextStyle(fontSize: 12))
          : null,
      trailing: IconButton(
        icon: const Icon(Icons.close, size: 18),
        onPressed: onRemove,
        padding: EdgeInsets.zero,
        constraints: const BoxConstraints(minWidth: 32, minHeight: 32),
      ),
    );
  }
}

class _EmptyState extends StatelessWidget {
  final IconData icon;
  final String title;
  final String subtitle;

  const _EmptyState({
    required this.icon,
    required this.title,
    required this.subtitle,
  });

  @override
  Widget build(BuildContext context) {
    return Center(
      child: Column(
        mainAxisAlignment: MainAxisAlignment.center,
        children: [
          Icon(
            icon,
            size: 80,
            color: Theme.of(context).textTheme.bodySmall?.color,
          ),
          const SizedBox(height: 16),
          Text(
            title,
            style: const TextStyle(fontSize: 18, fontWeight: FontWeight.bold),
          ),
          const SizedBox(height: 8),
          Text(
            subtitle,
            style: TextStyle(
              color: Theme.of(context).textTheme.bodySmall?.color,
            ),
          ),
        ],
      ),
    );
  }
}
