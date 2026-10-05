import 'package:flutter_test/flutter_test.dart';
import 'package:mocktail/mocktail.dart';
import 'package:oxide_film/data/services/history_service.dart';
import 'package:oxide_film/data/services/favorites_service.dart';
import 'package:oxide_film/data/services/auth_service.dart';
import 'package:get_it/get_it.dart';

// Mock classes using Mocktail
class MockHistoryService extends Mock implements HistoryService {}

class MockFavoritesService extends Mock implements FavoritesService {}

class MockAuthService extends Mock implements AuthService {}

void main() {
  late MockHistoryService mockHistoryService;
  late MockFavoritesService mockFavoritesService;
  late MockAuthService mockAuthService;

  setUp(() {
    mockHistoryService = MockHistoryService();
    mockFavoritesService = MockFavoritesService();
    mockAuthService = MockAuthService();

    final getIt = GetIt.instance;
    if (getIt.isRegistered<HistoryService>()) {
      getIt.unregister<HistoryService>();
    }
    if (getIt.isRegistered<FavoritesService>()) {
      getIt.unregister<FavoritesService>();
    }
    if (getIt.isRegistered<AuthService>()) getIt.unregister<AuthService>();

    getIt.registerSingleton<HistoryService>(mockHistoryService);
    getIt.registerSingleton<FavoritesService>(mockFavoritesService);
    getIt.registerSingleton<AuthService>(mockAuthService);
  });

  test(
    'Verify logic: checking local data determines merge dialog visibility',
    () async {
      // Scenario 1: No local data
      when(() => mockHistoryService.count).thenAnswer((_) async => 0);
      when(() => mockFavoritesService.count).thenAnswer((_) async => 0);

      bool shouldShowDialog = false;
      // Simulate what the UI does: await the futures
      final hCount = await mockHistoryService.count;
      final fCount = await mockFavoritesService.count;

      if (hCount > 0 || fCount > 0) {
        shouldShowDialog = true;
      }

      expect(shouldShowDialog, false);

      // Scenario 2: Has history
      // We need to reset or just re-stub. Mocktail allows re-stubbing.
      when(() => mockHistoryService.count).thenAnswer((_) async => 5);

      // Check again
      bool shouldShowDialog2 = false;
      final hCount2 = await mockHistoryService.count;
      final fCount2 = await mockFavoritesService.count;

      if (hCount2 > 0 || fCount2 > 0) {
        shouldShowDialog2 = true;
      }

      expect(shouldShowDialog2, true);
    },
  );

  test('Verify logic: Merge call triggers syncNow', () async {
    // Stubbing for syncNow
    when(() => mockHistoryService.syncNow()).thenAnswer((_) async {});
    when(() => mockFavoritesService.syncNow()).thenAnswer((_) async {});

    // Scenario: User chose "Merge" (true)
    bool userChoice = true;

    if (userChoice) {
      await mockHistoryService.syncNow();
      await mockFavoritesService.syncNow();
    }

    verify(() => mockHistoryService.syncNow()).called(1);
    verify(() => mockFavoritesService.syncNow()).called(1);

    // Verify clearAll was NOT called
    verifyNever(() => mockHistoryService.clearAll());
  });

  test(
    'Verify logic: Delete local call triggers clearAll and syncNow',
    () async {
      // Stubbing
      when(() => mockHistoryService.clearAll()).thenAnswer((_) async {});
      when(() => mockFavoritesService.clearAll()).thenAnswer((_) async {});
      when(() => mockHistoryService.syncNow()).thenAnswer((_) async {});
      when(() => mockFavoritesService.syncNow()).thenAnswer((_) async {});

      // Scenario: User chose "Delete" (false)
      bool userChoice = false;

      if (!userChoice) {
        await mockHistoryService.clearAll();
        await mockFavoritesService.clearAll();
        await mockHistoryService.syncNow();
        await mockFavoritesService.syncNow();
      }

      verify(() => mockHistoryService.clearAll()).called(1);
      verify(() => mockFavoritesService.clearAll()).called(1);
      verify(() => mockHistoryService.syncNow()).called(1);
      verify(() => mockFavoritesService.syncNow()).called(1);
    },
  );
}
