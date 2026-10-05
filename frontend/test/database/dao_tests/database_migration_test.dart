import 'package:drift/drift.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:kadrbox/data/database/app_database.dart';
import 'package:kadrbox/data/database/dao/settings_dao.dart';

import '../../helpers/in_memory_db.dart';

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();

  group('Database Initialization & Migration Tests', () {
    test('Default settings are populated on onCreate', () async {
      final db = createTestAppDatabase();
      final settingsDao = SettingsDao(db);

      // Verify defaults
      expect(await settingsDao.getSetting('theme'), 'dark');
      expect(await settingsDao.getSetting('default_quality'), 'auto');
      expect(await settingsDao.getSetting('player_type'), 'internal');
      expect(await settingsDao.getSetting('auto_play_next'), 'true');
      expect(await settingsDao.getSetting('remember_position'), 'true');
      expect(await settingsDao.getSetting('subtitle_language'), 'uk');

      await db.close();
    });

    test('Migration step < 10 keeps rows of providers that still exist', () async {
      final db = createTestAppDatabase();

      // The step-10 upgrade is a data-only cleanup of sources the app no longer
      // ships. It must be narrow: rows belonging to a source that is still in
      // the catalog have to survive untouched.
      await db
          .into(db.enabledProviders)
          .insert(
            EnabledProvidersCompanion.insert(providerId: 'src_a'),
            mode: InsertMode.insertOrIgnore,
          );
      await db
          .into(db.favorites)
          .insert(
            FavoritesCompanion.insert(
              mediaId: 'm1',
              providerId: 'src_a',
              title: 'Kept Movie',
              mediaType: 'movie',
            ),
            mode: InsertMode.insertOrIgnore,
          );
      await db
          .into(db.watchHistory)
          .insert(
            WatchHistoryCompanion.insert(
              mediaId: 'm1',
              providerId: 'src_a',
              title: 'Kept History',
              mediaType: 'movie',
            ),
            mode: InsertMode.insertOrIgnore,
          );
      await db
          .into(db.storedMediaItems)
          .insert(
            StoredMediaItemsCompanion.insert(
              id: 'm1',
              providerId: 'src_a',
              title: 'Kept Cache',
              mediaType: 'movie',
            ),
            mode: InsertMode.insertOrReplace,
          );

      // Verify the rows exist before the upgrade logic runs.
      var favs = await db.select(db.favorites).get();
      expect(favs.any((f) => f.providerId == 'src_a'), isTrue);

      // Execute migration callback for from=9, to=10
      final migrator = db.createMigrator();
      await db.migration.onUpgrade(migrator, 9, 10);

      // Verify every table kept the rows of a source that still exists.
      favs = await db.select(db.favorites).get();
      expect(favs.any((f) => f.providerId == 'src_a'), isTrue);

      final hist = await db.select(db.watchHistory).get();
      expect(hist.any((h) => h.providerId == 'src_a'), isTrue);

      final provs = await db.select(db.enabledProviders).get();
      expect(provs.any((p) => p.providerId == 'src_a'), isTrue);

      final cached = await db.select(db.storedMediaItems).get();
      expect(cached.any((c) => c.providerId == 'src_a'), isTrue);

      await db.close();
    });
  });
}
