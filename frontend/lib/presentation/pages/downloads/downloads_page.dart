import 'dart:io';

import 'package:flutter/material.dart';
import 'package:flutter/foundation.dart';
import 'package:get_it/get_it.dart';
import 'package:go_router/go_router.dart';
import 'package:cached_network_image/cached_network_image.dart';

import '../../../core/l10n/app_strings.dart';
import '../../../domain/entities/entities.dart';
import '../../../data/database/app_database.dart';
import '../../../data/services/download_service.dart';
import '../../theme/app_theme.dart';
import '../../widgets/custom_titlebar.dart';

/// Downloads page for offline content
class DownloadsPage extends StatefulWidget {
  const DownloadsPage({super.key});

  @override
  State<DownloadsPage> createState() => _DownloadsPageState();
}

class _DownloadsPageState extends State<DownloadsPage>
    with SingleTickerProviderStateMixin {
  final _downloadService = GetIt.instance<DownloadService>();
  late TabController _tabController;

  @override
  void initState() {
    super.initState();
    _tabController = TabController(length: 2, vsync: this);
    _downloadService.addListener(_onUpdate);
  }

  @override
  void dispose() {
    _tabController.dispose();
    _downloadService.removeListener(_onUpdate);
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
    final s = AppStrings.of(
      context,
    ); // Moved up to be accessible for the 'canDownload' check

    if (!_downloadService.canDownload) {
      return Scaffold(
        body: Column(
          children: [
            if (_isDesktop) const CustomTitleBar(),
            _buildAppBar(s), // Pass 's' here
            Expanded(
              child: Center(
                child: Column(
                  mainAxisAlignment: MainAxisAlignment.center,
                  children: [
                    const Icon(Icons.cloud_off, size: 64, color: Colors.grey),
                    const SizedBox(height: 16),
                    Text(
                      s.downloadsUnavailable, // Use the new string
                      style: Theme.of(context).textTheme.headlineSmall,
                    ),
                    const SizedBox(height: 8),
                    Text(s.downloadsUnavailableDesc), // Use the new string
                  ],
                ),
              ),
            ),
          ],
        ),
      );
    }

    return Scaffold(
      body: Column(
        children: [
          if (_isDesktop) const CustomTitleBar(),
          _buildAppBar(s),
          TabBar(
            controller: _tabController,
            tabs: [
              Tab(text: s.downloaded),
              Tab(text: s.inQueue),
            ],
            indicatorColor: AppTheme.primaryColor,
            labelColor: AppTheme.primaryColor,
            unselectedLabelColor: AppTheme.textSecondary,
          ),
          Expanded(
            child: TabBarView(
              controller: _tabController,
              children: [_buildCompletedList(s), _buildActiveList(s)],
            ),
          ),
        ],
      ),
    );
  }

  Widget _buildAppBar(AppStrings s) {
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
            tooltip: s.back,
          ),
          const SizedBox(width: 8),
          const Icon(Icons.download_done),
          const SizedBox(width: 8),
          Text(
            s.downloads,
            style: const TextStyle(fontSize: 20, fontWeight: FontWeight.bold),
          ),
          const Spacer(),
          FutureBuilder<int>(
            future: _downloadService.getTotalSize(),
            builder: (context, snapshot) {
              final size = snapshot.data ?? 0;
              return Text(
                _downloadService.formatSize(size),
                style: TextStyle(color: AppTheme.textSecondary),
              );
            },
          ),
          if (_downloadService.completed.isNotEmpty) ...[
            const SizedBox(width: 8),
            IconButton(
              icon: const Icon(Icons.delete_sweep),
              onPressed: () => _showClearDialog(s),
              tooltip: s.clearAll,
            ),
          ],
        ],
      ),
    );
  }

  Widget _buildCompletedList(AppStrings s) {
    final items = _downloadService.completed;

    if (items.isEmpty) {
      return _buildEmptyState(
        icon: Icons.download_done,
        title: s.noDownloads,
        subtitle: s.noDownloadsDesc,
      );
    }

    return ListView.builder(
      padding: const EdgeInsets.all(16),
      itemCount: items.length,
      itemBuilder: (context, index) {
        final item = items[index];
        return _DownloadCard(
          download: item,
          s: s,
          formatSize: _downloadService.formatSize,
          onPlay: () => _playDownload(item, s),
          onDelete: () => _deleteDownload(item, s),
        );
      },
    );
  }

  Widget _buildActiveList(AppStrings s) {
    final items = _downloadService.active;

    if (items.isEmpty) {
      return _buildEmptyState(
        icon: Icons.cloud_download,
        title: s.noActiveDownloads,
        subtitle: s.noActiveDownloadsDesc,
      );
    }

    return ListView.builder(
      padding: const EdgeInsets.all(16),
      itemCount: items.length,
      itemBuilder: (context, index) {
        final item = items[index];
        return _ActiveDownloadCard(
          download: item,
          s: s,
          formatSize: _downloadService.formatSize,
          onPause: item.status == DownloadStatus.downloading
              ? () => _downloadService.pauseDownload(item.id)
              : null,
          onResume: item.status == DownloadStatus.paused
              ? () => _downloadService.resumeDownload(item.id)
              : null,
          onCancel: () => _downloadService.cancelDownload(item.id),
        );
      },
    );
  }

  Widget _buildEmptyState({
    required IconData icon,
    required String title,
    required String subtitle,
  }) {
    return Center(
      child: Column(
        mainAxisAlignment: MainAxisAlignment.center,
        children: [
          Icon(icon, size: 64, color: Colors.grey),
          const SizedBox(height: 16),
          Text(title, style: Theme.of(context).textTheme.headlineSmall),
          const SizedBox(height: 8),
          Text(
            subtitle,
            style: Theme.of(
              context,
            ).textTheme.bodyMedium?.copyWith(color: AppTheme.textSecondary),
          ),
        ],
      ),
    );
  }

  void _playDownload(Download download, AppStrings s) {
    // Check if file exists
    final file = File(download.localPath);
    if (!file.existsSync()) {
      ScaffoldMessenger.of(
        context,
      ).showSnackBar(SnackBar(content: Text(s.fileNotFound)));
      return;
    }

    String title = download.title;
    if (download.season != null && download.episode != null) {
      title += ' - S${download.season}E${download.episode}';
    }

    final mediaType = ContentType.values.firstWhere(
      (e) => e.name == download.mediaType,
      orElse: () => ContentType.unknown,
    );

    context.push(
      '/player',
      extra: {
        'url': download.localPath,
        'title': title,
        'subtitle': download.voiceover ?? download.quality,
        'mediaId': download.mediaId,
        'providerId': download.providerId,
        'posterUrl': download.posterUrl,
        'mediaType': mediaType,
        'season': download.season,
        'episode': download.episode,
        'episodeTitle': download.episodeTitle,
        'isOffline': true,
      },
    );
  }

  void _deleteDownload(Download download, AppStrings s) {
    showDialog(
      context: context,
      builder: (context) => AlertDialog(
        backgroundColor: AppTheme.surfaceColor,
        title: Text(s.deleteDownload),
        content: Text('${s.confirmDelete} "${download.title}"?'),
        actions: [
          TextButton(
            onPressed: () => Navigator.pop(context),
            child: Text(s.cancel),
          ),
          TextButton(
            onPressed: () {
              Navigator.pop(context);
              _downloadService.deleteDownload(download.id);
            },
            child: Text(s.delete, style: TextStyle(color: AppTheme.errorColor)),
          ),
        ],
      ),
    );
  }

  void _showClearDialog(AppStrings s) {
    showDialog(
      context: context,
      builder: (context) => AlertDialog(
        backgroundColor: AppTheme.surfaceColor,
        title: Text(s.clearAllDownloads),
        content: Text(s.clearAllDownloadsConfirm),
        actions: [
          TextButton(
            onPressed: () => Navigator.pop(context),
            child: Text(s.cancel),
          ),
          TextButton(
            onPressed: () {
              Navigator.pop(context);
              _downloadService.clearAll();
            },
            child: Text(s.clear, style: TextStyle(color: AppTheme.errorColor)),
          ),
        ],
      ),
    );
  }
}

/// Card for completed downloads
class _DownloadCard extends StatelessWidget {
  final Download download;
  final AppStrings s;
  final String Function(int) formatSize;
  final VoidCallback onPlay;
  final VoidCallback onDelete;

  const _DownloadCard({
    required this.download,
    required this.s,
    required this.formatSize,
    required this.onPlay,
    required this.onDelete,
  });

  @override
  Widget build(BuildContext context) {
    return Card(
      margin: const EdgeInsets.only(bottom: 12),
      color: AppTheme.surfaceColor,
      shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(12)),
      child: InkWell(
        onTap: onPlay,
        borderRadius: BorderRadius.circular(12),
        child: Padding(
          padding: const EdgeInsets.all(12),
          child: Row(
            children: [
              // Poster
              ClipRRect(
                borderRadius: BorderRadius.circular(8),
                child:
                    download.localPosterPath != null &&
                        File(download.localPosterPath!).existsSync()
                    ? Image.file(
                        File(download.localPosterPath!),
                        width: 60,
                        height: 90,
                        fit: BoxFit.cover,
                      )
                    : download.posterUrl != null
                    ? CachedNetworkImage(
                        imageUrl: download.posterUrl!,
                        width: 60,
                        height: 90,
                        fit: BoxFit.cover,
                        errorWidget: (_, __, ___) => _buildPosterPlaceholder(),
                      )
                    : _buildPosterPlaceholder(),
              ),
              const SizedBox(width: 12),

              // Info
              Expanded(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Text(
                      download.title,
                      style: const TextStyle(fontWeight: FontWeight.bold),
                      maxLines: 2,
                      overflow: TextOverflow.ellipsis,
                    ),
                    if (download.season != null && download.episode != null)
                      Text(
                        '${s.seasonLabel} ${download.season}, ${s.episodeLabel} ${download.episode}',
                        style: TextStyle(
                          color: AppTheme.textSecondary,
                          fontSize: 12,
                        ),
                      ),
                    const SizedBox(height: 4),
                    Row(
                      children: [
                        _InfoChip(
                          icon: Icons.high_quality,
                          label: download.quality,
                        ),
                        const SizedBox(width: 8),
                        _InfoChip(
                          icon: Icons.storage,
                          label: formatSize(
                            download.fileSizeBytes > 0
                                ? download.fileSizeBytes
                                : download.downloadedBytes,
                          ),
                        ),
                        if (download.duration != null &&
                            download.duration! > 0) ...[
                          const SizedBox(width: 8),
                          _InfoChip(
                            icon: Icons.timer,
                            label: _formatDuration(download.duration!),
                          ),
                        ],
                      ],
                    ),
                  ],
                ),
              ),

              // Actions
              Column(
                children: [
                  IconButton(
                    icon: const Icon(Icons.play_circle_filled, size: 40),
                    color: AppTheme.primaryColor,
                    onPressed: onPlay,
                  ),
                  IconButton(
                    icon: const Icon(Icons.delete_outline),
                    onPressed: onDelete,
                    color: AppTheme.textMuted,
                  ),
                ],
              ),
            ],
          ),
        ),
      ),
    );
  }

  Widget _buildPosterPlaceholder() {
    return Container(
      width: 60,
      height: 90,
      color: AppTheme.darkCard,
      child: const Icon(Icons.movie, size: 32),
    );
  }

  String _formatDuration(int seconds) {
    final duration = Duration(seconds: seconds);
    final hours = duration.inHours;
    final minutes = duration.inMinutes.remainder(60);
    if (hours > 0) {
      return '$hoursгод $minutesхв';
    }
    return '$minutesхв';
  }
}

/// Card for active downloads
class _ActiveDownloadCard extends StatelessWidget {
  final Download download;
  final AppStrings s;
  final String Function(int) formatSize;
  final VoidCallback? onPause;
  final VoidCallback? onResume;
  final VoidCallback onCancel;

  const _ActiveDownloadCard({
    required this.download,
    required this.s,
    required this.formatSize,
    this.onPause,
    this.onResume,
    required this.onCancel,
  });

  @override
  Widget build(BuildContext context) {
    final isPaused = download.status == DownloadStatus.paused;
    final isFailed = download.status == DownloadStatus.failed;

    return Card(
      margin: const EdgeInsets.only(bottom: 12),
      color: AppTheme.surfaceColor,
      shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(12)),
      child: Padding(
        padding: const EdgeInsets.all(12),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Row(
              children: [
                // Poster
                ClipRRect(
                  borderRadius: BorderRadius.circular(8),
                  child:
                      download.localPosterPath != null &&
                          File(download.localPosterPath!).existsSync()
                      ? Image.file(
                          File(download.localPosterPath!),
                          width: 50,
                          height: 75,
                          fit: BoxFit.cover,
                        )
                      : download.posterUrl != null
                      ? CachedNetworkImage(
                          imageUrl: download.posterUrl!,
                          width: 50,
                          height: 75,
                          fit: BoxFit.cover,
                          errorWidget: (_, __, ___) => Container(
                            width: 50,
                            height: 75,
                            color: AppTheme.darkCard,
                            child: const Icon(Icons.movie),
                          ),
                        )
                      : Container(
                          width: 50,
                          height: 75,
                          color: AppTheme.darkCard,
                          child: const Icon(Icons.movie),
                        ),
                ),
                const SizedBox(width: 12),

                // Info
                Expanded(
                  child: Column(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      Text(
                        download.title,
                        style: const TextStyle(fontWeight: FontWeight.bold),
                        maxLines: 1,
                        overflow: TextOverflow.ellipsis,
                      ),
                      const SizedBox(height: 4),
                      Text(
                        _getStatusText(),
                        style: TextStyle(
                          color: isFailed
                              ? AppTheme.errorColor
                              : AppTheme.textMuted,
                          fontSize: 12,
                        ),
                      ),
                    ],
                  ),
                ),

                // Actions
                if (isPaused && onResume != null)
                  IconButton(
                    icon: const Icon(Icons.play_arrow),
                    onPressed: onResume,
                  )
                else if (!isFailed && onPause != null)
                  IconButton(icon: const Icon(Icons.pause), onPressed: onPause),
                IconButton(
                  icon: const Icon(Icons.close),
                  onPressed: onCancel,
                  color: AppTheme.textMuted,
                ),
              ],
            ),
            const SizedBox(height: 8),

            // Progress bar
            ClipRRect(
              borderRadius: BorderRadius.circular(4),
              child: LinearProgressIndicator(
                value: download.progress,
                minHeight: 6,
                backgroundColor: AppTheme.darkCard,
                color: isFailed
                    ? AppTheme.errorColor
                    : isPaused
                    ? Colors.orange
                    : AppTheme.primaryColor,
              ),
            ),
            const SizedBox(height: 8),

            // Progress text
            Row(
              mainAxisAlignment: MainAxisAlignment.spaceBetween,
              children: [
                Text(
                  '${(download.progress * 100).toStringAsFixed(1)}%',
                  style: TextStyle(fontSize: 12, color: AppTheme.textMuted),
                ),
                Builder(
                  builder: (context) {
                    final ds = GetIt.I<DownloadService>();
                    final speed = ds.getDownloadSpeed(download.id);
                    final remaining = ds.getRemainingTime(download.id);
                    return Text(
                      '${formatSize(download.downloadedBytes)} / ${formatSize(download.fileSizeBytes > 0 ? download.fileSizeBytes : download.downloadedBytes)}${speed.isNotEmpty ? " ($speed)${remaining.isNotEmpty ? " • $remaining" : ""}" : ""}',
                      style: TextStyle(fontSize: 12, color: AppTheme.textMuted),
                    );
                  },
                ),
              ],
            ),
          ],
        ),
      ),
    );
  }

  String _getStatusText() {
    switch (download.status) {
      case DownloadStatus.pending:
        return s.pending;
      case DownloadStatus.downloading:
        return s.downloadingStatus;
      case DownloadStatus.paused:
        return s.pausedStatus;
      case DownloadStatus.failed:
        return s.failedStatus;
      case DownloadStatus.completed:
        return s.completedStatus;
    }
  }
}

/// Info chip widget
class _InfoChip extends StatelessWidget {
  final IconData icon;
  final String label;

  const _InfoChip({required this.icon, required this.label});

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 6, vertical: 2),
      decoration: BoxDecoration(
        color: AppTheme.darkCard,
        borderRadius: BorderRadius.circular(4),
      ),
      child: Row(
        mainAxisSize: MainAxisSize.min,
        children: [
          Icon(icon, size: 12, color: AppTheme.textMuted),
          const SizedBox(width: 4),
          Text(
            label,
            style: TextStyle(fontSize: 11, color: AppTheme.textMuted),
          ),
        ],
      ),
    );
  }
}
