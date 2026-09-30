import 'dart:math';
import 'package:flutter/foundation.dart';
import '../../core/utils/logger.dart';
import '../../core/constants/content_constants.dart';
import '../../domain/entities/entities.dart';
import '../providers/provider_registry.dart';
import 'history_service.dart';

import 'favorites_service.dart';
import '../database/dao/media_items_dao.dart';

/// Service for generating personalized content recommendations
class RecommendationService extends ChangeNotifier {
  static const String _tag = 'RecommendationService';

  final HistoryService _historyService;
  final FavoritesService _favoritesService;
  final ProviderRegistry _providerRegistry;
  final MediaItemsDao _mediaItemsDao;

  List<MediaItem> _recommendations = [];
  List<MediaItem> get recommendations => _recommendations;

  bool _isLoading = false;
  bool get isLoading => _isLoading;

  String? _error;
  String? get error => _error;

  RecommendationService({
    required HistoryService historyService,
    required FavoritesService favoritesService,
    required ProviderRegistry providerRegistry,
    required MediaItemsDao mediaItemsDao,
  }) : _historyService = historyService,
       _favoritesService = favoritesService,
       _providerRegistry = providerRegistry,
       _mediaItemsDao = mediaItemsDao;

  /// Initialize and load recommendations
  Future<void> init() async {
    await refresh();
  }

  /// Refresh recommendations
  Future<void> refresh() async {
    if (_isLoading) return;

    try {
      _isLoading = true;
      _error = null;
      notifyListeners();

      final recommendations = await _generateRecommendations();

      _recommendations = recommendations;
      Logger.i(
        'Generated ${_recommendations.length} recommendations',
        tag: _tag,
      );
    } catch (e, stack) {
      Logger.e(
        'Failed to generate recommendations',
        error: e,
        stackTrace: stack,
        tag: _tag,
      );
      _error = e.toString();
    } finally {
      _isLoading = false;
      notifyListeners();
    }
  }

  Future<List<MediaItem>> _generateRecommendations() async {
    final history = _historyService.history.take(20).toList();
    final favorites = _favoritesService.favorites.take(10).toList();

    if (history.isEmpty && favorites.isEmpty) {
      Logger.d(
        'No history or favorites found, falling back to popular content',
        tag: _tag,
      );
      return _getPopularFallback();
    }

    final providers = _providerRegistry.homeProviders;
    if (providers.isEmpty) return [];

    // 1. Визначаємо розподіл типів контенту з history (mediaType завжди є в history)
    final typeCounts = <String, int>{};
    for (final h in history) {
      final t = h.mediaType;
      typeCounts[t] = (typeCounts[t] ?? 0) + 2;
    }

    // 2. Визначаємо жанри з кешованих MediaItem (якщо є — bonus signal)
    final genreCounts = <String, int>{};
    for (final h in history.take(10)) {
      final cached = await _mediaItemsDao.get(h.mediaId, h.providerId);
      if (cached?.genres != null) {
        for (final genre in cached!.genres!) {
          genreCounts[genre] = (genreCounts[genre] ?? 0) + 2;
        }
      }
    }
    for (final f in favorites.take(5)) {
      final cached = await _mediaItemsDao.get(f.mediaId, f.providerId);
      if (cached?.genres != null) {
        for (final genre in cached!.genres!) {
          genreCounts[genre] = (genreCounts[genre] ?? 0) + 3;
        }
      }
    }

    final results = <MediaItem>[];

    // 3. Якщо є жанри — шукаємо по жанрах
    if (genreCounts.isNotEmpty) {
      final sortedGenres = genreCounts.entries.toList()
        ..sort((a, b) => b.value.compareTo(a.value));
      final topGenres = sortedGenres.take(3).map((e) => e.key).toList();

      Logger.d('Top genres for recommendations: ${topGenres.join(', ')}', tag: _tag);

      for (final genre in topGenres) {
        final provider = providers[Random().nextInt(providers.length)];
        try {
          final slug = ProviderGenreMappings.getSlugForProvider(provider.id, genre);
          final items = await provider.getByCategory(slug, page: 1);
          results.addAll(items.take(10));
        } catch (e) {
          Logger.w('Failed to fetch genre $genre: $e', tag: _tag);
        }
      }
    }

    // 4. Доповнюємо по топ типу контенту (завжди є з history)
    if (results.length < 15 && typeCounts.isNotEmpty) {
      final sortedTypes = typeCounts.entries.toList()
        ..sort((a, b) => b.value.compareTo(a.value));
      final topType = sortedTypes.first.key;

      ContentType? contentType;
      try {
        contentType = ContentType.values.firstWhere((e) => e.name == topType);
      } catch (_) {}

      Logger.d('Using content type for recommendations: $topType', tag: _tag);

      // Беремо декілька провайдерів для різноманіття
      for (final provider in providers) {
        try {
          final items = await provider.getPopular(type: contentType, page: 1);
          results.addAll(items.take(8));
        } catch (e) {
          Logger.w('Failed to fetch popular for type $topType from ${provider.name}: $e', tag: _tag);
        }
      }
    }

    // 5. Fallback якщо все ще мало
    if (results.length < 15) {
      final popular = await _getPopularFallback();
      results.addAll(popular);
    }

    // 6. Дедуплікація і фільтрація вже переглянутого
    // Використовуємо uniqueId (providerId:mediaId) як primary key — точніше ніж title
    final viewedIds = history
        .map((h) => '${h.providerId}:${h.mediaId}')
        .toSet();
    final viewedTitles = history
        .map((h) => h.title.toLowerCase().trim())
        .toSet();
    final favoriteTitles = favorites
        .map((f) => f.title.toLowerCase().trim())
        .toSet();

    final uniqueResults = <MediaItem>[];
    final addedIds = <String>{};
    final addedTitles = <String>{};

    for (final item in results) {
      final id = item.uniqueId;
      final titleKey = item.title.toLowerCase().trim();

      if (viewedIds.contains(id)) continue;
      if (viewedTitles.contains(titleKey)) continue;
      if (favoriteTitles.contains(titleKey)) continue;
      if (addedIds.contains(id)) continue;
      if (addedTitles.contains(titleKey)) continue;

      uniqueResults.add(item);
      addedIds.add(id);
      addedTitles.add(titleKey);
    }

    // Перемішуємо для різноманіття
    uniqueResults.shuffle();

    return uniqueResults.take(20).toList();
  }

  Future<List<MediaItem>> _getPopularFallback() async {
    final providers = _providerRegistry.homeProviders;
    if (providers.isEmpty) return [];

    final results = <MediaItem>[];
    for (final provider in providers) {
      try {
        final items = await provider.getPopular(page: 1);
        results.addAll(items);
      } catch (e) {
        Logger.e(
          'Fallback popular fetch failed for ${provider.name}',
          error: e,
          tag: _tag,
        );
      }
    }
    return results;
  }
}
