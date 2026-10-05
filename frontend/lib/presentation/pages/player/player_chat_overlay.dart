import 'package:flutter/material.dart';
import '../../../core/utils/responsive_utils.dart';
import '../../../data/services/watch_party_service.dart';

class PlayerChatOverlay extends StatefulWidget {
  final WatchPartyService service;
  final VoidCallback onClose;

  const PlayerChatOverlay({
    super.key,
    required this.service,
    required this.onClose,
  });

  @override
  State<PlayerChatOverlay> createState() => _PlayerChatOverlayState();
}

class _PlayerChatOverlayState extends State<PlayerChatOverlay> {
  final _controller = TextEditingController();
  final _scrollController = ScrollController();

  @override
  void dispose() {
    _controller.dispose();
    _scrollController.dispose();
    super.dispose();
  }

  void _sendMessage() {
    final text = _controller.text.trim();
    if (text.isNotEmpty) {
      widget.service.sendChatMessage(text);
      _controller.clear();
      // Auto scroll to bottom
      WidgetsBinding.instance.addPostFrameCallback((_) {
        if (_scrollController.hasClients) {
          _scrollController.animateTo(
            _scrollController.position.maxScrollExtent,
            duration: const Duration(milliseconds: 300),
            curve: Curves.easeOut,
          );
        }
      });
    }
  }

  @override
  Widget build(BuildContext context) {
    final width = context.chatPanelWidth;
    final isCompact = context.isCompact;
    final padding = isCompact ? 6.0 : 8.0;
    final fontSize = isCompact ? 11.0 : 12.0;
    final headerFontSize = isCompact ? 13.0 : 14.0;

    return Container(
      width: width,
      color: Colors.black.withOpacity(0.85),
      child: SafeArea(
        left: false,
        child: Column(
          children: [
            // Header
            Padding(
              padding: EdgeInsets.all(padding),
              child: Row(
                mainAxisAlignment: MainAxisAlignment.spaceBetween,
                children: [
                  Expanded(
                    child: Text(
                      'Чат кімнати',
                      style: TextStyle(
                        color: Colors.white,
                        fontWeight: FontWeight.bold,
                        fontSize: headerFontSize,
                      ),
                      overflow: TextOverflow.ellipsis,
                    ),
                  ),
                  // Participant count badge
                  Container(
                    padding: const EdgeInsets.symmetric(
                      horizontal: 8,
                      vertical: 2,
                    ),
                    decoration: BoxDecoration(
                      color: Colors.white.withOpacity(0.1),
                      borderRadius: BorderRadius.circular(12),
                    ),
                    child: Row(
                      mainAxisSize: MainAxisSize.min,
                      children: [
                        Icon(
                          Icons.people,
                          color: Colors.white54,
                          size: isCompact ? 12 : 14,
                        ),
                        const SizedBox(width: 4),
                        Text(
                          '${widget.service.participants.length}',
                          style: TextStyle(
                            color: Colors.white54,
                            fontSize: isCompact ? 10 : 11,
                          ),
                        ),
                      ],
                    ),
                  ),
                  IconButton(
                    icon: Icon(
                      Icons.close,
                      color: Colors.white,
                      size: isCompact ? 20 : 24,
                    ),
                    onPressed: widget.onClose,
                    padding: EdgeInsets.all(isCompact ? 4 : 8),
                    constraints: BoxConstraints(
                      minWidth: isCompact ? 32 : 40,
                      minHeight: isCompact ? 32 : 40,
                    ),
                  ),
                ],
              ),
            ),

            // Messages List
            Expanded(
              child: ListenableBuilder(
                listenable: widget.service,
                builder: (context, _) {
                  final messages = widget.service.chatMessages;
                  if (messages.isEmpty) {
                    return Center(
                      child: Text(
                        'Поки немає повідомлень',
                        style: TextStyle(
                          color: Colors.white.withOpacity(0.5),
                          fontStyle: FontStyle.italic,
                          fontSize: fontSize,
                        ),
                      ),
                    );
                  }
                  return ListView.builder(
                    controller: _scrollController,
                    padding: EdgeInsets.symmetric(
                      horizontal: padding,
                      vertical: padding,
                    ),
                    itemCount: messages.length,
                    itemBuilder: (context, index) {
                      final msg = messages[index];
                      final isMe = msg.senderId == widget.service.myId;
                      return Padding(
                        padding: EdgeInsets.only(bottom: isCompact ? 6 : 8),
                        child: Column(
                          crossAxisAlignment: CrossAxisAlignment.start,
                          children: [
                            Row(
                              children: [
                                Flexible(
                                  child: Text(
                                    isMe ? 'Ви' : msg.senderName,
                                    style: TextStyle(
                                      color: isMe
                                          ? Colors.blueAccent
                                          : Colors.greenAccent,
                                      fontWeight: FontWeight.bold,
                                      fontSize: fontSize,
                                    ),
                                    overflow: TextOverflow.ellipsis,
                                  ),
                                ),
                                const SizedBox(width: 8),
                                Text(
                                  _formatTime(msg.timestamp),
                                  style: TextStyle(
                                    color: Colors.white.withOpacity(0.5),
                                    fontSize: isCompact ? 9 : 10,
                                  ),
                                ),
                              ],
                            ),
                            const SizedBox(height: 2),
                            Text(
                              msg.message,
                              style: TextStyle(
                                color: Colors.white,
                                fontSize: fontSize,
                              ),
                            ),
                          ],
                        ),
                      );
                    },
                  );
                },
              ),
            ),

            // Input
            Padding(
              padding: EdgeInsets.fromLTRB(
                padding,
                padding,
                padding,
                padding + MediaQuery.of(context).viewInsets.bottom,
              ),
              child: Row(
                children: [
                  Expanded(
                    child: TextField(
                      controller: _controller,
                      style: TextStyle(
                        color: Colors.white,
                        fontSize: fontSize + 1,
                      ),
                      decoration: InputDecoration(
                        hintText: 'Повідомлення...',
                        hintStyle: TextStyle(
                          color: Colors.white.withOpacity(0.5),
                          fontSize: fontSize + 1,
                        ),
                        border: OutlineInputBorder(
                          borderRadius: BorderRadius.circular(20),
                          borderSide: BorderSide.none,
                        ),
                        filled: true,
                        fillColor: Colors.white.withOpacity(0.1),
                        contentPadding: EdgeInsets.symmetric(
                          horizontal: isCompact ? 12 : 16,
                          vertical: isCompact ? 6 : 8,
                        ),
                        isDense: isCompact,
                      ),
                      onSubmitted: (_) => _sendMessage(),
                    ),
                  ),
                  SizedBox(width: isCompact ? 4 : 8),
                  IconButton(
                    icon: Icon(
                      Icons.send,
                      color: Colors.blueAccent,
                      size: isCompact ? 20 : 24,
                    ),
                    onPressed: _sendMessage,
                    padding: EdgeInsets.all(isCompact ? 4 : 8),
                    constraints: BoxConstraints(
                      minWidth: isCompact ? 36 : 44,
                      minHeight: isCompact ? 36 : 44,
                    ),
                  ),
                ],
              ),
            ),
          ],
        ),
      ),
    );
  }

  String _formatTime(DateTime dt) {
    return '${dt.hour.toString().padLeft(2, '0')}:${dt.minute.toString().padLeft(2, '0')}';
  }
}
