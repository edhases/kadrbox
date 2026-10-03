import 'package:flutter_test/flutter_test.dart';
import 'package:get_it/get_it.dart';
import 'package:oxide_film/data/services/watch_party_service.dart';
import 'package:oxide_film/data/services/oxide_server_service.dart';
import 'package:oxide_film/data/services/settings_service.dart';

// Manual Mocks
class MockWatchPartyBackend implements WatchPartyBackend {
  final List<String> log = [];

  Function(WatchPartyMessage)? onMessageCallback;
  bool isConnected = false;

  @override
  Future<void> connect({
    required String roomCode,
    required bool isHost,
    required String myId,
    required String myName,
    required Function(WatchPartyMessage) onMessage,
    void Function()? onDisconnected,
  }) async {
    log.add('connect(roomCode: $roomCode, isHost: $isHost)');
    onMessageCallback = onMessage;
    isConnected = true;
  }

  @override
  Future<void> disconnect() async {
    log.add('disconnect');
    isConnected = false;
  }

  @override
  void sendBroadcast(WatchPartyMessage message) {
    log.add('sendBroadcast(${message.type})');
  }

  @override
  void sendMessage(String targetId, WatchPartyMessage message) {
    log.add('sendMessage($targetId, ${message.type})');
  }
}

class MockSettingsService extends Fake implements SettingsService {
  String _name = 'Test User';

  @override
  SettingsState get state => SettingsState(watchPartyName: _name);

  @override
  Future<void> setWatchPartyName(String name) async {
    _name = name;
  }
}

class MockOxideServerService extends Fake implements OxideServerService {}

void main() {
  late WatchPartyService service;
  late MockWatchPartyBackend mockBackend;
  late MockSettingsService mockSettings;
  late MockOxideServerService mockServer;

  setUp(() {
    GetIt.I.reset();
    mockSettings = MockSettingsService();
    mockServer = MockOxideServerService();
    GetIt.I.registerSingleton<SettingsService>(mockSettings);
    GetIt.I.registerSingleton<OxideServerService>(mockServer);

    mockBackend = MockWatchPartyBackend();

    // Inject factory to return our mock backend
    service = WatchPartyService(
      server: mockServer,
      settings: mockSettings,
      backendFactory: (_) => mockBackend,
    );
  });

  group('WatchPartyService Connection', () {
    test('hostRoom connects using generated room code', () async {
      final result = await service.hostRoom(
        mediaUrl: 'http://test.com',
        mediaTitle: 'Test Movie',
      );

      expect(result, isTrue);
      expect(service.isHost, isTrue);
      expect(service.currentRoomCode, isNotNull);
      expect(service.state, WatchPartyState.connected);
      expect(
        mockBackend.log,
        contains(predicate((String s) => s.startsWith('connect(roomCode: '))),
      );
    });

    test('joinRoom connects using provided room code', () async {
      const roomCode = 'ABC123';
      final result = await service.joinRoom(roomCode);

      expect(result, isTrue);
      expect(service.isHost, isFalse);
      expect(service.currentRoomCode, equals(roomCode));
      expect(service.state, WatchPartyState.connected);
      expect(
        mockBackend.log,
        contains('connect(roomCode: $roomCode, isHost: false)'),
      );
    });
  });

  group('WatchPartyService Message Handling', () {
    test('Handles userJoined message', () async {
      await service.joinRoom('ROOM');

      // Simulate incoming message
      final msg = WatchPartyMessage(
        type: WatchPartyMessageType.userJoined,
        senderId: 'user2',
        senderName: 'Alice',
      );
      mockBackend.onMessageCallback?.call(msg);

      expect(service.participants.length, 2); // Me + Alice
      expect(service.participants.last.name, 'Alice');
    });

    test('Handles chat message', () async {
      await service.joinRoom('ROOM');

      final msg = WatchPartyMessage(
        type: WatchPartyMessageType.chat,
        senderId: 'user2',
        senderName: 'Alice',
        payload: 'Hello World',
      );
      mockBackend.onMessageCallback?.call(msg);

      expect(service.chatMessages.length, 1);
      expect(service.chatMessages.first.message, 'Hello World');
    });

    test('Handles userLeft message', () async {
      await service.joinRoom('ROOM');

      // First, add a user
      final joinMsg = WatchPartyMessage(
        type: WatchPartyMessageType.userJoined,
        senderId: 'user2',
        senderName: 'Alice',
      );
      mockBackend.onMessageCallback?.call(joinMsg);
      expect(service.participants.length, 2);

      // Now remove user
      final leftMsg = WatchPartyMessage(
        type: WatchPartyMessageType.userLeft,
        senderId: 'user2',
        senderName: 'Alice',
      );
      mockBackend.onMessageCallback?.call(leftMsg);

      expect(service.participants.length, 1);
      expect(service.participants.any((p) => p.id == 'user2'), isFalse);
    });

    test('Multiple participants management', () async {
      await service.joinRoom('ROOM');

      // Add multiple users
      for (final name in ['Alice', 'Bob', 'Charlie']) {
        mockBackend.onMessageCallback?.call(
          WatchPartyMessage(
            type: WatchPartyMessageType.userJoined,
            senderId: 'user_$name',
            senderName: name,
          ),
        );
      }

      expect(service.participants.length, 4); // Me + 3 others
      expect(
        service.participants.map((p) => p.name).toList(),
        containsAll(['Alice', 'Bob', 'Charlie']),
      );
    });

    test('leaveRoom calls disconnect and resets state', () async {
      await service.joinRoom('ROOM');
      expect(service.state, WatchPartyState.connected);

      await service.leaveRoom();

      expect(service.state, WatchPartyState.idle);
      expect(service.currentRoomCode, isNull);
      expect(mockBackend.log, contains('disconnect'));
    });
  });

  group('WatchPartyService Sync Logic', () {
    // Callbacks to verify actions
    double? lastSpeed;
    Duration? lastSeek;

    setUp(() async {
      // Reset state for sync tests
      lastSpeed = null;
      lastSeek = null;

      service.onSpeedChanged = (speed) => lastSpeed = speed;
      service.onSeek = (pos) => lastSeek = pos;
      // Drift is asserted via service.correctionMode, which the same logic
      // sets, so this callback does not need its own captured variable.

      // Connect as client
      await service.joinRoom('ROOM');
    });

    void simulateSyncMessage({required int hostPosMs, required double speed}) {
      final msg = WatchPartyMessage(
        type: WatchPartyMessageType.sync,
        senderId: 'host_id',
        senderName: 'Host',
        payload: {
          'position': hostPosMs,
          'isPlaying': true,
          'speed': speed,
          'mediaUrl': 'url',
          'mediaTitle': 'title',
        },
        // Timestamp is roughly now, so network delay ~0 for tested logic
        timestamp: DateTime.now(),
      );
      mockBackend.onMessageCallback?.call(msg);
    }

    // The sync handler compensates for network latency: it seeks to
    // `hostPosition + clamp(now - messageTimestamp, 0, 5000)`
    // (watch_party_service.dart _handleSyncMessage). That `DateTime.now()` is a
    // real read in production code, so the exact target moves by however long
    // the event loop took between building the message and handling it — one
    // millisecond under load, zero when the machine is idle. Asserting the bare
    // 30000 therefore failed only under parallel execution.
    //
    // This brackets that read instead of guessing it: `before` and `after`
    // sandwich the handler's `now`, and the message's timestamp falls in the
    // same window, so the compensation is provably within [0, after - before].
    // When both reads land in the same millisecond the window is empty and the
    // assertion is exact; when it is wider it is a genuine upper bound on
    // elapsed time, never a fudge factor around the expected value.
    ({int seekMs, Duration maxLatencyCompensation}) simulateSyncAndMeasureSeek({
      required int hostPosMs,
      required double speed,
    }) {
      final before = DateTime.now().toUtc();
      simulateSyncMessage(hostPosMs: hostPosMs, speed: speed);
      final after = DateTime.now().toUtc();

      final seek = lastSeek;
      if (seek == null)
        return (seekMs: -1, maxLatencyCompensation: Duration.zero);
      return (
        seekMs: seek.inMilliseconds,
        maxLatencyCompensation: after.difference(before),
      );
    }

    test('Drift < 5s (ignore threshold) - No Correction', () {
      // Host at 10s, Me at 12s (Drift 2000ms, less than 5s threshold)
      service.updateLocalPosition(const Duration(milliseconds: 12000));
      simulateSyncMessage(hostPosMs: 10000, speed: 1.0);

      expect(service.correctionMode, SyncCorrectionMode.none);
      expect(lastSpeed, isNull); // No speed change
      expect(lastSeek, isNull); // No seek
    });

    test('Drift 7s (behind) - Smooth Correction (Speed Up)', () {
      // Host at 17s, Me at 10s (Drift -7000ms, between 5s and 10s)
      service.updateLocalPosition(const Duration(milliseconds: 10000));
      simulateSyncMessage(hostPosMs: 17000, speed: 1.0);

      expect(service.correctionMode, SyncCorrectionMode.speedUp);
      expect(lastSpeed, closeTo(1.1, 0.01)); // 1.1x speed
    });

    test('Drift 7s (ahead) - Smooth Correction (Slow Down)', () {
      // Host at 10s, Me at 17s (Drift +7000ms, between 5s and 10s)
      service.updateLocalPosition(const Duration(milliseconds: 17000));
      simulateSyncMessage(hostPosMs: 10000, speed: 1.0);

      expect(service.correctionMode, SyncCorrectionMode.slowDown);
      expect(lastSpeed, closeTo(0.9, 0.01)); // 0.9x speed
    });

    test('Drift 12s (behind) - Fast Correction (Speed Up)', () {
      // Host at 22s, Me at 10s (Drift -12000ms, between 10s and 15s)
      service.updateLocalPosition(const Duration(milliseconds: 10000));
      simulateSyncMessage(hostPosMs: 22000, speed: 1.0);

      expect(service.correctionMode, SyncCorrectionMode.speedUp);
      expect(lastSpeed, closeTo(1.2, 0.01)); // 1.2x speed
    });

    test('Drift 20s (behind) - Hard Seek', () {
      // Host at 30s, Me at 10s (Drift -20000ms, > 15s threshold)
      service.updateLocalPosition(const Duration(milliseconds: 10000));
      final result = simulateSyncAndMeasureSeek(hostPosMs: 30000, speed: 1.0);

      expect(service.correctionMode, SyncCorrectionMode.hardSeek);
      expect(lastSeek, isNotNull);

      const hostPositionMs = 30000;
      expect(
        result.seekMs,
        inInclusiveRange(
          hostPositionMs,
          hostPositionMs + result.maxLatencyCompensation.inMilliseconds,
        ),
        reason:
            'a hard seek must land on the host position, adjusted only by the '
            'measured network latency (at most '
            '${result.maxLatencyCompensation.inMilliseconds}ms here)',
      );
    });
  });
}
