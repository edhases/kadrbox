import 'package:flutter/foundation.dart';
import 'package:smtc_windows/smtc_windows.dart';
import '../../../data/services/discord_rpc_service.dart';

import 'player_controller.dart';

/// Service to synchronize current media playback state and metadata
/// with platform System Media Transport Controls (Windows SMTC) and Discord Rich Presence.
class SystemMediaControlsService {
  final DiscordRpcService? _discordRpc;
  SMTCWindows? _smtc;
  PlayerController? _currentController;
  bool _isInitialized = false;

  SystemMediaControlsService({DiscordRpcService? discordRpc})
      : _discordRpc = discordRpc;

  bool get isSupported =>
      !kIsWeb && defaultTargetPlatform == TargetPlatform.windows;

  Future<void> initialize() async {
    if (!isSupported || _isInitialized) return;

    try {
      _smtc = SMTCWindows(
        config: const SMTCConfig(
          playEnabled: true,
          pauseEnabled: true,
          stopEnabled: false,
          nextEnabled: false,
          prevEnabled: false,
          fastForwardEnabled: true,
          rewindEnabled: true,
        ),
      );

      _smtc?.buttonPressStream.listen(_handleButtonPressed);
      _isInitialized = true;
    } catch (e) {
      debugPrint('Failed to initialize SMTCWindows: $e');
    }
  }

  void attachController(PlayerController controller) {
    if (!isSupported) return;
    _currentController = controller;

    _updateMetadata();
    _updatePlaybackStatus();
  }

  void detachController() {
    _currentController = null;
    _discordRpc?.clearPresence();
    if (!isSupported || !_isInitialized) return;

    try {
      _smtc?.setPlaybackStatus(PlaybackStatus.paused);
      _smtc?.clearMetadata();
    } catch (_) {}
  }

  void updatePlaybackStatus(bool isPlaying) {
    _updateDiscordPresence(isPlaying: isPlaying);
    if (!isSupported || !_isInitialized) return;

    try {
      _smtc?.setPlaybackStatus(
        isPlaying ? PlaybackStatus.playing : PlaybackStatus.paused,
      );
    } catch (_) {}
  }

  void updatePosition(Duration position) {
    if (!isSupported || !_isInitialized) return;

    try {
      _smtc?.setPosition(position);
    } catch (_) {}
  }

  void _updateMetadata() {
    if (!isSupported || !_isInitialized || _currentController == null) return;

    final controller = _currentController!;
    final title = controller.title ?? 'Фільм / Серіал';
    var artist = 'Oxide Film';

    final state = controller.state;
    if (state.currentSeason != null &&
        state.currentEpisode != null) {
      artist = 'Сезон ${state.currentSeason}, Серія ${state.currentEpisode}';
      if (state.currentEpisodeTitle != null &&
          state.currentEpisodeTitle!.isNotEmpty) {
        artist += ' - ${state.currentEpisodeTitle}';
      }
    }

    try {
      _smtc?.updateMetadata(
        MusicMetadata(
          title: title,
          artist: artist,
          thumbnail: controller.posterUrl,
        ),
      );
    } catch (e) {
      debugPrint('Error updating SMTC metadata: $e');
    }

    _updateDiscordPresence();
  }

  void _updatePlaybackStatus() {
    if (_currentController == null) return;
    updatePlaybackStatus(_currentController!.state.isPlaying);
  }

  void _handleButtonPressed(PressedButton button) {
    if (_currentController == null) return;

    switch (button) {
      case PressedButton.play:
      case PressedButton.pause:
        _currentController?.playOrPause();
        break;
      case PressedButton.fastForward:
        _currentController?.seekForward();
        break;
      case PressedButton.rewind:
        _currentController?.seekBackward();
        break;
      default:
        break;
    }
  }

  void _updateDiscordPresence({bool? isPlaying}) {
    final rpc = _discordRpc;
    final controller = _currentController;
    if (rpc == null || controller == null) return;

    final title = controller.title ?? 'Фільм / Серіал';
    final playing = isPlaying ?? controller.state.isPlaying;

    var state = playing ? 'Дивиться' : 'На паузі';
    if (controller.state.currentSeason != null &&
        controller.state.currentEpisode != null) {
      state += ' (S${controller.state.currentSeason}:E${controller.state.currentEpisode})';
    }

    final duration = controller.state.duration;
    final position = controller.state.position;
    int? endTimestamp;

    if (playing && duration > position) {
      final remainingSec = (duration - position).inSeconds;
      endTimestamp = (DateTime.now().millisecondsSinceEpoch ~/ 1000) + remainingSec;
    }

    rpc.updatePresence(
      details: title,
      state: state,
      endTimestamp: endTimestamp,
    );
  }

  void dispose() {
    try {
      _smtc?.dispose();
    } catch (_) {}
    _discordRpc?.dispose();
    _smtc = null;
    _isInitialized = false;
  }
}
