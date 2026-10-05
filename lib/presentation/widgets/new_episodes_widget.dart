import 'package:flutter/material.dart';
import 'package:get_it/get_it.dart';
import 'package:go_router/go_router.dart';

import '../../core/utils/image_headers.dart';
import 'package:cached_network_image/cached_network_image.dart';

import '../../data/services/episode_update_service.dart';
import '../../data/services/settings_service.dart';
import '../theme/app_theme.dart';

/// Badge showing new episodes count
class NewEpisodesBadge extends StatefulWidget {
  final Widget child;

  const NewEpisodesBadge({super.key, required this.child});

  @override
  State<NewEpisodesBadge> createState() => _NewEpisodesBadgeState();
}

class _NewEpisodesBadgeState extends State<NewEpisodesBadge> {
  final _updateService = GetIt.instance<EpisodeUpdateService>();

  @override
  void initState() {
    super.initState();
    _updateService.addListener(_onUpdate);
  }

  @override
  void dispose() {
    _updateService.removeListener(_onUpdate);
    super.dispose();
  }

  void _onUpdate() {
    if (mounted) setState(() {});
  }

  @override
  Widget build(BuildContext context) {
    final count = _updateService.newEpisodesCount;

    return Badge(
      isLabelVisible: count > 0,
      label: Text(count > 9 ? '9+' : count.toString()),
      child: widget.child,
    );
  }
}

/// Button to show new episodes sheet
class NewEpisodesButton extends StatefulWidget {
  const NewEpisodesButton({super.key});

  @override
  State<NewEpisodesButton> createState() => _NewEpisodesButtonState();
}

class _NewEpisodesButtonState extends State<NewEpisodesButton> {
  final _updateService = GetIt.instance<EpisodeUpdateService>();
  final _settings = GetIt.instance<SettingsService>();

  @override
  void initState() {
    super.initState();
    _updateService.addListener(_onUpdate);
  }

  @override
  void dispose() {
    _updateService.removeListener(_onUpdate);
    super.dispose();
  }

  void _onUpdate() {
    if (mounted) setState(() {});
  }

  @override
  Widget build(BuildContext context) {
    final hasNew = _updateService.hasNewEpisodes;
    final accentColor = Color(_settings.uiSettings.accentColor.colorValue);

    return IconButton(
      icon: Badge(
        isLabelVisible: hasNew,
        smallSize: 8,
        child: Icon(
          hasNew ? Icons.notifications_active : Icons.notifications_outlined,
          color: hasNew ? accentColor : null,
        ),
      ),
      onPressed: () => _showNewEpisodesSheet(context),
      tooltip: hasNew
          ? 'Р СњР С•Р Р†РЎвЂ“ РЎРѓР ВµРЎР‚РЎвЂ“РЎвЂ”'
          : 'Р С›Р Р…Р С•Р Р†Р В»Р ВµР Р…Р Р…РЎРЏ',
    );
  }

  void _showNewEpisodesSheet(BuildContext context) {
    showModalBottomSheet(
      context: context,
      isScrollControlled: true,
      backgroundColor: Colors.transparent,
      builder: (context) => const _NewEpisodesSheet(),
    );
  }
}

class _NewEpisodesSheet extends StatefulWidget {
  const _NewEpisodesSheet();

  @override
  State<_NewEpisodesSheet> createState() => _NewEpisodesSheetState();
}

class _NewEpisodesSheetState extends State<_NewEpisodesSheet> {
  final _updateService = GetIt.instance<EpisodeUpdateService>();
  final _settings = GetIt.instance<SettingsService>();

  @override
  void initState() {
    super.initState();
    _updateService.addListener(_onUpdate);
  }

  @override
  void dispose() {
    _updateService.removeListener(_onUpdate);
    super.dispose();
  }

  void _onUpdate() {
    if (mounted) setState(() {});
  }

  Color get _accentColor => Color(_settings.uiSettings.accentColor.colorValue);

  @override
  Widget build(BuildContext context) {
    return Container(
      constraints: BoxConstraints(
        maxHeight: MediaQuery.of(context).size.height * 0.7,
      ),
      decoration: const BoxDecoration(
        color: AppTheme.darkSurface,
        borderRadius: BorderRadius.vertical(top: Radius.circular(20)),
      ),
      child: Column(
        mainAxisSize: MainAxisSize.min,
        children: [
          // Handle
          Container(
            margin: const EdgeInsets.symmetric(vertical: 12),
            width: 40,
            height: 4,
            decoration: BoxDecoration(
              color: AppTheme.textMuted,
              borderRadius: BorderRadius.circular(2),
            ),
          ),

          // Header
          Padding(
            padding: const EdgeInsets.symmetric(horizontal: 16),
            child: Row(
              children: [
                Icon(Icons.notifications_active, color: _accentColor),
                const SizedBox(width: 12),
                const Expanded(
                  child: Text(
                    'Р СњР С•Р Р†РЎвЂ“ РЎРѓР ВµРЎР‚РЎвЂ“РЎвЂ”',
                    style: TextStyle(fontSize: 18, fontWeight: FontWeight.bold),
                  ),
                ),
                if (_updateService.isChecking)
                  const SizedBox(
                    width: 20,
                    height: 20,
                    child: CircularProgressIndicator(strokeWidth: 2),
                  )
                else
                  IconButton(
                    icon: const Icon(Icons.refresh),
                    onPressed: () =>
                        _updateService.checkForUpdates(force: true),
                    tooltip:
                        'Р СџР ВµРЎР‚Р ВµР Р†РЎвЂ“РЎР‚Р С‘РЎвЂљР С‘ Р С•Р Р…Р С•Р Р†Р В»Р ВµР Р…Р Р…РЎРЏ',
                  ),
              ],
            ),
          ),

          // Last check info
          Padding(
            padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 4),
            child: Row(
              children: [
                Icon(Icons.schedule, size: 14, color: AppTheme.textMuted),
                const SizedBox(width: 6),
                Text(
                  'Р С›РЎРѓРЎвЂљР В°Р Р…Р Р…РЎРЏ Р С—Р ВµРЎР‚Р ВµР Р†РЎвЂ“РЎР‚Р С”Р В°: ${_updateService.lastCheckFormatted}',
                  style: const TextStyle(
                    fontSize: 12,
                    color: AppTheme.textMuted,
                  ),
                ),
              ],
            ),
          ),

          const Divider(),

          // Content
          Expanded(child: _buildContent()),
        ],
      ),
    );
  }

  Widget _buildContent() {
    if (_updateService.error != null) {
      return Center(
        child: Column(
          mainAxisAlignment: MainAxisAlignment.center,
          children: [
            const Icon(
              Icons.error_outline,
              size: 48,
              color: AppTheme.errorColor,
            ),
            const SizedBox(height: 12),
            Text(
              'Р СџР С•Р СР С‘Р В»Р С”Р В° Р С—Р ВµРЎР‚Р ВµР Р†РЎвЂ“РЎР‚Р С”Р С‘',
              style: TextStyle(color: AppTheme.textPrimary),
            ),
            const SizedBox(height: 4),
            Text(
              _updateService.error!,
              style: const TextStyle(color: AppTheme.textMuted, fontSize: 12),
              textAlign: TextAlign.center,
            ),
            const SizedBox(height: 16),
            ElevatedButton.icon(
              onPressed: () => _updateService.checkForUpdates(force: true),
              icon: const Icon(Icons.refresh),
              label: const Text(
                'Р РЋР С—РЎР‚Р С•Р В±РЎС“Р Р†Р В°РЎвЂљР С‘ Р В·Р Р…Р С•Р Р†РЎС“',
              ),
            ),
          ],
        ),
      );
    }

    final episodes = _updateService.newEpisodes;

    if (episodes.isEmpty) {
      return Center(
        child: Column(
          mainAxisAlignment: MainAxisAlignment.center,
          children: [
            Icon(Icons.check_circle_outline, size: 64, color: _accentColor),
            const SizedBox(height: 16),
            const Text(
              'Р вЂ™РЎРѓР Вµ Р С—Р ВµРЎР‚Р ВµР С–Р В»РЎРЏР Р…РЎС“РЎвЂљР С•!',
              style: TextStyle(fontSize: 18, fontWeight: FontWeight.w600),
            ),
            const SizedBox(height: 8),
            const Text(
              'Р СњР ВµР СР В°РЎвЂќ Р Р…Р С•Р Р†Р С‘РЎвЂ¦ РЎРѓР ВµРЎР‚РЎвЂ“Р в„– РЎС“ Р Р†Р В°РЎв‚¬Р С‘РЎвЂ¦ РЎС“Р В»РЎР‹Р В±Р В»Р ВµР Р…Р С‘РЎвЂ¦',
              style: TextStyle(color: AppTheme.textMuted),
            ),
            const SizedBox(height: 24),
            OutlinedButton.icon(
              onPressed: () => _updateService.checkForUpdates(force: true),
              icon: const Icon(Icons.refresh),
              label: const Text(
                'Р СџР ВµРЎР‚Р ВµР Р†РЎвЂ“РЎР‚Р С‘РЎвЂљР С‘ Р В·Р В°РЎР‚Р В°Р В·',
              ),
            ),
          ],
        ),
      );
    }

    return Column(
      children: [
        // Clear all button
        if (episodes.length > 1)
          Padding(
            padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 8),
            child: Row(
              mainAxisAlignment: MainAxisAlignment.end,
              children: [
                TextButton.icon(
                  onPressed: () {
                    _updateService.clearAll();
                    Navigator.pop(context);
                  },
                  icon: const Icon(Icons.clear_all, size: 18),
                  label: const Text(
                    'Р С›РЎвЂЎР С‘РЎРѓРЎвЂљР С‘РЎвЂљР С‘ Р Р†РЎРѓР Вµ',
                  ),
                ),
              ],
            ),
          ),

        // Episodes list
        Expanded(
          child: ListView.builder(
            padding: const EdgeInsets.symmetric(horizontal: 16),
            itemCount: episodes.length,
            itemBuilder: (context, index) {
              final episode = episodes[index];
              return _EpisodeCard(
                episode: episode,
                accentColor: _accentColor,
                onTap: () {
                  _updateService.markAsSeen(
                    episode.mediaId,
                    episode.providerId,
                  );
                  Navigator.pop(context);
                  context.push(
                    '/details/${episode.providerId}/${Uri.encodeComponent(episode.mediaId)}',
                  );
                },
                onDismiss: () {
                  _updateService.markAsSeen(
                    episode.mediaId,
                    episode.providerId,
                  );
                },
              );
            },
          ),
        ),
      ],
    );
  }
}

class _EpisodeCard extends StatelessWidget {
  final NewEpisodeInfo episode;
  final Color accentColor;
  final VoidCallback onTap;
  final VoidCallback onDismiss;

  const _EpisodeCard({
    required this.episode,
    required this.accentColor,
    required this.onTap,
    required this.onDismiss,
  });

  @override
  Widget build(BuildContext context) {
    return Dismissible(
      key: Key('${episode.mediaId}:${episode.providerId}'),
      direction: DismissDirection.endToStart,
      onDismissed: (_) => onDismiss(),
      background: Container(
        alignment: Alignment.centerRight,
        padding: const EdgeInsets.only(right: 16),
        color: AppTheme.errorColor.withValues(alpha: 0.2),
        child: const Icon(Icons.check, color: AppTheme.successColor),
      ),
      child: Card(
        margin: const EdgeInsets.only(bottom: 8),
        color: AppTheme.darkCard,
        child: InkWell(
          onTap: onTap,
          borderRadius: BorderRadius.circular(12),
          child: Padding(
            padding: const EdgeInsets.all(12),
            child: Row(
              children: [
                // Poster
                ClipRRect(
                  borderRadius: BorderRadius.circular(8),
                  child: episode.posterUrl != null
                      ? CachedNetworkImage(
                          imageUrl: episode.posterUrl!,
                          cacheManager: posterCacheManager,
                          width: 60,
                          height: 90,
                          fit: BoxFit.cover,
                          placeholder: (_, __) => Container(
                            width: 60,
                            height: 90,
                            color: AppTheme.darkSurface,
                          ),
                          errorWidget: (_, __, ___) => Container(
                            width: 60,
                            height: 90,
                            color: AppTheme.darkSurface,
                            child: const Icon(
                              Icons.movie,
                              color: AppTheme.textMuted,
                            ),
                          ),
                        )
                      : Container(
                          width: 60,
                          height: 90,
                          color: AppTheme.darkSurface,
                          child: const Icon(
                            Icons.movie,
                            color: AppTheme.textMuted,
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
                        episode.title,
                        style: const TextStyle(
                          fontWeight: FontWeight.w600,
                          fontSize: 15,
                        ),
                        maxLines: 2,
                        overflow: TextOverflow.ellipsis,
                      ),
                      const SizedBox(height: 4),
                      Container(
                        padding: const EdgeInsets.symmetric(
                          horizontal: 8,
                          vertical: 2,
                        ),
                        decoration: BoxDecoration(
                          color: accentColor.withValues(alpha: 0.2),
                          borderRadius: BorderRadius.circular(4),
                        ),
                        child: Text(
                          episode.episodeLabel,
                          style: TextStyle(
                            color: accentColor,
                            fontSize: 12,
                            fontWeight: FontWeight.bold,
                          ),
                        ),
                      ),
                      if (episode.episodeTitle != null) ...[
                        const SizedBox(height: 4),
                        Text(
                          episode.episodeTitle!,
                          style: const TextStyle(
                            color: AppTheme.textSecondary,
                            fontSize: 13,
                          ),
                          maxLines: 1,
                          overflow: TextOverflow.ellipsis,
                        ),
                      ],
                    ],
                  ),
                ),

                // Arrow
                const Icon(Icons.chevron_right, color: AppTheme.textMuted),
              ],
            ),
          ),
        ),
      ),
    );
  }
}
