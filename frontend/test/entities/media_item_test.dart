import 'package:flutter_test/flutter_test.dart';

import 'package:oxide_film/domain/entities/entities.dart';

void main() {
  group('ContentFilter', () {
    test('should create with default values', () {
      const filter = ContentFilter();

      expect(filter.type, isNull);
      expect(filter.genres, isEmpty);
      expect(filter.yearRange.isEmpty, isTrue);
      expect(filter.countries, isEmpty);
      expect(filter.ratingRange.isEmpty, isTrue);
    });

    test('should filter by type correctly', () {
      const filter = ContentFilter(type: ContentType.movie);

      final items = [
        MediaItem(
          id: '1',
          providerId: 'test',
          title: 'Movie',
          type: ContentType.movie,
        ),
        MediaItem(
          id: '2',
          providerId: 'test',
          title: 'Series',
          type: ContentType.series,
        ),
      ];

      final filtered = filter.apply(items);

      expect(filtered.length, 1);
      expect(filtered.first.type, ContentType.movie);
    });

    test('should filter by year range correctly', () {
      const filter = ContentFilter(yearRange: YearRange(from: 2023, to: 2024));

      final items = [
        MediaItem(id: '1', providerId: 'test', title: 'Old Movie', year: 2020),
        MediaItem(id: '2', providerId: 'test', title: 'New Movie', year: 2023),
        MediaItem(
          id: '3',
          providerId: 'test',
          title: 'Newest Movie',
          year: 2024,
        ),
      ];

      final filtered = filter.apply(items);

      expect(filtered.length, 2);
      expect(filtered.every((m) => m.year == 2023 || m.year == 2024), isTrue);
    });

    test('should filter by minimum rating', () {
      const filter = ContentFilter(ratingRange: RatingRange(min: 7.0));

      final items = [
        MediaItem(id: '1', providerId: 'test', title: 'Low Rated', rating: 5.0),
        MediaItem(
          id: '2',
          providerId: 'test',
          title: 'High Rated',
          rating: 8.5,
        ),
        MediaItem(id: '3', providerId: 'test', title: 'No Rating'),
      ];

      final filtered = filter.apply(items);

      // Items with no rating should pass if we don't have a max
      // High rated item should pass
      expect(filtered.any((m) => m.rating == 8.5), isTrue);
      expect(
        filtered.every((m) => m.rating == null || m.rating! >= 7.0),
        isTrue,
      );
    });

    test('should pass through items without filter criteria', () {
      const filter = ContentFilter();

      final items = [
        MediaItem(id: '1', providerId: 'test', title: 'Movie 1'),
        MediaItem(id: '2', providerId: 'test', title: 'Movie 2'),
      ];

      final filtered = filter.apply(items);

      expect(filtered.length, 2);
    });

    test('copyWith should create modified filter', () {
      const original = ContentFilter(type: ContentType.movie);
      final modified = original.copyWith(
        ratingRange: const RatingRange(min: 5.0),
      );

      expect(original.ratingRange.isEmpty, isTrue);
      expect(modified.type, ContentType.movie);
      expect(modified.ratingRange.min, 5.0);
    });

    test('hasActiveFilters should correctly detect active filters', () {
      const empty = ContentFilter();
      expect(empty.hasActiveFilters, isFalse);

      const withType = ContentFilter(type: ContentType.movie);
      expect(withType.hasActiveFilters, isTrue);

      const withYear = ContentFilter(yearRange: YearRange(from: 2020));
      expect(withYear.hasActiveFilters, isTrue);

      const withRating = ContentFilter(ratingRange: RatingRange(min: 7.0));
      expect(withRating.hasActiveFilters, isTrue);
    });

    test('activeFilterCount should return correct count', () {
      const empty = ContentFilter();
      expect(empty.activeFilterCount, 0);

      const withType = ContentFilter(type: ContentType.movie);
      expect(withType.activeFilterCount, 1);

      const withMultiple = ContentFilter(
        type: ContentType.movie,
        yearRange: YearRange(from: 2020),
        ratingRange: RatingRange(min: 7.0),
      );
      expect(withMultiple.activeFilterCount, 3);
    });

    test('reset should clear all filters', () {
      const filter = ContentFilter(
        type: ContentType.movie,
        genres: {'action', 'comedy'},
        yearRange: YearRange(from: 2020, to: 2024),
        ratingRange: RatingRange(min: 7.0),
      );

      final reset = filter.reset();

      expect(reset.type, isNull);
      expect(reset.genres, isEmpty);
      expect(reset.yearRange.isEmpty, isTrue);
      expect(reset.ratingRange.isEmpty, isTrue);
    });
  });

  group('YearRange', () {
    test('should match years within range', () {
      const range = YearRange(from: 2020, to: 2024);

      expect(range.matches(2022), isTrue);
      expect(range.matches(2020), isTrue);
      expect(range.matches(2024), isTrue);
      expect(range.matches(2019), isFalse);
      expect(range.matches(2025), isFalse);
    });

    test('should handle open-ended ranges', () {
      const fromOnly = YearRange(from: 2020);
      expect(fromOnly.matches(2023), isTrue);
      expect(fromOnly.matches(2019), isFalse);

      const toOnly = YearRange(to: 2020);
      expect(toOnly.matches(2019), isTrue);
      expect(toOnly.matches(2021), isFalse);
    });

    test('should match null years when empty', () {
      const empty = YearRange();
      expect(empty.matches(null), isTrue);
      expect(empty.matches(2023), isTrue);
    });
  });

  group('RatingRange', () {
    test('should match ratings within range', () {
      const range = RatingRange(min: 6.0, max: 8.0);

      expect(range.matches(7.0), isTrue);
      expect(range.matches(6.0), isTrue);
      expect(range.matches(8.0), isTrue);
      expect(range.matches(5.0), isFalse);
      expect(range.matches(9.0), isFalse);
    });

    test('should handle min-only range', () {
      const range = RatingRange(min: 7.0);

      expect(range.matches(8.0), isTrue);
      expect(range.matches(7.0), isTrue);
      expect(range.matches(6.5), isFalse);
    });

    test('should match null ratings when empty', () {
      const empty = RatingRange();
      expect(empty.matches(null), isTrue);
      expect(empty.matches(8.0), isTrue);
    });
  });

  group('MediaItem', () {
    test('should create with required parameters', () {
      final item = MediaItem(
        id: '123',
        providerId: 'uakino',
        title: 'Test Movie',
      );

      expect(item.id, '123');
      expect(item.providerId, 'uakino');
      expect(item.title, 'Test Movie');
      expect(item.type, ContentType.unknown);
    });

    test('should create with all parameters', () {
      final item = MediaItem(
        id: '123',
        providerId: 'uakino',
        title: 'Test Movie',
        originalTitle: 'Original Title',
        posterUrl: 'https://example.com/poster.jpg',
        year: 2023,
        rating: 8.5,
        type: ContentType.movie,
        description: 'A test movie',
      );

      expect(item.originalTitle, 'Original Title');
      expect(item.posterUrl, 'https://example.com/poster.jpg');
      expect(item.year, 2023);
      expect(item.rating, 8.5);
      expect(item.type, ContentType.movie);
    });

    test('equality should work correctly', () {
      final item1 = MediaItem(id: '123', providerId: 'uakino', title: 'Test');

      final item2 = MediaItem(id: '123', providerId: 'uakino', title: 'Test');

      final item3 = MediaItem(id: '456', providerId: 'uakino', title: 'Test');

      expect(item1, equals(item2));
      expect(item1, isNot(equals(item3)));
    });
  });

  group('MediaDetails', () {
    test('isSeries should return true for series content', () {
      final details = MediaDetails(
        item: MediaItem(
          id: '1',
          providerId: 'test',
          title: 'Test Series',
          type: ContentType.series,
        ),
        seasons: [
          Season(number: 1, episodes: [Episode(number: 1, title: 'Ep 1')]),
        ],
      );

      expect(details.isSeries, isTrue);
    });

    test('isSeries should return false for movies', () {
      final details = MediaDetails(
        item: MediaItem(
          id: '1',
          providerId: 'test',
          title: 'Test Movie',
          type: ContentType.movie,
        ),
      );

      expect(details.isSeries, isFalse);
    });
  });
}
