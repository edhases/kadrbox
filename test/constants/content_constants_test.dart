import 'package:flutter_test/flutter_test.dart';

import 'package:oxide_film/core/constants/content_constants.dart';

void main() {
  group('ContentGenres', () {
    test('should have all common genres', () {
      expect(ContentGenres.all, contains('Бойовик'));
      expect(ContentGenres.all, contains('Комедія'));
      expect(ContentGenres.all, contains('Драма'));
      expect(ContentGenres.all, contains('Фантастика'));
      expect(ContentGenres.all, contains('Жахи'));
    });

    test('should not be empty', () {
      expect(ContentGenres.all, isNotEmpty);
    });

    test('should have unique values', () {
      final uniqueGenres = ContentGenres.all.toSet();
      expect(uniqueGenres.length, ContentGenres.all.length);
    });
  });

  group('ContentCountries', () {
    test('should have common countries', () {
      expect(ContentCountries.all, contains('США'));
      expect(ContentCountries.all, contains('Україна'));
    });

    test('should not be empty', () {
      expect(ContentCountries.all, isNotEmpty);
    });
  });

  group('ProviderGenreMappings', () {
    test('should return correct slug for UAKino', () {
      final slug = ProviderGenreMappings.getSlugForProvider(
        'uakino',
        'Бойовик',
      );
      expect(slug, isNotEmpty);
    });

    test('should use ContentGenres.toSlug for unknown provider', () {
      // For unknown providers, it uses ContentGenres.toSlug which maps 'Бойовик' -> 'action'
      final slug = ProviderGenreMappings.getSlugForProvider(
        'unknown_provider',
        'Бойовик',
      );
      expect(slug, 'action');
    });

    test('should return lowercase for unmapped genre', () {
      final slug = ProviderGenreMappings.getSlugForProvider(
        'uakino',
        'Незнаний жанр',
      );
      expect(slug, 'незнаний жанр');
    });
  });
}
