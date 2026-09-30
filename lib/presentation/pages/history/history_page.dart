import 'package:flutter/material.dart';
import 'package:flutter/foundation.dart';
import 'package:get_it/get_it.dart';
import 'package:go_router/go_router.dart';
import 'package:cached_network_image/cached_network_image.dart';
import 'dart:io';

import '../../../data/database/app_database.dart';
import '../../../data/services/history_service.dart';
import '../../../data/services/download_service.dart';
import '../../theme/app_theme.dart';
import '../../widgets/custom_titlebar.dart';
import '../../widgets/tv/focusable_card.dart';

/// History page - displays watch history
class HistoryPage extends StatefulWidget {
  const HistoryPage({super.key});

  @override
  State<HistoryPage> createState() => _HistoryPageState();
}

class _HistoryPageState extends State<HistoryPage> {
  final _historyService = GetIt.instance<HistoryService>();

  @override
  void initState() {
    super.initState();
    _historyService.addListener(_onUpdate);
  }

  @override
  void dispose() {
    _historyService.removeListener(_onUpdate);
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
          Expanded(child: _buildContent()),
        ],
      ),
    );
  }

  Widget _buildAppBar() {
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
            onPressed: () => context.pop(),
            tooltip: 'Назад',
          ),
          const SizedBox(width: 8),
          const Icon(Icons.history),
          const SizedBox(width: 8),
          const Text(
            'Історія переглядів',
            style: TextStyle(fontSize: 20, fontWeight: FontWeight.bold),
          ),
          const Spacer(),
          if (_historyService.history.isNotEmpty)
            IconButton(
              icon: const Icon(Icons.delete_sweep),
              onPressed: _showClearDialog,
              tooltip: 'Очистити історію',
            ),
        ],
      ),
    );
  }

  Widget _buildContent() {
    // Continue watching section
    final continueWatching = _historyService.continueWatching;
    final history = _historyService.history;

    if (history.isEmpty) {
      return _EmptyState(
        icon: Icons.history,
        title: 'Історія порожня',
        subtitle: 'Тут з\'являться переглянуті фільми та серіали',
      );
    }

    return ListView(
      padding: const EdgeInsets.all(16),
      children: [
        // Continue watching section
        if (continueWatching.isNotEmpty) ...[
          _SectionHeader(
            title: 'Продовжити перегляд',
            icon: Icons.play_circle_filled,
          ),
          const SizedBox(height: 8),
          SizedBox(
            height: _isDesktop ? 200 : 160,
            child: ListView.builder(
              scrollDirection: Axis.horizontal,
              itemCount: continueWatching.length,
              itemBuilder: (context, index) {
                final item = continueWatching[index];
                return Padding(
                  padding: EdgeInsets.only(
                    right: index < continueWatching.length - 1 ? 12 : 0,
                  ),
                  child: FocusableCard(
                    onTap: () => _playItem(item),
                    borderRadius: 12,
                    child: _ContinueWatchingCard(
                      item: item,
                      progress: _historyService.getProgress(item),
                      remaining: _historyService.formatRemaining(item),
                    ),
                  ),
                );
              },
            ),
          ),
          const SizedBox(height: 24),
        ],

        // Full history (lazy builder to avoid building 1000+ tiles at once)
        _SectionHeader(title: 'Вся історія', icon: Icons.history),
        const SizedBox(height: 8),
        ListView.builder(
          shrinkWrap: true,
          physics: const NeverScrollableScrollPhysics(),
          itemCount: history.length,
          itemBuilder: (context, index) {
            final item = history[index];
            return Padding(
              padding: const EdgeInsets.only(bottom: 8),
              child: FocusableCard(
                onTap: () => _openDetails(item),
                borderRadius: 8,
                child: _HistoryTile(
                  item: item,
                  onRemove: () => _removeItem(item),
                ),
              ),
            );
          },
        ),
      ],
    );
  }

  void _openDetails(WatchHistoryData item) {
    // Local files don't have details page
    if (item.providerId == 'local') {
      _playItem(item);
      return;
    }

    final encodedId = Uri.encodeComponent(item.mediaId);
    context.push('/details/${item.providerId}/$encodedId');
  }

  void _playItem(WatchHistoryData item) {
    // Check if this is a local file
    if (item.providerId == 'local') {
      // Verify file exists
      final file = File(item.mediaId);
      if (!file.existsSync()) {
        ScaffoldMessenger.of(
          context,
        ).showSnackBar(const SnackBar(content: Text('Файл не знайдено')));
        return;
      }

      context.push(
        '/player',
        extra: {
          'url': 'file://${item.mediaId}',
          'title': item.title,
          'isOffline': true,
        },
      );
    } else if (item.lastStreamUrl != null) {
      context.push(
        '/player',
        extra: {
          'url': item.lastStreamUrl,
          'title': item.title,
          'subtitle': item.episodeTitle,
        },
      );
    } else {
      _openDetails(item);
    }
  }

  void _removeItem(WatchHistoryData item) {
    _historyService.remove(item.mediaId, item.providerId);
    ScaffoldMessenger.of(
      context,
    ).showSnackBar(SnackBar(content: Text('${item.title} видалено з історії')));
  }

  void _showClearDialog() {
    showDialog(
      context: context,
      builder: (context) => AlertDialog(
        backgroundColor: AppTheme.surfaceColor,
        title: const Text('Очистити історію?'),
        content: const Text(
          'Ви впевнені, що хочете видалити всю історію переглядів? '
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
              _historyService.clearAll();
            },
            child: Text(
              'Очистити',
              style: TextStyle(color: AppTheme.errorColor),
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

class _SectionHeader extends StatelessWidget {
  final String title;
  final IconData icon;

  const _SectionHeader({required this.title, required this.icon});

  @override
  Widget build(BuildContext context) {
    return Row(
      children: [
        Icon(icon, size: 20, color: AppTheme.primaryColor),
        const SizedBox(width: 8),
        Text(
          title,
          style: TextStyle(
            fontSize: 16,
            fontWeight: FontWeight.bold,
            color: AppTheme.primaryColor,
          ),
        ),
      ],
    );
  }
}

class _ContinueWatchingCard extends StatelessWidget {
  final WatchHistoryData item;
  final double progress;
  final String remaining;

  const _ContinueWatchingCard({
    required this.item,
    required this.progress,
    required this.remaining,
  });

  @override
  Widget build(BuildContext context) {
    return SizedBox(
      width: 280,
      child: Card(
        clipBehavior: Clip.antiAlias,
        color: AppTheme.surfaceColor,
        shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(12)),
        child: Stack(
          fit: StackFit.expand,
          children: [
            // Background image
            Builder(
              builder: (context) {
                final localPath = GetIt.I<DownloadService>().getLocalPosterPath(
                  item.mediaId,
                  item.providerId,
                );
                return localPath != null
                    ? Image.file(
                        File(localPath),
                        fit: BoxFit.cover,
                        color: Colors.black54,
                        colorBlendMode: BlendMode.darken,
                      )
                    : item.posterUrl != null
                    ? CachedNetworkImage(
                        imageUrl: item.posterUrl!,
                        fit: BoxFit.cover,
                        color: Colors.black54,
                        colorBlendMode: BlendMode.darken,
                      )
                    : Container(color: AppTheme.backgroundColor);
              },
            ),

            // Content
            Padding(
              padding: const EdgeInsets.all(12),
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  // Title
                  Text(
                    item.title,
                    maxLines: 2,
                    overflow: TextOverflow.ellipsis,
                    style: const TextStyle(
                      color: Colors.white,
                      fontSize: 14,
                      fontWeight: FontWeight.bold,
                    ),
                  ),
                  if (item.episodeTitle != null) ...[
                    const SizedBox(height: 4),
                    Text(
                      item.episodeTitle!,
                      maxLines: 1,
                      overflow: TextOverflow.ellipsis,
                      style: TextStyle(
                        color: Colors.white.withValues(alpha: 0.7),
                        fontSize: 12,
                      ),
                    ),
                  ],
                  const Spacer(),
                  // Remaining time
                  Text(
                    remaining,
                    style: TextStyle(
                      color: Colors.white.withValues(alpha: 0.8),
                      fontSize: 11,
                    ),
                  ),
                  const SizedBox(height: 8),
                  // Progress bar
                  ClipRRect(
                    borderRadius: BorderRadius.circular(2),
                    child: LinearProgressIndicator(
                      value: progress,
                      backgroundColor: Colors.white24,
                      valueColor: AlwaysStoppedAnimation(AppTheme.primaryColor),
                      minHeight: 4,
                    ),
                  ),
                ],
              ),
            ),

            // Play button overlay
            Positioned.fill(
              child: Center(
                child: Container(
                  padding: const EdgeInsets.all(12),
                  decoration: BoxDecoration(
                    color: AppTheme.primaryColor,
                    shape: BoxShape.circle,
                  ),
                  child: const Icon(
                    Icons.play_arrow,
                    color: Colors.white,
                    size: 32,
                  ),
                ),
              ),
            ),
          ],
        ),
      ),
    );
  }
}

class _HistoryTile extends StatelessWidget {
  final WatchHistoryData item;
  final VoidCallback onRemove;

  const _HistoryTile({required this.item, required this.onRemove});

  @override
  Widget build(BuildContext context) {
    return Card(
      margin: EdgeInsets.zero,
      color: AppTheme.surfaceColor,
      shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(8)),
      child: Padding(
        padding: const EdgeInsets.all(8),
        child: Row(
          children: [
            // Poster
            ClipRRect(
              borderRadius: BorderRadius.circular(6),
              child: SizedBox(
                width: 60,
                height: 80,
                child: item.providerId == 'local'
                    ? Container(
                        color: AppTheme.backgroundColor,
                        child: Icon(
                          Icons.folder_open,
                          color: AppTheme.primaryColor,
                          size: 32,
                        ),
                      )
                    : Builder(
                        builder: (context) {
                          final localPath = GetIt.I<DownloadService>()
                              .getLocalPosterPath(
                                item.mediaId,
                                item.providerId,
                              );
                          return localPath != null
                              ? Image.file(File(localPath), fit: BoxFit.cover)
                              : item.posterUrl != null
                              ? CachedNetworkImage(
                                  imageUrl: item.posterUrl!,
                                  fit: BoxFit.cover,
                                )
                              : Container(
                                  color: AppTheme.backgroundColor,
                                  child: Icon(
                                    Icons.movie,
                                    color: AppTheme.textMuted,
                                  ),
                                );
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
                    item.title,
                    maxLines: 1,
                    overflow: TextOverflow.ellipsis,
                    style: const TextStyle(fontWeight: FontWeight.bold),
                  ),
                  if (item.episodeTitle != null)
                    Text(
                      item.episodeTitle!,
                      maxLines: 1,
                      overflow: TextOverflow.ellipsis,
                      style: TextStyle(color: AppTheme.textMuted, fontSize: 12),
                    ),
                  const SizedBox(height: 4),
                  Text(
                    _formatDate(item.watchedAt),
                    style: TextStyle(color: AppTheme.textMuted, fontSize: 11),
                  ),
                ],
              ),
            ),
            // Remove button
            IconButton(
              icon: Icon(Icons.close, color: AppTheme.textMuted),
              onPressed: onRemove,
              tooltip: 'Видалити',
            ),
          ],
        ),
      ),
    );
  }

  String _formatDate(DateTime date) {
    final now = DateTime.now();
    final diff = now.difference(date);

    if (diff.inDays == 0) {
      return 'Сьогодні';
    } else if (diff.inDays == 1) {
      return 'Вчора';
    } else if (diff.inDays < 7) {
      return '${diff.inDays} дні тому';
    } else {
      return '${date.day}.${date.month}.${date.year}';
    }
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
          Icon(icon, size: 80, color: AppTheme.textMuted),
          const SizedBox(height: 16),
          Text(
            title,
            style: const TextStyle(fontSize: 18, fontWeight: FontWeight.bold),
          ),
          const SizedBox(height: 8),
          Text(subtitle, style: TextStyle(color: AppTheme.textMuted)),
        ],
      ),
    );
  }
}
