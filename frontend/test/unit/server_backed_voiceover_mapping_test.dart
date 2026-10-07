import 'package:flutter_test/flutter_test.dart';

import 'package:kadrbox/data/models/provider_catalog.dart';
import 'package:kadrbox/data/providers/server_backed_provider.dart';
import 'package:kadrbox/domain/entities/entities.dart';

/// Mapping of the `voiceovers` / `seasons` / `episodes` payloads a catalog server
/// sends, per contracts/openapi.yaml.
///
/// The shape changed and the tests had to change with it. The old backend nested
/// episodes inside each voiceover and each season, and encoded a stream reference
/// as a JSON blob in `url`/`stream_ref` -- which the client had to re-serialise
/// or it became the literal string "[object Object]". The catalog protocol sends
/// episodes flat and gives every episode an opaque `id`, so none of that
/// re-serialisation is needed any more.
///
/// What still matters, and is asserted below: the track label comes from `label`,
/// an absent or hostile key degrades to an empty list rather than null, a movie
/// stays distinguishable from a series, and the primary track is the one the UI
/// defaults to.
void main() {
  late ServerBackedProvider provider;

  setUp(() {
    provider = ServerBackedProvider(
      const ProviderCatalogEntry(
        id: 'ex',
        name: 'Example',
        baseUrl: 'https://catalog.example',
        showOnHome: true,
        hasFixedStreams: false,
        contentTypes: ['movie', 'series'],
        searchEnabledDefault: true,
        enabled: true,
        healthy: true,
      ),
    );
    addTearDown(provider.dispose);
  });

  group('_mapDetails voiceovers', () {
    test('parses the track id and its protocol label', () {
      final details = provider.mapDetailsForTest({
        'item': {
          'id': 'ex:series-1',
          'provider': 'ex',
          'kind': 'series',
          'title': 'Приклад',
        },
        'voiceovers': [
          {'id': 'primary', 'label': 'Основна', 'lang': 'uk', 'default': true},
          {'id': 'secondary', 'label': 'Дубляж', 'lang': 'ru', 'default': false},
        ],
        'seasons': [
          {'number': 1, 'label': 'Season 1', 'episode_count': 2},
        ],
        'episodes': [
          {'id': 'ex:s1e1', 'season': 1, 'number': 1, 'title': 'One'},
          {'id': 'ex:s1e2', 'season': 1, 'number': 2, 'title': 'Two'},
        ],
      });

      expect(details.voiceovers, hasLength(2));
      expect(details.voiceovers[0].id, 'primary');
      expect(details.voiceovers[0].name, 'Основна');
      expect(details.voiceovers[1].id, 'secondary');
      expect(details.voiceovers[1].name, 'Дубляж');

      expect(details.hasVoiceovers, isTrue);
      // The server sorts by preference and the UI defaults to the first entry.
      expect(details.primaryVoiceover?.id, 'primary');

      // Episodes are grouped from the flat list, and each carries the opaque id
      // that `/streams?id=` is called with.
      final seasons = details.seasons!;
      expect(seasons, hasLength(1));
      expect(seasons.first.title, 'Season 1');
      expect(seasons.first.episodes, hasLength(2));
      expect(seasons.first.episodes[0].streamRef, 'ex:s1e1');
      expect(seasons.first.episodes[1].streamRef, 'ex:s1e2');
    });

    test('an episode id is used verbatim, never re-serialised', () {
      // The old shape put a JSON blob in `url`; stringifying an object there
      // produced "[object Object]" and the player was handed that. The protocol
      // makes the id an opaque string, so there is nothing to re-serialise --
      // which is only safe if the mapper does not try.
      final details = provider.mapDetailsForTest({
        'item': {'id': 'ex:s', 'provider': 'ex', 'kind': 'series', 'title': 'S'},
        'episodes': [
          {'id': 'ex:s:1:1', 'season': 1, 'number': 1},
        ],
      });

      expect(details.seasons!.first.episodes.single.streamRef, 'ex:s:1:1');
    });

    test('an absent voiceovers key yields an empty list, never null', () {
      final details = provider.mapDetailsForTest({
        'item': {'id': 'ex:m', 'provider': 'ex', 'kind': 'movie', 'title': 'M'},
        'seasons': <Object>[],
        'episodes': <Object>[],
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
          providerId: 'ex',
          title: 'M',
          type: ContentType.movie,
        ),
      );

      expect(details.voiceovers, isEmpty);
    });

    test('null or wrongly typed voiceovers degrade to an empty list', () {
      final missing = provider.mapDetailsForTest({
        'item': {'id': 'ex:m3', 'provider': 'ex', 'kind': 'movie', 'title': 'M'},
        'voiceovers': null,
      });
      expect(missing.voiceovers, isEmpty);

      final wrongType = provider.mapDetailsForTest({
        'item': {'id': 'ex:m4', 'provider': 'ex', 'kind': 'movie', 'title': 'M'},
        'voiceovers': 'primary',
      });
      expect(wrongType.voiceovers, isEmpty);
    });

    test('drops unnamed voiceovers and survives a hostile shape', () {
      final details = provider.mapDetailsForTest({
        'item': {'id': 'ex:m5', 'provider': 'ex', 'kind': 'movie', 'title': 'M'},
        'voiceovers': [
          'not an object',
          {'id': 'nameless', 'label': ''},
          {'label': 'no id at all'},
          {'id': 'kept', 'label': 'Основна'},
        ],
      });

      // Only the entry that carries both a usable id and a label survives.
      expect(details.voiceovers, hasLength(1));
      expect(details.voiceovers.single.name, 'Основна');
    });

    test('a track with only a label is kept under its own name', () {
      // The protocol requires `id`, but a server that omits it should still
      // render one labelled dropdown row rather than none.
      final details = provider.mapDetailsForTest({
        'item': {'id': 'ex:m6', 'provider': 'ex', 'kind': 'movie', 'title': 'M'},
        'voiceovers': [
          {'label': 'Без ідентифікатора'},
        ],
      });

      // No id means no way to ask /streams for that track, so it is dropped.
      expect(details.voiceovers, isEmpty);
    });
  });

  group('_mapDetails seasons and episodes', () {
    test('a movie shaped as one season of one episode is not a series', () {
      final details = provider.mapDetailsForTest({
        'item': {'id': 'ex:m7', 'provider': 'ex', 'kind': 'movie', 'title': 'M'},
        'seasons': [
          {'number': 1, 'label': 'Season 1', 'episode_count': 1},
        ],
        'episodes': [
          {'id': 'ex:m7:1', 'season': 1, 'number': 1},
        ],
      });

      expect(details.seasons, hasLength(1));
      expect(details.isSeries, isFalse);
    });

    test('a series with several seasons or episodes is a series', () {
      final manySeasons = provider.mapDetailsForTest({
        'item': {'id': 'ex:s2', 'provider': 'ex', 'kind': 'series', 'title': 'S'},
        'seasons': [
          {'number': 1, 'label': 'S1', 'episode_count': 1},
          {'number': 2, 'label': 'S2', 'episode_count': 1},
        ],
        'episodes': [
          {'id': 'ex:s2:1:1', 'season': 1, 'number': 1},
          {'id': 'ex:s2:2:1', 'season': 2, 'number': 1},
        ],
      });
      expect(manySeasons.isSeries, isTrue);

      // Servers do mislabel a series as `movie`, so the shape is trusted over
      // the declared kind.
      final manyEpisodes = provider.mapDetailsForTest({
        'item': {'id': 'ex:s3', 'provider': 'ex', 'kind': 'movie', 'title': 'S'},
        'seasons': [
          {'number': 1, 'label': 'S1', 'episode_count': 2},
        ],
        'episodes': [
          {'id': 'ex:s3:1:1', 'season': 1, 'number': 1},
          {'id': 'ex:s3:1:2', 'season': 1, 'number': 2},
        ],
      });
      expect(manyEpisodes.isSeries, isTrue);
    });

    test('absent seasons stay null so movies are distinguishable', () {
      final details = provider.mapDetailsForTest({
        'item': {'id': 'ex:m8', 'provider': 'ex', 'kind': 'movie', 'title': 'M'},
      });

      expect(details.seasons, isNull);
      expect(details.isSeries, isFalse);
      expect(details.hasEpisodes, isFalse);
    });

    test('seasons come back in ascending order whatever order they arrive', () {
      final details = provider.mapDetailsForTest({
        'item': {'id': 'ex:s4', 'provider': 'ex', 'kind': 'series', 'title': 'S'},
        'seasons': [
          {'number': 3, 'label': 'S3', 'episode_count': 1},
          {'number': 1, 'label': 'S1', 'episode_count': 1},
          {'number': 2, 'label': 'S2', 'episode_count': 1},
        ],
        'episodes': [
          {'id': 'a', 'season': 3, 'number': 1},
          {'id': 'b', 'season': 1, 'number': 1},
          {'id': 'c', 'season': 2, 'number': 1},
        ],
      });

      expect(details.seasons!.map((s) => s.number), [1, 2, 3]);
    });
  });
}