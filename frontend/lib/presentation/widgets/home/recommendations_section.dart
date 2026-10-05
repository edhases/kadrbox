import 'package:flutter/material.dart';
import 'package:get_it/get_it.dart';
import 'package:go_router/go_router.dart';
import 'package:cached_network_image/cached_network_image.dart';
import 'package:skeletonizer/skeletonizer.dart';

import '../../../data/services/recommendation_service.dart';
import '../../../domain/entities/media_item.dart';

class RecommendationsSection extends StatefulWidget {
  const RecommendationsSection({super.key});

  @override
  State<RecommendationsSection> createState() => _RecommendationsSectionState();
}

class _RecommendationsSectionState extends State<RecommendationsSection> {
  final _recommendationService = GetIt.instance<RecommendationService>();
  final _scrollController = ScrollController();

  @override
  void initState() {
    super.initState();
    // Recommendations are initialized by main app or lazily here
    if (_recommendationService.recommendations.isEmpty &&
        !_recommendationService.isLoading) {
      _recommendationService.init();
    }
  }

  @override
  void dispose() {
    _scrollController.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    return ListenableBuilder(
      listenable: _recommendationService,
      builder: (context, _) {
        if (_recommendationService.recommendations.isEmpty &&
            !_recommendationService.isLoading) {
          return const SizedBox.shrink();
        }

        return Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Padding(
              padding: const EdgeInsets.fromLTRB(16, 24, 16, 12),
              child: Text(
                'Рекомендовано вам',
                style: Theme.of(
                  context,
                ).textTheme.titleLarge?.copyWith(fontWeight: FontWeight.bold),
              ),
            ),
            SizedBox(
              height: 200,
              child: Skeletonizer(
                enabled:
                    _recommendationService.isLoading &&
                    _recommendationService.recommendations.isEmpty,
                child: ScrollConfiguration(
                  behavior: ScrollConfiguration.of(
                    context,
                  ).copyWith(scrollbars: false),
                  child: ListView.builder(
                    controller: _scrollController,
                    scrollDirection: Axis.horizontal,
                    padding: const EdgeInsets.symmetric(horizontal: 16),
                    itemCount:
                        _recommendationService.isLoading &&
                            _recommendationService.recommendations.isEmpty
                        ? 5
                        : _recommendationService.recommendations.length,
                    itemBuilder: (context, index) {
                      if (_recommendationService.isLoading &&
                          _recommendationService.recommendations.isEmpty) {
                        return const _RecommendationSkeletonCell();
                      }
                      final item =
                          _recommendationService.recommendations[index];
                      return _RecommendationCard(item: item);
                    },
                  ),
                ),
              ),
            ),
          ],
        );
      },
    );
  }
}

class _RecommendationSkeletonCell extends StatelessWidget {
  const _RecommendationSkeletonCell();

  @override
  Widget build(BuildContext context) {
    return Container(
      width: 120,
      margin: const EdgeInsets.only(right: 12),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Expanded(
            child: Container(
              decoration: BoxDecoration(
                color: Colors.white,
                borderRadius: BorderRadius.circular(12),
              ),
            ),
          ),
          const SizedBox(height: 8),
          Container(
            width: double.infinity,
            height: 14,
            decoration: BoxDecoration(
              color: Colors.white,
              borderRadius: BorderRadius.circular(4),
            ),
          ),
          const SizedBox(height: 4),
          Container(
            width: 40,
            height: 12,
            decoration: BoxDecoration(
              color: Colors.white,
              borderRadius: BorderRadius.circular(4),
            ),
          ),
        ],
      ),
    );
  }
}

class _RecommendationCard extends StatelessWidget {
  final MediaItem item;

  const _RecommendationCard({required this.item});

  @override
  Widget build(BuildContext context) {
    return Container(
      width: 120,
      margin: const EdgeInsets.only(right: 12),
      child: GestureDetector(
        onTap: () {
          context.push(
            '/details/${item.providerId}/${Uri.encodeComponent(item.id)}',
          );
        },
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Expanded(
              child: ClipRRect(
                borderRadius: BorderRadius.circular(10),
                child: Stack(
                  fit: StackFit.expand,
                  children: [
                    if (item.posterUrl != null)
                      CachedNetworkImage(
                        imageUrl: item.posterUrl!,
                        fit: BoxFit.cover,
                        placeholder: (_, __) => Container(
                          color: const Color(0xFF1E1E1E),
                          child: const Center(
                            child: SizedBox(
                              width: 20,
                              height: 20,
                              child: CircularProgressIndicator(
                                strokeWidth: 2,
                                color: Color(0xFF333333),
                              ),
                            ),
                          ),
                        ),
                        errorWidget: (_, __, ___) => Container(
                          color: const Color(0xFF1E1E1E),
                          child: const Icon(
                            Icons.movie_outlined,
                            size: 32,
                            color: Color(0xFF555555),
                          ),
                        ),
                      )
                    else
                      Container(
                        color: const Color(0xFF1E1E1E),
                        child: const Icon(
                          Icons.movie_outlined,
                          size: 32,
                          color: Color(0xFF555555),
                        ),
                      ),

                    // Rating badge
                    if (item.rating != null && item.rating! > 0)
                      Positioned(
                        top: 6,
                        right: 6,
                        child: Container(
                          padding: const EdgeInsets.symmetric(
                            horizontal: 5,
                            vertical: 2,
                          ),
                          decoration: BoxDecoration(
                            color: Colors.black.withOpacity(0.7),
                            borderRadius: BorderRadius.circular(4),
                          ),
                          child: Row(
                            mainAxisSize: MainAxisSize.min,
                            children: [
                              const Icon(
                                Icons.star,
                                size: 9,
                                color: Colors.amber,
                              ),
                              const SizedBox(width: 2),
                              Text(
                                item.rating!.toStringAsFixed(1),
                                style: const TextStyle(
                                  color: Colors.white,
                                  fontSize: 9,
                                  fontWeight: FontWeight.bold,
                                ),
                              ),
                            ],
                          ),
                        ),
                      ),
                  ],
                ),
              ),
            ),
            const SizedBox(height: 6),
            Text(
              item.title,
              maxLines: 2,
              overflow: TextOverflow.ellipsis,
              style: const TextStyle(
                fontWeight: FontWeight.w500,
                fontSize: 13,
                color: Colors.white,
                height: 1.3,
              ),
            ),
            if (item.year != null) ...[
              const SizedBox(height: 2),
              Text(
                item.year.toString(),
                style: const TextStyle(color: Color(0xFF8E8E93), fontSize: 12),
              ),
            ],
          ],
        ),
      ),
    );
  }
}
