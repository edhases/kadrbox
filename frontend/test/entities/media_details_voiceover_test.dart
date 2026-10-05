import 'package:flutter_test/flutter_test.dart';

import 'package:kadrbox/domain/entities/entities.dart';

/// Domain-level behaviour of [MediaDetails.voiceovers] and of the
/// `isSeries` heuristic.
///
/// The `isSeries` test in `media_item_test.dart` predates DLE providers and
/// passes only because it declares `ContentType.series`. These cases pin the
/// rule that actually matters: a movie reported as `seasons: [{1, [ep1]}]` must
/// not put a series selector on screen.
void main() {
  MediaItem movieItem({ContentType type = ContentType.movie}) =>
      MediaItem(id: 'x', providerId: 'rezka', title: 'Фільм', type: type);

  group('Voiceover', () {
    const voiceover = Voiceover(
      id: '1plus1',
      name: '1+1',
      seasons: [
        Season(number: 1, episodes: [Episode(number: 1), Episode(number: 2)]),
        Season(number: 2, episodes: [Episode(number: 1)]),
      ],
    );

    test('counts episodes across every season', () {
      expect(voiceover.episodeCount, 3);
    });

    test('defaults to no seasons', () {
      const empty = Voiceover(id: 'a', name: 'A');
      expect(empty.seasons, isEmpty);
      expect(empty.episodeCount, 0);
      expect(empty.seasonByNumber(1), isNull);
    });

    test('seasonByNumber finds the season and returns null otherwise', () {
      expect(voiceover.seasonByNumber(2)?.number, 2);
      expect(voiceover.seasonByNumber(9), isNull);
    });

    test('equality is by value, as the rest of the entities', () {
      expect(
        voiceover,
        const Voiceover(
          id: '1plus1',
          name: '1+1',
          seasons: [
            Season(
              number: 1,
              episodes: [Episode(number: 1), Episode(number: 2)],
            ),
            Season(number: 2, episodes: [Episode(number: 1)]),
          ],
        ),
      );
      expect(voiceover, isNot(const Voiceover(id: 'other', name: '1+1')));
    });
  });

  group('MediaDetails.voiceovers', () {
    test('defaults to an empty list so call sites never null-check', () {
      final details = MediaDetails(item: movieItem());
      expect(details.voiceovers, isEmpty);
      expect(details.hasVoiceovers, isFalse);
      expect(details.primaryVoiceover, isNull);
    });

    test('primaryVoiceover prefers a studio that actually has seasons', () {
      final details = MediaDetails(
        item: movieItem(type: ContentType.series),
        voiceovers: const [
          Voiceover(id: 'empty', name: 'Порожня'),
          Voiceover(
            id: 'full',
            name: '1+1',
            seasons: [
              Season(number: 1, episodes: [Episode(number: 1)]),
            ],
          ),
        ],
      );

      expect(details.hasVoiceovers, isTrue);
      expect(details.primaryVoiceover?.id, 'full');
    });

    test(
      'primaryVoiceover falls back to the first entry when none have seasons',
      () {
        final details = MediaDetails(
          item: movieItem(),
          voiceovers: const [
            Voiceover(id: 'a', name: 'A'),
            Voiceover(id: 'b', name: 'B'),
          ],
        );

        expect(details.primaryVoiceover?.id, 'a');
      },
    );

    test('copyWith preserves voiceovers and can replace them', () {
      const original = MediaDetails(
        item: MediaItem(
          id: 'x',
          providerId: 'rezka',
          title: 'Фільм',
          type: ContentType.movie,
        ),
        voiceovers: [Voiceover(id: 'a', name: 'A')],
      );

      expect(original.copyWith().voiceovers, original.voiceovers);
      expect(
        original.copyWith(director: 'Хтось').voiceovers,
        original.voiceovers,
      );

      final replaced = original.copyWith(
        voiceovers: const [Voiceover(id: 'b', name: 'B')],
      );
      expect(replaced.voiceovers.single.name, 'B');
    });

    test('voiceovers take part in equality', () {
      final base = MediaDetails(item: movieItem());
      final withVoiceover = base.copyWith(
        voiceovers: const [Voiceover(id: 'a', name: 'A')],
      );

      expect(base, isNot(withVoiceover));
    });
  });

  group('MediaDetails.isSeries', () {
    test('a movie reported as one season of one episode is NOT a series', () {
      final details = MediaDetails(
        item: movieItem(),
        seasons: const [
          Season(number: 1, episodes: [Episode(number: 1)]),
        ],
      );

      expect(details.isSeries, isFalse);
    });

    test('one season with two episodes IS a series', () {
      final details = MediaDetails(
        item: movieItem(),
        seasons: const [
          Season(number: 1, episodes: [Episode(number: 1), Episode(number: 2)]),
        ],
      );

      expect(details.isSeries, isTrue);
    });

    test('several single-episode seasons IS a series', () {
      final details = MediaDetails(
        item: movieItem(),
        seasons: const [
          Season(number: 1, episodes: [Episode(number: 1)]),
          Season(number: 2, episodes: [Episode(number: 1)]),
        ],
      );

      expect(details.isSeries, isTrue);
    });

    test('a season with no episodes at all is NOT a series', () {
      final details = MediaDetails(
        item: movieItem(),
        seasons: const [Season(number: 1, episodes: [])],
      );

      expect(details.isSeries, isFalse);
      expect(details.hasEpisodes, isFalse);
    });

    test('null seasons is NOT a series', () {
      expect(MediaDetails(item: movieItem()).isSeries, isFalse);
      expect(MediaDetails(item: movieItem()).hasEpisodes, isFalse);
    });

    test(
      'an explicitly declared series type wins for a one-part miniseries',
      () {
        final details = MediaDetails(
          item: movieItem(type: ContentType.series),
          seasons: const [
            Season(number: 1, episodes: [Episode(number: 1)]),
          ],
        );

        expect(details.isSeries, isTrue);
      },
    );

    test('a DLE series mislabelled as movie is still recognised', () {
      // This is the reported bug: DLE providers send `type: movie` for their
      // series, so the type alone can never be trusted.
      final details = MediaDetails(
        item: movieItem(type: ContentType.movie),
        seasons: const [
          Season(number: 1, episodes: [Episode(number: 1)]),
          Season(number: 2, episodes: [Episode(number: 1)]),
        ],
      );

      expect(details.isSeries, isTrue);
    });

    test('hasEpisodes reports episodes regardless of season count', () {
      final one = MediaDetails(
        item: movieItem(),
        seasons: const [
          Season(number: 1, episodes: [Episode(number: 1)]),
        ],
      );
      expect(one.hasEpisodes, isTrue);
      expect(one.isSeries, isFalse);
    });
  });
}
