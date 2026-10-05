import 'package:flutter/material.dart';

import '../../domain/entities/entities.dart';
import '../theme/app_theme.dart';

/// Bottom sheet for content filters and sorting
class FilterSheet extends StatefulWidget {
  final ContentFilter initialFilter;
  final List<String> availableGenres;
  final List<String> availableCountries;
  final bool showTypeFilter;
  final ValueChanged<ContentFilter> onApply;

  const FilterSheet({
    super.key,
    required this.initialFilter,
    this.availableGenres = const [],
    this.availableCountries = const [],
    this.showTypeFilter = true,
    required this.onApply,
  });

  /// Show filter sheet and return the selected filter
  static Future<ContentFilter?> show(
    BuildContext context, {
    required ContentFilter initialFilter,
    List<String> availableGenres = const [],
    List<String> availableCountries = const [],
    bool showTypeFilter = true,
  }) {
    return showModalBottomSheet<ContentFilter>(
      context: context,
      isScrollControlled: true,
      backgroundColor: AppTheme.surfaceColor,
      shape: const RoundedRectangleBorder(
        borderRadius: BorderRadius.vertical(top: Radius.circular(16)),
      ),
      builder: (context) => DraggableScrollableSheet(
        initialChildSize: 0.7,
        minChildSize: 0.5,
        maxChildSize: 0.95,
        expand: false,
        builder: (context, scrollController) => FilterSheet(
          initialFilter: initialFilter,
          availableGenres: availableGenres.isEmpty
              ? Genres.all
              : availableGenres,
          availableCountries: availableCountries.isEmpty
              ? Countries.all
              : availableCountries,
          showTypeFilter: showTypeFilter,
          onApply: (filter) => Navigator.pop(context, filter),
        ),
      ),
    );
  }

  @override
  State<FilterSheet> createState() => _FilterSheetState();
}

class _FilterSheetState extends State<FilterSheet> {
  late ContentFilter _filter;

  @override
  void initState() {
    super.initState();
    _filter = widget.initialFilter;
  }

  @override
  Widget build(BuildContext context) {
    return Column(
      children: [
        // Handle
        Container(
          margin: const EdgeInsets.symmetric(vertical: 12),
          width: 40,
          height: 4,
          decoration: BoxDecoration(
            color: Colors.grey[600],
            borderRadius: BorderRadius.circular(2),
          ),
        ),

        // Header
        Padding(
          padding: const EdgeInsets.symmetric(horizontal: 16),
          child: Row(
            mainAxisAlignment: MainAxisAlignment.spaceBetween,
            children: [
              Text(
                'Фільтри та сортування',
                style: Theme.of(
                  context,
                ).textTheme.titleLarge?.copyWith(fontWeight: FontWeight.bold),
              ),
              if (_filter.hasActiveFilters)
                TextButton(
                  onPressed: () => setState(() => _filter = _filter.reset()),
                  child: const Text('Скинути'),
                ),
            ],
          ),
        ),
        const Divider(),

        // Content
        Expanded(
          child: ListView(
            padding: const EdgeInsets.symmetric(horizontal: 16),
            children: [
              // Sort section
              _buildSection(title: 'Сортування', child: _buildSortOptions()),

              // Content type filter
              if (widget.showTypeFilter) ...[
                const SizedBox(height: 16),
                _buildSection(title: 'Тип контенту', child: _buildTypeFilter()),
              ],

              // Year filter
              const SizedBox(height: 16),
              _buildSection(title: 'Рік випуску', child: _buildYearFilter()),

              // Rating filter
              const SizedBox(height: 16),
              _buildSection(title: 'Рейтинг', child: _buildRatingFilter()),

              // Genre filter
              const SizedBox(height: 16),
              _buildSection(title: 'Жанри', child: _buildGenreFilter()),

              // Country filter
              if (widget.availableCountries.isNotEmpty) ...[
                const SizedBox(height: 16),
                _buildSection(title: 'Країни', child: _buildCountryFilter()),
              ],

              const SizedBox(height: 24),
            ],
          ),
        ),

        // Apply button
        SafeArea(
          child: Padding(
            padding: const EdgeInsets.all(16),
            child: Row(
              children: [
                // Filter count badge
                if (_filter.hasActiveFilters)
                  Container(
                    padding: const EdgeInsets.symmetric(
                      horizontal: 12,
                      vertical: 8,
                    ),
                    decoration: BoxDecoration(
                      color: AppTheme.primaryColor.withValues(alpha: 0.2),
                      borderRadius: BorderRadius.circular(8),
                    ),
                    child: Text(
                      '${_filter.activeFilterCount} фільтрів',
                      style: TextStyle(color: AppTheme.primaryColor),
                    ),
                  ),
                if (_filter.hasActiveFilters) const SizedBox(width: 12),

                // Apply button
                Expanded(
                  child: ElevatedButton(
                    onPressed: () => widget.onApply(_filter),
                    style: ElevatedButton.styleFrom(
                      padding: const EdgeInsets.symmetric(vertical: 16),
                    ),
                    child: const Text('Застосувати'),
                  ),
                ),
              ],
            ),
          ),
        ),
      ],
    );
  }

  Widget _buildSection({required String title, required Widget child}) {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Text(
          title,
          style: Theme.of(context).textTheme.titleSmall?.copyWith(
            fontWeight: FontWeight.bold,
            color: AppTheme.textMuted,
          ),
        ),
        const SizedBox(height: 8),
        child,
      ],
    );
  }

  Widget _buildSortOptions() {
    return Column(
      children: [
        // Sort by
        Wrap(
          spacing: 8,
          runSpacing: 8,
          children: SortOption.values.map((option) {
            final isSelected = _filter.sortBy == option;
            return ChoiceChip(
              label: Text(option.displayName),
              selected: isSelected,
              selectedColor: AppTheme.primaryColor,
              onSelected: (_) => setState(() {
                _filter = _filter.copyWith(sortBy: option);
              }),
            );
          }).toList(),
        ),
        const SizedBox(height: 8),

        // Sort direction
        Row(
          children: [
            const Text('Напрямок:'),
            const SizedBox(width: 12),
            ChoiceChip(
              label: const Row(
                mainAxisSize: MainAxisSize.min,
                children: [
                  Icon(Icons.arrow_downward, size: 16),
                  SizedBox(width: 4),
                  Text('Спадання'),
                ],
              ),
              selected: _filter.sortDirection == SortDirection.desc,
              selectedColor: AppTheme.primaryColor,
              onSelected: (_) => setState(() {
                _filter = _filter.copyWith(sortDirection: SortDirection.desc);
              }),
            ),
            const SizedBox(width: 8),
            ChoiceChip(
              label: const Row(
                mainAxisSize: MainAxisSize.min,
                children: [
                  Icon(Icons.arrow_upward, size: 16),
                  SizedBox(width: 4),
                  Text('Зростання'),
                ],
              ),
              selected: _filter.sortDirection == SortDirection.asc,
              selectedColor: AppTheme.primaryColor,
              onSelected: (_) => setState(() {
                _filter = _filter.copyWith(sortDirection: SortDirection.asc);
              }),
            ),
          ],
        ),
      ],
    );
  }

  Widget _buildTypeFilter() {
    return Wrap(
      spacing: 8,
      runSpacing: 8,
      children: [
        ChoiceChip(
          label: const Text('Всі'),
          selected: _filter.type == null,
          selectedColor: AppTheme.primaryColor,
          onSelected: (_) => setState(() {
            _filter = _filter.copyWith(clearType: true);
          }),
        ),
        ...ContentType.values.where((t) => t != ContentType.unknown).map((
          type,
        ) {
          final isSelected = _filter.type == type;
          return ChoiceChip(
            label: Text(type.displayName),
            selected: isSelected,
            selectedColor: AppTheme.primaryColor,
            onSelected: (_) => setState(() {
              _filter = _filter.copyWith(type: type);
            }),
          );
        }),
      ],
    );
  }

  Widget _buildYearFilter() {
    final ranges = [
      YearRange.all,
      YearRange.thisYear,
      YearRange.last5Years,
      YearRange.decade2020s,
      YearRange.decade2010s,
      YearRange.decade2000s,
      YearRange.before2000,
    ];

    return Wrap(
      spacing: 8,
      runSpacing: 8,
      children: ranges.map((range) {
        final isSelected = _filter.yearRange == range;
        return ChoiceChip(
          label: Text(range.displayName),
          selected: isSelected,
          selectedColor: AppTheme.primaryColor,
          onSelected: (_) => setState(() {
            _filter = _filter.copyWith(yearRange: range);
          }),
        );
      }).toList(),
    );
  }

  Widget _buildRatingFilter() {
    final ranges = [
      RatingRange.all,
      RatingRange.excellent,
      RatingRange.good,
      RatingRange.average,
    ];

    final labels = {
      RatingRange.all: 'Будь-який',
      RatingRange.excellent: '8+ Відмінно',
      RatingRange.good: '7-8 Добре',
      RatingRange.average: '5-7 Середньо',
    };

    return Wrap(
      spacing: 8,
      runSpacing: 8,
      children: ranges.map((range) {
        final isSelected = _filter.ratingRange == range;
        return ChoiceChip(
          label: Row(
            mainAxisSize: MainAxisSize.min,
            children: [
              if (range != RatingRange.all) ...[
                const Icon(Icons.star, size: 16, color: Colors.amber),
                const SizedBox(width: 4),
              ],
              Text(labels[range] ?? range.displayName),
            ],
          ),
          selected: isSelected,
          selectedColor: AppTheme.primaryColor,
          onSelected: (_) => setState(() {
            _filter = _filter.copyWith(ratingRange: range);
          }),
        );
      }).toList(),
    );
  }

  Widget _buildGenreFilter() {
    return Wrap(
      spacing: 8,
      runSpacing: 8,
      children: widget.availableGenres.map((genre) {
        final isSelected = _filter.genres.contains(genre);
        return FilterChip(
          label: Text(genre),
          selected: isSelected,
          selectedColor: AppTheme.primaryColor.withValues(alpha: 0.3),
          checkmarkColor: AppTheme.primaryColor,
          onSelected: (selected) => setState(() {
            final newGenres = Set<String>.from(_filter.genres);
            if (selected) {
              newGenres.add(genre);
            } else {
              newGenres.remove(genre);
            }
            _filter = _filter.copyWith(genres: newGenres);
          }),
        );
      }).toList(),
    );
  }

  Widget _buildCountryFilter() {
    return Wrap(
      spacing: 8,
      runSpacing: 8,
      children: widget.availableCountries.map((country) {
        final isSelected = _filter.countries.contains(country);
        return FilterChip(
          label: Text(country),
          selected: isSelected,
          selectedColor: AppTheme.primaryColor.withValues(alpha: 0.3),
          checkmarkColor: AppTheme.primaryColor,
          onSelected: (selected) => setState(() {
            final newCountries = Set<String>.from(_filter.countries);
            if (selected) {
              newCountries.add(country);
            } else {
              newCountries.remove(country);
            }
            _filter = _filter.copyWith(countries: newCountries);
          }),
        );
      }).toList(),
    );
  }
}

/// Compact filter button with badge
class FilterButton extends StatelessWidget {
  final ContentFilter filter;
  final VoidCallback onTap;

  const FilterButton({super.key, required this.filter, required this.onTap});

  @override
  Widget build(BuildContext context) {
    return Badge(
      isLabelVisible: filter.hasActiveFilters,
      label: Text('${filter.activeFilterCount}'),
      child: IconButton(
        icon: Icon(
          filter.hasActiveFilters
              ? Icons.filter_alt
              : Icons.filter_alt_outlined,
        ),
        onPressed: onTap,
        tooltip: 'Фільтри',
      ),
    );
  }
}

/// Sort button dropdown
class SortButton extends StatelessWidget {
  final SortOption currentSort;
  final SortDirection direction;
  final ValueChanged<SortOption> onSortChanged;
  final ValueChanged<SortDirection> onDirectionChanged;

  const SortButton({
    super.key,
    required this.currentSort,
    required this.direction,
    required this.onSortChanged,
    required this.onDirectionChanged,
  });

  @override
  Widget build(BuildContext context) {
    return PopupMenuButton<SortOption>(
      icon: Row(
        mainAxisSize: MainAxisSize.min,
        children: [
          const Icon(Icons.sort),
          Icon(
            direction == SortDirection.desc
                ? Icons.arrow_downward
                : Icons.arrow_upward,
            size: 14,
          ),
        ],
      ),
      tooltip: 'Сортування',
      onSelected: (option) {
        if (option == currentSort) {
          // Toggle direction
          onDirectionChanged(
            direction == SortDirection.desc
                ? SortDirection.asc
                : SortDirection.desc,
          );
        } else {
          onSortChanged(option);
        }
      },
      itemBuilder: (context) => SortOption.values.map((option) {
        final isSelected = option == currentSort;
        return PopupMenuItem(
          value: option,
          child: Row(
            children: [
              if (isSelected)
                Icon(
                  direction == SortDirection.desc
                      ? Icons.arrow_downward
                      : Icons.arrow_upward,
                  size: 18,
                  color: AppTheme.primaryColor,
                )
              else
                const SizedBox(width: 18),
              const SizedBox(width: 8),
              Text(
                option.displayName,
                style: TextStyle(
                  fontWeight: isSelected ? FontWeight.bold : null,
                  color: isSelected ? AppTheme.primaryColor : null,
                ),
              ),
            ],
          ),
        );
      }).toList(),
    );
  }
}

/// Quick filter chips row
class QuickFilterChips extends StatelessWidget {
  final ContentFilter filter;
  final ValueChanged<ContentFilter> onFilterChanged;

  const QuickFilterChips({
    super.key,
    required this.filter,
    required this.onFilterChanged,
  });

  @override
  Widget build(BuildContext context) {
    final chips = <Widget>[];

    // Type chip
    if (filter.type != null) {
      chips.add(
        _buildChip(
          label: filter.type!.displayName,
          onRemove: () => onFilterChanged(filter.copyWith(clearType: true)),
        ),
      );
    }

    // Year chip
    if (!filter.yearRange.isEmpty) {
      chips.add(
        _buildChip(
          label: filter.yearRange.displayName,
          onRemove: () =>
              onFilterChanged(filter.copyWith(yearRange: YearRange.all)),
        ),
      );
    }

    // Rating chip
    if (!filter.ratingRange.isEmpty) {
      chips.add(
        _buildChip(
          label: '★ ${filter.ratingRange.displayName}',
          onRemove: () =>
              onFilterChanged(filter.copyWith(ratingRange: RatingRange.all)),
        ),
      );
    }

    // Genre chips
    for (final genre in filter.genres) {
      chips.add(
        _buildChip(
          label: genre,
          onRemove: () {
            final newGenres = Set<String>.from(filter.genres)..remove(genre);
            onFilterChanged(filter.copyWith(genres: newGenres));
          },
        ),
      );
    }

    if (chips.isEmpty) return const SizedBox.shrink();

    return SingleChildScrollView(
      scrollDirection: Axis.horizontal,
      padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 8),
      child: Row(
        children: [
          ...chips.map(
            (chip) =>
                Padding(padding: const EdgeInsets.only(right: 8), child: chip),
          ),
          // Clear all button
          TextButton.icon(
            icon: const Icon(Icons.clear_all, size: 18),
            label: const Text('Очистити'),
            onPressed: () => onFilterChanged(filter.reset()),
            style: TextButton.styleFrom(foregroundColor: AppTheme.textMuted),
          ),
        ],
      ),
    );
  }

  Widget _buildChip({required String label, required VoidCallback onRemove}) {
    return Chip(
      label: Text(label),
      deleteIcon: const Icon(Icons.close, size: 16),
      onDeleted: onRemove,
      backgroundColor: AppTheme.primaryColor.withValues(alpha: 0.2),
      deleteIconColor: AppTheme.textPrimary,
      labelStyle: const TextStyle(fontSize: 12),
      materialTapTargetSize: MaterialTapTargetSize.shrinkWrap,
      visualDensity: VisualDensity.compact,
    );
  }
}
