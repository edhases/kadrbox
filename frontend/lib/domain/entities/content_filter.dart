import 'package:equatable/equatable.dart';
import '../../core/constants/content_constants.dart';
import 'media_item.dart';

/// Sort direction
enum SortDirection { asc, desc }

/// Available sort options
enum SortOption {
  popularity,
  rating,
  year,
  title,
  dateAdded;

  String get displayName {
    switch (this) {
      case SortOption.popularity:
        return 'Популярність';
      case SortOption.rating:
        return 'Рейтинг';
      case SortOption.year:
        return 'Рік';
      case SortOption.title:
        return 'Назва';
      case SortOption.dateAdded:
        return 'Дата додавання';
    }
  }
}

/// Year range for filtering
class YearRange extends Equatable {
  final int? from;
  final int? to;

  const YearRange({this.from, this.to});

  /// Current year
  static int get currentYear => DateTime.now().year;

  /// Predefined ranges
  static YearRange get all => const YearRange();
  static YearRange get thisYear =>
      YearRange(from: currentYear, to: currentYear);
  static YearRange get last5Years =>
      YearRange(from: currentYear - 5, to: currentYear);
  static YearRange get decade2020s => const YearRange(from: 2020, to: 2029);
  static YearRange get decade2010s => const YearRange(from: 2010, to: 2019);
  static YearRange get decade2000s => const YearRange(from: 2000, to: 2009);
  static YearRange get before2000 => const YearRange(to: 1999);

  bool get isEmpty => from == null && to == null;

  String get displayName {
    if (isEmpty) return 'Всі роки';
    if (from == to && from != null) return '$from';
    if (from != null && to != null) return '$from - $to';
    if (from != null) return 'Від $from';
    return 'До $to';
  }

  bool matches(int? year) {
    if (isEmpty) return true;
    // If filter is active but item has no year, exclude it
    if (year == null) return false;
    if (from != null && year < from!) return false;
    if (to != null && year > to!) return false;
    return true;
  }

  @override
  List<Object?> get props => [from, to];
}

/// Rating range for filtering
class RatingRange extends Equatable {
  final double? min;
  final double? max;

  const RatingRange({this.min, this.max});

  static RatingRange get all => const RatingRange();
  static RatingRange get excellent => const RatingRange(min: 8.0);
  static RatingRange get good => const RatingRange(min: 7.0, max: 7.99);
  static RatingRange get average => const RatingRange(min: 5.0, max: 6.99);
  static RatingRange get low => const RatingRange(max: 4.99);

  bool get isEmpty => min == null && max == null;

  String get displayName {
    if (isEmpty) return 'Будь-який';
    if (min != null && max == null) return '${min!.toStringAsFixed(1)}+';
    if (min == null && max != null) return 'До ${max!.toStringAsFixed(1)}';
    return '${min!.toStringAsFixed(1)} - ${max!.toStringAsFixed(1)}';
  }

  bool matches(double? rating) {
    if (isEmpty) return true;
    // If filter is active but item has no rating, exclude it
    if (rating == null) return false;
    if (min != null && rating < min!) return false;
    if (max != null && rating > max!) return false;
    return true;
  }

  @override
  List<Object?> get props => [min, max];
}

/// Content filter configuration
class ContentFilter extends Equatable {
  final ContentType? type;
  final Set<String> genres;
  final YearRange yearRange;
  final RatingRange ratingRange;
  final Set<String> countries;
  final SortOption sortBy;
  final SortDirection sortDirection;

  const ContentFilter({
    this.type,
    this.genres = const {},
    this.yearRange = const YearRange(),
    this.ratingRange = const RatingRange(),
    this.countries = const {},
    this.sortBy = SortOption.popularity,
    this.sortDirection = SortDirection.desc,
  });

  /// Check if any filters are active
  bool get hasActiveFilters =>
      type != null ||
      genres.isNotEmpty ||
      !yearRange.isEmpty ||
      !ratingRange.isEmpty ||
      countries.isNotEmpty;

  /// Number of active filters
  int get activeFilterCount {
    int count = 0;
    if (type != null) count++;
    if (genres.isNotEmpty) count++;
    if (!yearRange.isEmpty) count++;
    if (!ratingRange.isEmpty) count++;
    if (countries.isNotEmpty) count++;
    return count;
  }

  /// Create a copy with modifications
  ContentFilter copyWith({
    ContentType? type,
    Set<String>? genres,
    YearRange? yearRange,
    RatingRange? ratingRange,
    Set<String>? countries,
    SortOption? sortBy,
    SortDirection? sortDirection,
    bool clearType = false,
  }) {
    return ContentFilter(
      type: clearType ? null : (type ?? this.type),
      genres: genres ?? this.genres,
      yearRange: yearRange ?? this.yearRange,
      ratingRange: ratingRange ?? this.ratingRange,
      countries: countries ?? this.countries,
      sortBy: sortBy ?? this.sortBy,
      sortDirection: sortDirection ?? this.sortDirection,
    );
  }

  /// Reset all filters to default
  ContentFilter reset() {
    return const ContentFilter();
  }

  /// Apply filter to a list of items (client-side filtering)
  List<MediaItem> apply(List<MediaItem> items) {
    var filtered = items.where((item) {
      // Type filter
      if (type != null && item.type != type) return false;

      // Year filter
      if (!yearRange.matches(item.year)) return false;

      // Rating filter
      if (!ratingRange.matches(item.rating)) return false;

      // Genres filter (if item has genres, check if any match)
      if (genres.isNotEmpty) {
        if (item.genres == null || item.genres!.isEmpty) {
          // Item has no genres - skip filtering by genre for this item
          // to avoid excluding items where genres weren't parsed
        } else {
          // Check if any of item's genres match filter genres
          final hasMatchingGenre = item.genres!.any(
            (itemGenre) => genres.any(
              (filterGenre) =>
                  itemGenre.toLowerCase() == filterGenre.toLowerCase() ||
                  itemGenre.toLowerCase().contains(filterGenre.toLowerCase()) ||
                  filterGenre.toLowerCase().contains(itemGenre.toLowerCase()),
            ),
          );
          if (!hasMatchingGenre) return false;
        }
      }

      // Countries filter
      if (countries.isNotEmpty) {
        if (item.country == null || item.country!.isEmpty) {
          // Item has no country - skip filtering by country for this item
        } else {
          final hasMatchingCountry = countries.any(
            (filterCountry) =>
                item.country!.toLowerCase() == filterCountry.toLowerCase() ||
                item.country!.toLowerCase().contains(
                  filterCountry.toLowerCase(),
                ),
          );
          if (!hasMatchingCountry) return false;
        }
      }

      return true;
    }).toList();

    // Sort
    filtered.sort((a, b) {
      int comparison;
      switch (sortBy) {
        case SortOption.popularity:
          // Keep original order (already sorted by popularity from provider)
          comparison = 0;
          break;
        case SortOption.rating:
          comparison = (a.rating ?? 0).compareTo(b.rating ?? 0);
          break;
        case SortOption.year:
          comparison = (a.year ?? 0).compareTo(b.year ?? 0);
          break;
        case SortOption.title:
          comparison = a.title.toLowerCase().compareTo(b.title.toLowerCase());
          break;
        case SortOption.dateAdded:
          // Keep original order (should be sorted by date from provider)
          comparison = 0;
          break;
      }
      return sortDirection == SortDirection.desc ? -comparison : comparison;
    });

    return filtered;
  }

  @override
  List<Object?> get props => [
    type,
    genres,
    yearRange,
    ratingRange,
    countries,
    sortBy,
    sortDirection,
  ];
}

/// Common genres list
/// @deprecated Use ContentGenres from core/constants/content_constants.dart instead
class Genres {
  static List<String> get all => ContentGenres.all;
  static Map<String, String> get slugs => ContentGenres.toSlug;
}

/// Common countries list
/// @deprecated Use ContentCountries from core/constants/content_constants.dart instead
class Countries {
  static List<String> get all => ContentCountries.all;
}
