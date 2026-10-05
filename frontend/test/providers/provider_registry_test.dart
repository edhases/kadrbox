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

  ServerBackedProvider makeProvider(
    String id,
    String name, {
    bool home = true,
  }) {
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

    test(
      'homeProviders should exclude providers whose catalog says showOnHome=false',
      () {
        registry.register(makeProvider('uakino', 'UAKino', home: true));
        registry.register(makeProvider('dedicated', 'Dedicated', home: false));

        expect(registry.homeProviders.map((p) => p.id), contains('uakino'));
        expect(
          registry.homeProviders.map((p) => p.id),
          isNot(contains('dedicated')),
        );
      },
    );

    test(
      'backend catalog snapshot overrides the provider-owned showOnHome flag',
      () {
        registry.register(makeProvider('uakino', 'UAKino', home: true));
        registry.register(makeProvider('lavakino', 'Lavakino', home: true));

        registry.applyBackendCatalog(
          enabled: const {'uakino': true, 'lavakino': true},
          showOnHome: const {'uakino': true, 'lavakino': false},
          fixedStreams: const {'uakino': false, 'lavakino': false},
        );

        expect(registry.homeProviders.map((p) => p.id), ['uakino']);
      },
    );

    test('hasFixed falls back to the provider catalog flag', () {
      final fixed = makeProvider('fixedstreams', 'FixedStreams');
      expect(registry.hasFixed(fixed), isFalse);

      registry.applyBackendCatalog(
        enabled: const {'fixedstreams': true},
        showOnHome: const {'fixedstreams': true},
        fixedStreams: const {'fixedstreams': true},
      );
      expect(registry.hasFixed(fixed), isTrue);
    });

    test('unregister should remove provider', () {
      registry.register(makeProvider('uakino', 'UAKino'));
      expect(registry.getById('uakino'), isNotNull);

      registry.unregister('uakino');
      expect(registry.getById('uakino'), isNull);
    });
  });
}
