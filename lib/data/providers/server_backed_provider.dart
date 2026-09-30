import 'package:get_it/get_it.dart';

import '../../core/config/app_config.dart';
import '../../core/network/api_client.dart';
import '../../core/utils/logger.dart';
import '../../domain/entities/entities.dart';
import '../../domain/repositories/content_provider.dart';
import '../models/provider_catalog.dart';

/// Content provider backed by the Oxide backend (server-side parsing).
///
/// Created automatically for every catalog entry that has no local
/// implementation — so a provider added in backend Go code appears
/// in the app without an app update.
///
/// Search/details/streams are proxied through `/api/v1/content/*`.
/// Catalog/popular listings are not supported server-side yet,
/// so [getPopular]/[getNew]/[getByCategory] return empty lists.
class ServerBackedProvider extends ContentProvider {
  static const _tag = 'ServerBackedProvider';

  final ProviderCatalogEntry entry;

  ApiClient get _api => GetIt.instance<ApiClient>();

  ServerBackedProvider(this.entry);

  @override
  String get id => entry.id;

  @override
  String get name => entry.name;

  @override
  String? get iconUrl => entry.iconUrl;

  @override
  String get baseUrl => entry.baseUrl;

  @override
  bool get isEnabled => true;

  @override
  List<ContentType> get supportedTypes => entry.supportedTypes;

  String get _base => '${AppConfig.serverApiUrl}/content';

  @override
  Future<List<MediaItem>> search(
    String query, {
    ContentType? type,
    int page = 1,
  }) async {
    try {
      final list = await _api.getJsonList(
        '$_base/search',
        queryParameters: {'q': query, 'provider': id},
      );
      final items = list
          .whereType<Map>()
          .map((e) => _mapItem(Map<String, dynamic>.from(e)))
          .where((i) => i.title.isNotEmpty)
          .toList();
      if (type != null) {
        return items.where((i) => i.type == type).toList();
      }
      return items;
    } catch (e) {
      Logger.w('Server search failed for $id: $e', tag: _tag);
      return [];
    }
  }

  @override
  Future<MediaDetails> getDetails(String id) async {
    final res = await _api.getJson(
      '$_base/details',
      queryParameters: {'provider': this.id, 'url': id},
    );
    return _mapDetails(res);
  }

  @override
  Future<List<StreamSource>> getStreams(
    String id, {
    int? season,
    int? episode,
  }) async {
    try {
      final params = <String, dynamic>{'provider': this.id, 'url': id};
      if (season != null) params['season'] = season;
      if (episode != null) params['episode'] = episode;
      final res = await _api.getJson('$_base/streams', queryParameters: params);
      final streams = res['streams'];
      if (streams is! List) return [];
      return streams
          .whereType<Map>()
          .map((e) => _mapStream(Map<String, dynamic>.from(e)))
          .toList();
    } catch (e) {
      Logger.w('Server streams failed for $id: $e', tag: _tag);
      return [];
    }
  }

  @override
  Future<List<MediaItem>> getPopular({ContentType? type, int page = 1}) async {
    try {
      final params = <String, dynamic>{'provider': id, 'page': page};
      if (type != null) params['type'] = type.name;
      final list = await _api.getJsonList(
        '$_base/popular',
        queryParameters: params,
      );
      return list
          .whereType<Map>()
          .map((e) => _mapItem(Map<String, dynamic>.from(e)))
          .where((i) => i.title.isNotEmpty)
          .toList();
    } catch (e) {
      Logger.w('Server getPopular failed for $id: $e', tag: _tag);
      return [];
    }
  }

  @override
  Future<List<MediaItem>> getNew({ContentType? type, int page = 1}) async =>
      getPopular(type: type, page: page);

  @override
  Future<List<MediaItem>> getByCategory(
    String category, {
    ContentType? type,
    int page = 1,
  }) async {
    try {
      final params = <String, dynamic>{
        'provider': id,
        'category': category,
        'page': page,
      };
      if (type != null) params['type'] = type.name;
      final list = await _api.getJsonList(
        '$_base/category',
        queryParameters: params,
      );
      return list
          .whereType<Map>()
          .map((e) => _mapItem(Map<String, dynamic>.from(e)))
          .where((i) => i.title.isNotEmpty)
          .toList();
    } catch (e) {
      Logger.w('Server getByCategory failed for $id ($category): $e', tag: _tag);
      return [];
    }
  }

  // -- mapping ---------------------------------------------------------------

  /// Server item URL is used as the app-side id (details/streams need the URL).
  MediaItem _mapItem(Map<String, dynamic> json) {
    final url = json['url'] as String? ?? '';
    return MediaItem(
      id: url.isNotEmpty ? url : (json['id'] as String? ?? ''),
      providerId: id,
      title: json['title'] as String? ?? '',
      originalTitle: json['original_title'] as String?,
      posterUrl: json['poster_url'] as String?,
      year: (json['year'] as num?)?.toInt(),
      rating: (json['rating'] as num?)?.toDouble(),
      ratingSource: 'Site',
      type: _mapType(json['type'] as String?),
    );
  }

  MediaDetails _mapDetails(Map<String, dynamic> json) {
    final item = _mapItem(json);
    final seasonsJson = json['seasons'];
    List<Season>? seasons;
    if (seasonsJson is List) {
      seasons = seasonsJson.whereType<Map>().map((s) {
        final sm = Map<String, dynamic>.from(s);
        final eps = sm['episodes'];
        return Season(
          number: (sm['number'] as num?)?.toInt() ?? 0,
          title: sm['title'] as String?,
          episodes: eps is List
              ? eps.whereType<Map>().map((e) {
                  final em = Map<String, dynamic>.from(e);
                  return Episode(
                    number: (em['number'] as num?)?.toInt() ?? 0,
                    title: em['title'] as String?,
                  );
                }).toList()
              : const [],
        );
      }).toList();
    }
    final genres = json['genres'];
    return MediaDetails(
      item: item,
      fullDescription: json['description'] as String?,
      genres: genres is List ? genres.map((e) => e.toString()).toList() : null,
      seasons: seasons,
    );
  }

  StreamSource _mapStream(Map<String, dynamic> json) {
    final direct = json['direct_url'] as String?;
    final url =
        (direct?.isNotEmpty == true ? direct : json['url']) as String? ?? '';
    final headers = json['headers'];
    return StreamSource(
      url: url,
      quality: _mapQuality(json['quality'] as String?),
      type: _mapStreamType(url),
      headers: headers is Map
          ? headers.map((k, v) => MapEntry(k.toString(), v.toString()))
          : null,
    );
  }

  ContentType _mapType(String? t) => ContentType.values.firstWhere(
    (c) => c.name == t,
    orElse: () => ContentType.unknown,
  );

  StreamQuality _mapQuality(String? q) {
    final s = (q ?? '').toLowerCase();
    if (s.contains('4k') || s.contains('2160')) return StreamQuality.q4k;
    if (s.contains('1440')) return StreamQuality.q1440p;
    if (s.contains('1080')) return StreamQuality.q1080p;
    if (s.contains('720')) return StreamQuality.q720p;
    if (s.contains('480')) return StreamQuality.q480p;
    if (s.contains('360')) return StreamQuality.q360p;
    return StreamQuality.unknown;
  }

  StreamType _mapStreamType(String url) {
    final u = url.toLowerCase();
    if (u.endsWith('.m3u8') || u.contains('.m3u8?')) return StreamType.hls;
    if (u.endsWith('.mpd') || u.contains('.mpd?')) return StreamType.dash;
    if (u.startsWith('magnet:')) return StreamType.torrent;
    return StreamType.direct;
  }
}
