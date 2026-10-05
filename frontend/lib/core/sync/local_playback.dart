import 'dart:async';
import 'playback_controller.dart';

/// Local playback controller for solo viewing
///
/// Commands go directly to the player without any network sync
class LocalPlaybackController implements PlaybackController {
  final _stateController = StreamController<PlaybackState>.broadcast();
  final _commandController = StreamController<PlaybackCommand>.broadcast();

  PlaybackState _currentState = const PlaybackState();

  @override
  PlaybackMode get mode => PlaybackMode.local;

  @override
  Stream<PlaybackState> get stateStream => _stateController.stream;

  @override
  Stream<PlaybackCommand> get commandStream => _commandController.stream;

  @override
  void requestPlay() {
    final command = PlayCommand(timestamp: DateTime.now());
    _commandController.add(command);
    _updatePlaying(true);
  }

  @override
  void requestPause() {
    final command = PauseCommand(timestamp: DateTime.now());
    _commandController.add(command);
    _updatePlaying(false);
  }

  @override
  void requestSeek(Duration position) {
    final command = SeekCommand(position: position, timestamp: DateTime.now());
    _commandController.add(command);
  }

  @override
  void updateState(PlaybackState state) {
    _currentState = state.copyWith(mode: PlaybackMode.local);
    _stateController.add(_currentState);
  }

  void _updatePlaying(bool isPlaying) {
    _currentState = _currentState.copyWith(isPlaying: isPlaying);
    _stateController.add(_currentState);
  }

  @override
  void dispose() {
    _stateController.close();
    _commandController.close();
  }
}
