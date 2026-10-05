import 'package:equatable/equatable.dart';

import '../../../domain/entities/entities.dart';

/// Статус окремого підджерела всередині сегмента.
///
/// Comes from the upstream `meta.statuses` field and travels all the way to
/// the UI, so a timed-out source is distinguishable from a source that
/// legitimately returned nothing. Collapsing those two into "0 results"
/// is exactly what made the old source chips useless.
enum SourceStatus {
  ok,
  empty,
  timeout,
  error,
  unknown;

  static SourceStatus parse(Object? raw) {
    switch (raw) {
      case 'ok':
        return SourceStatus.ok;
      case 'empty':
        return SourceStatus.empty;
      case 'timeout':
      case 'deadline':
        return SourceStatus.timeout;
      case 'error':
      case 'failed':
        return SourceStatus.error;
      default:
        return SourceStatus.unknown;
    }
  }

  /// A source that could not be queried, as opposed to one that had nothing
  /// to offer. Only these justify the "N sources unavailable" banner.
  ///
  /// `unknown` is deliberately NOT a failure. It means the backend returned a
  /// status string this client version does not recognise — which implies the
  /// source did answer. Hiding it behind a warning banner would be wrong.
  bool get isFailure =>
      this == SourceStatus.timeout || this == SourceStatus.error;

  String get label => switch (this) {
    SourceStatus.ok => 'ok',
    SourceStatus.empty => 'немає результатів',
    SourceStatus.timeout => 'таймаут',
    SourceStatus.error => 'помилка',
    SourceStatus.unknown => 'невідомо',
  };
}

class SearchSourceStatus extends Equatable {
  const SearchSourceStatus({
    required this.key,
    required this.status,
    this.count = 0,
    this.elapsedMs,
  });

  final String key;
  final SourceStatus status;
  final int count;
  final int? elapsedMs;

  /// Whether the backend got an answer out of this source.
  bool get didAnswer => !status.isFailure;

  /// Whether the source answered but had nothing to offer.
  ///
  /// Derived rather than read off [status]: the status string originates from
  /// the upstream `meta.statuses` and is passed through by the backend without
  /// validation, so it is not a reliable place to encode "zero hits". The
  /// count is. Treating `status: "ok", count: 0` as `ok` would put a
  /// selectable chip reading "0" in the filter bar.
  bool get isEmpty => status == SourceStatus.empty || (didAnswer && count == 0);

  /// Whether this source is worth showing as a selectable filter.
  bool get hasResults => didAnswer && count > 0;

  factory SearchSourceStatus.fromJson(String key, Map<String, dynamic> json) {
    return SearchSourceStatus(
      key: key,
      status: SourceStatus.parse(json['status']),
      count: _asInt(json['count']),
      elapsedMs: _asIntOrNull(json['elapsed_ms']),
    );
  }

  @override
  List<Object?> get props => [key, status, count, elapsedMs];
}

/// One upstream segment (aggregator) that answered the query.
///
/// An array, not a fixed field, so a new segment can be added server-side
/// without breaking the client.
class SearchSegment extends Equatable {
  const SearchSegment({
    required this.id,
    required this.status,
    required this.count,
    this.sources = const {},
  });

  final String id;
  final String status;

  /// Raw candidate count reported by the segment, BEFORE server-side
  /// relevance filtering and clustering. Compared against
  /// `SearchResponse.items.length` this is what proves deduplication ran.
  final int count;

  final Map<String, SearchSourceStatus> sources;

  factory SearchSegment.fromJson(Map<String, dynamic> json) {
    final rawSources = json['sources'];
    final sources = <String, SearchSourceStatus>{};
    if (rawSources is Map) {
      for (final entry in rawSources.entries) {
        final value = entry.value;
        if (value is Map) {
          sources[entry.key.toString()] = SearchSourceStatus.fromJson(
            entry.key.toString(),
            Map<String, dynamic>.from(value),
          );
        }
      }
    }
    return SearchSegment(
      id: json['id']?.toString() ?? 'unknown',
      status: json['status']?.toString() ?? 'unknown',
      count: _asInt(json['count']),
      sources: sources,
    );
  }

  /// Sources that failed, in display order.
  List<SearchSourceStatus> get failures =>
      sources.values.where((s) => !s.didAnswer).toList()
        ..sort((a, b) => a.key.compareTo(b.key));

  /// Sources that answered but had nothing to offer.
  List<SearchSourceStatus> get emptySources =>
      sources.values.where((s) => s.isEmpty).toList()
        ..sort((a, b) => a.key.compareTo(b.key));

  /// True when the segment as a whole produced nothing usable.
  ///
  /// Derived from the per-source statuses rather than the segment's own
  /// `status` string, because only the per-source map reflects what actually
  /// happened to each backend.
  bool get isFailure => failures.isNotEmpty && count == 0;

  @override
  List<Object?> get props => [id, status, count, sources];
}

/// A concrete source a single clustered item can be played from.
class SearchItemSource extends Equatable {
  const SearchItemSource({
    required this.providerId,
    required this.sourceKey,
    required this.itemId,
    this.url,
  });

  final String providerId;
  final String sourceKey;
  final String itemId;
  final String? url;

  factory SearchItemSource.fromJson(Map<String, dynamic> json) {
    return SearchItemSource(
      providerId: json['provider_id']?.toString() ?? '',
      sourceKey: json['source_key']?.toString() ?? '',
      itemId: json['item_id']?.toString() ?? '',
      url: json['url']?.toString(),
    );
  }

  @override
  List<Object?> get props => [providerId, sourceKey, itemId, url];
}

/// Builds a [MediaItem] from a raw server object.
///
/// Injected rather than reimplemented here on purpose: `ServerBackedProvider`
/// already owns the single canonical mapping from wire JSON to [MediaItem].
/// A second copy would drift, and a drifted item parser is exactly the
/// failure mode that made search results untrustworthy.
typedef MediaItemBuilder = MediaItem Function(Map<String, dynamic> json);

/// A scored, clustered search result.
///
/// The server already merged cross-provider duplicates into one item, so
/// this is a straight render target — the client does no merging.
class ScoredMediaItem extends Equatable {
  const ScoredMediaItem({
    required this.item,
    required this.score,
    required this.matchedBy,
    required this.clusterKey,
    required this.sources,
  });

  final MediaItem item;

  /// Normalised 0..1 relevance, computed server-side.
  final double score;

  /// How the title matched: `title_exact`, `tokens_exact`,
  /// `substring_exact`, `all_tokens`, `token_set`, `partial_tokens`.
  final String matchedBy;

  /// Server-side dedup key (`canonicalTitle|year|type`).
  final String clusterKey;

  final List<SearchItemSource> sources;

  factory ScoredMediaItem.fromJson(
    Map<String, dynamic> json,
    MediaItemBuilder buildItem,
  ) {
    final rawSources = json['sources'];
    final sources = <SearchItemSource>[];
    if (rawSources is List) {
      for (final s in rawSources) {
        if (s is Map) {
          sources.add(SearchItemSource.fromJson(Map<String, dynamic>.from(s)));
        }
      }
    }
    return ScoredMediaItem(
      item: buildItem(json),
      score: _asDouble(json['score']),
      matchedBy: json['matched_by']?.toString() ?? '',
      clusterKey: json['cluster']?.toString() ?? '',
      sources: sources,
    );
  }

  bool get isExactMatch =>
      matchedBy == 'title_exact' || matchedBy == 'tokens_exact';

  /// Distinct upstream sources this item is playable from.
  int get sourceCount => sources.length;

  /// e.g. `alpha + beta` - used for the card badge.
  String get sourceLabel => sources
      .map((s) => s.sourceKey)
      .where((k) => k.isNotEmpty)
      .toSet()
      .join(' + ');

  @override
  List<Object?> get props => [item, score, matchedBy, clusterKey, sources];
}

/// The full server-side search envelope.
class SearchEnvelope extends Equatable {
  const SearchEnvelope({
    required this.query,
    required this.canonical,
    required this.tookMs,
    required this.segments,
    required this.items,
    required this.filteredOut,
    this.nextPage = 0,
    this.hasMore = false,
  });

  const SearchEnvelope.empty(this.query)
    : canonical = '',
      tookMs = 0,
      segments = const [],
      items = const [],
      filteredOut = 0,
      nextPage = 0,
      hasMore = false;

  final String query;

  /// Server-side normalised query (noise tokens stripped, confusables folded).
  final String canonical;

  final int tookMs;
  final List<SearchSegment> segments;
  final List<ScoredMediaItem> items;

  /// How many candidates the server dropped as irrelevant. Surfaced in the
  /// UI so a heavily filtered search is visibly different from one that
  /// simply had no candidates.
  final int filteredOut;

  final int nextPage;
  final bool hasMore;

  factory SearchEnvelope.fromJson(
    Map<String, dynamic> json,
    MediaItemBuilder buildItem,
  ) {
    final segments = <SearchSegment>[];
    final rawSegments = json['segments'];
    if (rawSegments is List) {
      for (final s in rawSegments) {
        if (s is Map) {
          segments.add(SearchSegment.fromJson(Map<String, dynamic>.from(s)));
        }
      }
    }
    final items = <ScoredMediaItem>[];
    final rawItems = json['items'];
    if (rawItems is List) {
      for (final i in rawItems) {
        if (i is Map) {
          items.add(
            ScoredMediaItem.fromJson(Map<String, dynamic>.from(i), buildItem),
          );
        }
      }
    }
    return SearchEnvelope(
      query: json['query']?.toString() ?? '',
      canonical: json['canonical']?.toString() ?? '',
      tookMs: _asInt(json['took_ms']),
      segments: segments,
      items: items,
      filteredOut: _asInt(json['filtered_out']),
      nextPage: _asInt(json['next_page']),
      hasMore: json['has_more'] == true,
    );
  }

  /// Every source status across every segment, flattened.
  List<SearchSourceStatus> get allSources => [
    for (final segment in segments) ...segment.sources.values,
  ];

  /// Sources that returned results without erroring.
  ///
  /// Counts sources that actually answered, not sources that were asked —
  /// a search that hit 6 sources but got 2 answers must not report "2/6" as
  /// if the other 4 were healthy-but-quiet.
  int get answeredCount => allSources.where((s) => s.didAnswer).length;

  /// Total number of sources the backend attempted.
  int get askedCount => allSources.length;

  /// Sources that could not be reached. This is what the
  /// "N sources unavailable" banner is built from.
  List<SearchSourceStatus> get failedSources {
    final failed = allSources.where((s) => s.status.isFailure).toList()
      ..sort((a, b) => a.key.compareTo(b.key));
    return failed;
  }

  bool get isEmpty => items.isEmpty;

  @override
  List<Object?> get props => [
    query,
    canonical,
    tookMs,
    segments,
    items,
    filteredOut,
    nextPage,
    hasMore,
  ];
}

int _asInt(Object? v) {
  if (v is int) return v;
  if (v is num) return v.toInt();
  if (v is String) return int.tryParse(v) ?? 0;
  return 0;
}

int? _asIntOrNull(Object? v) {
  if (v == null) return null;
  if (v is int) return v;
  if (v is num) return v.toInt();
  if (v is String) return int.tryParse(v);
  return null;
}

double _asDouble(Object? v) {
  if (v is num) return v.toDouble();
  if (v is String) return double.tryParse(v) ?? 0;
  return 0;
}
