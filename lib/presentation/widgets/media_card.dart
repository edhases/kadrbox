import 'dart:io';
import 'dart:ui';
import 'package:flutter/material.dart';
import 'package:cached_network_image/cached_network_image.dart';
import 'package:get_it/get_it.dart';

import 'package:flutter/services.dart';
import '../../data/services/settings_service.dart';
import '../../data/services/download_service.dart';
import '../../domain/entities/entities.dart';
import 'common/skeleton.dart';

/// Media item card for grids and lists
class MediaCard extends StatefulWidget {
  final MediaItem item;
  final VoidCallback? onTap;
  final bool isFocused; // Deprecated but kept for compatibility
  /// Namespace для Hero тегу — дозволяє мати один item у різних місцях UI
  /// без конфлікту Hero анімацій (напр. 'grid', 'recommendations', 'banner')
  final String heroNamespace;

  const MediaCard({
    super.key,
    required this.item,
    this.onTap,
    this.isFocused = false,
    this.heroNamespace = 'grid',
  });

  @override
  State<MediaCard> createState() => _MediaCardState();
}

class _MediaCardState extends State<MediaCard> {
  final _settings = GetIt.instance<SettingsService>();
  final _focusNode = FocusNode();
  bool _isHovered = false;
  bool _hasFocus = false;

  UISettings get _ui => _settings.uiSettings;
  Color get _accentColor => Color(_ui.accentColor.colorValue);

  @override
  void initState() {
    super.initState();
    _focusNode.addListener(_onFocusChange);
  }

  @override
  void dispose() {
    _focusNode.removeListener(_onFocusChange);
    _focusNode.dispose();
    super.dispose();
  }

  void _onFocusChange() {
    if (mounted) setState(() => _hasFocus = _focusNode.hasFocus);
  }

  @override
  Widget build(BuildContext context) {
    final isFocused = _hasFocus || _isHovered || widget.isFocused;
    final animationsEnabled = _ui.animationsEnabled;

    return Focus(
      focusNode: _focusNode,
      onKeyEvent: (node, event) {
        if (event is KeyDownEvent &&
            (event.logicalKey == LogicalKeyboardKey.select ||
                event.logicalKey == LogicalKeyboardKey.enter)) {
          widget.onTap?.call();
          return KeyEventResult.handled;
        }
        return KeyEventResult.ignored;
      },
      child: MouseRegion(
        onEnter: (_) => setState(() => _isHovered = true),
        onExit: (_) => setState(() => _isHovered = false),
        child: GestureDetector(
          onTap: () {
            HapticFeedback.selectionClick();
            widget.onTap?.call();
          },
          child: AnimatedContainer(
            duration: animationsEnabled
                ? const Duration(milliseconds: 200)
                : Duration.zero,
            curve: Curves.easeOutCubic,
            transform: isFocused && animationsEnabled
                ? (Matrix4.identity()
                    ..setEntry(0, 0, 1.05)
                    ..setEntry(1, 1, 1.05))
                : Matrix4.identity(),
            transformAlignment: Alignment.center,
            decoration: BoxDecoration(
              borderRadius: BorderRadius.circular(_ui.posterSize.borderRadius),
              boxShadow: isFocused
                  ? [
                      BoxShadow(
                        color: _accentColor.withValues(alpha: 0.4),
                        blurRadius: 16,
                        spreadRadius: 2,
                      ),
                    ]
                  : null,
            ),
            child: ClipRRect(
              borderRadius: BorderRadius.circular(_ui.posterSize.borderRadius),
              child: Stack(
                fit: StackFit.expand,
                children: [
                  // Poster image (cover fit)
                  _buildPoster(),

                  // Gradient overlay (always on bottom for readability)
                  _buildGradientOverlay(),

                  // Content info (title + year • genre / rating)
                  _buildInfoOverlay(),

                  // Provider badge (top-left)
                  _buildProviderBadge(),

                  // Type badge (below provider)
                  if (widget.item.type != ContentType.unknown)
                    _buildTypeBadge(),

                  // Rating badge (top-right)
                  if (widget.item.rating != null && _ui.showRatings)
                    _buildRatingBadge(),

                  // Year badge (only if overlay style is hidden and year exists)
                  if (_ui.cardInfoStyle == CardInfoStyle.hidden &&
                      widget.item.year != null &&
                      _ui.showYears)
                    _buildYearBadge(),

                  // Focus border
                  if (isFocused) _buildFocusBorder(),
                ],
              ),
            ),
          ),
        ),
      ),
    );
  }

  Widget _buildPoster() {
    final localPath = GetIt.I<DownloadService>().getLocalPosterPath(
      widget.item.id,
      widget.item.providerId,
    );

    return Hero(
      tag:
          'media_poster_${widget.heroNamespace}_${widget.item.providerId}_${widget.item.id}',
      child: localPath != null
          ? Image.file(
              File(localPath),
              fit: BoxFit.cover,
              cacheWidth: 400,
              gaplessPlayback: true,
            )
          : widget.item.posterUrl != null
          ? CachedNetworkImage(
              imageUrl: widget.item.posterUrl!,
              fit: BoxFit.cover,
              memCacheHeight: 400,
              maxWidthDiskCache: 400,
              placeholder: (context, url) => const Skeleton(
                width: double.infinity,
                height: double.infinity,
                borderRadius: 0,
              ),
              errorWidget: (context, url, error) => _buildPlaceholder(),
            )
          : _buildPlaceholder(),
    );
  }

  Widget _buildPlaceholder() {
    return Container(
      color: Theme.of(context).cardColor,
      child: Column(
        mainAxisAlignment: MainAxisAlignment.center,
        children: [
          Icon(
            _getTypeIcon(),
            size: 48,
            color: Theme.of(context).textTheme.bodySmall?.color,
          ),
          const SizedBox(height: 8),
          Padding(
            padding: const EdgeInsets.symmetric(horizontal: 8),
            child: Text(
              widget.item.title,
              textAlign: TextAlign.center,
              maxLines: 2,
              overflow: TextOverflow.ellipsis,
              style: TextStyle(
                color: Theme.of(context).textTheme.bodyMedium?.color,
                fontSize: 12,
              ),
            ),
          ),
        ],
      ),
    );
  }

  IconData _getTypeIcon() {
    switch (widget.item.type) {
      case ContentType.movie:
        return Icons.movie_outlined;
      case ContentType.series:
        return Icons.tv_outlined;
      case ContentType.cartoon:
        return Icons.animation_outlined;
      case ContentType.anime:
        return Icons.animation;
      default:
        return Icons.video_library_outlined;
    }
  }

  Widget _buildGradientOverlay() {
    if (_ui.cardInfoStyle == CardInfoStyle.hidden)
      return const SizedBox.shrink();

    return Positioned(
      left: 0,
      right: 0,
      bottom: 0,
      height: 96,
      child: Container(
        decoration: BoxDecoration(
          gradient: LinearGradient(
            begin: Alignment.topCenter,
            end: Alignment.bottomCenter,
            colors: [
              Colors.transparent,
              Colors.black.withValues(alpha: 0.3),
              Colors.black.withValues(alpha: 0.92),
            ],
            stops: const [0.0, 0.4, 1.0],
          ),
        ),
      ),
    );
  }

  Widget _buildInfoOverlay() {
    if (_ui.cardInfoStyle == CardInfoStyle.hidden)
      return const SizedBox.shrink();

    final parts = <String>[];
    if (widget.item.year != null && _ui.showYears) {
      parts.add('${widget.item.year}');
    }
    if (widget.item.genres != null && widget.item.genres!.isNotEmpty) {
      parts.add(widget.item.genres!.first);
    } else if (widget.item.type != ContentType.unknown) {
      parts.add(widget.item.type.displayName);
    }

    final subtitle = parts.join(' • ');

    return Positioned(
      left: 8,
      right: 8,
      bottom: 8,
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        mainAxisSize: MainAxisSize.min,
        children: [
          // 1st line: Title (1 line max or up to 2 if short)
          Text(
            widget.item.title,
            maxLines: subtitle.isNotEmpty ? 1 : 2,
            overflow: TextOverflow.ellipsis,
            style: const TextStyle(
              color: Colors.white,
              fontSize: 13,
              fontWeight: FontWeight.w600,
              height: 1.2,
            ),
          ),
          if (subtitle.isNotEmpty) ...[
            const SizedBox(height: 2),
            // 2nd line: Year • Genre
            Text(
              subtitle,
              maxLines: 1,
              overflow: TextOverflow.ellipsis,
              style: TextStyle(
                color: Colors.white.withValues(alpha: 0.72),
                fontSize: 11,
                fontWeight: FontWeight.w400,
              ),
            ),
          ],
        ],
      ),
    );
  }

  Widget _buildRatingBadge() {
    final rawRating = widget.item.rating!;
    if (rawRating < 0) return const SizedBox.shrink();

    return Positioned(
      top: 8,
      right: 8,
      child: ClipRRect(
        borderRadius: BorderRadius.circular(6),
        child: BackdropFilter(
          filter: ImageFilter.blur(sigmaX: 8, sigmaY: 8),
          child: Container(
            padding: const EdgeInsets.symmetric(horizontal: 6, vertical: 3),
            decoration: BoxDecoration(
              color: Colors.black.withValues(alpha: 0.55),
              borderRadius: BorderRadius.circular(6),
              border: Border.all(color: Colors.white.withValues(alpha: 0.15)),
            ),
            child: Row(
              mainAxisSize: MainAxisSize.min,
              children: [
                const Icon(Icons.star_rounded, size: 12, color: Colors.amber),
                const SizedBox(width: 3),
                Text(
                  rawRating > 10
                      ? (rawRating / 10).toStringAsFixed(1)
                      : rawRating.toStringAsFixed(1),
                  style: const TextStyle(
                    color: Colors.white,
                    fontSize: 10,
                    fontWeight: FontWeight.bold,
                  ),
                ),
              ],
            ),
          ),
        ),
      ),
    );
  }

  Widget _buildProviderBadge() {
    return Positioned(
      top: 8,
      left: 8,
      child: ClipRRect(
        borderRadius: BorderRadius.circular(6),
        child: BackdropFilter(
          filter: ImageFilter.blur(sigmaX: 8, sigmaY: 8),
          child: Container(
            padding: const EdgeInsets.symmetric(horizontal: 6, vertical: 3),
            decoration: BoxDecoration(
              color: Colors.black.withValues(alpha: 0.55),
              borderRadius: BorderRadius.circular(6),
              border: Border.all(color: Colors.white.withValues(alpha: 0.15)),
            ),
            child: Text(
              widget.item.providerId.toUpperCase(),
              style: const TextStyle(
                color: Colors.white,
                fontSize: 9,
                fontWeight: FontWeight.bold,
                letterSpacing: 0.4,
              ),
            ),
          ),
        ),
      ),
    );
  }

  Widget _buildTypeBadge() {
    final typeIcon = _getTypeIcon();

    return Positioned(
      top: 32,
      left: 8,
      child: ClipRRect(
        borderRadius: BorderRadius.circular(6),
        child: BackdropFilter(
          filter: ImageFilter.blur(sigmaX: 8, sigmaY: 8),
          child: Container(
            padding: const EdgeInsets.symmetric(horizontal: 5, vertical: 3),
            decoration: BoxDecoration(
              color: Colors.black.withValues(alpha: 0.55),
              borderRadius: BorderRadius.circular(6),
              border: Border.all(color: Colors.white.withValues(alpha: 0.15)),
            ),
            child: Row(
              mainAxisSize: MainAxisSize.min,
              children: [
                Icon(typeIcon, size: 10, color: Colors.white70),
                const SizedBox(width: 3),
                Text(
                  widget.item.type.shortName,
                  style: const TextStyle(
                    color: Colors.white,
                    fontSize: 8,
                    fontWeight: FontWeight.bold,
                  ),
                ),
              ],
            ),
          ),
        ),
      ),
    );
  }

  Widget _buildYearBadge() {
    return Positioned(
      bottom: 8,
      left: 8,
      child: ClipRRect(
        borderRadius: BorderRadius.circular(6),
        child: BackdropFilter(
          filter: ImageFilter.blur(sigmaX: 8, sigmaY: 8),
          child: Container(
            padding: const EdgeInsets.symmetric(horizontal: 6, vertical: 3),
            decoration: BoxDecoration(
              color: Colors.black.withValues(alpha: 0.55),
              borderRadius: BorderRadius.circular(6),
              border: Border.all(color: Colors.white.withValues(alpha: 0.15)),
            ),
            child: Text(
              '${widget.item.year}',
              style: const TextStyle(
                color: Colors.white,
                fontSize: 10,
                fontWeight: FontWeight.w500,
              ),
            ),
          ),
        ),
      ),
    );
  }

  Widget _buildFocusBorder() {
    return Positioned.fill(
      child: Container(
        decoration: BoxDecoration(
          borderRadius: BorderRadius.circular(_ui.posterSize.borderRadius),
          border: Border.all(color: _accentColor, width: 3),
        ),
      ),
    );
  }
}
