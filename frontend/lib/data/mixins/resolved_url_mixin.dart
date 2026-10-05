import 'package:get_it/get_it.dart';

import '../providers/provider_registry.dart';

/// Mixin for providers to get resolved base URL from ProviderRegistry
///
/// This mixin provides [effectiveBaseUrl] that automatically uses
/// the resolved URL if domain has changed (detected via HTTP redirects).
mixin ResolvedUrlMixin {
  /// Provider ID - must be implemented by the class using this mixin
  String get id;

  /// Default base URL - must be implemented by the class using this mixin
  String get baseUrl;

  /// Get the effective base URL (resolved or default)
  ///
  /// Returns the resolved URL from ProviderRegistry if available,
  /// otherwise falls back to the default [baseUrl].
  String get effectiveBaseUrl {
    try {
      final registry = GetIt.instance<ProviderRegistry>();
      final resolved = registry.getResolvedUrl(id);
      return resolved.isNotEmpty ? resolved : baseUrl;
    } catch (_) {
      return baseUrl;
    }
  }
}
