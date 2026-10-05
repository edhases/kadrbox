// Tests for WatchPartyService using a fake backend to simulate incoming messages

import 'package:flutter_test/flutter_test.dart';
import 'package:oxide_film/data/services/watch_party_service.dart';
import 'package:oxide_film/data/services/oxide_server_service.dart';
import 'package:oxide_film/data/services/settings_service.dart';

class MockOxideServerService extends Fake implements OxideServerService {}

class MockSettingsService extends Fake implements SettingsService {
  @override
  SettingsState get state => const SettingsState();
}

class _FakeBackend implements WatchPartyBackend {
  late Function(WatchPartyMessage) _onMessage;
  bool connected = false;

  @override
  Future<void> connect({
    required String roomCode,
    required bool isHost,
    required String myId,
    required String myName,
    required Function(WatchPartyMessage) onMessage,
    void Function()? onDisconnected,
  }) async {
    connected = true;
    _onMessage = onMessage;
  }

  @override
  void sendBroadcast(WatchPartyMessage message) {
    // No-op for test; we can simulate incoming message using emit
  }

  @override
  void sendMessage(String targetId, WatchPartyMessage message) {
    // No-op
  }

  @override
  Future<void> disconnect() async {
    connected = false;
  }

  void emit(WatchPartyMessage message) {
    // _onMessage might not be initialized if connect wasn't called
    // In this test, connect is called by joinRoom/hostRoom
    _onMessage(message);
  }
}

void main() {
  group('WatchPartyService (fake backend)', () {
    test('client reacts to play and pause messages from backend', () async {
      final fake = _FakeBackend();
      final service = WatchPartyService(
        server: MockOxideServerService(),
        settings: MockSettingsService(),
        backendFactory: (_) => fake,
      );

      bool callbackCalled = false;
      bool? lastPlayState;

      service.onPlayPauseChanged = (isPlaying) {
        callbackCalled = true;
        lastPlayState = isPlaying;
      };

      // Join a room (client) - this will call backend.connect and set up onMessage
      final joined = await service.joinRoom('ABC123');
      expect(joined, isTrue);
      expect(service.state, WatchPartyState.connected);

      // Simulate incoming play message
      fake.emit(
        WatchPartyMessage(type: WatchPartyMessageType.play, senderId: 'host'),
      );

      expect(service.isPlaying, isTrue);
      expect(callbackCalled, isTrue);
      expect(lastPlayState, isTrue);

      // Simulate incoming pause message
      callbackCalled = false;
      fake.emit(
        WatchPartyMessage(type: WatchPartyMessageType.pause, senderId: 'host'),
      );

      expect(service.isPlaying, isFalse);
      expect(callbackCalled, isTrue);
      expect(lastPlayState, isFalse);

      await service.leaveRoom();
    });

    test(
      'seek message updates position and triggers onSeek callback',
      () async {
        final fake = _FakeBackend();
        final service = WatchPartyService(
          server: MockOxideServerService(),
          settings: MockSettingsService(),
          backendFactory: (_) => fake,
        );

        Duration? seekedTo;
        service.onSeek = (pos) => seekedTo = pos;

        final joined = await service.joinRoom('XYZ789');
        expect(joined, isTrue);

        fake.emit(
          WatchPartyMessage(
            type: WatchPartyMessageType.seek,
            senderId: 'host',
            payload: 45000, // 45 seconds in ms
          ),
        );

        expect(seekedTo, Duration(milliseconds: 45000));

        await service.leaveRoom();
      },
    );
  });
}
