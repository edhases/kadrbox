import 'dart:async';
import 'dart:convert';
import 'dart:io';
import 'package:flutter/foundation.dart';

/// Lightweight, native Discord Rich Presence client for Desktop (Windows & Linux).
/// Connects directly to Discord local IPC pipe/socket without legacy C++ or ffi conflicts.
class DiscordRpcService {
  static const String _defaultClientId = '123456789012345678'; // Default generic Client ID
  Socket? _socket;
  RandomAccessFile? _pipe;
  bool _isConnected = false;
  String _clientId = _defaultClientId;

  bool get isSupported =>
      !kIsWeb &&
      (defaultTargetPlatform == TargetPlatform.windows ||
          defaultTargetPlatform == TargetPlatform.linux ||
          defaultTargetPlatform == TargetPlatform.macOS);

  Future<void> initialize({String? clientId}) async {
    if (!isSupported) return;
    if (clientId != null && clientId.isNotEmpty) {
      _clientId = clientId;
    }
    await _connect();
  }

  Future<void> _connect() async {
    if (!isSupported || _isConnected) return;

    try {
      if (defaultTargetPlatform == TargetPlatform.windows) {
        for (var i = 0; i < 10; i++) {
          try {
            final pipePath = r'\\.\pipe\discord-ipc-' + i.toString();
            final file = File(pipePath);
            _pipe = await file.open(mode: FileMode.writeOnlyAppend);
            _isConnected = true;
            break;
          } catch (_) {}
        }
      } else {
        final runtimeDir = Platform.environment['XDG_RUNTIME_DIR'] ??
            Platform.environment['TMPDIR'] ??
            Platform.environment['TMP'] ??
            '/tmp';
        for (var i = 0; i < 10; i++) {
          try {
            final socketPath = '$runtimeDir/discord-ipc-$i';
            _socket = await Socket.connect(
              InternetAddress(socketPath, type: InternetAddressType.unix),
              0,
            );
            _isConnected = true;
            break;
          } catch (_) {}
        }
      }

      if (_isConnected) {
        // Send handshake (Opcode 0 = Handshake)
        final handshakePayload = jsonEncode({
          'v': 1,
          'client_id': _clientId,
        });
        await _sendPacket(0, handshakePayload);
      }
    } catch (e) {
      debugPrint('Discord RPC connection failed (Discord likely not running): $e');
      _isConnected = false;
      try {
        _pipe?.close();
      } catch (_) {}
      try {
        _socket?.destroy();
      } catch (_) {}
      _pipe = null;
      _socket = null;
    }
  }

  Future<void> updatePresence({
    required String details,
    String? state,
    int? startTimestamp,
    int? endTimestamp,
  }) async {
    if (!isSupported) return;
    if (!_isConnected) {
      await _connect();
      if (!_isConnected) return;
    }

    try {
      final activity = <String, dynamic>{
        'details': details,
        'assets': {
          'large_image': 'oxide_logo',
          'large_text': 'Oxide Film',
        },
      };
      if (state != null) {
        activity['state'] = state;
      }
      if (startTimestamp != null || endTimestamp != null) {
        final timestamps = <String, dynamic>{};
        if (startTimestamp != null) timestamps['start'] = startTimestamp;
        if (endTimestamp != null) timestamps['end'] = endTimestamp;
        activity['timestamps'] = timestamps;
      }

      final payload = jsonEncode({
        'cmd': 'SET_ACTIVITY',
        'args': {
          'pid': pid,
          'activity': activity,
        },
        'nonce': DateTime.now().millisecondsSinceEpoch.toString(),
      });

      // Opcode 1 = Frame
      await _sendPacket(1, payload);
    } catch (e) {
      debugPrint('Failed to send Discord presence: $e');
    }
  }

  Future<void> clearPresence() async {
    if (!isSupported || !_isConnected) return;

    try {
      final payload = jsonEncode({
        'cmd': 'SET_ACTIVITY',
        'args': {
          'pid': pid,
          'activity': null,
        },
        'nonce': DateTime.now().millisecondsSinceEpoch.toString(),
      });
      await _sendPacket(1, payload);
    } catch (_) {}
  }

  Future<void> _sendPacket(int opcode, String jsonStr) async {
    final payloadBytes = utf8.encode(jsonStr);
    final buffer = ByteData(8 + payloadBytes.length);

    // Discord IPC Header: 4 bytes opcode (int32 le), 4 bytes length (int32 le)
    buffer.setInt32(0, opcode, Endian.little);
    buffer.setInt32(4, payloadBytes.length, Endian.little);

    final fullData = Uint8List.view(buffer.buffer);
    fullData.setRange(8, 8 + payloadBytes.length, payloadBytes);

    if (_pipe != null) {
      await _pipe!.writeFrom(fullData);
      await _pipe!.flush();
    } else if (_socket != null) {
      _socket!.add(fullData);
      await _socket!.flush();
    }
  }

  void dispose() {
    clearPresence();
    try {
      _pipe?.close();
    } catch (_) {}
    try {
      _socket?.destroy();
    } catch (_) {}
    _pipe = null;
    _socket = null;
    _isConnected = false;
  }
}
