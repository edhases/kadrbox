// Lightweight fake/stub for media player functionality used by PlayerController tests.
// This avoids launching real media_kit in unit tests and provides a small
// stream-based API that mirrors the media_kit Player streams used by
// PlayerController (_player.stream.*).

import 'dart:async';
import 'package:media_kit/media_kit.dart';

/// Minimal snapshot similar to Player.state used in tests
class FakePlayerStateSnapshot {
  final bool playing;
  final Duration position;
  final Duration duration;

  FakePlayerStateSnapshot(this.playing, this.position, this.duration);
}

class FakePlayer {
  bool _isPlaying = false;
  Duration _position = Duration.zero;
  Duration _duration = Duration(seconds: 0);
  double _rate = 1.0;
  double _volume = 100.0;

  // Stream controllers backing the 'stream' namespace
  final StreamController<bool> _playingController =
      StreamController.broadcast();
  final StreamController<bool> _completedController =
      StreamController.broadcast();
  final StreamController<Duration> _positionController =
      StreamController.broadcast();
  final StreamController<Duration> _durationController =
      StreamController.broadcast();
  final StreamController<bool> _bufferingController =
      StreamController.broadcast();
  final StreamController<String> _errorController =
      StreamController.broadcast();
  final StreamController<Tracks> _tracksController =
      StreamController.broadcast();
  final StreamController<Track> _trackController = StreamController.broadcast();
  final StreamController<double> _volumeController =
      StreamController.broadcast();

  // Public accessor similar to player.stream
  FakePlayerStreams get stream => FakePlayerStreams._(this);

  // Simple state snapshot similar to Player.state
  FakePlayerStateSnapshot get state =>
      FakePlayerStateSnapshot(_isPlaying, _position, _duration);

  // Emit helpers used in tests
  void emitPlaying(bool playing) {
    _isPlaying = playing;
    _playingController.add(playing);
  }

  void emitCompleted(bool completed) {
    _completedController.add(completed);
  }

  void emitPosition(Duration pos) {
    _position = pos;
    _positionController.add(pos);
  }

  void emitDuration(Duration d) {
    _duration = d;
    _durationController.add(d);
  }

  void emitBuffering(bool b) => _bufferingController.add(b);
  void emitError(String e) => _errorController.add(e);

  void emitTracks(Tracks t) => _tracksController.add(t);
  void emitTrack(Track t) => _trackController.add(t);
  void emitVolume(double v) {
    _volume = v;
    _volumeController.add(v);
  }

  // Basic player control methods
  Future<void> open(dynamic media) async {
    // simulate open: reset position and emit duration if available
    emitPosition(Duration.zero);
    // optionally emit duration (test can call setDuration/emitDuration)
  }

  Future<void> play() async {
    emitPlaying(true);
  }

  Future<void> pause() async {
    emitPlaying(false);
  }

  Future<void> playOrPause() async {
    emitPlaying(!_isPlaying);
  }

  Future<void> stop() async {
    emitPlaying(false);
    emitPosition(Duration.zero);
  }

  Future<void> seek(Duration pos) async {
    emitPosition(pos);
  }

  Future<void> setVideoTrack(VideoTrack track) async {
    // notify selected track
    emitTrack(Track(video: track));
  }

  Future<void> setRate(double r) async {
    _rate = r;
  }

  Future<void> setVolume(double v) async {
    _volume = v;
    emitVolume(v);
  }

  // Synchronous getters used in some tests
  bool get isPlaying => _isPlaying;
  Duration get position => _position;
  Duration get duration => _duration;
  double get rate => _rate;
  double get volume => _volume;

  void dispose() {
    _playingController.close();
    _completedController.close();
    _positionController.close();
    _durationController.close();
    _bufferingController.close();
    _errorController.close();
    _tracksController.close();
    _trackController.close();
    _volumeController.close();
  }
}

/// Provides the stream-like API under `.stream` used by PlayerController
class FakePlayerStreams {
  final FakePlayer _parent;
  FakePlayerStreams._(this._parent);

  Stream<bool> get playing => _parent._playingController.stream;
  Stream<bool> get completed => _parent._completedController.stream;
  Stream<Duration> get position => _parent._positionController.stream;
  Stream<Duration> get duration => _parent._durationController.stream;
  Stream<bool> get buffering => _parent._bufferingController.stream;
  Stream<String> get error => _parent._errorController.stream;
  Stream<Tracks> get tracks => _parent._tracksController.stream;
  Stream<Track> get track => _parent._trackController.stream;
  Stream<double> get volume => _parent._volumeController.stream;
}
