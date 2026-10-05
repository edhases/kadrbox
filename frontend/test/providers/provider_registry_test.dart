import 'package:flutter_test/flutter_test.dart';

import 'package:kadrbox/data/models/provider_catalog.dart';
import 'package:kadrbox/data/providers/provider_registry.dart';
import 'package:kadrbox/data/providers/server_backed_provider.dart';
import 'package:kadrbox/domain/entities/entities.dart';

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
      registry.register(makeProvider('src_a', 'Source A'));
      registry.register(makeProvider('src_b', 'Source B'));

      expect(registry.all.length, 2);
    });

    test('should not register duplicate providers', () {
      final provider = makeProvider('src_a', 'Source A');
      registry.register(provider);
      registry.register(provider);

      // Second registration with same ID should be ignored
      expect(registry.all.length, 1);
    });

    test('getById should return correct provider', () {
      registry.register(makeProvider('src_a', 'Source A'));
      registry.register(makeProvider('src_b', 'Source B'));

      final result = registry.getById('src_a');

      expect(result, isNotNull);
      expect(result?.id, 'src_a');
    });

    test('getById should return null for unknown id', () {
      registry.register(makeProvider('src_a', 'Source A'));

      final result = registry.getById('unknown');

      expect(result, isNull);
    });

    test('getByContentType should return matching providers', () {
      registry.register(makeProvider('src_a', 'Source A'));
      registry.register(makeProvider('src_b', 'Source B'));

      final movieProviders = registry.getByContentType(ContentType.movie);
      final animeProviders = registry.getByContentType(ContentType.anime);

      expect(movieProviders.length, 2);
      expect(animeProviders.length, 2);
    });

    test(
      'homeProviders should exclude providers whose catalog says showOnHome=false',
      () {
        registry.register(makeProvider('src_a', 'Source A', home: true));
        registry.register(makeProvider('dedicated', 'Dedicated', home: false));

        expect(registry.homeProviders.map((p) => p.id), contains('src_a'));
        expect(
          registry.homeProviders.map((p) => p.id),
          isNot(contains('dedicated')),
        );
      },
    );

    test(
      'backend catalog snapshot overrides the provider-owned showOnHome flag',
      () {
        registry.register(makeProvider('src_a', 'Source A', home: true));
        registry.register(makeProvider('src_c', 'Source C', home: true));

        registry.applyBackendCatalog(
          enabled: const {'src_a': true, 'src_c': true},
          showOnHome: const {'src_a': true, 'src_c': false},
          fixedStreams: const {'src_a': false, 'src_c': false},
        );

        expect(registry.homeProviders.map((p) => p.id), ['src_a']);
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
      registry.register(makeProvider('src_a', 'Source A'));
      expect(registry.getById('src_a'), isNotNull);

      registry.unregister('src_a');
      expect(registry.getById('src_a'), isNull);
    });
  });
}
