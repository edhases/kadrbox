import 'package:flutter/material.dart';
import '../../../core/l10n/app_strings.dart';
import 'package:flutter/foundation.dart';
import 'package:flutter/services.dart';
import 'package:get_it/get_it.dart';
import 'package:go_router/go_router.dart';
import 'package:cached_network_image/cached_network_image.dart';

import '../../../data/providers/provider_registry.dart';
import '../../../data/services/favorites_service.dart';
import '../../../data/services/download_service.dart';
import '../../../domain/entities/entities.dart';
import '../../theme/app_theme.dart';
import '../../widgets/custom_titlebar.dart';
import '../../widgets/common/skeleton.dart' as custom;
import '../../widgets/tv/focusable_card.dart';
import '../../widgets/rating_badge.dart';
import '../../widgets/details/similar_content_section.dart';

/// Media details page with improved layout
class DetailsPage extends StatefulWidget {
  final String providerId;
  final String mediaId;

  const DetailsPage({
    super.key,
    required this.providerId,
    required this.mediaId,
  });

  @override
  State<DetailsPage> createState() => _DetailsPageState();
}

class _DetailsPageState extends State<DetailsPage> {
  final _registry = GetIt.instance<ProviderRegistry>();
  final _favoritesService = GetIt.instance<FavoritesService>();

  MediaDetails? _details;
  List<StreamSource> _streams = [];
  StreamSource? _selectedStream;
  bool _isLoading = true;
  String? _error;
  bool _isFavorite = false;

  // Series support
  int? _selectedSeason;
  int? _selectedEpisode;
  bool _isLoadingEpisode = false;

  @override
  void initState() {
    super.initState();
    _loadDetails();
    _checkFavorite();
  }

  Future<void> _checkFavorite() async {
    final isFav = _favoritesService.isFavorite(
      widget.mediaId,
      widget.providerId,
    );
    if (mounted) setState(() => _isFavorite = isFav);
  }

  Future<void> _toggleFavorite() async {
    if (_details == null) return;

    try {
      if (_isFavorite) {
        await _favoritesService.remove(widget.mediaId, widget.providerId);
        if (mounted) {
          setState(() => _isFavorite = false);
          ScaffoldMessenger.of(context).showSnackBar(
            const SnackBar(
              content: Text('Видалено з обраного'),
              duration: Duration(seconds: 2),
            ),
          );
        }
      } else {
        await _favoritesService.add(_details!.item);
        if (mounted) {
          setState(() => _isFavorite = true);
          ScaffoldMessenger.of(context).showSnackBar(
            const SnackBar(
              content: Text('Додано до обраного'),
              duration: Duration(seconds: 2),
            ),
          );
        }
      }
    } catch (e) {
      debugPrint('Failed to toggle favorite: $e');
      if (mounted) {
        ScaffoldMessenger.of(
          context,
        ).showSnackBar(SnackBar(content: Text('Помилка: $e')));
      }
    }
  }

  Future<void> _loadDetails() async {
    setState(() {
      _isLoading = true;
      _error = null;
    });

    try {
      final provider = _registry.getById(widget.providerId);
      if (provider == null) {
        throw Exception('Провайдер не знайдено');
      }

      final details = await provider.getDetails(widget.mediaId);
      final streams = await provider.getStreams(widget.mediaId);

      debugPrint('Loaded ${streams.length} streams for ${details.item.title}');
      for (final s in streams) {
        debugPrint(
          '  Stream: ${s.quality.displayName} - ${s.voiceover ?? "default"}',
        );
      }

      if (mounted) {
        setState(() {
          _details = details;
          _streams = streams;
          _selectedStream = streams.isNotEmpty ? streams.first : null;
          _isLoading = false;
        });
      }
    } catch (e) {
      debugPrint('Failed to load details: $e');
      if (mounted) {
        setState(() {
          _error = e.toString();
          _isLoading = false;
        });
      }
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
          Expanded(child: _buildContent()),
        ],
      ),
    );
  }

  Widget _buildContent() {
    if (_isLoading) {
      return const Center(child: CircularProgressIndicator());
    }

    if (_error != null) {
      return _buildError();
    }

    if (_details == null) {
      return const Center(child: Text('Дані не знайдено'));
    }

    // Desktop: horizontal layout (poster left, info right)
    // Mobile: vertical layout
    if (_isDesktop) {
      return _buildDesktopLayout();
    } else {
      return _buildMobileLayout();
    }
  }

  Widget _buildDesktopLayout() {
    return Row(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        // Back button + Poster on left
        SizedBox(
          width: 350,
          child: Column(
            children: [
              // Back row
              Padding(
                padding: const EdgeInsets.all(16),
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
                    const Spacer(),
                  ],
                ),
              ),
              // Poster
              Expanded(
                child: Padding(
                  padding: const EdgeInsets.fromLTRB(16, 0, 16, 16),
                  child: ClipRRect(
                    borderRadius: BorderRadius.circular(12),
                    child: Hero(
                      tag:
                          'media_poster_${widget.providerId}_${widget.mediaId}',
                      child: _buildPoster(),
                    ),
                  ),
                ),
              ),
            ],
          ),
        ),

        // Info on right
        Expanded(
          child: SingleChildScrollView(
            padding: const EdgeInsets.all(24),
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                _buildInfo(),
                const SizedBox(height: 32),
                if (_details != null)
                  SimilarContentSection(
                    providerId: widget.providerId,
                    details: _details!,
                  ),
                const SizedBox(height: 32),
              ],
            ),
          ),
        ),
      ],
    );
  }

  Widget _buildMobileLayout() {
    return CustomScrollView(
      slivers: [
        // App bar with poster
        SliverAppBar(
          expandedHeight: 300,
          pinned: true,
          leading: IconButton(
            icon: const Icon(Icons.arrow_back),
            onPressed: () {
              if (context.canPop()) {
                context.pop();
              } else {
                context.go('/');
              }
            },
          ),
          flexibleSpace: FlexibleSpaceBar(
            title: Text(
              _details!.item.title,
              style: const TextStyle(
                shadows: [Shadow(blurRadius: 8, color: Colors.black)],
              ),
            ),
            background: _buildPosterBackground(),
          ),
        ),
        SliverToBoxAdapter(
          child: Padding(
            padding: const EdgeInsets.all(16),
            child: _buildInfo(),
          ),
        ),
        SliverToBoxAdapter(
          child: Padding(
            padding: const EdgeInsets.fromLTRB(16, 0, 16, 32),
            child: _details != null
                ? SimilarContentSection(
                    providerId: widget.providerId,
                    details: _details!,
                  )
                : const SizedBox.shrink(),
          ),
        ),
      ],
    );
  }

  Widget _buildPoster() {
    if (_details?.item.posterUrl != null) {
      return CachedNetworkImage(
        imageUrl: _details!.item.posterUrl!,
        fit: BoxFit.contain,
        placeholder: (context, url) => const custom.Skeleton(
          width: double.infinity,
          height: double.infinity,
          borderRadius: 0,
        ),
        errorWidget: (context, url, error) => Container(
          color: Theme.of(context).cardColor,
          child: Icon(
            Icons.movie,
            size: 64,
            color: Theme.of(context).textTheme.bodySmall?.color,
          ),
        ),
      );
    }
    return Container(
      color: Theme.of(context).cardColor,
      child: Icon(
        Icons.movie,
        size: 64,
        color: Theme.of(context).textTheme.bodySmall?.color,
      ),
    );
  }

  Widget _buildPosterBackground() {
    if (_details?.item.posterUrl != null) {
      return Stack(
        fit: StackFit.expand,
        children: [
          Hero(
            tag: 'media_poster_${widget.providerId}_${widget.mediaId}',
            child: CachedNetworkImage(
              imageUrl: _details!.item.posterUrl!,
              fit: BoxFit.cover,
              memCacheHeight: 800,
              maxWidthDiskCache: 800,
            ),
          ),
          Container(
            decoration: BoxDecoration(
              gradient: LinearGradient(
                begin: Alignment.topCenter,
                end: Alignment.bottomCenter,
                colors: [
                  Colors.transparent,
                  Colors.black.withValues(alpha: 0.7),
                ],
              ),
            ),
          ),
        ],
      );
    }
    return Container(color: Theme.of(context).cardColor);
  }

  Widget _buildInfo() {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        // Title (desktop only, mobile shows in app bar)
        if (_isDesktop) ...[
          Text(
            _details?.item.title ?? 'Movie Title Placeholder',
            style: Theme.of(context).textTheme.headlineMedium?.copyWith(
              fontWeight: FontWeight.bold,
              color: Theme.of(context).textTheme.titleLarge?.color,
            ),
          ),
          const SizedBox(height: 16),
        ],

        // Info chips (year, rating, genre, etc.)
        _buildInfoRow(),
        const SizedBox(height: 24),

        // Voiceover/Quality selector (if multiple streams)
        if (_streams.length > 1) ...[
          _buildStreamSelector(),
          const SizedBox(height: 16),
        ],

        // Region block warning
        if (_streams.isEmpty && widget.providerId == 'hdrezka') ...[
          Container(
            width: double.infinity,
            padding: const EdgeInsets.all(12),
            decoration: BoxDecoration(
              color: Colors.orange.withValues(alpha: 0.1),
              borderRadius: BorderRadius.circular(12),
              border: Border.all(color: Colors.orange.withValues(alpha: 0.3)),
            ),
            child: Row(
              children: [
                const Icon(Icons.warning_amber_rounded, color: Colors.orange),
                const SizedBox(width: 12),
                Expanded(
                  child: Text(
                    AppStrings.of(context).regionBlocked,
                    style: const TextStyle(color: Colors.orange),
                  ),
                ),
              ],
            ),
          ),
          const SizedBox(height: 24),
        ],

        // Play button + Favorite button + Watch Party
        Row(
          children: [
            Expanded(
              child: ElevatedButton.icon(
                onPressed: _streams.isNotEmpty
                    ? () {
                        HapticFeedback.lightImpact();
                        _playStream();
                      }
                    : null,
                icon: const Icon(Icons.play_arrow),
                label: Text(_streams.isNotEmpty ? 'Дивитися' : 'Немає джерел'),
                style: ElevatedButton.styleFrom(
                  padding: const EdgeInsets.symmetric(vertical: 16),
                  disabledBackgroundColor: Colors.grey[800],
                ),
              ),
            ),
            const SizedBox(width: 12),
            IconButton.filled(
              onPressed: _streams.isNotEmpty
                  ? () {
                      HapticFeedback.lightImpact();
                      _startWatchParty();
                    }
                  : null,
              icon: const Icon(Icons.groups),
              style: IconButton.styleFrom(
                backgroundColor: Theme.of(context).cardColor,
                padding: const EdgeInsets.all(16),
              ),
              tooltip: 'Спільний перегляд',
            ),
            const SizedBox(width: 12),
            IconButton.filled(
              onPressed: () {
                HapticFeedback.lightImpact();
                _toggleFavorite();
              },
              icon: Icon(
                _isFavorite ? Icons.favorite : Icons.favorite_border,
                color: _isFavorite ? Colors.red : null,
              ),
              style: IconButton.styleFrom(
                backgroundColor: Theme.of(context).cardColor,
                padding: const EdgeInsets.all(16),
              ),
              tooltip: _isFavorite
                  ? 'Видалити з обраного'
                  : 'Додати до обраного',
            ),
            const SizedBox(width: 12),
            IconButton.filled(
              onPressed: _streams.isNotEmpty && widget.providerId == 'hdrezka'
                  ? () {
                      HapticFeedback.lightImpact();
                      _downloadContent();
                    }
                  : null,
              icon: const Icon(Icons.download),
              style: IconButton.styleFrom(
                backgroundColor: Theme.of(context).cardColor,
                padding: const EdgeInsets.all(16),
              ),
              tooltip: 'Завантажити',
            ),
          ],
        ),
        const SizedBox(height: 24),

        // Description
        if (_details?.fullDescription != null &&
            _details!.fullDescription!.isNotEmpty) ...[
          Text(
            'Опис',
            style: Theme.of(
              context,
            ).textTheme.titleMedium?.copyWith(fontWeight: FontWeight.bold),
          ),
          const SizedBox(height: 8),
          Text(
            _details!.fullDescription!,
            style: Theme.of(context).textTheme.bodyMedium,
          ),
          const SizedBox(height: 24),
        ],

        // Director
        if (_details?.director != null && _details!.director!.isNotEmpty) ...[
          _buildDetailSection(
            icon: Icons.movie_creation,
            title: 'Режисер',
            content: _details!.director!,
          ),
          const SizedBox(height: 16),
        ],

        // All Genres
        if (_details?.genres != null && _details!.genres!.isNotEmpty) ...[
          Text(
            'Жанри',
            style: Theme.of(
              context,
            ).textTheme.titleMedium?.copyWith(fontWeight: FontWeight.bold),
          ),
          const SizedBox(height: 8),
          Wrap(
            spacing: 8,
            runSpacing: 8,
            children: _details!.genres!.map((genre) {
              return Chip(
                avatar: const Icon(Icons.category, size: 16),
                label: Text(genre),
                backgroundColor: Theme.of(
                  context,
                ).colorScheme.primary.withValues(alpha: 0.15),
                side: BorderSide(
                  color: Theme.of(
                    context,
                  ).colorScheme.primary.withValues(alpha: 0.3),
                ),
              );
            }).toList(),
          ),
          const SizedBox(height: 16),
        ],

        // Cast
        if (_details?.actors != null && _details!.actors!.isNotEmpty) ...[
          Text(
            'Актори',
            style: Theme.of(
              context,
            ).textTheme.titleMedium?.copyWith(fontWeight: FontWeight.bold),
          ),
          const SizedBox(height: 8),
          Wrap(
            spacing: 8,
            runSpacing: 8,
            children: _details!.actors!.take(10).map((actor) {
              return Chip(
                avatar: const Icon(Icons.person, size: 16),
                label: Text(actor),
                backgroundColor: Colors.purple.withValues(alpha: 0.15),
                side: BorderSide(color: Colors.purple.withValues(alpha: 0.3)),
              );
            }).toList(),
          ),
          const SizedBox(height: 24),
        ],

        // Seasons & Episodes (for series)
        if (_details?.isSeries == true) ...[
          _buildSeasonsSection(),
          const SizedBox(height: 24),
        ],

        // Note: Stream list is already handled by _buildStreamSelector() above
        // which provides voiceover/quality dropdown. No need to show flat list.
      ],
    );
  }

  Widget _buildStreamSelector() {
    // Check if this is a series with episodes
    final hasEpisodes = _streams.any((s) => s.episode != null);

    if (hasEpisodes) {
      return _buildSeriesStreamSelector();
    }

    // For movies - simple voiceover/quality selector
    return _buildMovieStreamSelector();
  }

  Widget _buildSeriesStreamSelector() {
    // Group streams by voiceover
    final voiceovers = <String, List<StreamSource>>{};
    for (final stream in _streams) {
      final key = stream.voiceover ?? 'Оригінал';
      voiceovers.putIfAbsent(key, () => []).add(stream);
    }

    final currentVoiceover =
        _selectedStream?.voiceover ?? voiceovers.keys.first;

    // Get episodes for current voiceover, sorted
    final episodesForVoiceover = (voiceovers[currentVoiceover] ?? [])
      ..sort((a, b) => (a.episode ?? 0).compareTo(b.episode ?? 0));

    return Container(
      padding: const EdgeInsets.all(12),
      decoration: BoxDecoration(
        color: AppTheme.darkCard,
        borderRadius: BorderRadius.circular(12),
        border: Border.all(color: AppTheme.borderColor),
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Row(
            children: [
              Icon(Icons.settings, size: 18, color: Colors.grey[400]),
              const SizedBox(width: 8),
              Text(
                'Налаштування відтворення',
                style: TextStyle(
                  color: Colors.grey[400],
                  fontSize: 12,
                  fontWeight: FontWeight.w500,
                ),
              ),
            ],
          ),
          const SizedBox(height: 12),

          // Voiceover dropdown
          if (voiceovers.length > 1) ...[
            Row(
              children: [
                const Icon(Icons.record_voice_over, size: 20),
                const SizedBox(width: 12),
                Expanded(
                  child: DropdownButtonFormField<String>(
                    initialValue: currentVoiceover,
                    decoration: const InputDecoration(
                      labelText: 'Озвучка',
                      isDense: true,
                      border: OutlineInputBorder(),
                    ),
                    items: voiceovers.keys.map((v) {
                      final count = voiceovers[v]!.length;
                      return DropdownMenuItem(
                        value: v,
                        child: Text(
                          '$v ($count серій)',
                          overflow: TextOverflow.ellipsis,
                        ),
                      );
                    }).toList(),
                    onChanged: (value) {
                      if (value != null) {
                        final streams = voiceovers[value]!;
                        // Sort by episode number
                        streams.sort(
                          (a, b) => (a.episode ?? 0).compareTo(b.episode ?? 0),
                        );
                        setState(() {
                          _selectedStream = streams.first;
                        });
                      }
                    },
                  ),
                ),
              ],
            ),
            const SizedBox(height: 12),
          ],

          // Episode selector label
          Row(
            children: [
              const Icon(Icons.movie, size: 20),
              const SizedBox(width: 12),
              Text(
                'Серії (${episodesForVoiceover.length})',
                style: const TextStyle(
                  fontSize: 14,
                  fontWeight: FontWeight.w500,
                ),
              ),
            ],
          ),
          const SizedBox(height: 8),

          // Episodes grid
          Wrap(
            spacing: 8,
            runSpacing: 8,
            children: episodesForVoiceover.map((stream) {
              final isSelected = _selectedStream == stream;
              final episodeNum = stream.episode ?? 0;
              return FocusableCard(
                onTap: () {
                  setState(() {
                    _selectedStream = stream;
                  });
                },
                borderRadius: 8,
                child: Container(
                  padding: const EdgeInsets.symmetric(
                    horizontal: 12,
                    vertical: 8,
                  ),
                  decoration: BoxDecoration(
                    color: isSelected
                        ? Theme.of(context).colorScheme.primary
                        : Theme.of(context).colorScheme.surface,
                    borderRadius: BorderRadius.circular(8),
                    border: Border.all(
                      color: isSelected
                          ? Theme.of(context).colorScheme.primary
                          : Theme.of(context).dividerColor,
                    ),
                  ),
                  child: Text(
                    episodeNum > 0
                        ? '$episodeNum'
                        : (stream.episodeTitle ?? '?'),
                    style: TextStyle(
                      color: isSelected ? Colors.white : Colors.grey[300],
                      fontWeight: isSelected
                          ? FontWeight.bold
                          : FontWeight.normal,
                    ),
                  ),
                ),
              );
            }).toList(),
          ),
        ],
      ),
    );
  }

  Widget _buildMovieStreamSelector() {
    // Group streams by voiceover
    final voiceovers = <String, List<StreamSource>>{};
    for (final stream in _streams) {
      final key = stream.voiceover ?? 'Оригінал';
      voiceovers.putIfAbsent(key, () => []).add(stream);
    }

    // Get unique qualities for current voiceover (by quality name)
    final currentVoiceover =
        _selectedStream?.voiceover ?? voiceovers.keys.first;
    final streamsForVoiceover = voiceovers[currentVoiceover] ?? _streams;

    // Get unique qualities only
    final uniqueQualities = <String, StreamSource>{};
    for (final s in streamsForVoiceover) {
      uniqueQualities.putIfAbsent(s.quality.displayName, () => s);
    }
    final availableQualities = uniqueQualities.values.toList();

    return Container(
      padding: const EdgeInsets.all(12),
      decoration: BoxDecoration(
        color: Theme.of(context).cardColor,
        borderRadius: BorderRadius.circular(12),
        border: Border.all(color: Theme.of(context).dividerColor),
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Row(
            children: [
              Icon(Icons.settings, size: 18, color: Colors.grey[400]),
              const SizedBox(width: 8),
              Text(
                'Налаштування відтворення',
                style: TextStyle(
                  color: Colors.grey[400],
                  fontSize: 12,
                  fontWeight: FontWeight.w500,
                ),
              ),
            ],
          ),
          const SizedBox(height: 12),

          // Voiceover dropdown
          if (voiceovers.length > 1) ...[
            Row(
              children: [
                const Icon(Icons.record_voice_over, size: 20),
                const SizedBox(width: 12),
                Expanded(
                  child: DropdownButtonFormField<String>(
                    initialValue: currentVoiceover,
                    decoration: const InputDecoration(
                      labelText: 'Озвучка',
                      isDense: true,
                      border: OutlineInputBorder(),
                    ),
                    items: voiceovers.keys.map((v) {
                      return DropdownMenuItem(
                        value: v,
                        child: Text(v, overflow: TextOverflow.ellipsis),
                      );
                    }).toList(),
                    onChanged: (value) {
                      if (value != null) {
                        final streams = voiceovers[value]!;
                        setState(() {
                          _selectedStream = streams.first;
                        });
                      }
                    },
                  ),
                ),
              ],
            ),
            const SizedBox(height: 12),
          ],

          // Quality dropdown
          if (availableQualities.length > 1)
            Row(
              children: [
                const Icon(Icons.high_quality, size: 20),
                const SizedBox(width: 12),
                Expanded(
                  child: DropdownButtonFormField<StreamSource>(
                    initialValue: _selectedStream,
                    decoration: const InputDecoration(
                      labelText: 'Якість',
                      isDense: true,
                      border: OutlineInputBorder(),
                    ),
                    items: availableQualities.map((s) {
                      return DropdownMenuItem(
                        value: s,
                        child: Text(s.quality.displayName),
                      );
                    }).toList(),
                    onChanged: (value) {
                      if (value != null) {
                        setState(() {
                          _selectedStream = value;
                        });
                      }
                    },
                  ),
                ),
              ],
            ),
        ],
      ),
    );
  }

  Widget _buildInfoRow() {
    final items = <Widget>[];

    // Safety check for skeleton loading
    if (_details == null) {
      // Return a skeleton-friendly placeholder structure
      return Wrap(
        spacing: 12,
        runSpacing: 8,
        children: [
          _buildInfoChip(Icons.movie, 'Movie Type'),
          _buildInfoChip(Icons.calendar_today, '2024'),
          _buildInfoChip(Icons.star, '8.5'),
          _buildInfoChip(Icons.access_time, '2h 30m'),
        ],
      );
    }

    // Content type
    items.add(
      _buildInfoChip(
        _getTypeIcon(_details!.item.type),
        _details!.item.type.displayName,
        color: _getTypeColor(_details!.item.type),
      ),
    );

    // Year
    if (_details!.item.year != null) {
      items.add(_buildInfoChip(Icons.calendar_today, '${_details!.item.year}'));
    }

    // Rating
    if (_details!.item.rating != null && _details!.item.rating! >= 0) {
      items.add(
        RatingBadge(
          rating: _details!.item.rating,
          source: _details!.item.ratingSource,
          color: _getTypeColor(_details!.item.type) == AppTheme.textSecondary
              ? null
              : null, // Let RatingBadge decide color based on score
        ),
      );
    }

    // Duration
    if (_details!.duration != null) {
      final dur = _details!.duration!;
      final hours = dur.inHours;
      final minutes = dur.inMinutes.remainder(60);
      final durText = hours > 0 ? '$hoursг $minutesхв' : '$minutesхв';
      items.add(_buildInfoChip(Icons.access_time, durText));
    }

    // Genres (show up to 2)
    if (_details!.genres != null && _details!.genres!.isNotEmpty) {
      final genresText = _details!.genres!.take(2).join(', ');
      items.add(_buildInfoChip(Icons.category, genresText));
    }

    // Country (first one)
    if (_details!.countries != null && _details!.countries!.isNotEmpty) {
      items.add(_buildInfoChip(Icons.public, _details!.countries!.first));
    }

    return Wrap(spacing: 12, runSpacing: 8, children: items);
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
      default:
        return AppTheme.textSecondary;
    }
  }

  Widget _buildInfoChip(IconData icon, String text, {Color? color}) {
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 6),
      decoration: BoxDecoration(
        color: color?.withValues(alpha: 0.15) ?? AppTheme.darkCard,
        borderRadius: BorderRadius.circular(20),
        border: Border.all(
          color: color?.withValues(alpha: 0.3) ?? AppTheme.darkBorder,
        ),
      ),
      child: Row(
        mainAxisSize: MainAxisSize.min,
        children: [
          Icon(icon, size: 16, color: color ?? AppTheme.textSecondary),
          const SizedBox(width: 6),
          Text(text, style: TextStyle(color: color ?? AppTheme.textSecondary)),
        ],
      ),
    );
  }

  Widget _buildDetailSection({
    required IconData icon,
    required String title,
    required String content,
  }) {
    return Row(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Icon(icon, size: 20, color: AppTheme.primaryColor),
        const SizedBox(width: 12),
        Expanded(
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Text(
                title,
                style: TextStyle(color: AppTheme.textSecondary, fontSize: 12),
              ),
              const SizedBox(height: 4),
              Text(
                content,
                style: const TextStyle(fontWeight: FontWeight.w500),
              ),
            ],
          ),
        ),
      ],
    );
  }

  Widget _buildStreamTile(StreamSource stream) {
    return ListTile(
      leading: Icon(
        stream.type == StreamType.hls
            ? Icons.stream
            : stream.type == StreamType.torrent
            ? Icons.download
            : Icons.play_circle,
        color: AppTheme.primaryColor,
      ),
      title: Text(stream.quality.displayName),
      subtitle: stream.voiceover != null ? Text(stream.voiceover!) : null,
      trailing: const Icon(Icons.chevron_right),
      onTap: () => _playStream(stream: stream),
    );
  }

  Widget _buildError() {
    return Center(
      child: Column(
        mainAxisAlignment: MainAxisAlignment.center,
        children: [
          const Icon(Icons.error_outline, size: 64, color: Colors.red),
          const SizedBox(height: 16),
          Text('Помилка: $_error'),
          const SizedBox(height: 16),
          ElevatedButton(
            onPressed: _loadDetails,
            child: const Text('Спробувати знову'),
          ),
          const SizedBox(height: 8),
          TextButton(
            onPressed: () {
              if (context.canPop()) {
                context.pop();
              } else {
                context.go('/');
              }
            },
            child: const Text('Назад'),
          ),
        ],
      ),
    );
  }

  Future<void> _downloadContent() async {
    if (_streams.isEmpty || _details == null) return;

    final stream = _selectedStream ?? _streams.first;

    try {
      final downloadService = GetIt.I<DownloadService>();

      // For series, we need to pass season/episode
      await downloadService.downloadContent(
        item: _details!.item,
        source: stream,
        season: _selectedSeason,
        episode: _selectedEpisode,
        episodeTitle: _details!.seasons != null && _selectedSeason != null
            ? _details!.seasons!
                  .firstWhere(
                    (s) => s.number == _selectedSeason,
                    orElse: () => _details!.seasons!.first,
                  )
                  .episodes
                  .firstWhere(
                    (e) => e.number == _selectedEpisode,
                    orElse: () => _details!.seasons!.first.episodes.first,
                  )
                  .title
            : null,
        duration: _details!.duration?.inSeconds,
      );

      if (mounted) {
        ScaffoldMessenger.of(context).showSnackBar(
          const SnackBar(
            content: Text('Завантаження розпочато'),
            duration: Duration(seconds: 2),
          ),
        );
      }
    } catch (e) {
      if (mounted) {
        ScaffoldMessenger.of(context).showSnackBar(
          SnackBar(
            content: Text('Помилка завантаження: $e'),
            backgroundColor: Colors.red,
          ),
        );
      }
    }
  }

  void _playStream({StreamSource? stream}) {
    final source = stream ?? _selectedStream ?? _streams.first;
    final title = _details?.item.title ?? '';
    final subtitle = source.voiceover ?? source.quality.displayName;

    context.push(
      '/player',
      extra: {
        'url': source.url,
        'title': title,
        'subtitle': subtitle,
        'streams': _streams,
        'mediaId': widget.mediaId,
        'providerId': widget.providerId,
        'posterUrl': _details?.item.posterUrl,
        'mediaType': _details?.item.type,
        'season': source.season ?? _selectedSeason,
        'episode': source.episode ?? _selectedEpisode,
        'episodeTitle': source.episodeTitle,
      },
    );
  }

  void _startWatchParty() {
    if (_streams.isEmpty) return;

    final source = _streams.first;
    final title = _details?.item.title ?? 'Медіа';

    context.push(
      '/watch-party',
      extra: {'mediaUrl': source.url, 'mediaTitle': title},
    );
  }

  Widget _buildSeasonsSection() {
    if (_details == null || _details!.seasons == null) {
      return const SizedBox.shrink();
    }
    final seasons = _details!.seasons!;

    // Auto-select first season if not selected
    _selectedSeason ??= seasons.first.number;

    final currentSeason = seasons.firstWhere(
      (s) => s.number == _selectedSeason,
      orElse: () => seasons.first,
    );

    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Text(
          'Сезони та серії',
          style: Theme.of(
            context,
          ).textTheme.titleMedium?.copyWith(fontWeight: FontWeight.bold),
        ),
        const SizedBox(height: 12),

        // Season selector
        SingleChildScrollView(
          scrollDirection: Axis.horizontal,
          child: Row(
            children: seasons.map((season) {
              final isSelected = season.number == _selectedSeason;
              return Padding(
                padding: const EdgeInsets.only(right: 8),
                child: ChoiceChip(
                  label: Text('Сезон ${season.number}'),
                  selected: isSelected,
                  selectedColor: AppTheme.primaryColor,
                  onSelected: (_) => _selectSeason(season.number),
                ),
              );
            }).toList(),
          ),
        ),
        const SizedBox(height: 16),

        // Episodes grid
        if (currentSeason.episodes.isNotEmpty)
          GridView.builder(
            shrinkWrap: true,
            physics: const NeverScrollableScrollPhysics(),
            gridDelegate: const SliverGridDelegateWithMaxCrossAxisExtent(
              maxCrossAxisExtent: 80,
              childAspectRatio: 1.5,
              mainAxisSpacing: 8,
              crossAxisSpacing: 8,
            ),
            itemCount: currentSeason.episodes.length,
            itemBuilder: (context, index) {
              final episode = currentSeason.episodes[index];
              final isSelected =
                  _selectedEpisode == episode.number &&
                  _selectedSeason == currentSeason.number;
              final isLoading = isSelected && _isLoadingEpisode;

              return Material(
                color: isSelected ? AppTheme.primaryColor : AppTheme.darkCard,
                borderRadius: BorderRadius.circular(8),
                child: InkWell(
                  borderRadius: BorderRadius.circular(8),
                  onTap: isLoading
                      ? null
                      : () => _selectEpisode(
                          currentSeason.number,
                          episode.number,
                        ),
                  child: Center(
                    child: isLoading
                        ? const SizedBox(
                            width: 20,
                            height: 20,
                            child: CircularProgressIndicator(
                              strokeWidth: 2,
                              color: Colors.white,
                            ),
                          )
                        : Text(
                            '${episode.number}',
                            style: TextStyle(
                              fontWeight: isSelected
                                  ? FontWeight.bold
                                  : FontWeight.normal,
                            ),
                          ),
                  ),
                ),
              );
            },
          ),

        // Episode streams
        if (_selectedEpisode != null && _streams.isNotEmpty) ...[
          const SizedBox(height: 16),
          Text(
            'Серія $_selectedEpisode - доступні джерела',
            style: Theme.of(
              context,
            ).textTheme.bodyMedium?.copyWith(fontWeight: FontWeight.bold),
          ),
          const SizedBox(height: 8),
          ..._streams.take(5).map((stream) => _buildStreamTile(stream)),
        ],
      ],
    );
  }

  void _selectSeason(int season) {
    setState(() {
      _selectedSeason = season;
      _selectedEpisode = null;
      _streams = [];
    });
  }

  Future<void> _selectEpisode(int season, int episode) async {
    setState(() {
      _selectedSeason = season;
      _selectedEpisode = episode;
      _isLoadingEpisode = true;
      _streams = [];
    });

    try {
      final provider = _registry.getById(widget.providerId);
      if (provider == null) {
        throw Exception('Провайдер ${widget.providerId} не знайдено');
      }

      final streams = await provider.getStreams(
        widget.mediaId,
        season: season,
        episode: episode,
      );

      if (mounted) {
        setState(() {
          _streams = streams;
        });

        // Auto-play if streams found
        if (streams.isNotEmpty) {
          _playEpisode(season, episode, streams.first);
        }
      }
    } catch (e) {
      debugPrint('Failed to load episode streams: $e');
      if (mounted) {
        ScaffoldMessenger.of(
          context,
        ).showSnackBar(SnackBar(content: Text('Помилка завантаження: $e')));
      }
    } finally {
      if (mounted) {
        setState(() => _isLoadingEpisode = false);
      }
    }
  }

  void _playEpisode(int season, int episode, StreamSource stream) {
    final title = _details?.item.title ?? '';
    final episodeTitle = 'S${season}E$episode';

    context.push(
      '/player',
      extra: {
        'url': stream.url,
        'title': '$title - $episodeTitle',
        'subtitle': stream.voiceover ?? stream.quality.displayName,
        'streams': _streams,
        'mediaId': widget.mediaId,
        'providerId': widget.providerId,
        'posterUrl': _details?.item.posterUrl,
        'season': season,
        'episode': episode,
      },
    );
  }
}
