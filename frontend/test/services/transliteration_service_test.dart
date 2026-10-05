import 'package:flutter_test/flutter_test.dart';
import 'package:oxide_film/data/services/smart_search/transliteration_service.dart';

void main() {
  late TransliterationService service;

  setUp(() {
    service = TransliterationService();
  });

  group('TransliterationService', () {
    test('isCyrillic detects Cyrillic text', () {
      expect(service.isCyrillic('Дюна'), isTrue);
      expect(service.isCyrillic('Dune'), isFalse);
      expect(service.isCyrillic('Гаррі Поттер'), isTrue);
      expect(service.isCyrillic('Harry Potter'), isFalse);
    });

    test('isLatin detects Latin text', () {
      expect(service.isLatin('Dune'), isTrue);
      expect(service.isLatin('Дюна'), isFalse);
      expect(service.isLatin('Breaking Bad'), isTrue);
    });

    test('ukrainianToLatin transliterates correctly according to standard', () {
      expect(service.ukrainianToLatin('дюна'), 'diuna');
      expect(service.ukrainianToLatin('гаррі'), 'harri');
      expect(service.ukrainianToLatin('кіно'), 'kino');
      expect(service.ukrainianToLatin('щит'), 'shchyt');
      expect(service.ukrainianToLatin('єнот'), 'ienot');
    });

    test(
      'generateSearchVariants preserves original and adds latin for Cyrillic',
      () {
        final variants = service.generateSearchVariants('Дюна');
        expect(variants, contains('дюна'));
        expect(variants, contains('diuna'));
        expect(variants.length, 2);
      },
    );

    test(
      'generateSearchVariants does NOT corrupt Latin queries with pseudo-dictionaries',
      () {
        // "bad" should NOT turn into "берега", "prison" should NOT turn into "в'язниця"
        final variants = service.generateSearchVariants('breaking bad');
        expect(variants, ['breaking bad']);
        expect(variants.contains('пуститися берега'), isFalse);
      },
    );

    test(
      'normalizeQuery strips unwanted symbols while preserving Cyrillic & Latin',
      () {
        expect(
          service.normalizeQuery('  Дюна: Частина 2!  '),
          'дюна частина 2',
        );
        expect(
          service.normalizeQuery('Dune: Part Two (2024)'),
          'dune part two 2024',
        );
      },
    );

    test(
      'areSimilarQueries matches Ukrainian and its Latin transliteration',
      () {
        expect(service.areSimilarQueries('Дюна', 'diuna'), isTrue);
        expect(service.areSimilarQueries('Кіно', 'kino'), isTrue);
        expect(service.areSimilarQueries('Дюна', 'матриця'), isFalse);
      },
    );
  });
}
