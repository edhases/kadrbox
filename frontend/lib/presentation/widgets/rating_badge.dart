import 'package:flutter/material.dart';

/// Rating badge showing a rating from a specific source (IMDb, TMDB, Сайт, etc.)
class RatingBadge extends StatelessWidget {
  final double? rating;
  final String? source;
  final bool compact;
  final Color? color;

  const RatingBadge({
    super.key,
    this.rating,
    this.source,
    this.compact = false,
    this.color,
  });

  @override
  Widget build(BuildContext context) {
    if (rating == null || rating! < 0) {
      return const SizedBox.shrink();
    }

    // Normalize: if > 10, treat as percentage
    final displayRating = rating! > 10
        ? (rating! / 10).clamp(0.0, 10.0)
        : rating!;

    final badgeColor = color ?? _getRatingColor(context, displayRating);
    final sourceLabel = source;

    // Use InfoChip-like styling for consistency
    return Container(
      padding: EdgeInsets.symmetric(
        horizontal: compact ? 8 : 12,
        vertical: compact ? 4 : 6,
      ),
      decoration: BoxDecoration(
        color: badgeColor.withValues(alpha: 0.15),
        borderRadius: BorderRadius.circular(20),
        border: Border.all(color: badgeColor.withValues(alpha: 0.3)),
      ),
      child: Row(
        mainAxisSize: MainAxisSize.min,
        children: [
          Icon(Icons.star_rounded, size: compact ? 14 : 16, color: badgeColor),
          const SizedBox(width: 4),
          if (sourceLabel != null && sourceLabel.isNotEmpty && !compact) ...[
            Text(
              sourceLabel,
              style: TextStyle(
                color: badgeColor,
                fontSize: compact ? 10 : 12,
                fontWeight: FontWeight.w500,
              ),
            ),
            const SizedBox(width: 4),
          ],
          Text(
            displayRating.toStringAsFixed(1),
            style: TextStyle(
              color: badgeColor,
              fontSize: compact ? 10 : 12,
              fontWeight: FontWeight.bold,
            ),
          ),
        ],
      ),
    );
  }

  Color _getRatingColor(BuildContext context, double rating) {
    if (rating >= 7.0) return Colors.green;
    if (rating >= 5.0) return Colors.orange;
    return Theme.of(context).colorScheme.error;
  }
}
