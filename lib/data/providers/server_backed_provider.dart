import 'dart:convert';
import 'package:dio/dio.dart';
import 'package:flutter/foundation.dart' show visibleForTesting;
import 'package:get_it/get_it.dart';

import '../../core/config/app_config.dart';
import '../../core/error/exceptions.dart';
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
/// Search/details/streams/popular/category are proxied through
/// `/api/v1/content/*`; the backend owns parsing, scoring and clustering.
///
/// ## Error contract
///
/// [search], [getStreams], [getPopular], [getNew] and [getByCategory] propagate
/// transport and server errors (`NetworkException` / `ServerException` from
/// `ApiClient`) instead of returning `[]`. An empty list therefore means
/// "the server answered successfully with no results", which is a
/// distinguishable state. Callers that aggregate several providers must wrap
/// these calls in a try/catch (home_page, category_page,
/// provider_page and recommendation_service already do).
///
/// ## Request hygiene
///
/// Home startup fires the same popular/details URL from several uncoordinated
/// paths at once (home grid, recommendations, catalog sync). Identical
/// concurrent reads therefore share one network call ([_coalesce]). Details
/// bodies are additionally cached briefly ([_detailsTtl]) because the same
/// 47KB payload was refetched a minute later. Identical 5xx/network failures
/// on idempotent catalog reads are suppressed briefly ([_failureTtl]) so a
/// burst from several UI paths does not hammer a struggling upstream.
/// Signed stream URLs are never cached — only coalesced while in flight.
class ServerBackedProvider extends ContentProvider {
  static const _tag = 'ServerBackedProvider';

  final ProviderCatalogEntry entry;

  ApiClient get _api => GetIt.instance<ApiClient>();

  ServerBackedProvider(this.entry, {DateTime Function()? clock})
    : _now = clock ?? DateTime.now;

  final DateTime Function() _now;

  /// Identical reads currently awaiting a response, keyed by a string that
  /// captures everything actually sent to the server.
  final Map<String, Future<Object?>> _inFlight = {};

  /// Recently fetched details, by request key. Catalog metadata is stable on
  /// a two-minute timescale; signed playback URLs are never stored here.
  final Map<String, _Timed<MediaDetails>> _detailsCache = {};
  static const Duration _detailsTtl = Duration(seconds: 120);

  /// Recent 5xx/network failures on idempotent catalog reads, by request key.
  /// Short enough that an explicit user retry still reaches the server.
  final Map<String, _Timed<Object>> _failureCache = {};
  static const Duration _failureTtl = Duration(seconds: 30);

  /// Run [load], or join an identical read that is already in flight.
  ///
  /// Every waiter gets the same result or the same error. The entry is
  /// removed as soon as the future settles, so this only merges genuinely
  /// concurrent calls — sequential calls always hit the network (or the
  /// caches above) again.
  Future<T> _coalesce<T>(String key, Future<T> Function() load) {
    final existing = _inFlight[key];
    if (existing is Future<T>) return existing;
    final future = load();
    _inFlight[key] = future;
    // Both callbacks consume the outcome, so the derived future always
    // completes normally. Using whenComplete here instead would leave an
    // unlistened erroring future behind on every shared failure — an
    // unhandled async error in production.
    future.then<void>(
      (_) {
        if (identical(_inFlight[key], future)) _inFlight.remove(key);
      },
      onError: (_) {
        if (identical(_inFlight[key], future)) _inFlight.remove(key);
      },
    );
    return future;
  }

  /// Re-run [load], suppressing it when the identical read failed with a
  /// retryable error moments ago. 4xx, auth and parsing errors are never
  /// suppressed: those are either the caller's fault or permanent.
  Future<T> _guarded<T>(String key, Future<T> Function() load) {
    final failure = _failureCache[key];
    if (failure != null) {
      if (_now().isAfter(failure.until)) {
        _failureCache.remove(key);
      } else {
        throw failure.value;
      }
    }
    return _coalesce(key, () async {
      try {
        return await load();
      } catch (e) {
        if (e is NetworkException ||
            (e is ServerException && e.statusCode >= 500)) {
          _failureCache[key] = _Timed(e, _now().add(_failureTtl));
        }
        rethrow;
      }
    });
  }

  MediaDetails? _cachedDetails(String key) {
    final entry = _detailsCache[key];
    if (entry == null) return null;
    if (_now().isAfter(entry.until)) {
      _detailsCache.remove(key);
      return null;
    }
    return entry.value;
  }

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
    // The key covers what is actually sent: `type` is narrowed client-side
    // afterwards and `page` is deliberately not sent at all.
    final envelope = await _coalesce('search:$query', () async {
      final rawData = await _api.getRawJson(
        '$_base/search',
        queryParameters: {'q': query},
      );

      final payload = (rawData is Map && rawData['data'] is Map)
          ? rawData['data']
          : (rawData is Map && rawData['data'] is List)
          ? rawData['data']
          : rawData;

      if (payload is Map) {
        return SearchEnvelope.fromJson(
          Map<String, dynamic>.from(payload),
          _mapItem,
        );
      } else if (payload is List) {
        return _legacyEnvelope(query, payload);
      } else {
        throw FormatException(
          'Unexpected /content/search response: '
          '${rawData.runtimeType} (${rawData.toString().length} chars)',
          query,
        );
      }
    });

    if (type != null) {
      return SearchEnvelope(
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
    // Per-provider path. The search UI uses [searchEnvelope] instead; this stays
    // for callers that genuinely need exactly one source.
    //
    // The endpoint answers the search ENVELOPE even with ?provider= — the server
    // deliberately serves one shape for both — so this must unwrap it rather
    // than hand the object to getJsonList, which would find no list and return
    // an empty result: a search that silently finds nothing, with no error.
    final items = await _coalesce('search-one:$id:$query', () async {
      final rawData = await _api.getRawJson(
        '$_base/search',
        queryParameters: {'q': query, 'provider': id},
      );

      final payload = (rawData is Map && rawData['data'] is Map)
          ? rawData['data']
          : (rawData is Map && rawData['data'] is List)
          ? rawData['data']
          : rawData;

      final List<dynamic> list;
      if (payload is Map) {
        list = SearchEnvelope.fromJson(
          Map<String, dynamic>.from(payload),
          _mapItem,
        ).items.map((s) => s.item).toList();
      } else if (payload is List) {
        list = payload;
      } else {
        throw FormatException(
          'Unexpected /content/search response for provider $id: '
          '${rawData.runtimeType}',
          query,
        );
      }

      return list
          .whereType<Map>()
          .map((e) => _mapItem(Map<String, dynamic>.from(e)))
          .where((i) => i.title.isNotEmpty)
          .toList();
    });
    final result = List<MediaItem>.of(items);
    if (type != null) {
      return result.where((i) => i.type == type).toList();
    }
    return result;
  }

  @override
  Future<MediaDetails> getDetails(String id) async {
    final key = 'details:${this.id}:$id';
    final cached = _cachedDetails(key);
    if (cached != null) return cached;

    final details = await _guarded(key, () async {
      final res = await _api.getJson(
        '$_base/details',
        queryParameters: {'provider': this.id, 'url': id},
      );
      return _mapDetails(res);
    });
    _detailsCache[key] = _Timed(details, _now().add(_detailsTtl));
    return details;
  }

  @override
  Future<List<StreamSource>> getStreams(
    String id, {
    int? season,
    int? episode,
  }) async {
    // Signed playback URLs are never cached; identical concurrent reads share
    // one request and each waiter gets its own list.
    final key = 'streams:${this.id}:$id:s$season:e$episode';
    final streams = await _coalesce(key, () async {
      final params = <String, dynamic>{'provider': this.id, 'url': id};
      if (season != null) params['season'] = season;
      if (episode != null) params['episode'] = episode;
      final res = await _api.getJson('$_base/streams', queryParameters: params);
      final streams = res['streams'];
      if (streams is! List) return <StreamSource>[];
      return streams
          .whereType<Map>()
          .map((e) => _mapStream(Map<String, dynamic>.from(e)))
          .toList();
    });
    return List<StreamSource>.of(streams);
  }

  @override
  Future<List<MediaItem>> getPopular({ContentType? type, int page = 1}) async {
    final params = <String, dynamic>{'provider': id, 'page': page};
    if (type != null) params['type'] = type.name;
    // A null body (Eneyida answers `200` with JSON `null`) is normalised to
    // `[]` by ApiClient.getJsonList, so an empty catalogue stays an empty
    // catalogue and never a crash.
    final items = await _guarded('popular:$params', () async {
      final list = await _api.getJsonList(
        '$_base/popular',
        queryParameters: params,
      );
      return list
          .whereType<Map>()
          .map((e) => _mapItem(Map<String, dynamic>.from(e)))
          .where((i) => i.title.isNotEmpty)
          .toList();
    });
    return List<MediaItem>.of(items);
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
    final items = await _guarded('category:$params', () async {
      final list = await _api.getJsonList(
        '$_base/category',
        queryParameters: params,
      );
      return list
          .whereType<Map>()
          .map((e) => _mapItem(Map<String, dynamic>.from(e)))
          .where((i) => i.title.isNotEmpty)
          .toList();
    });
    return List<MediaItem>.of(items);
  }

  // --- mapping ---------------------------------------------------------------

  /// Test seam for [_mapStreamType].
  @visibleForTesting
  StreamType mapStreamTypeForTest(String url) => _mapStreamType(url);

  /// Test seam for [_mapStream].
  @visibleForTesting
  StreamSource mapStreamForTest(Map<String, dynamic> json) => _mapStream(json);

  /// Test seam for [_mapItem].
  @visibleForTesting
  MediaItem mapItemForTest(Map<String, dynamic> json) => _mapItem(json);

  /// Test seam for [_mapDetails].
  @visibleForTesting
  MediaDetails mapDetailsForTest(Map<String, dynamic> json) =>
      _mapDetails(json);

  /// Server item URL or ID is mapped to MediaItem.
  MediaItem _mapItem(Map<String, dynamic> json) {
    final url = json['url'] as String? ?? '';
    final idVal = json['id'] as String? ?? '';
    // The unified search response embeds the real provider_id in each item
    // (e.g. "uakino", "lavakino"). Falling back to this.id ensures the
    // provider-specific catalogue views still work when provider_id is absent.
    final providerIdFromJson = json['provider_id'] as String?;
    return MediaItem(
      id: url.isNotEmpty ? url : idVal,
      providerId: (providerIdFromJson != null && providerIdFromJson.isNotEmpty)
          ? providerIdFromJson
          : id,
      title: json['title'] as String? ?? '',
      originalTitle: json['original_title'] as String?,
      posterUrl: json['poster_url'] as String?,
      year: (json['year'] as num?)?.toInt(),
      rating: (json['rating'] as num?)?.toDouble(),
      ratingSource: 'Site',
      type: _mapType(json['type'] as String?),
      url: url.isNotEmpty ? url : null,
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
    final player = _asString(json['player']) ?? _asString(json['source_name']);
    return StreamSource(
      url: url,
      quality: _mapQuality(_asString(json['quality'])),
      type: _mapStreamType(url),
      language: language,
      voiceover: voiceover ?? player,
      sourceName: player,
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

/// A value with a deadline: a cached response or a recent failure.
class _Timed<T> {
  const _Timed(this.value, this.until);

  final T value;
  final DateTime until;
}
