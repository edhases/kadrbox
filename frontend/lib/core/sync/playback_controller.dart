import 'package:equatable/equatable.dart';

/// Playback mode for the player
enum PlaybackMode {
  /// Solo playback - commands go directly to player
  local,

  /// Watch Party mode - commands sync via P2P
  party,
}

/// Commands that can be sent to control playback
sealed class PlaybackCommand extends Equatable {
  final DateTime timestamp;

  const PlaybackCommand({required this.timestamp});

  @override
  List<Object?> get props => [timestamp];
}

/// Command to play the video
class PlayCommand extends PlaybackCommand {
  const PlayCommand({required super.timestamp});
}

/// Command to pause the video
class PauseCommand extends PlaybackCommand {
  const PauseCommand({required super.timestamp});
}

/// Command to seek to a specific position
class SeekCommand extends PlaybackCommand {
  final Duration position;

  const SeekCommand({required this.position, required super.timestamp});

  @override
  List<Object?> get props => [position, timestamp];
}

/// Command to change playback rate (for drift correction)
class SetRateCommand extends PlaybackCommand {
  final double rate;

  const SetRateCommand({required this.rate, required super.timestamp});

  @override
  List<Object?> get props => [rate, timestamp];
}

/// Current state of playback
class PlaybackState extends Equatable {
  final bool isPlaying;
  final Duration position;
  final Duration duration;
  final double rate;
  final PlaybackMode mode;
  final int? partyMemberCount;

  const PlaybackState({
    this.isPlaying = false,
    this.position = Duration.zero,
    this.duration = Duration.zero,
    this.rate = 1.0,
    this.mode = PlaybackMode.local,
    this.partyMemberCount,
  });

  PlaybackState copyWith({
    bool? isPlaying,
    Duration? position,
    Duration? duration,
    double? rate,
    PlaybackMode? mode,
    int? partyMemberCount,
  }) {
    return PlaybackState(
      isPlaying: isPlaying ?? this.isPlaying,
      position: position ?? this.position,
      duration: duration ?? this.duration,
      rate: rate ?? this.rate,
      mode: mode ?? this.mode,
      partyMemberCount: partyMemberCount ?? this.partyMemberCount,
    );
  }

  @override
  List<Object?> get props => [
    isPlaying,
    position,
    duration,
    rate,
    mode,
    partyMemberCount,
  ];
}

/// Abstract interface for playback control
///
/// This abstraction allows seamless switching between:
/// - LocalPlaybackController (solo mode)
/// - RemotePlaybackController (Watch Party P2P mode)
abstract class PlaybackController {
  /// Current playback mode
  PlaybackMode get mode;

  /// Stream of playback states
  Stream<PlaybackState> get stateStream;

  /// Stream of commands for the player to execute
  ///
  /// In local mode: commands are emitted immediately
  /// In party mode: commands are emitted after P2P sync
  Stream<PlaybackCommand> get commandStream;

  /// Request to play
  /// In party mode, this will be synced with all peers
  void requestPlay();

  /// Request to pause
  /// In party mode, this will be synced with all peers
  void requestPause();

  /// Request to seek to a specific position
  /// In party mode, this will be synced with all peers
  void requestSeek(Duration position);

  /// Update local playback state (called by player)
  void updateState(PlaybackState state);

  /// Dispose resources
  void dispose();
}
