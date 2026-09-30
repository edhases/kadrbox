import 'dart:convert';
import 'package:dio/dio.dart';
import 'package:flutter/foundation.dart' show visibleForTesting;
import 'package:get_it/get_it.dart';

import '../../core/config/app_config.dart';
import '../../core/network/api_client.dart';
import '../../core/utils/logger.dart';
import '../../domain/entities/entities.dart';
import '../../domain/repositories/content_provider.dart';
import '../models/provider_catalog.dart';
import '../services/search/search_envelope.dart';

/// Outcome of probing a stream URL with a HEAD (or ranged GET) request.
///
/// A probe answers one question: will libmpv get media bytes, or an HTML
/// page? `Failed to recognize file format.` in the field almost always means
/// the server handed back markup.
enum StreamProbeResult {
  /// `.m3u8` playlist — playable as HLS.
  hls,

  /// `.mpd` manifest — playable as DASH.
  dash,

  /// A media container (or an extension-less URL that serves one).
  direct,

  /// The URL serves HTML — a player page, captcha or login wall.
  iframe,

  /// 401/403 — missing Referer/UA or an expired token.
  forbidden,

  /// 404/410/5xx — the resource is gone.
  dead,

  /// DNS/TCP/TLS/timeout failure.
  unreachable,

  /// Probe could not run at all (unsupported URL, client error).
  unknown,
}

/// A probed stream: what it turned out to be, plus why.
class StreamProbe {
  const StreamProbe({
    required this.url,
    required this.result,
    this.httpStatus,
    this.contentType,
  });

  final String url;
  final StreamProbeResult result;
  final int? httpStatus;
  final String? contentType;

  /// True when this URL is safe to hand to libmpv.
  bool get isPlayable =>
      result == StreamProbeResult.hls ||
      result == StreamProbeResult.dash ||
      result == StreamProbeResult.direct;

  @override
  String toString() =>
      'StreamProbe(${result.name}, status=$httpStatus, type=$contentType)';
}

/// Content provider backed by the Oxide backend (server-side parsing).
///
/// Created automatically for every catalog entry that has no local
/// implementation — so a provider added in backend Go code appears
/// in the app without an app update.
///
/// Search/details/streams are proxied through `/api/v1/content/*`.
/// Catalog/popular listings are not supported server-side yet,
/// so [getPopular]/[getNew]/[getByCategory] return empty lists.
///
/// ## Error contract
///
/// [search], [getStreams], [getPopular], [getNew] and [getByCategory] propagate
/// transport and server errors (`NetworkException` / `ServerException` from
/// `ApiClient`) instead of returning `[]`. An empty list therefore means
/// "the server answered successfully with no results", which is a
/// distinguishable state. Callers that aggregate several providers must wrap
/// these calls in a try/catch (search_service, home_page, category_page,
/// provider_page and recommendation_service already do).
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
  bool get showOnHome => entry.showOnHome;

  @override
  bool get hasFixedStreams => entry.hasFixedStreams;

  @override
  List<ContentType> get supportedTypes => entry.supportedTypes;

  String get _base => '${AppConfig.serverApiUrl}/content';

  /// Server-side intelligent search.
  ///
  /// Unlike [search], this omits the `provider` parameter, which makes the
  /// backend run the whole pipeline: one upstream call per segment, relevance
  /// scoring with a hard cutoff, and cross-provider clustering. The client
  /// receives an already-ranked, already-deduplicated set and only renders it.
  ///
  /// Errors propagate — a transport failure must stay distinguishable from
  /// a genuine "no results", otherwise the UI shows an empty screen with no
  /// diagnostic and the user cannot tell a dead backend from an empty catalogue.
  ///
  /// [page] is accepted for call-site compatibility but deliberately NOT sent.
  /// The backend currently reads only `q` and keys its cache on the query plan
  /// hash alone, so sending `page` would silently return page 1 again for every
  /// page. Content-type narrowing is therefore applied here, client-side.
  ///
  /// A response shape that is neither the current envelope nor the legacy flat
  /// list THROWS. Returning an empty envelope there would render a successful
  /// search that found nothing — the exact silent failure this method is
  /// supposed to make impossible.
  Future<SearchEnvelope> searchEnvelope(
    String query, {
    ContentType? type,
    int page = 1,
  }) async {
    final rawData = await _api.getRawJson(
      '$_base/search',
      queryParameters: {'q': query},
    );

    SearchEnvelope envelope;
    if (rawData is Map) {
      envelope = SearchEnvelope.fromJson(
        Map<String, dynamic>.from(rawData),
        _mapItem,
      );
    } else if (rawData is List) {
      envelope = _legacyEnvelope(query, rawData);
    } else {
      throw FormatException(
        'Unexpected /content/search response: '
        '${rawData.runtimeType} (${rawData.toString().length} chars)',
        query,
      );
    }

    if (type != null) {
      envelope = SearchEnvelope(
        query: envelope.query,
        canonical: envelope.canonical,
        tookMs: envelope.tookMs,
        segments: envelope.segments,
        items: envelope.items
            .where((s) => s.item.type == type)
            .toList(growable: false),
        filteredOut: envelope.filteredOut,
        nextPage: envelope.nextPage,
        hasMore: envelope.hasMore,
      );
    }
    return envelope;
  }

  /// Wrap a pre-envelope (flat `List<MediaItem>`) response into an envelope.
  ///
  /// This exists only so the app keeps working against a backend that has not
  /// been redeployed yet. It deliberately invents NOTHING:
  ///
  /// * `score` is 0, not 1.0 — the old server never scored anything, and a
  ///   fabricated perfect score would make unranked garbage look authoritative.
  /// * `matchedBy` is 'legacy' — an honest marker, not one of the server's
  ///   documented values.
  /// * `sources` is empty and the segment reports no sources. A flat list
  ///   genuinely carries no source attribution, so the UI correctly shows no
  ///   per-source chips instead of inventing a "bandera" source that the
  ///   server never mentioned.
  ///
  /// Delete this once every deployed backend runs the envelope contract.
  SearchEnvelope _legacyEnvelope(String query, List<dynamic> rawData) {
    Logger.w(
      'Backend returned a legacy flat search list; it is out of date. '
      'Scores, dedup and per-source stats are unavailable until it is '
      'redeployed.',
      tag: 'Search',
    );

    final items = <ScoredMediaItem>[];
    for (final raw in rawData) {
      if (raw is! Map) continue;
      final item = _mapItem(Map<String, dynamic>.from(raw));
      items.add(
        ScoredMediaItem(
          item: item,
          score: 0,
          matchedBy: 'legacy',
          clusterKey: item.id,
          sources: const [],
        ),
      );
    }

    return SearchEnvelope(
      query: query,
      canonical: query,
      tookMs: 0,
      segments: [
        SearchSegment(
          id: 'legacy',
          status: 'unknown',
          count: items.length,
          // No sources: the legacy shape never reported any. An empty map
          // keeps `askedCount` at 0, which the stats line treats as
          // "not reported" rather than "0 of 0".
        ),
      ],
      items: items,
      filteredOut: 0,
    );
  }

  @override
  Future<List<MediaItem>> search(
    String query, {
    ContentType? type,
    int page = 1,
  }) async {
    // Legacy per-provider path. Kept for callers that genuinely need one
    // provider; the search UI uses [searchEnvelope] instead.
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
  }

  @override
  Future<List<MediaItem>> getPopular({ContentType? type, int page = 1}) async {
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
  }

  // --- mapping ---------------------------------------------------------------

  /// Test seam for [_mapStreamType].
  @visibleForTesting
  StreamType mapStreamTypeForTest(String url) => _mapStreamType(url);

  /// Test seam for [_mapStream].
  @visibleForTesting
  StreamSource mapStreamForTest(Map<String, dynamic> json) => _mapStream(json);

  /// Test seam for [_mapDetails].
  @visibleForTesting
  MediaDetails mapDetailsForTest(Map<String, dynamic> json) =>
      _mapDetails(json);

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
                  final rawRef = em['stream_ref'] ?? em['url'];
                  final String? refStr = rawRef is String
                      ? rawRef
                      : (rawRef != null ? jsonEncode(rawRef) : null);
                  return Episode(
                    number: (em['number'] as num?)?.toInt() ?? 0,
                    title: em['title'] as String?,
                    streamRef: refStr,
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
    // Guarded: `url` may arrive as a non-String (number, null, object) and an
    // unguarded `as String?` cast inside `.toLowerCase()` throws a TypeError
    // that swallows the whole stream list.
    final direct = _asString(json['direct_url']);
    final url = (direct != null && direct.isNotEmpty)
        ? direct
        : (_asString(json['url']) ?? '');
    final headers = json['headers'];
    final language = _asString(json['language']);
    final voiceover =
        _asString(json['voiceover']) ??
        _asString(json['audio']) ??
        _asString(json['dub']);
    return StreamSource(
      url: url,
      quality: _mapQuality(_asString(json['quality'])),
      type: _mapStreamType(url),
      language: language,
      voiceover: voiceover,
      headers: headers is Map
          ? headers.map((k, v) => MapEntry(k.toString(), v.toString()))
          : null,
    );
  }

  /// Type-safe String coercion for values coming from the backend.
  static String? _asString(Object? value) {
    if (value == null) return null;
    if (value is String) return value;
    return value.toString();
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

  /// Extensions that are unambiguously playable media.
  static const Set<String> _mediaExtensions = {
    '.m3u8',
    '.mpd',
    '.mp4',
    '.m4v',
    '.mkv',
    '.webm',
    '.mov',
    '.avi',
    '.ts',
    '.m4s',
    '.mp3',
    '.aac',
    '.flac',
    '.ogg',
  };

  /// Path segments that mark a URL as a player page rather than media.
  static const List<String> _playerPathMarkers = [
    '/embed',
    '/player',
    '/iframe',
    '/watch',
    '/video_frame',
    '/videoplayer',
  ];

  /// Classifies a stream URL into a [StreamType].
  ///
  /// Parses `Uri.parse(url).path` rather than the raw string: the previous
  /// `endsWith`/`contains` approach could not tell an HTML page from media,
  /// missed extension-less HLS endpoints, and treated `.m3u8` appearing in a
  /// query string as HLS.
  StreamType _mapStreamType(String url) {
    if (url.isEmpty) return StreamType.direct;

    final lower = url.toLowerCase();
    if (lower.startsWith('magnet:')) return StreamType.torrent;

    final path = _pathOf(url);
    final ext = _extensionOf(path);

    // Player-page shapes are HTML by definition — never classify as playable
    // even if they happen to contain a media-looking substring.
    if (_looksLikePlayerPage(path)) return StreamType.iframe;

    // Check the *path* extension only, so `?next=/x.m3u8` cannot win.
    if (ext == '.m3u8') return StreamType.hls;
    if (ext == '.mpd') return StreamType.dash;
    if (_mediaExtensions.contains(ext)) return StreamType.direct;

    // A trailing slash on a path that is not `/` means a directory/HTML page.
    if (path.endsWith('/')) return StreamType.iframe;

    // Extension-less: could be HLS or a player page. Leave it as direct and
    // let the probe decide on demand.
    return StreamType.direct;
  }

  /// Path component of [url], or an empty string when it cannot be parsed.
  static String _pathOf(String url) {
    try {
      return Uri.parse(url).path.toLowerCase();
    } catch (_) {
      return '';
    }
  }

  /// Lowercased extension of [path] including the dot, or '' when there is
  /// none. Multi-part extensions like `.tar.gz` are not special-cased here.
  static String _extensionOf(String path) {
    final slash = path.lastIndexOf('/');
    final lastSegment = slash >= 0 ? path.substring(slash + 1) : path;
    final dot = lastSegment.lastIndexOf('.');
    if (dot <= 0 || dot == lastSegment.length - 1) return '';
    return lastSegment.substring(dot);
  }

  /// True when [path] has the shape of an HTML player page.
  static bool _looksLikePlayerPage(String path) {
    if (path.isEmpty) return false;
    if (path.endsWith('/')) return true;
    for (final marker in _playerPathMarkers) {
      if (path.contains(marker)) return true;
    }
    return false;
  }

  // --- stream probing -------------------------------------------------------

  /// Per-session probe cache, keyed by URL.
  ///
  /// A stream is probed at most once per app run.
  static final Map<String, StreamProbe> _probeCache = {};

  /// Returns a cached probe result, or null when this URL was never probed.
  static StreamProbe? cachedProbe(String url) => _probeCache[url];

  /// Probes [stream] to find out what the URL really serves.
  ///
  /// Lazy and on-demand only — never called while rendering a list. Call it
  /// from the error path or from a user-triggered "check sources" action.
  ///
  /// Returns null when the URL is already unambiguously media by extension
  /// (probing those is pure cost).
  Future<StreamProbe?> probeStream(StreamSource stream) async {
    final url = stream.url;
    if (url.isEmpty) return null;

    final cached = _probeCache[url];
    if (cached != null) return cached;

    // Skip the probe entirely when the extension already tells us the truth.
    if (_mapStreamType(url) != StreamType.direct ||
        _extensionOf(_pathOf(url)).isNotEmpty) {
      Logger.d('Probe skipped, extension is conclusive: $url', tag: _tag);
      return null;
    }

    final probe = await _probeUrl(url, stream.headers);
    _probeCache[url] = probe;
    Logger.i('Probe $url -> $probe', tag: _tag);
    return probe;
  }

  Future<StreamProbe> _probeUrl(
    String url,
    Map<String, String>? headers,
  ) async {
    try {
      final uri = Uri.parse(url);
      if (!uri.hasScheme || uri.host.isEmpty) {
        return StreamProbe(url: url, result: StreamProbeResult.unknown);
      }

      final response = await _api.dio.head<Object?>(
        url,
        options: Options(
          headers: {...?headers},
          responseType: ResponseType.bytes,
          followRedirects: true,
          validateStatus: (s) => s != null && s < 400,
        ),
      );
      return _classifyProbe(
        url,
        response.statusCode ?? 0,
        _contentTypeOf(response),
      );
    } on DioException catch (e) {
      final status = e.response?.statusCode;

      // Some CDNs reject HEAD outright (405) — fall back to a ranged GET.
      if (status == 405 ||
          status == 501 ||
          status == null && _isMethodIssue(e)) {
        return _probeUrlWithGet(url, headers, e);
      }
      if (status != null) {
        return _classifyProbe(url, status, _contentTypeOf(e.response));
      }
      return StreamProbe(
        url: url,
        result: StreamProbeResult.unreachable,
        httpStatus: null,
      );
    } catch (e) {
      Logger.w('Probe failed for $url: $e', tag: _tag);
      return StreamProbe(url: url, result: StreamProbeResult.unknown);
    }
  }

  Future<StreamProbe> _probeUrlWithGet(
    String url,
    Map<String, String>? headers,
    DioException headError,
  ) async {
    try {
      final response = await _api.dio.get<List<int>>(
        url,
        options: Options(
          headers: {...?headers, 'Range': 'bytes=0-1023'},
          responseType: ResponseType.bytes,
          followRedirects: true,
          validateStatus: (s) => s != null && s < 400,
        ),
      );
      final status = response.statusCode ?? 0;
      final probe = _classifyProbe(url, status, _contentTypeOf(response));
      if (probe.result != StreamProbeResult.unknown) return probe;
      // No Content-Type: sniff the first bytes for known magic numbers.
      return _classifyByMagicNumber(url, status, response.data);
    } catch (e) {
      // If the GET also failed, classify from that failure.
      if (e is DioException && e.response != null) {
        return _classifyProbe(
          url,
          e.response!.statusCode ?? 0,
          _contentTypeOf(e.response),
        );
      }
      return StreamProbe(
        url: url,
        result: StreamProbeResult.unreachable,
        httpStatus: headError.response?.statusCode,
      );
    }
  }

  static bool _isMethodIssue(DioException e) {
    final m = (e.message ?? '').toLowerCase();
    return m.contains('405') || m.contains('method');
  }

  static String? _contentTypeOf(Response<dynamic>? response) {
    final ct = response?.headers.value('content-type');
    if (ct == null || ct.isEmpty) return null;
    return ct.toLowerCase();
  }

  /// Maps an HTTP status + Content-Type onto a [StreamProbeResult].
  static StreamProbe _classifyProbe(
    String url,
    int status,
    String? contentType,
  ) {
    if (status == 401 || status == 403) {
      return StreamProbe(
        url: url,
        result: StreamProbeResult.forbidden,
        httpStatus: status,
        contentType: contentType,
      );
    }
    if (status == 404 || status == 410 || status >= 500) {
      return StreamProbe(
        url: url,
        result: StreamProbeResult.dead,
        httpStatus: status,
        contentType: contentType,
      );
    }
    if (status == 405 || status == 501) {
      return StreamProbe(
        url: url,
        result: StreamProbeResult.unknown,
        httpStatus: status,
        contentType: contentType,
      );
    }
    if (status < 200 || status >= 400) {
      return StreamProbe(
        url: url,
        result: StreamProbeResult.dead,
        httpStatus: status,
        contentType: contentType,
      );
    }

    final ct = contentType ?? '';
    if (ct.contains('mpegurl') || ct.contains('m3u8')) {
      return StreamProbe(
        url: url,
        result: StreamProbeResult.hls,
        httpStatus: status,
        contentType: contentType,
      );
    }
    if (ct.contains('dash+xml')) {
      return StreamProbe(
        url: url,
        result: StreamProbeResult.dash,
        httpStatus: status,
        contentType: contentType,
      );
    }
    if (ct.contains('text/html') ||
        ct.contains('application/xhtml') ||
        ct.contains('text/plain') && ct.contains('html')) {
      return StreamProbe(
        url: url,
        result: StreamProbeResult.iframe,
        httpStatus: status,
        contentType: contentType,
      );
    }
    if (ct.startsWith('video/') || ct.startsWith('audio/')) {
      return StreamProbe(
        url: url,
        result: StreamProbeResult.direct,
        httpStatus: status,
        contentType: contentType,
      );
    }
    if (ct.contains('octet-stream')) {
      return StreamProbe(
        url: url,
        result: StreamProbeResult.direct,
        httpStatus: status,
        contentType: contentType,
      );
    }

    // 2xx with no usable Content-Type — caller may sniff bytes.
    return StreamProbe(
      url: url,
      result: StreamProbeResult.unknown,
      httpStatus: status,
      contentType: contentType,
    );
  }

  /// Identifies container/manifest formats from the first bytes of a body.
  static StreamProbe _classifyByMagicNumber(
    String url,
    int status,
    List<int>? bytes,
  ) {
    if (bytes == null || bytes.isEmpty) {
      return StreamProbe(
        url: url,
        result: StreamProbeResult.unknown,
        httpStatus: status,
      );
    }

    final head = String.fromCharCodes(bytes.take(64)).trimLeft().toLowerCase();

    if (head.startsWith('#extm3u')) {
      return StreamProbe(
        url: url,
        result: StreamProbeResult.hls,
        httpStatus: status,
      );
    }
    if (head.startsWith('<?xml') || head.contains('<mpd')) {
      return StreamProbe(
        url: url,
        result: StreamProbeResult.dash,
        httpStatus: status,
      );
    }
    if (head.startsWith('<!doctype html') || head.startsWith('<html')) {
      return StreamProbe(
        url: url,
        result: StreamProbeResult.iframe,
        httpStatus: status,
      );
    }
    // ISO base media (ftyp) — mp4/m4v/mov/m4a.
    if (bytes.length > 12 &&
        bytes[4] == 0x66 &&
        bytes[5] == 0x74 &&
        bytes[6] == 0x79 &&
        bytes[7] == 0x70) {
      return StreamProbe(
        url: url,
        result: StreamProbeResult.direct,
        httpStatus: status,
      );
    }
    // Matroska/WebM EBML header.
    if (bytes.length > 3 &&
        bytes[0] == 0x1A &&
        bytes[1] == 0x45 &&
        bytes[2] == 0xDF &&
        bytes[3] == 0xA3) {
      return StreamProbe(
        url: url,
        result: StreamProbeResult.direct,
        httpStatus: status,
      );
    }

    return StreamProbe(
      url: url,
      result: StreamProbeResult.unknown,
      httpStatus: status,
    );
  }
}
