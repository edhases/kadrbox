import 'package:dio/dio.dart';
import 'package:flutter/foundation.dart' show visibleForTesting;
import 'package:get_it/get_it.dart';

import '../../core/error/exceptions.dart';
import '../../core/network/api_client.dart';
import '../../core/utils/logger.dart';
import '../../core/utils/stream_url.dart';
import '../../domain/entities/entities.dart';
import '../../domain/repositories/content_provider.dart';
import '../models/provider_catalog.dart';
import '../services/catalog_client.dart';
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

/// Content provider backed by the Kadrbox backend (server-side parsing).
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

  /// The user's own catalog server.
  ///
  /// This used to be `${AppConfig.serverApiUrl}/content`, our backend. Those
  /// endpoints were deleted when the parsers left the repository, so every read
  /// answered 404 with Go's `404 page not found` -- while the catalog the user
  /// had just registered sat there, never being asked anything.
  ///
  /// It is created per provider rather than resolved from GetIt, because each
  /// registered catalog has its own base URL and its own Dio with the no-auth,
  /// no-cookie policy that boundary requires.
  late final CatalogClient _catalog = CatalogClient(baseUrl: entry.baseUrl);

  /// Releases the catalog client's HTTP resources.
  void dispose() => _catalog.dispose();

  ServerBackedProvider(this.entry, {DateTime Function()? clock})
    : _now = clock ?? DateTime.now;

  /// Whether the catalog itself sits on loopback or a private network.
  ///
  /// It changes one rule only: a *remote* catalog must not be able to point the
  /// player's socket at the device's own LAN. A catalog the user deliberately
  /// pointed at `127.0.0.1` is their own machine, so refusing its own stream
  /// URLs would break local development for no security gain.
  late final bool _catalogIsLocal = urlPointsAtLocalNetwork(entry.baseUrl);

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
        // Suppress only what is worth repeating. A catalog 404 or 403 is the
        // caller's own doing, so repeating it would just serve nothing while
        // looking like an empty catalogue. CatalogException.isRetryable carries
        // the same rule as the status-code check below: transport failures and
        // 5xx yes, client errors no.
        final retryable = e is NetworkException ||
            (e is ServerException && e.statusCode >= 500) ||
            (e is CatalogException && e.isRetryable);
        if (retryable) {
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

  /// Maps the app's content types onto the protocol's closed [MediaKind] enum.
  ///
  /// The protocol made `kind` a closed enum on purpose -- an unknown kind is
  /// skipped by the client, whereas an open string would let a server mint
  /// shapes with no rendering path. So this must return null rather than
  /// inventing a value: sending `cartoon` would be rejected, and sending
  /// `movie` would silently return films when the user asked for cartoons.
  static String? _kindOf(ContentType? type) => switch (type) {
    ContentType.movie => 'movie',
    ContentType.series => 'series',
    ContentType.dorama => 'tv_show',
    ContentType.anime => 'anime',
    // The enum has no cartoon. `other` is the honest answer: it is the
    // protocol's catch-all, and it cannot be confused with a specific kind.
    ContentType.cartoon => 'other',
    ContentType.unknown || null => null,
  };

  /// Maps the protocol's `kind` back onto a content type.
  static ContentType _typeOf(String? kind) => switch (kind) {
    'movie' || 'documentary' => ContentType.movie,
    'series' || 'episode' => ContentType.series,
    'anime' => ContentType.anime,
    'tv_show' => ContentType.dorama,
    // `other` stays unknown: the protocol's catch-all must not be guessed at.
    _ => ContentType.unknown,
  };

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
      final raw = await _catalog.search(q: query);

      // The protocol's SearchResponse is the envelope shape directly -- no
      // `data` unwrapping, because the catalog contract has no such wrapper.
      return SearchEnvelope.fromJson(raw, _mapItem);
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
      // One catalog, so `searchEnvelope` is already the single-source answer:
      // there is no `provider` parameter in the protocol to narrow with.
      final raw = await _catalog.search(q: query, kind: _kindOf(type));
      final items = SearchEnvelope.fromJson(raw, _mapItem).items;

      return items
          .map((s) => s.item)
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
      final res = await _catalog.details(id: id);
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
      final res = await _catalog.streams(
        id: id,
        season: season,
        episode: episode,
      );
      final raw = res['streams'];
      if (raw is! List) return <StreamSource>[];

      final sources = <StreamSource>[];
      for (final entry in raw.whereType<Map>()) {
        final source = _mapStream(Map<String, dynamic>.from(entry));
        // The protocol makes rejecting the url a client obligation, not a
        // request: a hostile catalog must not be able to point the player's
        // socket at the device's own LAN, a `file://` path, or a UNC share.
        // Skipped with a log line rather than surfaced as an error, because
        // dropping one stream out of several is the documented behaviour.
        if (!_streamUrlIsAcceptable(source.url)) {
          Logger.w(
            'Catalog $name offered a stream url the client refused: '
            '${_refusalReason(source.url)}',
            tag: _tag,
          );
          continue;
        }
        sources.add(source);
      }

      // Subtitles are a sibling array on the protocol's StreamsResponse, not a
      // per-stream field, so they are attached to the first source that plays.
      final subtitles = _mapSubtitles(res['subtitles']);
      if (sources.isNotEmpty && subtitles.isNotEmpty) {
        sources[0] = sources[0].copyWith(subtitles: subtitles);
      }
      return sources;
    });
    return List<StreamSource>.of(streams);
  }

  @override
  Future<List<MediaItem>> getPopular({ContentType? type, int page = 1}) async {
    return _browse(type: type, key: 'popular:${type?.name}:$page');
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
    // There is no category endpoint in the protocol. The caller's category is a
    // slug like "movie" or "series", which is exactly the protocol's `kind`
    // parameter -- so it is forwarded as one rather than invented as a query.
    // An unrecognised slug gets no `kind` at all, which returns everything: a
    // wrong-but-browseable row beats an empty screen.
    return _browse(
      type: type,
      kind: _kindOf(type) ?? _kindForSlug(category),
      key: 'category:$category:${type?.name}:$page',
    );
  }

  /// Browse listing: `/search` with no `q`.
  ///
  /// PROTOCOL.md defines `/search?q=` as "search and browse", so this is the
  /// protocol's own browse path rather than a substitute for a missing
  /// `/popular`. `page` is not sent: it is an opaque continuation token, and
  /// synthesising `"2"` would return page one again on every page.
  Future<List<MediaItem>> _browse({
    ContentType? type,
    String? kind,
    required String key,
  }) async {
    final items = await _guarded(key, () async {
      // An explicit kind (from a category slug) wins; otherwise the caller's
      // content type decides, and no type at all means browse everything.
      final raw = await _catalog.search(kind: kind ?? _kindOf(type));
      final list = SearchEnvelope.fromJson(raw, _mapItem).items;
      return list
          .map((s) => s.item)
          .where((i) => i.title.isNotEmpty)
          .toList();
    });
    return List<MediaItem>.of(items);
  }

  /// Maps a category slug onto a protocol kind, or null when it is not one.
  static String? _kindForSlug(String slug) => switch (slug.toLowerCase()) {
    'movie' || 'movies' || 'film' => 'movie',
    'series' || 'tv' || 'tv_show' || 'tvshow' => 'series',
    'anime' => 'anime',
    'cartoon' => 'other',
    'dorama' => 'tv_show',
    'documentary' || 'doc' => 'documentary',
    _ => null,
  };

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

  /// A protocol `Item` mapped to [MediaItem].
  ///
  /// The catalog's `id` is opaque and is the only id the protocol ever accepts
  /// back, so it is what `MediaItem.id` carries. The old backend shape put a
  /// browsable URL in `id`, which is exactly what the protocol forbids: an id
  /// that embeds a parseable structure. `external_id` is diagnostic only and is
  /// deliberately never used to build a URL.
  MediaItem _mapItem(Map<String, dynamic> json) {
    final id = _asString(json['id']) ?? '';
    final providerKey = _asString(json['provider']) ?? id;
    final genres = json['genres'];
    return MediaItem(
      id: id,
      providerId: providerKey.isEmpty ? id : providerKey,
      title: _asString(json['title']) ?? '',
      originalTitle: _asString(json['original_title']),
      posterUrl: _asString(json['poster_url']),
      year: (json['year'] as num?)?.toInt(),
      rating: (json['rating'] as num?)?.toDouble(),
      // The protocol has no rating source field. Labelling it honestly beats
      // inventing a provider name.
      ratingSource: null,
      type: _typeOf(_asString(json['kind']) ?? _asString(json['type'])),
      description: _asString(json['description']),
      genres: genres is List
          ? genres.map((e) => e.toString()).toList()
          : null,
    );
  }

  /// A protocol `DetailsResponse` mapped to [MediaDetails].
  ///
  /// The item sits under `item`, not at the top level as the old backend had
  /// it, and the episodes arrive as one flat list across all seasons rather
  /// than nested inside each season.
  MediaDetails _mapDetails(Map<String, dynamic> json) {
    final rawItem = json['item'];
    final item = _mapItem(
      rawItem is Map
          ? Map<String, dynamic>.from(rawItem)
          : const <String, dynamic>{},
    );
    final genres = json['genres'] ?? (rawItem is Map ? rawItem['genres'] : null);
    return MediaDetails(
      item: item,
      fullDescription: rawItem is Map
          ? _asString(rawItem['description'])
          : null,
      genres: genres is List
          ? genres.map((e) => e.toString()).toList()
          : null,
      seasons: _mapProtocolSeasons(json),
      voiceovers: _mapVoiceovers(json['voiceovers']),
    );
  }

  /// Groups the flat protocol `episodes` array into seasons.
  ///
  /// The protocol ships episodes flat and ordered across all seasons "so a
  /// details screen does not need a request per episode". The app's
  /// [MediaDetails] nests them, so the grouping happens here.
  ///
  /// Returns null -- not `[]` -- when there are no episodes at all: null is what
  /// `MediaDetails` reads as "this is a movie", and an empty list would render
  /// an episode picker over a film.
  List<Season>? _mapProtocolSeasons(Map<String, dynamic> json) {
    final rawSeasons = json['seasons'];
    final rawEpisodes = json['episodes'];

    final summaries = <int, Season>{}; // keyed by season number
    if (rawSeasons is List) {
      for (final entry in rawSeasons.whereType<Map>()) {
        final sm = Map<String, dynamic>.from(entry);
        final number = (sm['number'] as num?)?.toInt();
        if (number == null) continue;
        summaries[number] = Season(
          number: number,
          title: _asString(sm['label']) ?? _asString(sm['title']),
          episodes: const [],
        );
      }
    }

    final grouped = <int, List<Episode>>{};
    if (rawEpisodes is List) {
      for (final entry in rawEpisodes.whereType<Map>()) {
        final em = Map<String, dynamic>.from(entry);
        final season = (em['season'] as num?)?.toInt();
        if (season == null) continue;
        (grouped[season] ??= <Episode>[]).add(_mapProtocolEpisode(em));
        summaries.putIfAbsent(
          season,
          () => Season(number: season, episodes: const []),
        );
      }
    }

    if (grouped.isEmpty) return null;

    final numbers = grouped.keys.toList()..sort();
    return numbers
        .map(
          (n) => Season(
            number: n,
            title: summaries[n]?.title,
            episodes: grouped[n]!,
          ),
        )
        .toList(growable: false);
  }

  Episode _mapProtocolEpisode(Map<String, dynamic> em) => Episode(
    number: (em['number'] as num?)?.toInt() ?? 0,
    title: _asString(em['title']),
    duration: em['duration_ms'] is num
        ? Duration(milliseconds: (em['duration_ms'] as num).toInt())
        : null,
    // The opaque episode id is what `/streams?id=` expects, with season and
    // episode passed alongside it.
    streamRef: _asString(em['id']),
  );

  /// Parses the protocol `voiceovers` array. Always returns a list, never null.
  ///
  /// The protocol labels a track with `label` and requires it, so unlike the old
  /// backend nothing is dropped for lacking a name. `lang` and `default` have no
  /// home on [Voiceover] yet, so they are read and ignored rather than stuffed
  /// into `name` -- a "Ukrainian (uk)" label invented here would be worse than
  /// an honest plain one.
  List<Voiceover> _mapVoiceovers(Object? raw) {
    if (raw is! List) return const [];
    final result = <Voiceover>[];
    for (final entry in raw.whereType<Map>()) {
      final vm = Map<String, dynamic>.from(entry);
      final id = _asString(vm['id']);
      final label = _asString(vm['label']) ?? _asString(vm['name']);
      if (id == null || id.isEmpty || label == null || label.isEmpty) continue;
      result.add(Voiceover(id: id, name: label));
    }
    return result;
  }

  /// A protocol `Stream` mapped to [StreamSource].
  ///
  /// `quality` is a display label by contract -- the client must not parse it to
  /// decide codec or resolution -- so the guess below is best-effort labelling
  /// only, and `unknown` is a perfectly good outcome.
  StreamSource _mapStream(Map<String, dynamic> json) {
    // Guarded: `url` may arrive as a non-String (number, null, object) and an
    // unguarded `as String?` cast inside `.toLowerCase()` throws a TypeError
    // that swallows the whole stream list.
    final direct = _asString(json['direct_url']);
    final url = (direct != null && direct.isNotEmpty)
        ? direct
        : (_asString(json['url']) ?? '');
    final headers = _allowedHeaders(json['headers']);
    return StreamSource(
      url: url,
      quality: _mapQuality(_asString(json['quality'])),
      type: _mapStreamType(url),
      voiceover: _asString(json['voiceover']),
      headers: headers.isEmpty ? null : headers,
    );
  }

  /// The protocol's header allow-list, enforced on the client's side.
  ///
  /// It says so itself: "allow-list is exactly Referer, User-Agent, Origin; any
  /// other header is dropped without being sent" and "never send Authorization,
  /// Cookie or any other client credential, even when the server asks for it by
  /// name". A catalog server is a third party, and this is the one place its
  /// instructions meet the player's sockets.
  static const Set<String> _allowedHeaderNames = {
    'referer',
    'user-agent',
    'origin',
  };

  static Map<String, String> _allowedHeaders(Object? raw) {
    if (raw is! Map) return const {};
    final result = <String, String>{};
    for (final entry in raw.entries) {
      final name = entry.key.toString();
      if (!_allowedHeaderNames.contains(name.toLowerCase())) {
        Logger.w(
          'Catalog asked for a header the client will not send: $name',
          tag: _tag,
        );
        continue;
      }
      final value = entry.value;
      if (value == null) continue;
      result[name] = value is String ? value : value.toString();
    }
    return result;
  }

  /// Parses the protocol's sibling `subtitles` array.
  List<Subtitle> _mapSubtitles(Object? raw) {
    if (raw is! List) return const [];
    final result = <Subtitle>[];
    for (final entry in raw.whereType<Map>()) {
      final sm = Map<String, dynamic>.from(entry);
      final url = _asString(sm['url']) ?? '';
      final lang = _asString(sm['lang']);
      if (url.isEmpty || lang == null || lang.isEmpty) continue;
      if (!_streamUrlIsAcceptable(url)) continue;
      result.add(
        Subtitle(
          url: url,
          language: lang,
          label: _asString(sm['label']),
          format: _mapSubtitleFormat(_asString(sm['format'])),
        ),
      );
    }
    return result;
  }

  /// The protocol's enum is `[srt, vtt, ass, ssa]`; the app's is missing `ssa`.
  ///
  /// `ssa` is an older container for the same thing `ass` describes, so it maps
  /// onto `ass` rather than being dropped or guessed at. An unrecognised format
  /// becomes `srt`, the format the player can always handle.
  static SubtitleFormat _mapSubtitleFormat(String? format) =>
      switch (format?.toLowerCase()) {
        'vtt' => SubtitleFormat.vtt,
        'ass' || 'ssa' => SubtitleFormat.ass,
        _ => SubtitleFormat.srt,
      };

  /// Whether the player may be pointed at [url].
  ///
  /// The protocol states these as mandatory client constraints, and they are
  /// the sharp end of the whole design: a `file://` url handed to a player is a
  /// local-file read whose bytes the catalog server then gets back.
  ///
  /// Skipped entirely when the catalog itself is on loopback or a private
  /// network: a user who pointed the app at `127.0.0.1` is running their own
  /// server, and refusing its own stream urls would break local development
  /// without protecting anything.
  bool _streamUrlIsAcceptable(String url) {
    if (_catalogIsLocal) return url.startsWith('http');
    return validateMediaUrl(url) == null;
  }

  static String _refusalReason(String url) => validateMediaUrl(url) ?? 'unknown';

  /// Type-safe String coercion for values coming from the backend.
  static String? _asString(Object? value) {
    if (value == null) return null;
    if (value is String) return value;
    return value.toString();
  }

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
