import '../../domain/entities/media_item.dart';

/// One entry of the backend provider catalog (GET /api/v1/content/providers).
class ProviderCatalogEntry {
  final String id;
  final String name;
  final String baseUrl;
  final String? iconUrl;
  final bool showOnHome;
  final bool hasFixedStreams;
  final List<String> contentTypes;
  final bool searchEnabledDefault;
  final bool enabled;
  final bool healthy;

  const ProviderCatalogEntry({
    required this.id,
    required this.name,
    required this.baseUrl,
    this.iconUrl,
    required this.showOnHome,
    required this.hasFixedStreams,
    required this.contentTypes,
    required this.searchEnabledDefault,
    required this.enabled,
    required this.healthy,
  });

  factory ProviderCatalogEntry.fromJson(Map<String, dynamic> json) {
    return ProviderCatalogEntry(
      id: json['id'] as String? ?? '',
      name: json['name'] as String? ?? '',
      baseUrl: json['baseUrl'] as String? ?? '',
      iconUrl: json['iconUrl'] as String?,
      showOnHome: json['showOnHome'] as bool? ?? true,
      hasFixedStreams: json['hasFixedStreams'] as bool? ?? false,
      contentTypes:
          (json['contentTypes'] as List?)?.map((e) => e.toString()).toList() ??
          const ['movie', 'series'],
      searchEnabledDefault: json['searchEnabledDefault'] as bool? ?? true,
      enabled: json['enabled'] as bool? ?? true,
      healthy: json['healthy'] as bool? ?? true,
    );
  }

  Map<String, dynamic> toJson() => {
    'id': id,
    'name': name,
    'baseUrl': baseUrl,
    if (iconUrl != null) 'iconUrl': iconUrl,
    'showOnHome': showOnHome,
    'hasFixedStreams': hasFixedStreams,
    'contentTypes': contentTypes,
    'searchEnabledDefault': searchEnabledDefault,
    'enabled': enabled,
    'healthy': healthy,
  };

  /// Map backend type keys to app [ContentType].
  List<ContentType> get supportedTypes => contentTypes
      .map(
        (t) => ContentType.values.firstWhere(
          (c) => c.name == t,
          orElse: () => ContentType.unknown,
        ),
      )
      .where((c) => c != ContentType.unknown)
      .toList();
}

/// Parsed catalog response with version for cache invalidation.
class ProviderCatalog {
  final int version;
  final List<ProviderCatalogEntry> providers;

  const ProviderCatalog({required this.version, required this.providers});

  factory ProviderCatalog.fromJson(Map<String, dynamic> json) {
    return ProviderCatalog(
      version: (json['version'] as num?)?.toInt() ?? 0,
      providers:
          (json['providers'] as List?)
              ?.whereType<Map>()
              .map(
                (e) =>
                    ProviderCatalogEntry.fromJson(Map<String, dynamic>.from(e)),
              )
              .where((e) => e.id.isNotEmpty)
              .toList() ??
          const [],
    );
  }

  Map<String, dynamic> toJson() => {
    'version': version,
    'providers': providers.map((e) => e.toJson()).toList(),
  };
}
