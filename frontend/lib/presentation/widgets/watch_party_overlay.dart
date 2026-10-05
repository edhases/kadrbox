import 'dart:async';
import 'package:flutter/material.dart';
import '../../../data/services/watch_party_service.dart';

/// Overlay widget for watch party controls during video playback
class WatchPartyOverlay extends StatefulWidget {
  final WatchPartyService service;
  final VoidCallback? onPlay;
  final VoidCallback? onPause;
  final Function(Duration)? onSeek;
  final Duration currentPosition;
  final bool isPlaying;

  const WatchPartyOverlay({
    super.key,
    required this.service,
    this.onPlay,
    this.onPause,
    this.onSeek,
    required this.currentPosition,
    required this.isPlaying,
  });

  @override
  State<WatchPartyOverlay> createState() => _WatchPartyOverlayState();
}

class _WatchPartyOverlayState extends State<WatchPartyOverlay> {
  bool _showChat = false;
  final _chatController = TextEditingController();
  final _chatScrollController = ScrollController();
  Timer? _syncTimer;

  @override
  void initState() {
    super.initState();
    widget.service.addListener(_onServiceChanged);

    // Set up sync callbacks
    widget.service.onPlayPauseChanged = (isPlaying) {
      if (isPlaying) {
        widget.onPlay?.call();
      } else {
        widget.onPause?.call();
      }
    };

    widget.service.onSeek = (position) {
      widget.onSeek?.call(position);
    };

    // If host, start sync timer
    if (widget.service.isHost) {
      _syncTimer = Timer.periodic(const Duration(seconds: 2), (_) {
        widget.service.syncPosition(widget.currentPosition, widget.isPlaying);
      });
    }
  }

  @override
  void dispose() {
    _syncTimer?.cancel();
    widget.service.removeListener(_onServiceChanged);
    _chatController.dispose();
    _chatScrollController.dispose();
    super.dispose();
  }

  void _onServiceChanged() {
    setState(() {});

    // Auto-scroll chat
    if (_chatScrollController.hasClients && _showChat) {
      WidgetsBinding.instance.addPostFrameCallback((_) {
        _chatScrollController.animateTo(
          _chatScrollController.position.maxScrollExtent,
          duration: const Duration(milliseconds: 200),
          curve: Curves.easeOut,
        );
      });
    }
  }

  @override
  Widget build(BuildContext context) {
    if (widget.service.state != WatchPartyState.connected) {
      return const SizedBox.shrink();
    }

    final theme = Theme.of(context);

    return Stack(
      children: [
        // Chat panel (slides in from right)
        AnimatedPositioned(
          duration: const Duration(milliseconds: 300),
          curve: Curves.easeInOut,
          right: _showChat ? 0 : -320,
          top: 0,
          bottom: 80,
          width: 320,
          child: Container(
            decoration: BoxDecoration(
              color: Colors.black.withValues(alpha: 0.85),
              borderRadius: const BorderRadius.only(
                topLeft: Radius.circular(16),
                bottomLeft: Radius.circular(16),
              ),
            ),
            child: Column(
              children: [
                // Header
                Container(
                  padding: const EdgeInsets.all(12),
                  decoration: BoxDecoration(
                    border: Border(
                      bottom: BorderSide(
                        color: Colors.white.withValues(alpha: 0.1),
                      ),
                    ),
                  ),
                  child: Row(
                    children: [
                      const Icon(Icons.chat, color: Colors.white, size: 20),
                      const SizedBox(width: 8),
                      const Expanded(
                        child: Text(
                          'Чат',
                          style: TextStyle(
                            color: Colors.white,
                            fontWeight: FontWeight.bold,
                          ),
                        ),
                      ),
                      Container(
                        padding: const EdgeInsets.symmetric(
                          horizontal: 8,
                          vertical: 2,
                        ),
                        decoration: BoxDecoration(
                          color: Colors.white.withValues(alpha: 0.1),
                          borderRadius: BorderRadius.circular(12),
                        ),
                        child: Row(
                          mainAxisSize: MainAxisSize.min,
                          children: [
                            const Icon(
                              Icons.people,
                              color: Colors.white54,
                              size: 14,
                            ),
                            const SizedBox(width: 4),
                            Text(
                              '${widget.service.participants.length}',
                              style: const TextStyle(
                                color: Colors.white54,
                                fontSize: 12,
                              ),
                            ),
                          ],
                        ),
                      ),
                    ],
                  ),
                ),

                // Messages
                Expanded(
                  child: ListView.builder(
                    controller: _chatScrollController,
                    padding: const EdgeInsets.all(8),
                    itemCount: widget.service.chatMessages.length,
                    itemBuilder: (context, index) {
                      final message = widget.service.chatMessages[index];
                      final isMe = message.senderId == widget.service.myId;

                      return Padding(
                        padding: const EdgeInsets.only(bottom: 4),
                        child: Row(
                          crossAxisAlignment: CrossAxisAlignment.start,
                          children: [
                            Text(
                              '${message.senderName}: ',
                              style: TextStyle(
                                color: isMe
                                    ? theme.colorScheme.primary
                                    : Colors.amber,
                                fontWeight: FontWeight.bold,
                                fontSize: 12,
                              ),
                            ),
                            Expanded(
                              child: Text(
                                message.message,
                                style: const TextStyle(
                                  color: Colors.white,
                                  fontSize: 12,
                                ),
                              ),
                            ),
                          ],
                        ),
                      );
                    },
                  ),
                ),

                // Input
                Container(
                  padding: const EdgeInsets.all(8),
                  child: Row(
                    children: [
                      Expanded(
                        child: TextField(
                          controller: _chatController,
                          style: const TextStyle(
                            color: Colors.white,
                            fontSize: 14,
                          ),
                          decoration: InputDecoration(
                            hintText: 'Повідомлення...',
                            hintStyle: TextStyle(
                              color: Colors.white.withValues(alpha: 0.5),
                            ),
                            filled: true,
                            fillColor: Colors.white.withValues(alpha: 0.1),
                            border: OutlineInputBorder(
                              borderRadius: BorderRadius.circular(20),
                              borderSide: BorderSide.none,
                            ),
                            contentPadding: const EdgeInsets.symmetric(
                              horizontal: 16,
                              vertical: 8,
                            ),
                          ),
                          onSubmitted: _sendMessage,
                        ),
                      ),
                      const SizedBox(width: 8),
                      IconButton(
                        onPressed: () => _sendMessage(_chatController.text),
                        icon: Icon(
                          Icons.send,
                          color: theme.colorScheme.primary,
                        ),
                      ),
                    ],
                  ),
                ),
              ],
            ),
          ),
        ),

        // Bottom bar
        Positioned(
          left: 0,
          right: 0,
          bottom: 0,
          child: Container(
            padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 8),
            decoration: BoxDecoration(
              gradient: LinearGradient(
                begin: Alignment.topCenter,
                end: Alignment.bottomCenter,
                colors: [
                  Colors.transparent,
                  Colors.black.withValues(alpha: 0.8),
                ],
              ),
            ),
            child: Row(
              children: [
                // Party indicator
                Container(
                  padding: const EdgeInsets.symmetric(
                    horizontal: 12,
                    vertical: 6,
                  ),
                  decoration: BoxDecoration(
                    color: theme.colorScheme.primary.withValues(alpha: 0.8),
                    borderRadius: BorderRadius.circular(16),
                  ),
                  child: Row(
                    mainAxisSize: MainAxisSize.min,
                    children: [
                      const Icon(Icons.groups, color: Colors.white, size: 16),
                      const SizedBox(width: 4),
                      Text(
                        'Спільний перегляд (${widget.service.participants.length})',
                        style: const TextStyle(
                          color: Colors.white,
                          fontSize: 12,
                          fontWeight: FontWeight.w500,
                        ),
                      ),
                    ],
                  ),
                ),

                const Spacer(),

                // Host controls
                if (widget.service.isHost) ...[
                  IconButton(
                    onPressed: () {
                      if (widget.isPlaying) {
                        widget.service.pause();
                        widget.onPause?.call();
                      } else {
                        widget.service.play();
                        widget.onPlay?.call();
                      }
                    },
                    icon: Icon(
                      widget.isPlaying ? Icons.pause : Icons.play_arrow,
                      color: Colors.white,
                    ),
                  ),
                ],

                // Chat toggle
                Badge(
                  isLabelVisible:
                      widget.service.chatMessages.isNotEmpty && !_showChat,
                  label: Text('${widget.service.chatMessages.length}'),
                  child: IconButton(
                    onPressed: () {
                      setState(() {
                        _showChat = !_showChat;
                      });
                    },
                    icon: Icon(
                      _showChat ? Icons.chat : Icons.chat_bubble_outline,
                      color: _showChat
                          ? theme.colorScheme.primary
                          : Colors.white,
                    ),
                  ),
                ),

                // Leave button
                IconButton(
                  onPressed: () async {
                    final confirmed = await showDialog<bool>(
                      context: context,
                      builder: (context) => AlertDialog(
                        title: const Text('Вийти з кімнати?'),
                        content: const Text(
                          'Ви покинете спільний перегляд. Продовжити?',
                        ),
                        actions: [
                          TextButton(
                            onPressed: () => Navigator.pop(context, false),
                            child: const Text('Скасувати'),
                          ),
                          TextButton(
                            onPressed: () => Navigator.pop(context, true),
                            child: const Text('Вийти'),
                          ),
                        ],
                      ),
                    );

                    if (confirmed == true) {
                      await widget.service.leaveRoom();
                    }
                  },
                  icon: const Icon(Icons.exit_to_app, color: Colors.white70),
                ),
              ],
            ),
          ),
        ),
      ],
    );
  }

  void _sendMessage(String text) {
    if (text.trim().isEmpty) return;

    widget.service.sendChatMessage(text.trim());
    _chatController.clear();
  }
}

/// Mini indicator for watch party status
class WatchPartyIndicator extends StatelessWidget {
  final WatchPartyService service;
  final VoidCallback? onTap;

  const WatchPartyIndicator({super.key, required this.service, this.onTap});

  @override
  Widget build(BuildContext context) {
    if (service.state != WatchPartyState.connected) {
      return const SizedBox.shrink();
    }

    final theme = Theme.of(context);

    return GestureDetector(
      onTap: onTap,
      child: Container(
        padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 8),
        decoration: BoxDecoration(
          color: theme.colorScheme.primary.withValues(alpha: 0.9),
          borderRadius: BorderRadius.circular(20),
          boxShadow: [
            BoxShadow(
              color: theme.colorScheme.primary.withValues(alpha: 0.3),
              blurRadius: 8,
              offset: const Offset(0, 2),
            ),
          ],
        ),
        child: Row(
          mainAxisSize: MainAxisSize.min,
          children: [
            const Icon(Icons.groups, color: Colors.white, size: 20),
            const SizedBox(width: 8),
            Text(
              '${service.participants.length}',
              style: const TextStyle(
                color: Colors.white,
                fontWeight: FontWeight.bold,
              ),
            ),
            if (service.chatMessages.isNotEmpty) ...[
              const SizedBox(width: 8),
              Container(
                width: 8,
                height: 8,
                decoration: const BoxDecoration(
                  color: Colors.amber,
                  shape: BoxShape.circle,
                ),
              ),
            ],
          ],
        ),
      ),
    );
  }
}
