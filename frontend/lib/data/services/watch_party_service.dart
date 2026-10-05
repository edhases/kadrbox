import 'dart:async';
import 'dart:convert';
import 'dart:io' show WebSocket;
import 'dart:math';
import 'package:flutter/material.dart';
import 'package:peerdart/peerdart.dart';
import 'package:get_it/get_it.dart';
import 'oxide_server_service.dart';
import 'settings_service.dart';
import '../../core/utils/logger.dart';

/// Watch party connection backend type
enum WatchPartyBackendType { server, peerdart, none }

/// Room state for watch party
enum WatchPartyState { idle, hosting, joining, connected, error }

/// Message types for watch party protocol
enum WatchPartyMessageType {
  sync, // Sync playback position
  play, // Play command
  pause, // Pause command
  seek, // Seek to position
  speed, // Playback speed change
  chat, // Chat message
  userJoined, // User joined notification
  userLeft, // User left notification
  roomInfo, // Room information
  requestSync, // Request sync from host
  unknown, // Unknown message type
}

/// Watch party message
class WatchPartyMessage {
  final WatchPartyMessageType type;
  final String senderId;
  final String? senderName;
  final dynamic payload;
  final DateTime timestamp;

  WatchPartyMessage({
    required this.type,
    required this.senderId,
    this.senderName,
    this.payload,
    DateTime? timestamp,
  }) : timestamp = timestamp ?? DateTime.now().toUtc();

  factory WatchPartyMessage.fromJson(Map<String, dynamic> json) {
    return WatchPartyMessage(
      type: WatchPartyMessageType.values.firstWhere(
        (t) =>
            t.name ==
            (json['action'] ?? json['type']), // Support both for migration
        orElse: () => WatchPartyMessageType.unknown,
      ),
      senderId: json['senderId'] ?? '',
      senderName: json['senderName'],
      payload: json['payload'],
      timestamp: json['timestamp'] != null
          ? DateTime.parse(json['timestamp']).toUtc()
          : DateTime.now().toUtc(),
    );
  }

  Map<String, dynamic> toJson() => {
    'action': type.name, // Renamed from 'type' to avoid collision
    'senderId': senderId,
    'senderName': senderName,
    'payload': payload,
    'timestamp': timestamp.toUtc().toIso8601String(),
  };
}

/// Chat message
class ChatMessage {
  final String id;
  final String senderId;
  final String senderName;
  final String message;
  final DateTime timestamp;

  ChatMessage({
    required this.id,
    required this.senderId,
    required this.senderName,
    required this.message,
    required this.timestamp,
  });
}

/// Participant in watch party
class WatchPartyParticipant {
  final String id;
  final String name;
  final bool isHost;
  final DateTime joinedAt;

  WatchPartyParticipant({
    required this.id,
    required this.name,
    required this.isHost,
    required this.joinedAt,
  });
}

/// Watch party room info
class WatchPartyRoom {
  final String id;
  final String hostId;
  final String hostName;
  final String? mediaTitle;
  final String? mediaUrl;

  WatchPartyRoom({
    required this.id,
    required this.hostId,
    required this.hostName,
    this.mediaTitle,
    this.mediaUrl,
  });

  String get roomCode => id;
}

/// Backend interface
abstract class WatchPartyBackend {
  Future<void> connect({
    required String roomCode,
    required bool isHost,
    required String myId,
    required String myName,
    required Function(WatchPartyMessage) onMessage,
    void Function()? onDisconnected,
  });
  Future<void> disconnect();
  void sendBroadcast(WatchPartyMessage message);
  void sendMessage(String targetId, WatchPartyMessage message);
}

/// Oxide Go Server WebSocket Backend Implementation
class _OxideServerBackend implements WatchPartyBackend {
  final OxideServerService _server;
  WebSocket? _ws;
  StreamSubscription? _sub;
  String? _roomCode;
  final String _tag = 'WatchParty_OxideServer';

  bool _isDisconnecting = false;

  _OxideServerBackend(this._server);

  @override
  Future<void> connect({
    required String roomCode,
    required bool isHost,
    required String myId,
    required String myName,
    required Function(WatchPartyMessage) onMessage,
    void Function()? onDisconnected,
  }) async {
    _roomCode = roomCode;
    _isDisconnecting = false;

    try {
      _ws = await _server.connectWatchParty(
        roomCode: roomCode,
        userId: myId,
        userName: myName,
      );

      _sub = _ws!.listen(
        (data) {
          try {
            final decoded = jsonDecode(data.toString());
            if (decoded is Map<String, dynamic>) {
              final msg = WatchPartyMessage.fromJson(decoded);
              onMessage(msg);
            }
          } catch (e) {
            Logger.w('Failed to decode WS message: $e', tag: _tag);
          }
        },
        onError: (err) {
          Logger.e('WS error: $err', tag: _tag);
          if (!_isDisconnecting) {
            onDisconnected?.call();
          }
        },
        onDone: () {
          Logger.i('WS connection closed', tag: _tag);
          if (!_isDisconnecting) {
            onDisconnected?.call();
          }
        },
      );
    } catch (e) {
      Logger.e(
        'Failed to connect to Oxide Server WebSocket',
        tag: _tag,
        error: e,
      );
      rethrow;
    }
  }

  @override
  Future<void> disconnect() async {
    _isDisconnecting = true;
    await _sub?.cancel();
    _sub = null;
    await _ws?.close();
    _ws = null;
  }

  @override
  void sendBroadcast(WatchPartyMessage message) {
    if (_ws != null && _ws!.readyState == WebSocket.open) {
      final json = jsonEncode({...message.toJson(), 'roomCode': _roomCode});
      _ws!.add(json);
    }
  }

  @override
  void sendMessage(String targetId, WatchPartyMessage message) {
    sendBroadcast(message);
  }
}

/// PeerDart Backend Implementation
class _PeerDartBackend implements WatchPartyBackend {
  Peer? _peer;
  final List<DataConnection> _connections = [];
  final String _tag = 'WatchParty_PeerDart';
  // DataConnection? _hostConnection; // Removed unused field

  @override
  Future<void> connect({
    required String roomCode,
    required bool isHost,
    required String myId,
    required String myName,
    required Function(WatchPartyMessage) onMessage,
    void Function()? onDisconnected,
  }) async {
    try {
      final peerId = isHost ? 'oxide-$roomCode' : null; // Custom ID for host
      _peer = Peer(id: peerId);

      _peer!.on('close').listen((_) {
        onDisconnected?.call();
      });
      _peer!.on('disconnected').listen((_) {
        onDisconnected?.call();
      });

      final completer = Completer<void>();

      _peer!.on('open').listen((id) {
        Logger.d('PeerDart open: $id', tag: _tag);
        if (!completer.isCompleted) completer.complete();
      });

      _peer!.on('error').listen((err) {
        Logger.e('PeerDart error', tag: _tag, error: err);
        if (!completer.isCompleted) completer.completeError(err);
      });

      Future<void> waitPeerOpen() => completer.future.timeout(
        const Duration(seconds: 10),
        onTimeout: () {
          throw TimeoutException(
            'Не вдалося з\'єднатися з сигнальним сервером PeerDart',
          );
        },
      );

      // Host logic
      if (isHost) {
        _peer!.on<DataConnection>('connection').listen((conn) {
          Logger.d('PeerDart client connected', tag: _tag);
          _connections.add(conn);
          _setupConnection(conn, onMessage);
        });
      }
      // Client logic
      else {
        // Wait for peer to open before connecting
        await waitPeerOpen();

        final hostId = 'oxide-$roomCode';
        Logger.d('Connecting to host: $hostId', tag: _tag);

        final conn = _peer!.connect(hostId);
        _connections.add(conn);

        final connCompleter = Completer<void>();

        conn.on('open').listen((_) {
          Logger.d('Connected to host', tag: _tag);
          if (!connCompleter.isCompleted) connCompleter.complete();
        });

        // Timeout for connection
        Future.delayed(const Duration(seconds: 15), () {
          if (!connCompleter.isCompleted) {
            connCompleter.completeError(Exception('Peer connection timeout'));
          }
        });

        _setupConnection(conn, onMessage);

        // Wait for connection to actually open before returning
        // so that _send operations don't fire into a closed pipe
        await connCompleter.future;
      }

      if (isHost) {
        await waitPeerOpen();
      }
    } catch (e) {
      Logger.e('PeerDart init failed', tag: _tag, error: e);
      _peer?.dispose();
      _peer = null;
      rethrow;
    }
  }

  void _setupConnection(
    DataConnection conn,
    Function(WatchPartyMessage) onMessage,
  ) {
    conn.on('data').listen((data) {
      try {
        // PeerDart data is dynamic, usually String or Map
        final json = data is String ? jsonDecode(data) : data;
        final message = WatchPartyMessage.fromJson(json);
        onMessage(message);
      } catch (e) {
        Logger.w('PeerDart parse error: $e', tag: _tag);
      }
    });

    conn.on('close').listen((_) {
      _connections.remove(conn);
    });
  }

  @override
  Future<void> disconnect() async {
    _connections.clear();
    _peer?.dispose();
    _peer = null;
  }

  @override
  void sendBroadcast(WatchPartyMessage message) {
    final json = jsonEncode(message.toJson());
    for (var conn in _connections) {
      conn.send(json);
    }
  }

  @override
  void sendMessage(String targetId, WatchPartyMessage message) {
    // PeerDart doesn't expose remote peer ID easily on DataConnection in all versions,
    // but we just broadcast for now as per Supabase logic.
    sendBroadcast(message);
  }
}

/// Sync correction mode for drift handling
enum SyncCorrectionMode {
  /// No correction needed
  none,

  /// Speed up playback (we're behind)
  speedUp,

  /// Slow down playback (we're ahead)
  slowDown,

  /// Hard seek required (too far out of sync)
  hardSeek,
}

/// Watch party service for synchronized viewing
/// Uses Supabase Realtime with automatic fallback to PeerDart (P2P)
///
/// Fallback Strategy:
/// 1. Try Supabase (Cloud)
/// 2. If fails, try PeerDart (P2P)
///
/// Sync Strategy:
/// - Drift < 2s: Ignore (acceptable)
/// - Drift 2-5s: Smooth correction via playback speed (1.1x or 0.9x)
/// - Drift 5-10s: Fast correction (1.2x or 0.8x)
/// - Drift > 10s: Hard seek
class WatchPartyService extends ChangeNotifier {
  static const String _tag = 'WatchParty';

  // Sync thresholds (in milliseconds)
  static const int _driftIgnoreThreshold = 5000; // < 5s: ignore
  static const int _driftSmoothThreshold = 10000; // 5-10s: smooth correction
  static const int _driftFastThreshold = 15000; // 10-15s: fast correction
  // > 15s: hard seek

  WatchPartyState _state = WatchPartyState.idle;
  WatchPartyBackendType _backendType = WatchPartyBackendType.none;
  WatchPartyBackend? _backend;

  WatchPartyRoom? _room;
  final List<WatchPartyParticipant> _participants = [];
  final List<ChatMessage> _chatMessages = [];
  String? _error;

  // User info
  final String _myId = DateTime.now().millisecondsSinceEpoch.toString();
  String _myName = 'User';
  bool _isHost = false;
  String? _currentRoomCode;

  // Playback state
  bool _isPlaying = false;
  Duration _currentPosition = Duration.zero;
  String? _currentMediaUrl;
  String? _currentMediaTitle;
  double _playbackSpeed = 1.0;
  double _baseSyncSpeed = 1.0; // Speed set by user (not correction)

  // Sync state
  SyncCorrectionMode _correctionMode = SyncCorrectionMode.none;
  Duration _lastHostPosition = Duration.zero;
  int _consecutiveBufferEvents = 0;
  bool _isCorrecting = false;

  // Callbacks
  Function(bool isPlaying)? onPlayPauseChanged;
  Function(Duration position)? onSeek;
  Function(double speed)? onSpeedChanged;
  Function(String url, String title)? onMediaChanged;
  Function(int qualityDelta)?
  onQualityAdjustRequested; // +1 = increase, -1 = decrease
  Function(SyncCorrectionMode mode, int driftMs)? onSyncStatusChanged;

  // Getters
  WatchPartyState get state => _state;
  WatchPartyBackendType get backendType => _backendType;
  WatchPartyRoom? get room => _room;
  List<WatchPartyParticipant> get participants =>
      List.unmodifiable(_participants);
  List<ChatMessage> get chatMessages => List.unmodifiable(_chatMessages);
  String? get error => _error;
  bool get isHost => _isHost;
  String get myId => _myId;
  String? get currentRoomCode => _currentRoomCode;
  String get myName => _myName;
  double get playbackSpeed => _playbackSpeed;
  bool get isPlaying => _isPlaying;
  SyncCorrectionMode get correctionMode => _correctionMode;
  bool get isSynced => _correctionMode == SyncCorrectionMode.none;

  // Dependency Injection for testing & backend selection
  final OxideServerService _server;
  final WatchPartyBackend Function(WatchPartyBackendType)? _backendFactory;

  WatchPartyService({
    required OxideServerService server,
    required SettingsService settings,
    WatchPartyBackend Function(WatchPartyBackendType)? backendFactory,
  }) : _server = server,
       _backendFactory = backendFactory {
    _loadName(settings);
  }

  Future<void> _loadName(SettingsService settings) async {
    final savedName = settings.state.watchPartyName;
    if (savedName.isNotEmpty) {
      _myName = savedName;
    }
  }

  void setMyName(String name) {
    _myName = name;
    notifyListeners();
    try {
      GetIt.I<SettingsService>().setWatchPartyName(name);
    } catch (e) {
      Logger.e('Failed to save name', tag: _tag, error: e);
    }
  }

  String _generateRoomCode() {
    const chars = 'ABCDEFGHJKLMNPQRSTUVWXYZ23456789';
    final random = Random();
    return List.generate(
      6,
      (index) => chars[random.nextInt(chars.length)],
    ).join();
  }

  // --------------------------------------------------------------------------
  // Host Logic
  // --------------------------------------------------------------------------

  Future<bool> hostRoom({String? mediaUrl, String? mediaTitle}) async {
    _state = WatchPartyState.hosting;
    _error = null;
    notifyListeners();

    final roomCode = _generateRoomCode();
    _currentRoomCode = roomCode;
    _currentMediaUrl = mediaUrl;
    _currentMediaTitle = mediaTitle;
    _isHost = true;

    // 1. Try Oxide Server WebSocket
    try {
      Logger.i('Hosting with Oxide Server: $roomCode', tag: _tag);
      await _initBackend(WatchPartyBackendType.server, roomCode);
    } catch (e) {
      Logger.w(
        'Oxide Server hosting failed ($e), falling back to PeerDart',
        tag: _tag,
      );

      // 2. Fallback to PeerDart
      try {
        await _initBackend(WatchPartyBackendType.peerdart, roomCode);
      } catch (e2) {
        Logger.e('PeerDart hosting failed', tag: _tag, error: e2);
        _setError('Connection failed: $e2');
        return false;
      }
    }

    _setupHostState(roomCode, mediaUrl, mediaTitle);
    return true;
  }

  void _setupHostState(String roomCode, String? mediaUrl, String? mediaTitle) {
    _room = WatchPartyRoom(
      id: roomCode,
      hostId: _myId,
      hostName: _myName,
      mediaTitle: mediaTitle,
      mediaUrl: mediaUrl,
    );

    _participants.add(
      WatchPartyParticipant(
        id: _myId,
        name: _myName,
        isHost: true,
        joinedAt: DateTime.now(),
      ),
    );

    _state = WatchPartyState.connected;
    notifyListeners();

    _startHeartbeat();
  }

  Timer? _heartbeatTimer;

  void _startHeartbeat() {
    _heartbeatTimer?.cancel();
    _heartbeatTimer = Timer.periodic(const Duration(seconds: 2), (_) {
      // Host always syncs position (even when paused) for new joiners
      if (_isHost) {
        syncPosition(_currentPosition, _isPlaying);
      }
    });
  }

  void _stopHeartbeat() {
    _heartbeatTimer?.cancel();
    _heartbeatTimer = null;
  }

  // --------------------------------------------------------------------------
  // Client Logic
  // --------------------------------------------------------------------------

  Future<bool> joinRoom(String roomCode) async {
    _state = WatchPartyState.joining;
    _error = null;
    notifyListeners();

    _isHost = false;
    _currentRoomCode = roomCode.toUpperCase();

    // 1. Try Oxide Server WebSocket
    try {
      Logger.i('Joining with Oxide Server: $_currentRoomCode', tag: _tag);
      await _initBackend(WatchPartyBackendType.server, _currentRoomCode!);
    } catch (e) {
      Logger.w(
        'Oxide Server join failed ($e), falling back to PeerDart',
        tag: _tag,
      );

      // 2. Fallback to PeerDart
      try {
        await _initBackend(WatchPartyBackendType.peerdart, _currentRoomCode!);
      } catch (e2) {
        Logger.e('PeerDart join failed', tag: _tag, error: e2);
        _setError('Connection failed: $e2');
        return false;
      }
    }

    _setupClientState();
    return true;
  }

  void _setupClientState() {
    _participants.add(
      WatchPartyParticipant(
        id: _myId,
        name: _myName,
        isHost: false,
        joinedAt: DateTime.now(),
      ),
    );

    // Send join message
    _send(
      WatchPartyMessage(
        type: WatchPartyMessageType.userJoined,
        senderId: _myId,
        senderName: _myName,
      ),
    );

    // Request sync
    _send(
      WatchPartyMessage(
        type: WatchPartyMessageType.requestSync,
        senderId: _myId,
      ),
    );

    _state = WatchPartyState.connected;
    notifyListeners();
  }

  // --------------------------------------------------------------------------
  // Common Backend Init
  // --------------------------------------------------------------------------

  Future<void> _initBackend(WatchPartyBackendType type, String roomCode) async {
    _backendType = type;

    if (_backendFactory != null) {
      _backend = _backendFactory(type);
    } else if (type == WatchPartyBackendType.server) {
      _backend = _OxideServerBackend(_server);
    } else {
      _backend = _PeerDartBackend();
    }

    await _backend!.connect(
      roomCode: roomCode,
      isHost: _isHost,
      myId: _myId,
      myName: _myName,
      onMessage: _onMessageReceived,
      onDisconnected: _onBackendDisconnected,
    );
  }

  void _onBackendDisconnected() {
    if (_state == WatchPartyState.connected ||
        _state == WatchPartyState.hosting ||
        _state == WatchPartyState.joining) {
      Logger.w('WatchParty connection lost unexpectedly', tag: _tag);
      _stopHeartbeat();
      _state = WatchPartyState.error;
      _error = 'З\'єднання з кімнатою перервано';
      notifyListeners();
    }
  }

  void _setError(String msg) {
    _state = WatchPartyState.error;
    _error = msg;
    notifyListeners();
  }

  // --------------------------------------------------------------------------
  // Message Handling
  // --------------------------------------------------------------------------

  void _onMessageReceived(WatchPartyMessage message) {
    // Automatically add any unknown user to participants list
    if (message.senderId != _myId &&
        !_participants.any((p) => p.id == message.senderId)) {
      _participants.add(
        WatchPartyParticipant(
          id: message.senderId,
          name: message.senderName ?? 'Unknown',
          isHost: _room != null && message.senderId == _room!.hostId,
          joinedAt: DateTime.now(),
        ),
      );
      notifyListeners();
    }

    switch (message.type) {
      case WatchPartyMessageType.userJoined:
        Logger.d('User joined: ${message.senderName}', tag: _tag);

        // Host sends sync to new user
        if (_isHost) {
          syncPosition(_currentPosition, _isPlaying);
        }
        break;

      case WatchPartyMessageType.userLeft:
        _participants.removeWhere((p) => p.id == message.senderId);
        notifyListeners();
        break;

      case WatchPartyMessageType.sync:
        if (!_isHost) {
          _handleSync(message);
        }
        break;

      case WatchPartyMessageType.play:
        _isPlaying = true;
        onPlayPauseChanged?.call(true);
        notifyListeners();
        break;

      case WatchPartyMessageType.pause:
        _isPlaying = false;
        onPlayPauseChanged?.call(false);
        notifyListeners();
        break;

      case WatchPartyMessageType.seek:
        _currentPosition = Duration(milliseconds: message.payload as int);
        onSeek?.call(_currentPosition);
        notifyListeners();
        break;

      case WatchPartyMessageType.speed:
        final speed = (message.payload as num).toDouble();
        _playbackSpeed = speed;
        onSpeedChanged?.call(speed);
        notifyListeners();
        break;

      case WatchPartyMessageType.chat:
        _chatMessages.add(
          ChatMessage(
            id: DateTime.now().millisecondsSinceEpoch.toString(),
            senderId: message.senderId,
            senderName: message.senderName ?? 'Unknown',
            message: message.payload as String,
            timestamp: message.timestamp,
          ),
        );
        notifyListeners();
        break;

      case WatchPartyMessageType.requestSync:
        if (_isHost) {
          syncPosition(_currentPosition, _isPlaying);
        }
        break;

      case WatchPartyMessageType.roomInfo:
        // Handled in sync usually
        break;

      case WatchPartyMessageType.unknown:
        Logger.w(
          'Received unknown message type: ${message.payload}',
          tag: _tag,
        );
        break;
    }
  }

  void _handleSync(WatchPartyMessage message) {
    if (message.payload is! Map<String, dynamic>) {
      Logger.w(
        'Invalid sync payload type: ${message.payload.runtimeType}',
        tag: _tag,
      );
      return;
    }
    final payload = message.payload as Map<String, dynamic>;
    final hostPosition = Duration(milliseconds: payload['position'] ?? 0);
    final hostTimestamp = message.timestamp;

    // Update base speed from host
    final hostSpeed = (payload['speed'] as num?)?.toDouble();
    if (hostSpeed != null) {
      _baseSyncSpeed = hostSpeed;
    }

    // Account for network latency (estimate based on timestamp)
    // Both timestamps must be UTC to avoid timezone differences causing massive drift
    final nowUtc = DateTime.now().toUtc();
    final networkDelay = nowUtc.difference(hostTimestamp);

    // Clamp network delay to [0, 5000] ms.
    // - If it's < 0, host clock is faster than client clock (impossible latency physically).
    // - If it's > 5000, prevent huge jumps in case of offline buffering or lag.
    Duration cappedDelay = networkDelay;
    if (cappedDelay.inMilliseconds < 0) {
      cappedDelay = Duration.zero;
    } else if (cappedDelay.inMilliseconds > 5000) {
      cappedDelay = const Duration(milliseconds: 5000);
    }

    final adjustedHostPosition = hostPosition + cappedDelay;

    _lastHostPosition = adjustedHostPosition;

    // Calculate drift (positive = we're ahead, negative = we're behind)
    final driftMs = (_currentPosition - adjustedHostPosition).inMilliseconds;
    final absDrift = driftMs.abs();

    // Determine correction mode
    final previousMode = _correctionMode;

    if (absDrift <= _driftIgnoreThreshold) {
      // Within acceptable range - no correction needed
      _correctionMode = SyncCorrectionMode.none;
      if (_isCorrecting) {
        _isCorrecting = false;
        // Restore base speed
        if (_playbackSpeed != _baseSyncSpeed) {
          _playbackSpeed = _baseSyncSpeed;
          onSpeedChanged?.call(_playbackSpeed);
        }
      }
    } else if (absDrift <= _driftSmoothThreshold) {
      // 2-5 seconds: smooth correction
      _correctionMode = driftMs > 0
          ? SyncCorrectionMode.slowDown
          : SyncCorrectionMode.speedUp;
      _applySmoothCorrection(driftMs);
    } else if (absDrift <= _driftFastThreshold) {
      // 5-10 seconds: fast correction
      _correctionMode = driftMs > 0
          ? SyncCorrectionMode.slowDown
          : SyncCorrectionMode.speedUp;
      _applyFastCorrection(driftMs);
    } else {
      // > 10 seconds: hard seek
      _correctionMode = SyncCorrectionMode.hardSeek;
      Logger.i(
        'Sync drift too large (${absDrift}ms). Hard seeking to host position.',
        tag: _tag,
      );
      _currentPosition = adjustedHostPosition;
      onSeek?.call(_currentPosition);
      _isCorrecting = false;
      // Restore base speed after seek
      if (_playbackSpeed != _baseSyncSpeed) {
        _playbackSpeed = _baseSyncSpeed;
        onSpeedChanged?.call(_playbackSpeed);
      }
    }

    // Notify about sync status change
    if (_correctionMode != previousMode) {
      onSyncStatusChanged?.call(_correctionMode, driftMs);
      Logger.d(
        'Sync mode changed: $previousMode -> $_correctionMode (drift: ${driftMs}ms)',
        tag: _tag,
      );
    }

    final wasPlaying = _isPlaying;
    _isPlaying = payload['isPlaying'] ?? false;

    // Update room info if needed
    if (_room == null && payload.containsKey('mediaTitle')) {
      _room = WatchPartyRoom(
        id: _currentRoomCode ?? '',
        hostId: message.senderId,
        hostName: message.senderName ?? 'Host',
        mediaUrl: payload['mediaUrl'],
        mediaTitle: payload['mediaTitle'],
      );
      _currentMediaUrl = payload['mediaUrl'];
      _currentMediaTitle = payload['mediaTitle'];
      onMediaChanged?.call(_currentMediaUrl!, _currentMediaTitle!);
    }

    // Only call onSeek for hard seek mode (already called above when needed)
    // For smooth/fast correction, we use speed adjustment instead of seeking
    // Note: onSeek is already called in the hardSeek branch above

    if (_isPlaying != wasPlaying) {
      onPlayPauseChanged?.call(_isPlaying);
    }

    notifyListeners();
  }

  // --------------------------------------------------------------------------
  // Actions
  // --------------------------------------------------------------------------

  Future<void> leaveRoom() async {
    _stopHeartbeat(); // Stop timer
    _send(
      WatchPartyMessage(
        type: WatchPartyMessageType.userLeft,
        senderId: _myId,
        senderName: _myName,
      ),
    );

    await _backend?.disconnect();
    _backend = null;
    _backendType = WatchPartyBackendType.none;

    _room = null;
    _participants.clear();
    _chatMessages.clear();
    _isHost = false;
    _currentRoomCode = null; // Reset room code
    _state = WatchPartyState.idle;
    notifyListeners();
  }

  void play() {
    _isPlaying = true;
    _send(
      WatchPartyMessage(
        type: WatchPartyMessageType.play,
        senderId: _myId,
        senderName: _myName,
      ),
    );
    notifyListeners();
  }

  void pause() {
    _isPlaying = false;
    _send(
      WatchPartyMessage(
        type: WatchPartyMessageType.pause,
        senderId: _myId,
        senderName: _myName,
      ),
    );
    notifyListeners();
  }

  void seek(Duration position) {
    _currentPosition = position;
    _send(
      WatchPartyMessage(
        type: WatchPartyMessageType.seek,
        senderId: _myId,
        senderName: _myName,
        payload: position.inMilliseconds,
      ),
    );
    notifyListeners();
  }

  void requestSync() {
    _send(
      WatchPartyMessage(
        type: WatchPartyMessageType.requestSync,
        senderId: _myId,
      ),
    );
  }

  void setSpeed(double speed) {
    _baseSyncSpeed = speed; // Remember user's preferred speed
    _playbackSpeed = speed;
    _send(
      WatchPartyMessage(
        type: WatchPartyMessageType.speed,
        senderId: _myId,
        senderName: _myName,
        payload: speed,
      ),
    );
    notifyListeners();
  }

  /// Apply smooth correction for 2-5 second drift
  void _applySmoothCorrection(int driftMs) {
    _isCorrecting = true;
    final correctionSpeed = driftMs > 0
        ? _baseSyncSpeed *
              0.9 // We're ahead, slow down
        : _baseSyncSpeed * 1.1; // We're behind, speed up

    if (_playbackSpeed != correctionSpeed) {
      _playbackSpeed = correctionSpeed;
      onSpeedChanged?.call(_playbackSpeed);
      Logger.d(
        'Smooth correction: speed set to ${correctionSpeed}x (drift: ${driftMs}ms)',
        tag: _tag,
      );
    }
  }

  /// Apply fast correction for 5-10 second drift
  void _applyFastCorrection(int driftMs) {
    _isCorrecting = true;
    final correctionSpeed = driftMs > 0
        ? _baseSyncSpeed *
              0.8 // We're ahead, slow down more
        : _baseSyncSpeed * 1.2; // We're behind, speed up more

    if (_playbackSpeed != correctionSpeed) {
      _playbackSpeed = correctionSpeed;
      onSpeedChanged?.call(_playbackSpeed);
      Logger.d(
        'Fast correction: speed set to ${correctionSpeed}x (drift: ${driftMs}ms)',
        tag: _tag,
      );
    }
  }

  /// Report buffering event (for adaptive quality)
  void reportBuffering(bool isBuffering) {
    if (isBuffering) {
      _consecutiveBufferEvents++;
      if (_consecutiveBufferEvents >= 3) {
        // Too many buffer events, request quality reduction
        Logger.i(
          'Too many buffer events ($_consecutiveBufferEvents), requesting quality reduction',
          tag: _tag,
        );
        onQualityAdjustRequested?.call(-1);
        _consecutiveBufferEvents = 0;
      }
    } else {
      // Reset on successful playback
      _consecutiveBufferEvents = 0;
    }
  }

  /// Get current sync drift in milliseconds (for UI)
  int getCurrentDriftMs() {
    return (_currentPosition - _lastHostPosition).inMilliseconds;
  }

  void updateLocalPosition(Duration position) {
    _currentPosition = position;
  }

  void syncPosition(Duration position, bool isPlaying) {
    if (!_isHost) return;

    _currentPosition = position;
    _isPlaying = isPlaying;

    _send(
      WatchPartyMessage(
        type: WatchPartyMessageType.sync,
        senderId: _myId,
        senderName: _myName,
        payload: {
          'position': position.inMilliseconds,
          'isPlaying': isPlaying,
          'speed': _playbackSpeed,
          'mediaUrl': _currentMediaUrl,
          'mediaTitle': _currentMediaTitle,
        },
      ),
    );
  }

  void sendChatMessage(String message) {
    final chatMsg = ChatMessage(
      id: DateTime.now().millisecondsSinceEpoch.toString(),
      senderId: _myId,
      senderName: _myName,
      message: message,
      timestamp: DateTime.now(),
    );
    _chatMessages.add(chatMsg);

    _send(
      WatchPartyMessage(
        type: WatchPartyMessageType.chat,
        senderId: _myId,
        senderName: _myName,
        payload: message,
      ),
    );
    notifyListeners();
  }

  void _send(WatchPartyMessage message) {
    _backend?.sendBroadcast(message);
  }
}
