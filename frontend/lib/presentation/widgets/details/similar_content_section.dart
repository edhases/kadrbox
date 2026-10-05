import 'package:flutter/material.dart';
import 'package:get_it/get_it.dart';
import 'package:go_router/go_router.dart';

import '../../../data/providers/provider_registry.dart';
import '../../../domain/entities/entities.dart';
import '../media_card.dart';
import '../common/skeleton.dart';

class SimilarContentSection extends StatefulWidget {
  final String providerId;
  final MediaDetails details;

  const SimilarContentSection({
    super.key,
    required this.providerId,
    required this.details,
  });

  @override
  State<SimilarContentSection> createState() => _SimilarContentSectionState();
}

class _SimilarContentSectionState extends State<SimilarContentSection> {
  final _registry = GetIt.instance<ProviderRegistry>();

  List<MediaItem> _items = [];
  bool _isLoading = true;
  String? _error;

  @override
  void initState() {
    super.initState();
    _loadSimilar();
  }

  @override
  void didUpdateWidget(SimilarContentSection oldWidget) {
    super.didUpdateWidget(oldWidget);
    if (oldWidget.details.item.id != widget.details.item.id ||
        oldWidget.providerId != widget.providerId) {
      _loadSimilar();
    }
  }

  Future<void> _loadSimilar() async {
    if (!mounted) return;

    setState(() {
      _isLoading = true;
      _error = null;
    });

    try {
      final provider = _registry.getById(widget.providerId);
      if (provider == null) {
        if (mounted) setState(() => _isLoading = false);
        return;
      }

      final items = await provider.getSimilar(
        widget.details.item.id,
        widget.details,
      );

      if (mounted) {
        setState(() {
          _items = items;
          _isLoading = false;
        });
      }
    } catch (e) {
      if (mounted) {
        setState(() {
          _error = e.toString();
          _isLoading = false;
        });
      }
    }
  }

  @override
  Widget build(BuildContext context) {
    if (_isLoading) {
      return _buildSkeleton();
    }

    if (_error != null || _items.isEmpty) {
      return const SizedBox.shrink();
    }

    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Padding(
          padding: const EdgeInsets.symmetric(horizontal: 0),
          child: Text(
            'Схоже',
            style: Theme.of(
              context,
            ).textTheme.titleLarge?.copyWith(fontWeight: FontWeight.bold),
          ),
        ),
        const SizedBox(height: 16),
        SizedBox(
          height: 220, // Adjust height based on MediaCard aspect ratio
          child: ListView.separated(
            scrollDirection: Axis.horizontal,
            itemCount: _items.length,
            separatorBuilder: (context, index) => const SizedBox(width: 12),
            itemBuilder: (context, index) {
              final item = _items[index];
              return SizedBox(
                width: 140,
                child: MediaCard(
                  item: item,
                  onTap: () {
                    context.push(
                      '/details/${item.providerId}/${Uri.encodeComponent(item.id)}',
                    );
                  },
                ),
              );
            },
          ),
        ),
      ],
    );
  }

  Widget _buildSkeleton() {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        const Skeleton(width: 100, height: 24),
        const SizedBox(height: 16),
        SizedBox(
          height: 220,
          child: ListView.separated(
            scrollDirection: Axis.horizontal,
            itemCount: 5,
            separatorBuilder: (context, index) => const SizedBox(width: 12),
            itemBuilder: (context, index) {
              return const SizedBox(
                width: 140,
                child: Skeleton(width: 140, height: 220, borderRadius: 12),
              );
            },
          ),
        ),
      ],
    );
  }
}
