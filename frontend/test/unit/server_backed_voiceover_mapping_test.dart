import 'package:flutter_test/flutter_test.dart';

import 'package:kadrbox/data/models/provider_catalog.dart';
import 'package:kadrbox/data/providers/server_backed_provider.dart';
import 'package:kadrbox/domain/entities/entities.dart';

/// Mapping of the `voiceovers` / `seasons` payloads the backend sends for
/// DLE-style providers.
///
/// The backend was already returning studio names with their own season trees;
/// `_mapDetails` dropped the field on the floor, so the details page fell back
/// to grouping streams and rendered CDN names ("CDN Alpha", "CDN Beta") where
/// studios ("1+1", "Postmodern") belong.
void main() {
  late ServerBackedProvider provider;

  setUp(() {
    provider = ServerBackedProvider(
      const ProviderCatalogEntry(
        id: 'rezka',
        name: 'Rezka',
        baseUrl: 'https://rezka.ag',
        showOnHome: true,
        hasFixedStreams: false,
        contentTypes: ['movie', 'series'],
        searchEnabledDefault: true,
        enabled: true,
        healthy: true,
      ),
    );
  });

  group('_mapDetails voiceovers', () {
    test('parses studio id, name and its own season tree', () {
      final details = provider.mapDetailsForTest({
        'id': 'series_1',
        'title': 'Місячний Збитий Бібліотекар',
        'type': 'series',
        'voiceovers': [
          {
            'id': '1plus1',
            'name': '1+1',
            'seasons': [
              {
                'number': 1,
                'title': 'Сезон 1',
                'episodes': [
                  {
                    'number': 1,
                    'title': 'Початок',
                    'url': '{"source":"rezka","ref":{"season":1,"episode":1}}',
                  },
                  {'number': 2, 'title': 'Далі'},
                ],
              },
              {
                'number': 2,
                'episodes': [
                  {
                    'number': 1,
                    'stream_ref': {'season': 2, 'episode': 1},
                  },
                ],
              },
            ],
          },
          {
            'id': 'postmodern',
            'name': 'Postmodern',
            'seasons': [
              {
                'number': 1,
                'episodes': [
                  {
                    'number': 1,
                    'stream_ref': {'season': 1, 'episode': 1},
                  },
                ],
              },
            ],
          },
        ],
      });

      expect(details.voiceovers, hasLength(2));

      final first = details.voiceovers[0];
      expect(first.id, '1plus1');
      expect(first.name, '1+1');
      expect(first.seasons, hasLength(2));
      expect(first.seasons[0].number, 1);
      expect(first.seasons[0].episodes, hasLength(2));
      expect(first.seasons[0].episodes[0].title, 'Початок');
      // A structured ref must be serialised, not stringified as "[object
      // Object]" — the client hands this straight back to getStreams.
      expect(
        first.seasons[0].episodes[0].streamRef,
        '{"source":"rezka","ref":{"season":1,"episode":1}}',
      );
      expect(
        first.seasons[1].episodes[0].streamRef,
        '{"season":2,"episode":1}',
      );
      expect(first.episodeCount, 3);

      final second = details.voiceovers[1];
      expect(second.id, 'postmodern');
      expect(second.name, 'Postmodern');
      expect(second.episodeCount, 1);

      expect(details.hasVoiceovers, isTrue);
      // The backend sorts by dub weight, so the first entry is the preferred
      // studio and must be the one the UI defaults to.
      expect(details.primaryVoiceover?.name, '1+1');
    });

    test('absent key yields an empty list, never null', () {
      final details = provider.mapDetailsForTest({
        'id': 'movie_1',
        'title': 'Фільм',
        'type': 'movie',
        'seasons': [
          {
            'number': 1,
            'episodes': [
              {'number': 1, 'url': 'ref-1'},
            ],
          },
        ],
      });

      expect(details.voiceovers, isEmpty);
      expect(details.voiceovers, isNotNull);
      expect(details.hasVoiceovers, isFalse);
      expect(details.primaryVoiceover, isNull);
    });

    test('defaults to an empty list when constructed directly', () {
      final details = MediaDetails(
        item: MediaItem(
          id: 'movie_2',
          providerId: 'rezka',
          title: 'Фільм',
          type: ContentType.movie,
        ),
      );

      expect(details.voiceovers, isEmpty);
    });

    test('null or wrongly typed key degrades to an empty list', () {
      final details = provider.mapDetailsForTest({
        'id': 'movie_3',
        'title': 'Фільм',
        'voiceovers': null,
      });
      expect(details.voiceovers, isEmpty);

      final wrongType = provider.mapDetailsForTest({
        'id': 'movie_4',
        'title': 'Фільм',
        'voiceovers': '1+1',
      });
      expect(wrongType.voiceovers, isEmpty);
    });

    test('drops nameless voiceovers and survives a hostile shape', () {
      final details = provider.mapDetailsForTest({
        'id': 'movie_5',
        'title': 'Фільм',
        'voiceovers': [
          'not an object',
          {'id': 'nameless', 'name': '', 'seasons': []},
          {'id': 'kept', 'name': 'Студія', 'seasons': 'not a list'},
        ],
      });

      expect(details.voiceovers, hasLength(1));
      expect(details.voiceovers[0].name, 'Студія');
      expect(details.voiceovers[0].seasons, isEmpty);
    });

    test('falls back to the name when the studio id is missing', () {
      final details = provider.mapDetailsForTest({
        'id': 'movie_6',
        'title': 'Фільм',
        'voiceovers': [
          {'name': 'Основна', 'seasons': []},
        ],
      });

      expect(details.voiceovers[0].id, 'Основна');
    });

    test('a movie shaped as one season of one episode is not a series', () {
      final details = provider.mapDetailsForTest({
        'id': 'movie_7',
        'title': 'Фільм',
        'type': 'movie',
        'seasons': [
          {
            'number': 1,
            'episodes': [
              {'number': 1, 'url': 'ref-7'},
            ],
          },
        ],
      });

      expect(details.seasons, hasLength(1));
      expect(details.isSeries, isFalse);
    });

    test('a series with several seasons or episodes is a series', () {
      final manySeasons = provider.mapDetailsForTest({
        'id': 'series_2',
        'title': 'Серіал',
        'type': 'movie',
        'seasons': [
          {
            'number': 1,
            'episodes': [
              {'number': 1},
            ],
          },
          {
            'number': 2,
            'episodes': [
              {'number': 1},
            ],
          },
        ],
      });
      expect(manySeasons.isSeries, isTrue);

      // DLE providers mislabel their series as `movie`, so the shape has to be
      // trusted over the type.
      final manyEpisodes = provider.mapDetailsForTest({
        'id': 'series_3',
        'title': 'Серіал',
        'type': 'movie',
        'seasons': [
          {
            'number': 1,
            'episodes': [
              {'number': 1},
              {'number': 2},
            ],
          },
        ],
      });
      expect(manyEpisodes.isSeries, isTrue);
    });

    test('absent seasons stay null so movies are distinguishable', () {
      final details = provider.mapDetailsForTest({
        'id': 'movie_8',
        'title': 'Фільм',
        'type': 'movie',
      });

      expect(details.seasons, isNull);
      expect(details.isSeries, isFalse);
      expect(details.hasEpisodes, isFalse);
    });
  });

  group('_mapStream voiceover vs player', () {
    test('reads studio and CDN as separate fields', () {
      final stream = provider.mapStreamForTest({
        'url': 'https://cdn.tv/master.m3u8',
        'voiceover': '1+1',
        'player': 'CDN Alpha',
        'language': 'uk',
      });

      expect(stream.voiceover, '1+1');
      expect(stream.sourceName, 'CDN Alpha');
      expect(stream.language, 'uk');
    });

    test('a CDN name never leaks into voiceover', () {
      // This is the exact bug: DLE streams carry only `player`, and the old
      // `voiceover ?? player` fallback put "CDN Beta" into the studio dropdown.
      final stream = provider.mapStreamForTest({
        'url': 'https://cdn.tv/master.m3u8',
        'player': 'CDN Beta',
      });

      expect(stream.voiceover, isNull);
      expect(stream.sourceName, 'CDN Beta');
    });

    test('falls back through audio/dub aliases for the studio only', () {
      expect(
        provider.mapStreamForTest({'url': 'a', 'audio': 'Студія'}).voiceover,
        'Студія',
      );
      expect(
        provider.mapStreamForTest({'url': 'a', 'dub': 'Дубляж'}).voiceover,
        'Дубляж',
      );
      expect(
        provider.mapStreamForTest({
          'url': 'a',
          'dub': 'Дубляж',
          'player': 'UAHD',
        }).sourceName,
        'UAHD',
      );
    });

    test('source_name is accepted as the player alias', () {
      final stream = provider.mapStreamForTest({
        'url': 'https://cdn.tv/a.m3u8',
        'source_name': 'Zenith',
      });

      expect(stream.sourceName, 'Zenith');
      expect(stream.voiceover, isNull);
    });

    test('a stream with neither reports null for both', () {
      final stream = provider.mapStreamForTest({
        'url': 'https://cdn.tv/a.m3u8',
      });

      expect(stream.voiceover, isNull);
      expect(stream.sourceName, isNull);
    });
  });
}
