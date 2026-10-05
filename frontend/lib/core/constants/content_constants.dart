/// Centralized constants for content filtering and categorization
///
/// This file serves as the Single Source of Truth for genres, countries,
/// and related mappings across the entire application.
/// All providers and filters should use these constants.
library;

/// Available genres with Ukrainian display names and URL slugs
class ContentGenres {
  ContentGenres._();

  /// All available genres in display format
  static const List<String> all = [
    'Бойовик',
    'Комедія',
    'Драма',
    'Жахи',
    'Трилер',
    'Фантастика',
    'Фентезі',
    'Мелодрама',
    'Пригоди',
    'Детектив',
    'Кримінал',
    'Історичний',
    'Біографія',
    'Документальний',
    'Мюзикл',
    'Військовий',
    'Сімейний',
    'Спорт',
    'Вестерн',
    'Аніме',
  ];

  /// Genre display name to URL slug mapping
  static const Map<String, String> toSlug = {
    'Бойовик': 'action',
    'Комедія': 'comedy',
    'Драма': 'drama',
    'Жахи': 'horror',
    'Трилер': 'thriller',
    'Фантастика': 'sci-fi',
    'Фентезі': 'fantasy',
    'Мелодрама': 'romance',
    'Пригоди': 'adventure',
    'Детектив': 'mystery',
    'Кримінал': 'crime',
    'Історичний': 'history',
    'Біографія': 'biography',
    'Документальний': 'documentary',
    'Мюзикл': 'musical',
    'Військовий': 'war',
    'Сімейний': 'family',
    'Спорт': 'sport',
    'Вестерн': 'western',
    'Аніме': 'anime',
  };

  /// URL slug to display name mapping (reverse of toSlug)
  static final Map<String, String> fromSlug = {
    for (final entry in toSlug.entries) entry.value: entry.key,
  };

  /// Get slug for a genre, returns lowercase genre name if not found
  static String getSlug(String genre) {
    return toSlug[genre] ?? genre.toLowerCase();
  }

  /// Get display name for a slug, returns capitalized slug if not found
  static String getDisplayName(String slug) {
    return fromSlug[slug] ?? _capitalize(slug);
  }

  static String _capitalize(String s) {
    if (s.isEmpty) return s;
    return s[0].toUpperCase() + s.substring(1);
  }
}

/// Available countries for content filtering
class ContentCountries {
  ContentCountries._();

  /// All available countries in display format
  static const List<String> all = [
    'США',
    'Україна',
    'Великобританія',
    'Франція',
    'Німеччина',
    'Японія',
    'Південна Корея',
    'Китай',
    'Індія',
    'Канада',
    'Австралія',
    'Італія',
    'Іспанія',
    'Польща',
    'Туреччина',
    'Бразилія',
    'Мексика',
    'Швеція',
    'Норвегія',
    'Данія',
  ];

  /// Country display name to URL slug mapping
  static const Map<String, String> toSlug = {
    'США': 'usa',
    'Україна': 'ukraine',
    'Великобританія': 'uk',
    'Франція': 'france',
    'Німеччина': 'germany',
    'Японія': 'japan',
    'Південна Корея': 'south-korea',
    'Китай': 'china',
    'Індія': 'india',
    'Канада': 'canada',
    'Австралія': 'australia',
    'Італія': 'italy',
    'Іспанія': 'spain',
    'Польща': 'poland',
    'Туреччина': 'turkey',
    'Бразилія': 'brazil',
    'Мексика': 'mexico',
    'Швеція': 'sweden',
    'Норвегія': 'norway',
    'Данія': 'denmark',
  };

  /// URL slug to display name mapping
  static final Map<String, String> fromSlug = {
    for (final entry in toSlug.entries) entry.value: entry.key,
  };

  /// Get slug for a country
  static String getSlug(String country) {
    return toSlug[country] ?? country.toLowerCase().replaceAll(' ', '-');
  }

  /// Get display name for a slug
  static String getDisplayName(String slug) {
    return fromSlug[slug] ?? slug;
  }
}

/// Provider-specific genre mappings
/// Different providers may use different slugs for the same genres
class ProviderGenreMappings {
  ProviderGenreMappings._();

  /// Slug overrides registered by source id.
  ///
  /// Empty by default: the app must not know which catalogue servers exist, so
  /// no server-specific genre table ships with it. A server that needs custom
  /// slugs supplies them at runtime.
  static const Map<String, Map<String, String>> overrides = {};

  /// Get the slug for a genre as understood by [providerId].
  ///
  /// Prefers a registered override for that source, then the canonical
  /// [ContentGenres.toSlug] table, then [fallback], then the lowercased genre.
  static String getSlugForProvider(
    String providerId,
    String genre, {
    String? fallback,
  }) {
    final mapping = overrides[providerId] ?? ContentGenres.toSlug;
    return mapping[genre] ?? fallback ?? ContentGenres.getSlug(genre);
  }
}
