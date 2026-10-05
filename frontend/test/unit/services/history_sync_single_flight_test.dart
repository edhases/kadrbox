// Regression tests for the missing mutual exclusion between cloud pull and
// cloud push in `HistoryService`.
//
// `HistoryService` is a GetIt singleton. The auth listener kicks off a pull
// while the 5-minute timer may be pushing 500 items. Both used to flip the same
// `_isSyncing` boolean and both `notifyListeners()`, so whichever finished first
// cleared the flag while the other was still writing; and because both mutate
// the same history keys through a non-atomic DAO path, they produced the
// duplicate rows covered by `history_dao_atomic_test.dart`.

import 'dart:async';

import 'package:flutter/foundation.dart' show VoidCallback;
import 'package:flutter_test/flutter_test.dart';
import 'package:mocktail/mocktail.dart';
import 'package:kadrbox/data/database/app_database.dart';
import 'package:kadrbox/data/database/dao/history_dao.dart';
import 'package:kadrbox/data/services/auth_service.dart';
import 'package:kadrbox/data/services/history_service.dart';
import 'package:kadrbox/data/services/kadrbox_server_service.dart';

import '../../helpers/in_memory_db.dart';

class _MockServer extends Mock implements KadrboxServerService {}

class _MockAuthService extends Mock implements AuthService {}

/// Waits until the test has installed a completer under [name], then awaits it.
/// This lets a fake HTTP call block until the test explicitly releases it.
Future<void> _waitForGate(
  Map<String, Completer<void>> gate,
  String name,
) async {
  while (true) {
    final completer = gate[name];
    if (completer != null) return completer.future;
    await Future<void>.delayed(const Duration(milliseconds: 1));
  }
}

void main() {
  late AppDatabase db;
  late HistoryDao dao;
  late _MockServer server;
  late _MockAuthService authService;
  late HistoryService service;

  int getHistoryCalls = 0;
  int saveHistoryCalls = 0;
  final gate = <String, Completer<void>>{};

  /// Releases a gated call. Exits immediately when nothing is gated, so a test
  /// can await it before starting the sync.
  Future<void> release(String name) async {
    final completer = gate.remove(name);
    if (completer != null && !completer.isCompleted) completer.complete();
  }

  setUp(() {
    db = createTestAppDatabase();
    dao = HistoryDao(db);
    server = _MockServer();
    authService = _MockAuthService();
    getHistoryCalls = 0;
    saveHistoryCalls = 0;
    gate.clear();

    when(() => authService.isAuthenticated).thenReturn(true);
    when(
      () => authService.currentUser,
    ).thenReturn((id: 'u1', email: 'u@example.com', name: 'U'));
    when(() => server.isAuthenticated).thenReturn(true);

    when(
      () => server.getHistory(
        limit: any(named: 'limit'),
        offset: any(named: 'offset'),
      ),
    ).thenAnswer((_) async {
      getHistoryCalls++;
      if (gate.containsKey('pull')) await _waitForGate(gate, 'pull');
      return <Map<String, dynamic>>[];
    });

    when(
      () => server.saveHistoryProgress(
        mediaId: any(named: 'mediaId'),
        providerId: any(named: 'providerId'),
        title: any(named: 'title'),
        posterUrl: any(named: 'posterUrl'),
        year: any(named: 'year'),
        mediaType: any(named: 'mediaType'),
        positionMs: any(named: 'positionMs'),
        durationMs: any(named: 'durationMs'),
        season: any(named: 'season'),
        episode: any(named: 'episode'),
        episodeTitle: any(named: 'episodeTitle'),
        lastStreamUrl: any(named: 'lastStreamUrl'),
        voiceover: any(named: 'voiceover'),
        watchedAt: any(named: 'watchedAt'),
      ),
    ).thenAnswer((_) async {
      saveHistoryCalls++;
    });

    service = HistoryService(
      database: db,
      server: server,
      authService: authService,
    );
  });

  tearDown(() async {
    service.dispose();
    await db.close();
  });

  Future<void> seedLocalRow(String mediaId) => dao.saveProgress(
    mediaId: mediaId,
    providerId: 'src_a',
    title: mediaId,
    mediaType: 'movie',
    positionMs: 1,
    durationMs: 10,
  );

  test(
    'two concurrent syncNow passes collapse onto a single HTTP pull',
    () async {
      gate['pull'] = Completer<void>();

      final first = service.syncNow();
      // Let the first pull actually reach the gate.
      while (getHistoryCalls < 1) {
        await Future<void>.delayed(const Duration(milliseconds: 1));
      }
      expect(service.isSyncing, isTrue);

      // Second concurrent pass: must not start another HTTP call.
      final second = service.syncNow();
      final third = service.syncNow();
      await Future<void>.delayed(const Duration(milliseconds: 20));
      expect(getHistoryCalls, 1);

      await release('pull');
      await Future.wait([first, second, third]);

      expect(getHistoryCalls, 1, reason: 'pulls must be de-duplicated');
      expect(service.isSyncing, isFalse);
    },
  );

  test('no push runs while a pull is in flight', () async {
    await seedLocalRow('m1');
    gate['pull'] = Completer<void>();

    final a = service.syncNow();
    while (getHistoryCalls < 1) {
      await Future<void>.delayed(const Duration(milliseconds: 1));
    }

    // Both passes are still parked on the pull, so nothing may be pushed yet.
    final b = service.syncNow();
    await Future<void>.delayed(const Duration(milliseconds: 20));
    expect(
      saveHistoryCalls,
      0,
      reason: 'push must not run concurrently against the same keys',
    );

    await release('pull');
    await Future.wait([a, b]);

    expect(service.isSyncing, isFalse);
    // Once the guard is released, a later pass does push.
    await service.syncNow();
    expect(saveHistoryCalls, greaterThanOrEqualTo(1));
  });

  test('isSyncing never reads false while work is in flight', () async {
    gate['pull'] = Completer<void>();
    final first = service.syncNow();
    while (getHistoryCalls < 1) {
      await Future<void>.delayed(const Duration(milliseconds: 1));
    }

    // Only start sampling once the pull is genuinely in flight, otherwise the
    // DAO streams' initial emissions count as "mid-flight false".
    var flightOver = false;
    final inFlightObservations = <bool>[];
    service.addListener(() {
      if (!flightOver) inFlightObservations.add(service.isSyncing);
    });

    final second = service.syncNow();
    await Future<void>.delayed(const Duration(milliseconds: 20));
    expect(service.isSyncing, isTrue);

    // The pull's own completion notification legitimately reports false, so
    // close the sampling window before releasing it.
    flightOver = true;
    await release('pull');
    await Future.wait([first, second]);

    expect(service.isSyncing, isFalse);
    expect(
      inFlightObservations,
      isNot(contains(false)),
      reason: 'listeners must never observe isSyncing == false mid-flight',
    );
  });

  test('auth-listener pull is de-duplicated against a syncNow pull', () async {
    gate['pull'] = Completer<void>();

    // Capture the listener HistoryService registers on AuthService and fire it
    // while a syncNow pass is already parked on the pull.
    final registered = <VoidCallback>[];
    when(() => authService.addListener(any())).thenAnswer((invocation) {
      registered.add(invocation.positionalArguments.first as VoidCallback);
      return null;
    });
    when(() => authService.removeListener(any())).thenReturn(null);

    final listener = HistoryService(
      database: db,
      server: server,
      authService: authService,
    );
    addTearDown(listener.dispose);

    // The startup pull from `_init` is already gated.
    while (getHistoryCalls < 1) {
      await Future<void>.delayed(const Duration(milliseconds: 1));
    }
    final afterStartupPull = getHistoryCalls;

    // Trigger the auth listener a few more times while the pull is in flight.
    for (final callback in registered) {
      callback();
    }
    await Future<void>.delayed(const Duration(milliseconds: 20));
    expect(
      getHistoryCalls,
      afterStartupPull,
      reason: 'listener-triggered pulls must join the in-flight flight',
    );

    await release('pull');
  });

  test('listener notifications after dispose do not throw', () async {
    gate['pull'] = Completer<void>();
    final run = service.syncNow();
    while (getHistoryCalls < 1) {
      await Future<void>.delayed(const Duration(milliseconds: 1));
    }

    service.dispose();
    await release('pull');
    // Would throw "used after being disposed" if dispose() were not guarded.
    await run;
  });
}
