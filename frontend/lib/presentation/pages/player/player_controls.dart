import 'package:collection/collection.dart';
import 'package:flutter/foundation.dart';
import 'package:flutter/material.dart';
import 'package:get_it/get_it.dart';
import 'package:go_router/go_router.dart';
import 'package:media_kit/media_kit.dart';
import 'package:kadrbox/data/services/video_player_service.dart';
import 'package:window_manager/window_manager.dart';

import '../../../core/di/injection.dart';
import '../../../core/l10n/app_strings.dart';
import '../../../data/services/download_service.dart';
import '../../../core/utils/responsive_utils.dart';
import '../../../data/services/watch_party_service.dart';
import '../../../domain/entities/entities.dart';
import '../../theme/app_theme.dart';
import 'player_controller.dart';

class PlayerControls extends StatelessWidget {
  final PlayerController controller;
  final bool showControls;
  final VoidCallback onToggleControls;
  final VoidCallback onToggleChat;
  final bool showChat;
  final int newChatMessages;
  final VoidCallback? onEnterPiP;

  const PlayerControls({
    super.key,
    required this.controller,
    required this.showControls,
    required this.onToggleControls,
    required this.onToggleChat,
    required this.showChat,
    required this.newChatMessages,
    this.onEnterPiP,
  });

  @override
  Widget build(BuildContext context) {
    // NOTE: the bar deliberately still renders while an error is showing.
    // It used to `return SizedBox.shrink()`, which removed the quality /
    // track / URL inspection affordance exactly when it is most useful.
    // Instead it renders disabled (dimmed, non-interactive) so the user can
    // still see what was being played and pick a different source.
    final hasError = controller.state.hasError;

    return AnimatedOpacity(
      opacity: showControls ? (hasError ? 0.45 : 1.0) : 0.0,
      duration: const Duration(milliseconds: 300),
      child: IgnorePointer(
        // Hidden controls or an active error both block input.
        ignoring: !showControls || hasError,
        child: Container(
          decoration: BoxDecoration(
            gradient: LinearGradient(
              begin: Alignment.topCenter,
              end: Alignment.bottomCenter,
              colors: [
                Colors.black.withValues(alpha: 0.7),
                Colors.transparent,
                Colors.transparent,
                Colors.black.withValues(alpha: 0.8),
              ],
              stops: const [0.0, 0.15, 0.75, 1.0],
            ),
          ),
          child: Column(
            children: [
              _TopBar(
                controller: controller,
                onToggleChat: onToggleChat,
                showChat: showChat,
                newChatMessages: newChatMessages,
                onEnterPiP: onEnterPiP,
              ),
              const Spacer(),
              _BottomControls(
                controller: controller,
                onToggleChat: onToggleChat,
                onEnterPiP: onEnterPiP,
              ),
            ],
          ),
        ),
      ),
    );
  }
}

class _TopBar extends StatelessWidget {
  final PlayerController controller;
  final VoidCallback onToggleChat;
  final bool showChat;
  final int newChatMessages;
  final VoidCallback? onEnterPiP;

  const _TopBar({
    required this.controller,
    required this.onToggleChat,
    required this.showChat,
    required this.newChatMessages,
    this.onEnterPiP,
  });

  @override
  Widget build(BuildContext context) {
    final screenWidth = MediaQuery.of(context).size.width;
    final isCompact = context.isCompact;
    final isMobile = context.isMobile;
    final useCompactControls = isCompact || screenWidth < 500;
    final padding = useCompactControls ? 6.0 : (isMobile ? 10.0 : 16.0);
    final iconSize = useCompactControls ? 20.0 : (isMobile ? 22.0 : 28.0);
    final titleFontSize = useCompactControls ? 13.0 : (isMobile ? 15.0 : 18.0);
    final subtitleFontSize = useCompactControls
        ? 10.0
        : (isMobile ? 11.0 : 14.0);

    return SafeArea(
      child: Padding(
        padding: EdgeInsets.symmetric(
          horizontal: padding,
          vertical: padding / 2,
        ),
        child: Row(
          children: [
            IconButton(
              focusColor: Colors.white24,
              icon: Icon(Icons.arrow_back, color: Colors.white, size: iconSize),
              onPressed: () => context.pop(),
              padding: EdgeInsets.all(useCompactControls ? 4 : 8),
              constraints: BoxConstraints(
                minWidth: useCompactControls ? 32 : 40,
                minHeight: useCompactControls ? 32 : 40,
              ),
            ),
            SizedBox(width: useCompactControls ? 4 : 10),
            Expanded(
              child: GestureDetector(
                behavior: HitTestBehavior.translucent,
                // Enable window dragging on desktop when not in fullscreen
                onPanStart: (_) {
                  if (!kIsWeb &&
                      (defaultTargetPlatform == TargetPlatform.windows ||
                          defaultTargetPlatform == TargetPlatform.linux ||
                          defaultTargetPlatform == TargetPlatform.macOS)) {
                    if (!controller.state.isFullscreen) {
                      windowManager.startDragging();
                    }
                  }
                },
                child: AnimatedBuilder(
                  animation: controller,
                  builder: (context, _) {
                    return Column(
                      crossAxisAlignment: CrossAxisAlignment.start,
                      mainAxisSize: MainAxisSize.min,
                      children: [
                        if (controller.title != null)
                          Text(
                            controller.title!,
                            style: TextStyle(
                              color: Colors.white,
                              fontFamily:
                                  'Roboto', // YouTube-like font preference if available
                              fontSize: titleFontSize,
                              fontWeight: FontWeight.w500,
                            ),
                            maxLines: 1,
                            overflow: TextOverflow.ellipsis,
                          ),
                        if (!useCompactControls)
                          Text(
                            controller.buildSubtitleText(null),
                            style: TextStyle(
                              color: Colors.white.withValues(alpha: 0.7),
                              fontSize: subtitleFontSize,
                            ),
                          ),
                      ],
                    );
                  },
                ),
              ),
            ),
            // On large screens, controls are at the bottom.
            // On mobile/compact, they remain in the top right menu or scattered.
            if (useCompactControls)
              _CompactMenu(controller: controller, iconSize: iconSize)
            else if (isMobile)
              _ExpandedMenu(
                controller: controller,
                iconSize: iconSize,
                onToggleChat: onToggleChat,
                showChat: showChat,
                newChatMessages: newChatMessages,
                onEnterPiP: onEnterPiP,
              )
            else
              // For Large screens, we might still want Chat button here or empty?
              // The user said "buttons ... moved down".
              // Keeping Chat here seems appropriate as it's an overlay toggle, not a playback setting.
              // But other settings move down.
              Row(
                children: [
                  // Window controls for desktop (non-fullscreen)
                  if (!kIsWeb &&
                      (defaultTargetPlatform == TargetPlatform.windows ||
                          defaultTargetPlatform == TargetPlatform.linux ||
                          defaultTargetPlatform == TargetPlatform.macOS) &&
                      !controller.state.isFullscreen) ...[
                    const SizedBox(width: 8),
                    _WindowControlButton(
                      icon: Icons.remove,
                      tooltip: 'Згорнути',
                      onPressed: () => windowManager.minimize(),
                    ),
                    _WindowControlButton(
                      icon: Icons.crop_square,
                      tooltip: 'Розгорнути',
                      onPressed: () async {
                        if (await windowManager.isMaximized()) {
                          await windowManager.unmaximize();
                        } else {
                          await windowManager.maximize();
                        }
                      },
                    ),
                    _WindowControlButton(
                      icon: Icons.close,
                      tooltip: 'Закрити',
                      onPressed: () => windowManager.close(),
                      isClose: true,
                    ),
                  ],
                ],
              ),
          ],
        ),
      ),
    );
  }
}

class _CompactMenu extends StatelessWidget {
  final PlayerController controller;
  final double iconSize;

  const _CompactMenu({required this.controller, required this.iconSize});

  @override
  Widget build(BuildContext context) {
    return PopupMenuButton<String>(
      icon: Icon(Icons.more_vert, color: Colors.white, size: iconSize),
      tooltip: 'Меню',
      itemBuilder: (context) => [
        if (controller.hasMultipleQualities())
          const PopupMenuItem(
            value: 'quality',
            child: ListTile(
              leading: Icon(Icons.high_quality),
              title: Text('Якість'),
              dense: true,
              contentPadding: EdgeInsets.zero,
            ),
          ),
        if (controller.hasMultipleVoiceovers())
          const PopupMenuItem(
            value: 'voice',
            child: ListTile(
              leading: Icon(Icons.record_voice_over),
              title: Text('Озвучка'),
              dense: true,
              contentPadding: EdgeInsets.zero,
            ),
          ),
        const PopupMenuItem(
          value: 'speed',
          child: ListTile(
            leading: Icon(Icons.speed),
            title: Text('Швидкість'),
            dense: true,
            contentPadding: EdgeInsets.zero,
          ),
        ),
        const PopupMenuItem(
          value: 'settings',
          child: ListTile(
            leading: Icon(Icons.settings),
            title: Text('Налаштування'),
            dense: true,
            contentPadding: EdgeInsets.zero,
          ),
        ),
        PopupMenuItem(
          value: 'fit',
          child: ListTile(
            leading: Icon(
              PlayerController.fitTooltips.contains(
                    controller.state.videoFit.toString(),
                  )
                  ? Icons.aspect_ratio
                  : Icons.fit_screen, // Simplify icon logic for now
            ),
            title: const Text('Масштаб'),
            dense: true,
            contentPadding: EdgeInsets.zero,
          ),
        ),
        const PopupMenuItem(
          value: 'pip',
          child: ListTile(
            leading: Icon(Icons.picture_in_picture_alt),
            title: Text('Картинка в картинці'),
            dense: true,
            contentPadding: EdgeInsets.zero,
          ),
        ),
      ],
      onSelected: (value) {
        switch (value) {
          case 'quality':
            _showQualitySheet(context, controller);
            break;
          case 'voice':
            _showVoiceoverSheet(context, controller);
            break;
          case 'audio_track':
            _showAudioTrackSheet(context, controller);
            break;
          case 'speed':
            _showSpeedSheet(context, controller);
            break;
          case 'settings':
            _showMainSettingsSheet(context, controller);
            break;
          case 'fit':
            controller.cycleFit();
            break;
          case 'pip':
            // videoPlayerService.enterNativePiP() – but we need access to it.
            // In PlayerControls it's available via GetIt usually or passed down.
            // It's passed as onEnterPiP in PlayerControls but maybe not here.
            // Let's check where _CompactMenu is defined.
            // I'll assume we can use GetIt if needed or pass the callback.
            // For now I'll use GetIt as it's common in this project.
            GetIt.I<VideoPlayerService>().enterNativePiP();
            break;
        }
      },
    );
  }
}

class _ExpandedMenu extends StatelessWidget {
  final PlayerController controller;
  final double iconSize;
  final VoidCallback onToggleChat;
  final bool showChat;
  final int newChatMessages;
  final VoidCallback? onEnterPiP;

  const _ExpandedMenu({
    required this.controller,
    required this.iconSize,
    required this.onToggleChat,
    required this.showChat,
    required this.newChatMessages,
    this.onEnterPiP,
  });

  @override
  Widget build(BuildContext context) {
    return Row(
      children: [
        if (controller.hasMultipleQualities())
          PopupMenuButton<dynamic>(
            icon: Icon(Icons.high_quality, color: Colors.white, size: iconSize),
            tooltip: 'Якість',
            itemBuilder: (context) => _buildQualityMenuItems(controller),
            onSelected: (value) {
              if (value is VideoTrack) {
                controller.setVideoTrack(value);
              } else if (value is StreamSource) {
                controller.switchStream(value);
              }
            },
          ),
        if (controller.hasMultipleVoiceovers())
          PopupMenuButton<StreamSource>(
            icon: Icon(
              Icons.record_voice_over,
              color: Colors.white,
              size: iconSize,
            ),
            tooltip: 'Озвучка',
            onSelected: controller.switchStream,
            itemBuilder: (context) => _buildVoiceoverMenuItems(controller),
          ),
        if (controller.hasMultipleAudioTracks())
          PopupMenuButton<AudioTrack>(
            icon: Icon(Icons.audiotrack, color: Colors.white, size: iconSize),
            tooltip: 'Аудіо доріжка',
            onSelected: controller.setAudioTrack,
            itemBuilder: (context) => _buildAudioTrackMenuItems(controller),
          ),
        PopupMenuButton<double>(
          icon: Icon(Icons.speed, color: Colors.white, size: iconSize),
          tooltip: 'Швидкість',
          onSelected: controller.setSpeed,
          itemBuilder: (context) => PlayerController.speeds.map((speed) {
            return PopupMenuItem(
              value: speed,
              child: Row(
                children: [
                  if (speed == controller.state.playbackSpeed)
                    const Icon(Icons.check, size: 18)
                  else
                    const SizedBox(width: 18),
                  const SizedBox(width: 8),
                  Text('${speed}x'),
                ],
              ),
            );
          }).toList(),
        ),
        IconButton(
          focusColor: Colors.white24,
          icon: Icon(Icons.settings, color: Colors.white, size: iconSize),
          tooltip: 'Налаштування',
          onPressed: () => _showMainSettingsSheet(context, controller),
        ),
        if (onEnterPiP != null)
          IconButton(
            focusColor: Colors.white24,
            icon: Icon(
              Icons.picture_in_picture_alt,
              color: Colors.white,
              size: iconSize,
            ),
            tooltip: 'Картинка в картинці',
            onPressed: onEnterPiP,
          ),
        // Chat button is in TopBar, so removed from here to avoid duplicate
        IconButton(
          focusColor: Colors.white24,
          icon: Icon(Icons.aspect_ratio, color: Colors.white, size: iconSize),
          tooltip: 'Масштаб',
          onPressed: controller.cycleFit,
        ),
      ],
    );
  }
}

class _BottomControls extends StatelessWidget {
  final PlayerController controller;
  final VoidCallback onToggleChat;
  final VoidCallback? onEnterPiP;

  const _BottomControls({
    required this.controller,
    required this.onToggleChat,
    this.onEnterPiP,
  });

  @override
  Widget build(BuildContext context) {
    final isCompact = context.isCompact;
    final isMobile = context.isMobile;
    final padding = ResponsiveUtils.getPlayerBottomPadding(context);
    final iconSize = isCompact ? 24.0 : (isMobile ? 28.0 : 28.0);
    final playPauseSize = isCompact ? 40.0 : (isMobile ? 44.0 : 48.0);
    final fontSize = isCompact ? 11.0 : 13.0;

    // YouTube Style for Large Screens (Tablet/Desktop)
    if (!isMobile) {
      return SafeArea(
        child: Padding(
          padding: EdgeInsets.fromLTRB(padding, 0, padding, padding),
          child: AnimatedBuilder(
            animation: controller,
            builder: (context, _) {
              final duration = controller.state.duration;
              return Column(
                mainAxisSize: MainAxisSize.min,
                children: [
                  // 1. Progress Bar (Top of controls)
                  SizedBox(
                    height: 16,
                    child: SlideTransition(
                      // Optional: nice animation logic here, keeping it simple
                      position: const AlwaysStoppedAnimation(Offset.zero),
                      child: SliderTheme(
                        data: SliderThemeData(
                          trackHeight: 4,
                          thumbShape: const RoundSliderThumbShape(
                            enabledThumbRadius: 6,
                          ),
                          overlayShape: const RoundSliderOverlayShape(
                            overlayRadius: 12,
                          ),
                          activeTrackColor: AppTheme.primaryColor,
                          inactiveTrackColor: Colors.white.withValues(
                            alpha: 0.3,
                          ),
                          thumbColor: AppTheme.primaryColor,
                          trackShape: _CustomTrackShape(), // Full width
                        ),
                        child: ValueListenableBuilder<Duration>(
                          valueListenable: controller.positionNotifier,
                          builder: (context, position, _) {
                            return Slider(
                              value: duration.inMilliseconds > 0
                                  ? (position.inMilliseconds /
                                            duration.inMilliseconds)
                                        .clamp(0.0, 1.0)
                                  : 0,
                              onChanged: (value) {
                                final seekTo = Duration(
                                  milliseconds:
                                      (value * duration.inMilliseconds).toInt(),
                                );
                                controller.seek(seekTo);
                              },
                            );
                          },
                        ),
                      ),
                    ),
                  ),
                  const SizedBox(height: 4),
                  // 2. Control Row
                  Row(
                    children: [
                      // Left Group: Controls + Volume + Time
                      IconButton(
                        focusColor: Colors.white24,
                        iconSize: iconSize,
                        icon: const Icon(Icons.replay_10, color: Colors.white),
                        onPressed: () => controller.seekBackward(),
                        tooltip: '-10s (J)',
                      ),
                      IconButton(
                        focusColor: Colors.white24,
                        iconSize: playPauseSize,
                        icon: Icon(
                          controller.state.isPlaying
                              ? Icons.pause_circle_filled
                              : Icons.play_circle_filled,
                          color: Colors.white,
                        ),
                        onPressed: controller.playOrPause,
                        tooltip: controller.state.isPlaying
                            ? 'Пауза (K)'
                            : 'Грати (K)',
                      ),
                      IconButton(
                        focusColor: Colors.white24,
                        iconSize: iconSize,
                        icon: const Icon(Icons.forward_10, color: Colors.white),
                        onPressed: () => controller.seekForward(),
                        tooltip: '+10s (L)',
                      ),

                      const SizedBox(width: 8),

                      // Volume with longer slider
                      if (!kIsWeb &&
                          (defaultTargetPlatform == TargetPlatform.windows ||
                              defaultTargetPlatform == TargetPlatform.linux ||
                              defaultTargetPlatform == TargetPlatform.macOS))
                        _VolumeSlider(controller: controller, width: 140),

                      const SizedBox(width: 12),

                      // Time
                      ValueListenableBuilder<Duration>(
                        valueListenable: controller.positionNotifier,
                        builder: (context, position, _) {
                          return Text(
                            '${_formatDuration(position)} / ${_formatDuration(duration)}',
                            style: TextStyle(
                              color: Colors.white,
                              fontSize: fontSize,
                              fontFamily: 'Roboto',
                            ),
                          );
                        },
                      ),

                      const Spacer(),

                      // Right Group: Settings + Fit + Fullscreen
                      if (controller.watchPartyService.state ==
                          WatchPartyState.connected)
                        IconButton(
                          focusColor: Colors.white24,
                          icon: Icon(
                            Icons.chat_bubble_outline,
                            color: Colors.white,
                            size: iconSize,
                          ),
                          tooltip: 'Чат',
                          onPressed: onToggleChat,
                        ),
                      if (onEnterPiP != null)
                        IconButton(
                          focusColor: Colors.white24,
                          icon: Icon(
                            Icons.picture_in_picture_alt,
                            color: Colors.white,
                            size: iconSize,
                          ),
                          tooltip: 'Картинка в картинці',
                          onPressed: onEnterPiP,
                        ),
                      IconButton(
                        focusColor: Colors.white24,
                        icon: Icon(
                          Icons.settings_outlined,
                          color: Colors.white,
                          size: iconSize,
                        ),
                        tooltip: 'Налаштування',
                        onPressed: () =>
                            _showMainSettingsSheet(context, controller),
                      ),
                      IconButton(
                        focusColor: Colors.white24,
                        icon: Icon(
                          controller.state.videoFit == BoxFit.contain
                              ? Icons.fit_screen
                              : Icons.aspect_ratio,
                          color: Colors.white,
                          size: iconSize,
                        ),
                        tooltip: 'Масштаб (Вписати/Розтягнути)',
                        onPressed: controller.cycleFit,
                      ),
                      IconButton(
                        focusColor: Colors.white24,
                        icon: Icon(
                          Icons.file_download_outlined,
                          color: Colors.white,
                          size: iconSize,
                        ),
                        tooltip: 'Завантажити',
                        onPressed:
                            (controller.isOffline ||
                                (controller.state.currentUrl.startsWith(
                                      'file',
                                    ) ||
                                    controller.state.currentUrl
                                        .toLowerCase()
                                        .startsWith('c:') ||
                                    controller.state.currentUrl.startsWith(
                                      '/',
                                    )))
                            ? null // Disable for local files
                            : () => _showDownloadDialog(context, controller),
                      ),
                      IconButton(
                        focusColor: Colors.white24,
                        icon: Icon(
                          controller.state.isFullscreen
                              ? Icons.fullscreen_exit
                              : Icons.fullscreen,
                          color: Colors.white,
                          size: iconSize,
                        ),
                        tooltip: 'Повноекранний режим (F)',
                        onPressed: controller.toggleFullscreen,
                      ),
                    ],
                  ),
                ],
              );
            },
          ),
        ),
      );
    }

    // Mobile Layout (Original, largely preserved but cleaned up)
    return SafeArea(
      child: Padding(
        padding: EdgeInsets.fromLTRB(padding, 0, padding, padding),
        child: AnimatedBuilder(
          animation: controller,
          builder: (context, _) {
            final duration = controller.state.duration;

            return Column(
              mainAxisSize: MainAxisSize.min,
              children: [
                ValueListenableBuilder<Duration>(
                  valueListenable: controller.positionNotifier,
                  builder: (context, position, _) {
                    return SliderTheme(
                      data: SliderThemeData(
                        trackHeight: isCompact ? 3 : 4,
                        thumbShape: RoundSliderThumbShape(
                          enabledThumbRadius: isCompact ? 6 : 8,
                        ),
                        overlayShape: RoundSliderOverlayShape(
                          overlayRadius: isCompact ? 12 : 16,
                        ),
                        activeTrackColor: AppTheme.primaryColor,
                        inactiveTrackColor: Colors.white.withValues(alpha: 0.3),
                        thumbColor: AppTheme.primaryColor,
                      ),
                      child: Slider(
                        value: duration.inMilliseconds > 0
                            ? (position.inMilliseconds /
                                      duration.inMilliseconds)
                                  .clamp(0.0, 1.0)
                            : 0,
                        onChanged: (value) {
                          final seekTo = Duration(
                            milliseconds: (value * duration.inMilliseconds)
                                .toInt(),
                          );
                          controller.seek(seekTo);
                        },
                      ),
                    );
                  },
                ),
                Row(
                  children: [
                    ValueListenableBuilder<Duration>(
                      valueListenable: controller.positionNotifier,
                      builder: (context, position, _) {
                        return Text(
                          '${_formatDuration(position)} / ${_formatDuration(duration)}',
                          style: TextStyle(
                            color: Colors.white,
                            fontSize: fontSize,
                          ),
                        );
                      },
                    ),
                    const Spacer(),
                    IconButton(
                      focusColor: Colors.white24,
                      iconSize: iconSize,
                      icon: const Icon(Icons.replay_10, color: Colors.white),
                      onPressed: () => controller.seekBackward(),
                    ),
                    IconButton(
                      focusColor: Colors.white24,
                      iconSize: playPauseSize,
                      icon: Icon(
                        controller.state.isPlaying
                            ? Icons.pause_circle_filled
                            : Icons.play_circle_filled,
                        color: Colors.white,
                      ),
                      onPressed: controller.playOrPause,
                    ),
                    IconButton(
                      focusColor: Colors.white24,
                      iconSize: iconSize,
                      icon: const Icon(Icons.forward_10, color: Colors.white),
                      onPressed: () => controller.seekForward(),
                    ),
                    const Spacer(),
                    IconButton(
                      focusColor: Colors.white24,
                      icon: Icon(
                        controller.state.isFullscreen
                            ? Icons.fullscreen_exit
                            : Icons.fullscreen,
                        color: Colors.white,
                        size: iconSize,
                      ),
                      onPressed: controller.toggleFullscreen,
                    ),
                  ],
                ),
              ],
            );
          },
        ),
      ),
    );
  }

  String _formatDuration(Duration duration) {
    String twoDigits(int n) => n.toString().padLeft(2, '0');
    final hours = duration.inHours;
    final minutes = duration.inMinutes.remainder(60);
    final seconds = duration.inSeconds.remainder(60);
    return hours > 0
        ? '$hours:${twoDigits(minutes)}:${twoDigits(seconds)}'
        : '${twoDigits(minutes)}:${twoDigits(seconds)}';
  }
}

class _CustomTrackShape extends RoundedRectSliderTrackShape {
  @override
  Rect getPreferredRect({
    required RenderBox parentBox,
    Offset offset = Offset.zero,
    required SliderThemeData sliderTheme,
    bool isEnabled = false,
    bool isDiscrete = false,
  }) {
    final double trackHeight = sliderTheme.trackHeight!;
    final double trackLeft = offset.dx;
    final double trackTop =
        offset.dy + (parentBox.size.height - trackHeight) / 2;
    final double trackWidth = parentBox.size.width;
    return Rect.fromLTWH(trackLeft, trackTop, trackWidth, trackHeight);
  }
}

// Helper methods for sheets and menu items
List<PopupMenuItem<dynamic>> _buildQualityMenuItems(
  PlayerController controller,
) {
  final items = <PopupMenuItem<dynamic>>[];
  final addedQualities = <String>{};

  // Helper to add menu item
  void addQualityItem(String label, dynamic value, bool isSelected) {
    if (addedQualities.contains(label)) return;
    addedQualities.add(label);

    items.add(
      PopupMenuItem(
        value: value,
        child: Row(
          children: [
            if (isSelected)
              const Icon(Icons.check, size: 18)
            else
              const SizedBox(width: 18),
            const SizedBox(width: 8),
            Text(label),
          ],
        ),
      ),
    );
  }

  // 1. Add HLS Video Tracks (seamless switching)
  if (controller.state.videoTracks.length > 1) {
    final sortedTracks = List<VideoTrack>.from(controller.state.videoTracks)
      ..sort((a, b) => (b.h ?? 0).compareTo(a.h ?? 0));

    for (final track in sortedTracks) {
      if (track.h != null && track.h! > 0) {
        addQualityItem(
          '${track.h}p',
          track,
          controller.state.selectedVideoTrack == track,
        );
      }
    }

    // Auto option for HLS
    addQualityItem(
      'Auto',
      VideoTrack.auto(),
      controller.state.selectedVideoTrack?.id == 'auto',
    );
  }

  // 2. Add Stream Sources (URL switching)
  // We add these even if we have video tracks, because high qualities might be separate files
  if (controller.streams != null) {
    final uniqueQualities = <StreamQuality, StreamSource>{};
    for (final s in controller.streams!) {
      // Skip unknown quality (Auto) if we already have HLS tracks
      // This prevents duplicate "Auto" options
      if (controller.state.videoTracks.length > 1 &&
          s.quality == StreamQuality.unknown &&
          s.voiceover == controller.state.currentVoiceover) {
        continue;
      }

      if (s.voiceover == controller.state.currentVoiceover) {
        // Only keep if we haven't seen this quality from HLS tracks yet
        // OR if the HLS track quality matches but we want to allow explicit stream selection?
        // Better to just show what's missing.
        // Actually, if we have "1080p" from HLS and "1080p" from Stream, HLS is better (instantly switches).
        // So we only add if NOT in addedQualities.
        uniqueQualities[s.quality] = s;
      }
    }

    final sorted = uniqueQualities.values.toList()
      ..sort((a, b) => b.quality.sortOrder.compareTo(a.quality.sortOrder));

    for (final s in sorted) {
      addQualityItem(
        s.quality.displayName,
        s,
        controller.state.currentUrl == s.url,
      );
    }
  }

  return items;
}

List<PopupMenuItem<StreamSource>> _buildVoiceoverMenuItems(
  PlayerController controller,
) {
  final items = <PopupMenuItem<StreamSource>>[];
  if (controller.streams != null) {
    final voiceovers = <String, StreamSource>{};
    // Group streams by voiceover
    final streamsByVoiceover = <String, List<StreamSource>>{};
    for (final s in controller.streams!) {
      if (s.voiceover != null) {
        streamsByVoiceover.putIfAbsent(s.voiceover!, () => []).add(s);
      }
    }

    // For each voiceover, pick the best representative stream
    // Priority:
    // 1. Match current quality
    // 2. Highest quality
    for (final entry in streamsByVoiceover.entries) {
      final streams = entry.value;
      StreamSource? bestMatch;

      // Try to find match for current quality
      if (controller.state.currentQuality != null) {
        bestMatch = streams.firstWhereOrNull(
          (s) => s.quality == controller.state.currentQuality,
        );
      }

      // Fallback to highest quality
      if (bestMatch == null) {
        streams.sort(
          (a, b) => b.quality.sortOrder.compareTo(a.quality.sortOrder),
        );
        bestMatch = streams.first;
      }

      voiceovers[entry.key] = bestMatch;
    }

    for (final entry in voiceovers.entries) {
      items.add(
        PopupMenuItem(
          value: entry.value,
          child: Row(
            children: [
              if (controller.state.currentVoiceover == entry.key)
                const Icon(Icons.check, size: 18)
              else
                const SizedBox(width: 18),
              const SizedBox(width: 8),
              Text(entry.key),
            ],
          ),
          onTap: () {
            // The value is handled by the caller, but we want to be sure
            // controller.switchStream(entry.value);
            // actually the caller is _showVoiceoverSheet which usually uses the value
          },
        ),
      );
    }
  }
  return items;
}

void _showMainSettingsSheet(BuildContext context, PlayerController controller) {
  showModalBottomSheet(
    context: context,
    backgroundColor: Colors.grey[900],
    shape: const RoundedRectangleBorder(
      borderRadius: BorderRadius.vertical(top: Radius.circular(16)),
    ),
    builder: (context) => SafeArea(
      child: Column(
        mainAxisSize: MainAxisSize.min,
        children: [
          const Padding(
            padding: EdgeInsets.symmetric(vertical: 12),
            child: Text(
              'Налаштування',
              style: TextStyle(fontSize: 18, fontWeight: FontWeight.bold),
            ),
          ),
          if (controller.hasMultipleQualities())
            ListTile(
              leading: const Icon(Icons.high_quality),
              title: const Text('Якість'),
              trailing: Text(
                controller.buildSubtitleText(null).split('•').first.trim(),
              ),
              onTap: () {
                Navigator.pop(context);
                _showQualitySheet(context, controller);
              },
            ),
          if (controller.hasMultipleVoiceovers())
            ListTile(
              leading: const Icon(Icons.record_voice_over),
              title: const Text('Озвучка'),
              trailing: Text(controller.state.currentVoiceover ?? ''),
              onTap: () {
                Navigator.pop(context);
                _showVoiceoverSheet(context, controller);
              },
            ),
          if (controller.hasMultipleAudioTracks())
            ListTile(
              leading: const Icon(Icons.audiotrack),
              title: const Text('Аудіо доріжка'),
              trailing: Text(
                controller.state.selectedAudioTrack?.language ??
                    controller.state.selectedAudioTrack?.title ??
                    controller.state.selectedAudioTrack?.id ??
                    '',
              ),
              onTap: () {
                Navigator.pop(context);
                _showAudioTrackSheet(context, controller);
              },
            ),
          ListTile(
            leading: const Icon(Icons.speed),
            title: const Text('Швидкість'),
            trailing: Text('${controller.state.playbackSpeed}x'),
            onTap: () {
              Navigator.pop(context);
              _showSpeedSheet(context, controller);
            },
          ),
          SwitchListTile(
            secondary: const Icon(Icons.headphones_outlined),
            title: const Text('Фонове аудіо (економія екрана)'),
            subtitle: const Text('Вимикає відеоряд для прослуховування'),
            value: controller.state.isAudioOnlyMode,
            onChanged: (val) {
              controller.toggleAudioOnlyMode();
              Navigator.pop(context);
            },
          ),
          ListTile(
            leading: const Icon(Icons.file_download_outlined),
            title: const Text('Завантажити'),
            enabled:
                !(controller.isOffline ||
                    (controller.state.currentUrl.startsWith('file') ||
                        controller.state.currentUrl.toLowerCase().startsWith(
                          'c:',
                        ) ||
                        controller.state.currentUrl.startsWith('/'))),
            onTap: () {
              Navigator.pop(context);
              _showDownloadDialog(context, controller);
            },
          ),
          const SizedBox(height: 16),
        ],
      ),
    ),
  );
}

void _showQualitySheet(BuildContext context, PlayerController controller) {
  showModalBottomSheet(
    context: context,
    backgroundColor: Colors.grey[900],
    isScrollControlled: true,
    builder: (context) => SafeArea(
      child: Container(
        constraints: BoxConstraints(
          maxHeight: MediaQuery.of(context).size.height * 0.8,
        ),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            const Padding(
              padding: EdgeInsets.all(16),
              child: Text(
                'Якість',
                style: TextStyle(fontSize: 18, fontWeight: FontWeight.bold),
              ),
            ),
            Flexible(
              child: ListView(
                shrinkWrap: true,
                children: [
                  if (controller.state.videoTracks.length > 1) ...[
                    // HLS Tracks
                    ...controller.state.videoTracks
                        .where((t) => t.h != null && t.h! > 0)
                        .toList()
                        .reversed // usually best quality first if sorted
                        .map((track) {
                          return ListTile(
                            leading:
                                controller.state.selectedVideoTrack == track
                                ? const Icon(Icons.check, size: 18)
                                : const SizedBox(width: 18),
                            title: Text('${track.h}p'),
                            onTap: () {
                              Navigator.pop(context);
                              controller.setVideoTrack(track);
                            },
                          );
                        }),
                    // Auto
                    ListTile(
                      leading: controller.state.selectedVideoTrack?.id == 'auto'
                          ? const Icon(Icons.check, size: 18)
                          : const SizedBox(width: 18),
                      title: const Text('Auto'),
                      onTap: () {
                        Navigator.pop(context);
                        controller.setVideoTrack(VideoTrack.auto());
                      },
                    ),
                  ],
                  if (controller.streams != null) ...[
                    // Manual Stream Sources
                    ...controller.streams!
                        .where(
                          (s) =>
                              !(controller.state.videoTracks.length > 1 &&
                                  s.quality == StreamQuality.unknown &&
                                  s.voiceover ==
                                      controller.state.currentVoiceover),
                        )
                        .where(
                          (s) =>
                              s.voiceover == controller.state.currentVoiceover,
                        )
                        .map((s) => MapEntry(s.quality, s))
                        .fold<Map<StreamQuality, StreamSource>>({}, (
                          map,
                          entry,
                        ) {
                          map.putIfAbsent(entry.key, () => entry.value);
                          return map;
                        })
                        .values
                        .toList()
                        .reversed // Best quality first logic approximation
                        .map((s) {
                          return ListTile(
                            leading: controller.state.currentUrl == s.url
                                ? const Icon(Icons.check, size: 18)
                                : const SizedBox(width: 18),
                            title: Text(s.quality.displayName),
                            onTap: () {
                              Navigator.pop(context);
                              controller.switchStream(s);
                            },
                          );
                        }),
                  ],
                ],
              ),
            ),
            const SizedBox(height: 16),
          ],
        ),
      ),
    ),
  );
}

void _showVoiceoverSheet(BuildContext context, PlayerController controller) {
  showModalBottomSheet(
    context: context,
    backgroundColor: Colors.grey[900],
    isScrollControlled: true,
    builder: (context) {
      final voiceovers = <String, StreamSource>{};
      if (controller.streams != null) {
        final streamsByVoiceover = <String, List<StreamSource>>{};
        for (final s in controller.streams!) {
          if (s.voiceover != null) {
            streamsByVoiceover.putIfAbsent(s.voiceover!, () => []).add(s);
          }
        }
        for (final entry in streamsByVoiceover.entries) {
          final streams = entry.value;
          StreamSource? bestMatch;
          if (controller.state.currentQuality != null) {
            bestMatch = streams.firstWhereOrNull(
              (s) => s.quality == controller.state.currentQuality,
            );
          }
          if (bestMatch == null) {
            streams.sort(
              (a, b) => b.quality.sortOrder.compareTo(a.quality.sortOrder),
            );
            bestMatch = streams.first;
          }
          voiceovers[entry.key] = bestMatch;
        }
      }

      return SafeArea(
        child: Container(
          constraints: BoxConstraints(
            maxHeight: MediaQuery.of(context).size.height * 0.8,
          ),
          child: Column(
            mainAxisSize: MainAxisSize.min,
            children: [
              const Padding(
                padding: EdgeInsets.all(16),
                child: Text(
                  'Озвучка',
                  style: TextStyle(fontSize: 18, fontWeight: FontWeight.bold),
                ),
              ),
              Flexible(
                child: ListView(
                  shrinkWrap: true,
                  children: voiceovers.entries.map((entry) {
                    return ListTile(
                      leading: controller.state.currentVoiceover == entry.key
                          ? const Icon(Icons.check, size: 18)
                          : const SizedBox(width: 18),
                      title: Text(entry.key),
                      onTap: () {
                        Navigator.pop(context);
                        controller.switchStream(entry.value);
                      },
                    );
                  }).toList(),
                ),
              ),
              const SizedBox(height: 16),
            ],
          ),
        ),
      );
    },
  );
}

void _showSpeedSheet(BuildContext context, PlayerController controller) {
  showModalBottomSheet(
    context: context,
    backgroundColor: Colors.grey[900],
    builder: (context) => SafeArea(
      child: Column(
        mainAxisSize: MainAxisSize.min,
        children: [
          const Padding(
            padding: EdgeInsets.all(16),
            child: Text(
              'Швидкість',
              style: TextStyle(fontSize: 18, fontWeight: FontWeight.bold),
            ),
          ),
          ...PlayerController.speeds.map(
            (speed) => ListTile(
              title: Text('${speed}x'),
              leading: speed == controller.state.playbackSpeed
                  ? const Icon(Icons.check, color: AppTheme.primaryColor)
                  : const SizedBox(width: 24),
              onTap: () {
                Navigator.pop(context);
                controller.setSpeed(speed);
              },
            ),
          ),
          const SizedBox(height: 16),
        ],
      ),
    ),
  );
}

class _VolumeSlider extends StatelessWidget {
  final PlayerController controller;
  final double width;

  const _VolumeSlider({required this.controller, this.width = 120});

  @override
  Widget build(BuildContext context) {
    final currentVolume = controller.state.volume;
    final isBoosted = currentVolume > 100.0;

    return Container(
      width: width,
      padding: const EdgeInsets.symmetric(horizontal: 8),
      child: Row(
        children: [
          IconButton(
            padding: EdgeInsets.zero,
            constraints: const BoxConstraints(),
            icon: Icon(
              _getVolumeIcon(currentVolume),
              color: isBoosted ? Colors.orangeAccent : Colors.white,
              size: 20,
            ),
            tooltip: currentVolume == 0 ? 'Увімкнути звук (M)' : 'Вимкнути звук (M)',
            onPressed: controller.toggleMute,
          ),
          const SizedBox(width: 6),
          Expanded(
            child: SliderTheme(
              data: SliderTheme.of(context).copyWith(
                thumbShape: const RoundSliderThumbShape(enabledThumbRadius: 6),
                overlayShape: const RoundSliderOverlayShape(overlayRadius: 14),
                trackHeight: 2,
                activeTrackColor: isBoosted ? Colors.orangeAccent : Colors.white,
                inactiveTrackColor: Colors.white24,
                thumbColor: isBoosted ? Colors.orangeAccent : Colors.white,
              ),
              child: Slider(
                value: currentVolume.clamp(0.0, 150.0),
                min: 0.0,
                max: 150.0,
                onChanged: (value) {
                  controller.setVolume(value);
                },
              ),
            ),
          ),
        ],
      ),
    );
  }

  IconData _getVolumeIcon(double volume) {
    if (volume == 0) return Icons.volume_off;
    if (volume < 50) return Icons.volume_down;
    if (volume <= 100) return Icons.volume_up;
    return Icons.campaign; // Boost icon
  }
}

/// Window control button for player (minimize, maximize, close)
class _WindowControlButton extends StatefulWidget {
  final IconData icon;
  final String tooltip;
  final VoidCallback onPressed;
  final bool isClose;

  const _WindowControlButton({
    required this.icon,
    required this.tooltip,
    required this.onPressed,
    this.isClose = false,
  });

  @override
  State<_WindowControlButton> createState() => _WindowControlButtonState();
}

class _WindowControlButtonState extends State<_WindowControlButton> {
  bool _isHovered = false;

  @override
  Widget build(BuildContext context) {
    return Tooltip(
      message: widget.tooltip,
      child: MouseRegion(
        onEnter: (_) => setState(() => _isHovered = true),
        onExit: (_) => setState(() => _isHovered = false),
        child: GestureDetector(
          onTap: widget.onPressed,
          child: Container(
            width: 36,
            height: 28,
            decoration: BoxDecoration(
              color: _isHovered
                  ? (widget.isClose ? Colors.red : Colors.white24)
                  : Colors.transparent,
              borderRadius: BorderRadius.circular(4),
            ),
            child: Icon(
              widget.icon,
              size: 16,
              color: _isHovered && widget.isClose
                  ? Colors.white
                  : Colors.white70,
            ),
          ),
        ),
      ),
    );
  }
}

// NOTE: _buildAudioTrackMenuItems is still needed for _ExpandedMenu (PopupMenu)
// We keep it as is, but we DO NOT use it in the sheet below.
List<PopupMenuItem<AudioTrack>> _buildAudioTrackMenuItems(
  PlayerController controller,
) {
  final items = <PopupMenuItem<AudioTrack>>[];
  for (final track in controller.state.audioTracks) {
    items.add(
      PopupMenuItem(
        value: track,
        child: Row(
          children: [
            if (controller.state.selectedAudioTrack == track)
              const Icon(Icons.check, size: 18)
            else
              const SizedBox(width: 18),
            const SizedBox(width: 8),
            Expanded(
              child: Text(
                _formatAudioTrackName(track),
                overflow: TextOverflow.ellipsis,
              ),
            ),
          ],
        ),
      ),
    );
  }
  return items;
}

String _formatAudioTrackName(AudioTrack track) {
  final lang = track.language?.toLowerCase();
  final title = track.title;

  if (title != null && title.isNotEmpty) {
    return title;
  }

  switch (lang) {
    case 'uk':
    case 'ukr':
    case 'ua':
      return 'Українська';
    case 'en':
    case 'eng':
    case 'us':
      return 'Англійська';
    case 'ru':
    case 'rus':
      return 'Російська';
    case 'ja':
    case 'jpn':
      return 'Японська';
    case 'fr':
    case 'fra':
      return 'Французька';
    case 'de':
    case 'deu':
      return 'Німецька';
    case 'it':
    case 'ita':
      return 'Італійська';
    case 'es':
    case 'spa':
      return 'Іспанська';
    default:
      return track.language?.toUpperCase() ?? track.id;
  }
}

void _showAudioTrackSheet(BuildContext context, PlayerController controller) {
  showModalBottomSheet(
    context: context,
    backgroundColor: Colors.grey[900],
    isScrollControlled: true,
    builder: (context) => SafeArea(
      child: Container(
        constraints: BoxConstraints(
          maxHeight: MediaQuery.of(context).size.height * 0.8,
        ),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            const Padding(
              padding: EdgeInsets.all(16),
              child: Text(
                'Аудіо доріжка',
                style: TextStyle(fontSize: 18, fontWeight: FontWeight.bold),
              ),
            ),
            Flexible(
              child: ListView(
                shrinkWrap: true,
                children: controller.state.audioTracks.map((track) {
                  return ListTile(
                    leading: controller.state.selectedAudioTrack == track
                        ? const Icon(Icons.check, size: 18)
                        : const SizedBox(width: 18),
                    title: Text(
                      _formatAudioTrackName(track),
                      overflow: TextOverflow.ellipsis,
                      // No Expanded here!
                    ),
                    onTap: () {
                      Navigator.pop(context);
                      controller.setAudioTrack(track);
                    },
                  );
                }).toList(),
              ),
            ),
            const SizedBox(height: 16),
          ],
        ),
      ),
    ),
  );
}

void _showDownloadDialog(BuildContext context, PlayerController controller) {
  final s = AppStrings.of(context);
  final downloadService = getIt<DownloadService>();

  if (controller.mediaItem == null || controller.currentSource == null) {
    ScaffoldMessenger.of(
      context,
    ).showSnackBar(SnackBar(content: Text(s.selectMediaFirst)));
    return;
  }

  showDialog(
    context: context,
    builder: (context) => AlertDialog(
      backgroundColor: Colors.grey[900],
      title: Text(s.downloads, style: const TextStyle(color: Colors.white)),
      content: Text(
        '${s.confirm}: ${controller.title}?',
        style: const TextStyle(color: Colors.white70),
      ),
      actions: [
        TextButton(
          onPressed: () => Navigator.pop(context),
          child: Text(s.cancel),
        ),
        TextButton(
          onPressed: () async {
            Navigator.pop(context);
            final success = await downloadService.downloadContent(
              item: controller.mediaItem!,
              source: controller.currentSource!,
              season: controller.state.currentSeason,
              episode: controller.state.currentEpisode,
              episodeTitle: controller.state.currentEpisodeTitle,
              duration: controller.state.duration.inSeconds,
            );
            if (success && context.mounted) {
              ScaffoldMessenger.of(context).showSnackBar(
                const SnackBar(content: Text('Завантаження розпочато')),
              );
            } else if (context.mounted) {
              ScaffoldMessenger.of(context).showSnackBar(
                SnackBar(content: Text(downloadService.lastError ?? s.error)),
              );
            }
          },
          child: Text(
            s.start,
            style: const TextStyle(color: AppTheme.primaryColor),
          ),
        ),
      ],
    ),
  );
}
