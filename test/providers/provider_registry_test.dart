import 'package:flutter_test/flutter_test.dart';

import 'package:oxide_film/data/models/provider_catalog.dart';
import 'package:oxide_film/data/providers/provider_registry.dart';
import 'package:oxide_film/data/providers/server_backed_provider.dart';
import 'package:oxide_film/domain/entities/entities.dart';

void main() {
  late ProviderRegistry registry;

  setUp(() {
    registry = ProviderRegistry();
  });

  ServerBackedProvider makeProvider(String id, String name, {bool home = true}) {
    return ServerBackedProvider(
      ProviderCatalogEntry(
        id: id,
        name: name,
        baseUrl: 'https://$id.example',
        showOnHome: home,
        hasFixedStreams: false,
        contentTypes: ['movie', 'series', 'anime'],
        searchEnabledDefault: true,
        enabled: true,
        healthy: true,
      ),
    );
  }

  group('ProviderRegistry', () {
    test('should register providers', () {
      registry.register(makeProvider('uakino', 'UAKino'));
      registry.register(makeProvider('eneyida', 'Eneyida'));

      expect(registry.all.length, 2);
    });

    test('should not register duplicate providers', () {
      final provider = makeProvider('uakino', 'UAKino');
      registry.register(provider);
      registry.register(provider);

      // Second registration with same ID should be ignored
      expect(registry.all.length, 1);
    });

    test('getById should return correct provider', () {
      registry.register(makeProvider('uakino', 'UAKino'));
      registry.register(makeProvider('eneyida', 'Eneyida'));

      final result = registry.getById('uakino');

      expect(result, isNotNull);
      expect(result?.id, 'uakino');
    });

    test('getById should return null for unknown id', () {
      registry.register(makeProvider('uakino', 'UAKino'));

      final result = registry.getById('unknown');

      expect(result, isNull);
    });

    test('getByContentType should return matching providers', () {
      registry.register(makeProvider('uakino', 'UAKino'));
      registry.register(makeProvider('eneyida', 'Eneyida'));

      final movieProviders = registry.getByContentType(ContentType.movie);
      final animeProviders = registry.getByContentType(ContentType.anime);

      expect(movieProviders.length, 2);
      expect(animeProviders.length, 2);
    });

    test('homeProviders should exclude separate providers', () {
      registry.register(makeProvider('uakino', 'UAKino', home: true));
      registry.register(makeProvider('hdrezka', 'HDRezka', home: false));

      expect(registry.homeProviders.map((p) => p.id), contains('uakino'));
      expect(registry.homeProviders.map((p) => p.id), isNot(contains('hdrezka')));
    });

    test('unregister should remove provider', () {
      registry.register(makeProvider('uakino', 'UAKino'));
      expect(registry.getById('uakino'), isNotNull);

      registry.unregister('uakino');
      expect(registry.getById('uakino'), isNull);
    });
  });
}
