import '../entities/entities.dart';

/// Abstract interface for content providers (sources)
///
/// Each catalogue source the user connects to must implement this interface
abstract class ContentProvider {
  /// Unique identifier for this provider
  String get id;

  /// Display name of the provider
  String get name;

  /// Provider icon/logo URL (optional)
  String? get iconUrl;

  /// Base URL of the provider (default/fallback)
  String get baseUrl;

  /// Effective base URL (may be different if domain changed via redirect)
  ///
  /// Providers should use this for API requests. By default returns [baseUrl],
  /// but can be overridden to use resolved URL from ProviderRegistry.
  String get effectiveBaseUrl => baseUrl;

  /// Whether this provider is currently enabled
  bool get isEnabled;

  /// Whether this provider should appear on the home page / in general categories.
  ///
  /// Providers with a dedicated page return `false` here. Default `true`.
  bool get showOnHome => true;

  /// Whether this provider's streams are fixed (quality/voiceover cannot be
  /// changed after playback starts). Default `false`.
  bool get hasFixedStreams => false;

  /// Content types supported by this provider
  List<ContentType> get supportedTypes;

  /// Search for media content
  ///
  /// [query] - search term
  /// [type] - optional filter by content type
  /// [page] - page number for pagination (1-indexed)
  Future<List<MediaItem>> search(
    String query, {
    ContentType? type,
    int page = 1,
  });

  /// Get detailed information about a specific media item
  ///
  /// [id] - media ID from this provider
  Future<MediaDetails> getDetails(String id);

  /// Get available streaming sources for playback
  ///
  /// [id] - media ID
  /// [season] - season number (for series)
  /// [episode] - episode number (for series)
  Future<List<StreamSource>> getStreams(String id, {int? season, int? episode});

  /// Get popular/trending content
  ///
  /// [type] - optional filter by content type
  /// [page] - page number for pagination
  Future<List<MediaItem>> getPopular({ContentType? type, int page = 1});

  /// Get new/recently added content
  ///
  /// [type] - optional filter by content type
  /// [page] - page number for pagination
  Future<List<MediaItem>> getNew({ContentType? type, int page = 1});

  /// Get category/genre listings (if supported)
  Future<List<String>> getCategories() async => [];

  /// Get content by category/genre
  Future<List<MediaItem>> getByCategory(
    String category, {
    ContentType? type,
    int page = 1,
  }) async => [];

  /// Get similar content based on current item details
  ///
  /// Providers can implement this using genre matching, tags, or specific API endpoints.
  Future<List<MediaItem>> getSimilar(String id, MediaDetails details) async =>
      [];
}
