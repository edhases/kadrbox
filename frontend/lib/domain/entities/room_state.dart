import 'package:equatable/equatable.dart';

/// State of a Watch Party room
enum RoomStatus {
  /// Room is being created
  creating,

  /// Waiting for peers to join
  waiting,

  /// Connected to peers
  connected,

  /// Room connection failed
  error,

  /// Room closed
  closed,
}

/// Role of the current user in the room
enum RoomRole {
  /// User created the room (source of truth for timing)
  host,

  /// User joined an existing room
  guest,
}

/// Information about a peer in the room
class RoomPeer extends Equatable {
  final String id;
  final String? name;
  final bool isConnected;

  const RoomPeer({required this.id, this.name, this.isConnected = false});

  RoomPeer copyWith({String? id, String? name, bool? isConnected}) {
    return RoomPeer(
      id: id ?? this.id,
      name: name ?? this.name,
      isConnected: isConnected ?? this.isConnected,
    );
  }

  @override
  List<Object?> get props => [id, name, isConnected];
}

/// Media being watched in the room
class RoomMedia extends Equatable {
  final String providerId;
  final String mediaId;
  final String? title;
  final int? season;
  final int? episode;

  const RoomMedia({
    required this.providerId,
    required this.mediaId,
    this.title,
    this.season,
    this.episode,
  });

  Map<String, dynamic> toJson() => {
    'providerId': providerId,
    'mediaId': mediaId,
    'title': title,
    'season': season,
    'episode': episode,
  };

  factory RoomMedia.fromJson(Map<String, dynamic> json) => RoomMedia(
    providerId: json['providerId'] as String,
    mediaId: json['mediaId'] as String,
    title: json['title'] as String?,
    season: json['season'] as int?,
    episode: json['episode'] as int?,
  );

  @override
  List<Object?> get props => [providerId, mediaId, title, season, episode];
}

/// Complete state of a Watch Party room
class RoomState extends Equatable {
  final String roomId;
  final RoomStatus status;
  final RoomRole role;
  final RoomMedia? media;
  final List<RoomPeer> peers;
  final String? error;

  const RoomState({
    required this.roomId,
    this.status = RoomStatus.creating,
    this.role = RoomRole.guest,
    this.media,
    this.peers = const [],
    this.error,
  });

  RoomState copyWith({
    String? roomId,
    RoomStatus? status,
    RoomRole? role,
    RoomMedia? media,
    List<RoomPeer>? peers,
    String? error,
  }) {
    return RoomState(
      roomId: roomId ?? this.roomId,
      status: status ?? this.status,
      role: role ?? this.role,
      media: media ?? this.media,
      peers: peers ?? this.peers,
      error: error ?? this.error,
    );
  }

  /// Number of connected peers (including self)
  int get connectedCount => peers.where((p) => p.isConnected).length + 1;

  @override
  List<Object?> get props => [roomId, status, role, media, peers, error];
}
