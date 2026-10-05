import 'package:flutter/material.dart';
import 'package:get_it/get_it.dart';
import 'package:go_router/go_router.dart';
import 'package:cached_network_image/cached_network_image.dart';

import '../../../data/services/history_service.dart';
import '../../../data/database/app_database.dart'; // For WatchHistoryData
import '../../../domain/entities/entities.dart';

// Removed unused skeleton import

/// Section that displays "Continue Watching" items from history
class ContinueWatchingSection extends StatefulWidget {
  const ContinueWatchingSection({super.key});

  @override
  State<ContinueWatchingSection> createState() =>
      _ContinueWatchingSectionState();
}

class _ContinueWatchingSectionState extends State<ContinueWatchingSection> {
  final _historyService = GetIt.instance<HistoryService>();
  final _scrollController = ScrollController();

  @override
  void initState() {
    super.initState();
    _historyService.addListener(_update);
  }

  @override
  void dispose() {
    _historyService.removeListener(_update);
    _scrollController.dispose();
    super.dispose();
  }

  void _update() {
    if (mounted) setState(() {});
  }

  @override
  Widget build(BuildContext context) {
    // List is already filtered by progress in HistoryService/DAO
    final items = _historyService.continueWatching;

    if (items.isEmpty) return const SizedBox.shrink();

    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Padding(
          padding: const EdgeInsets.fromLTRB(16, 4, 16, 12),
          child: Text(
            'Продовжити перегляд',
            style: Theme.of(
              context,
            ).textTheme.titleMedium?.copyWith(fontWeight: FontWeight.bold),
          ),
        ),
        SizedBox(
          height: 220,
          child: ScrollConfiguration(
            behavior: ScrollConfiguration.of(
              context,
            ).copyWith(scrollbars: false),
            child: ListView.separated(
              controller: _scrollController,
              padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 4),
              scrollDirection: Axis.horizontal,
              itemCount: items.length,
              separatorBuilder: (context, index) => const SizedBox(width: 12),
              itemBuilder: (context, index) {
                final item = items[index];
                return _ContinueWatchingCard(item: item);
              },
            ),
          ),
        ),
      ],
    );
  }
}

class _ContinueWatchingCard extends StatelessWidget {
  final WatchHistoryData item;

  const _ContinueWatchingCard({required this.item});

  @override
  Widget build(BuildContext context) {
    final progress = item.durationMs > 0
        ? (item.positionMs / item.durationMs).clamp(0.0, 1.0)
        : 0.0;

    return SizedBox(
      width: 220,
      child: Card(
        clipBehavior: Clip.antiAlias,
        shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(12)),
        child: InkWell(
          borderRadius: BorderRadius.circular(12),
          onTap: () {
            final isLocal = item.providerId == 'local';
            context.push(
              '/player',
              extra: {
                'url': isLocal
                    ? 'file://${item.mediaId}'
                    : (item.lastStreamUrl ?? ''),
                'mediaId': item.mediaId,
                'providerId': item.providerId,
                'title': item.title,
                'posterUrl': item.posterUrl,
                'mediaType': _mapMediaType(item.mediaType),
                'initialSeason': item.season,
                'initialEpisode': item.episode,
                'initialEpisodeTitle': item.episodeTitle,
                'isOffline': isLocal,
              },
            );
          },
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              // 16:9 thumbnail with progress bar flushed to bottom edge
              AspectRatio(
                aspectRatio: 16 / 9,
                child: Stack(
                  fit: StackFit.expand,
                  children: [
                    if (item.posterUrl != null && item.posterUrl!.isNotEmpty)
                      CachedNetworkImage(
                        imageUrl: item.posterUrl!,
                        fit: BoxFit.cover,
                        errorWidget: (_, __, ___) => Container(
                          color: Colors.grey[850],
                          child: const Icon(
                            Icons.movie_outlined,
                            color: Colors.white24,
                          ),
                        ),
                      )
                    else
                      Container(
                        color: Colors.grey[850],
                        child: const Icon(
                          Icons.movie_outlined,
                          color: Colors.white24,
                        ),
                      ),

                    // Subtle play icon
                    Center(
                      child: Container(
                        decoration: BoxDecoration(
                          color: Colors.black.withOpacity(0.30),
                          shape: BoxShape.circle,
                        ),
                        padding: const EdgeInsets.all(8),
                        child: const Icon(
                          Icons.play_arrow,
                          color: Colors.white,
                          size: 20,
                        ),
                      ),
                    ),

                    // Progress bar flush to bottom
                    Positioned(
                      left: 0,
                      right: 0,
                      bottom: 0,
                      child: LinearProgressIndicator(
                        value: progress,
                        backgroundColor: Colors.white.withOpacity(0.15),
                        color: Theme.of(context).colorScheme.primary,
                        minHeight: 3,
                      ),
                    ),
                  ],
                ),
              ),

              // Text info below the thumbnail
              Padding(
                padding: const EdgeInsets.fromLTRB(10, 8, 10, 10),
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Text(
                      item.title,
                      maxLines: 2,
                      overflow: TextOverflow.ellipsis,
                      style: const TextStyle(
                        fontWeight: FontWeight.w500,
                        fontSize: 13,
                        color: Colors.white,
                        height: 1.3,
                      ),
                    ),
                    const SizedBox(height: 4),
                    if (item.season != null && item.episode != null)
                      Text(
                        'S${item.season} E${item.episode}',
                        style: const TextStyle(
                          color: Color(0xFF8E8E93),
                          fontSize: 12,
                        ),
                      )
                    else
                      Text(
                        _formatRemaining(item.durationMs - item.positionMs),
                        style: const TextStyle(
                          color: Color(0xFF8E8E93),
                          fontSize: 12,
                        ),
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

  String _formatRemaining(int ms) {
    if (ms <= 0) return 'Завершено';
    final duration = Duration(milliseconds: ms);
    if (duration.inMinutes < 60) {
      return '${duration.inMinutes} хв';
    } else {
      return '${duration.inHours} год ${duration.inMinutes % 60} хв';
    }
  }

  ContentType _mapMediaType(String type) {
    // Basic mapping, fallback to movie
    return ContentType.values.firstWhere(
      (e) => e.name == type,
      orElse: () => ContentType.movie,
    );
  }
}
