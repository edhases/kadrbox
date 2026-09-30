import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:get_it/get_it.dart';
import 'package:go_router/go_router.dart';
import '../../../data/services/watch_party_service.dart';
import '../../../core/l10n/app_strings.dart';

/// Page for creating or joining a watch party
class WatchPartyPage extends StatefulWidget {
  final String? mediaUrl;
  final String? mediaTitle;

  const WatchPartyPage({super.key, this.mediaUrl, this.mediaTitle});

  @override
  State<WatchPartyPage> createState() => _WatchPartyPageState();
}

class _WatchPartyPageState extends State<WatchPartyPage> {
  final _service = GetIt.instance<WatchPartyService>();
  late final TextEditingController _nameController;
  final _roomCodeController = TextEditingController();
  final _chatController = TextEditingController();
  final _chatScrollController = ScrollController();

  bool _isInPlayer = false;
  bool _wasPlaying = false;
  String? _lastMediaUrl;

  @override
  void initState() {
    super.initState();
    _nameController = TextEditingController(text: _service.myName);
    _service.addListener(_onServiceChanged);
    _wasPlaying = _service.isPlaying;
    _lastMediaUrl = _service.room?.mediaUrl;
    // Check initial state after build
    WidgetsBinding.instance.addPostFrameCallback((_) {
      if (!_service.isHost &&
          _service.isPlaying &&
          _service.room?.mediaUrl != null) {
        _enterPlayer();
      }
    });
  }

  @override
  void dispose() {
    _service.removeListener(_onServiceChanged);
    // _service.dispose(); // Singleton, do not dispose
    _nameController.dispose();
    _roomCodeController.dispose();
    _chatController.dispose();
    _chatScrollController.dispose();
    super.dispose();
  }

  void _onServiceChanged() {
    setState(() {});

    final currentMediaUrl = _service.room?.mediaUrl;

    // Auto-navigate to player if playback starts or media is newly set, and we are not host
    if (!_service.isHost &&
        _service.state == WatchPartyState.connected &&
        currentMediaUrl != null) {
      final startedPlaying = _service.isPlaying && !_wasPlaying;
      final newMediaAssigned =
          currentMediaUrl != _lastMediaUrl && _lastMediaUrl == null;

      if (!_isInPlayer && (startedPlaying || newMediaAssigned)) {
        _enterPlayer();
      }
    }

    _wasPlaying = _service.isPlaying;
    _lastMediaUrl = currentMediaUrl;

    // Auto-scroll chat
    if (_chatScrollController.hasClients) {
      WidgetsBinding.instance.addPostFrameCallback((_) {
        _chatScrollController.animateTo(
          _chatScrollController.position.maxScrollExtent,
          duration: const Duration(milliseconds: 200),
          curve: Curves.easeOut,
        );
      });
    }
  }

  Future<void> _enterPlayer() async {
    if (_isInPlayer) return;
    if (_service.room?.mediaUrl == null) return;

    _isInPlayer = true;
    await context.push(
      '/player?url=${Uri.encodeComponent(_service.room!.mediaUrl!)}&title=${Uri.encodeComponent(_service.room!.mediaTitle ?? 'Movie')}',
    );
    _isInPlayer = false;
    // When we return, update _wasPlaying to avoid immediate re-trigger if still playing
    _wasPlaying = _service.isPlaying;
  }

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    final s = AppStrings.of(context);

    return Scaffold(
      appBar: AppBar(
        title: Text(s.watchParty),
        actions: [
          if (_service.state == WatchPartyState.connected)
            IconButton(
              icon: const Icon(Icons.exit_to_app),
              tooltip: s.leaveRoom,
              onPressed: () async {
                await _service.leaveRoom();
              },
            ),
        ],
      ),
      body: _buildBody(theme, s),
    );
  }

  Widget _buildBody(ThemeData theme, AppStrings s) {
    switch (_service.state) {
      case WatchPartyState.idle:
        return _buildIdleState(theme, s);
      case WatchPartyState.hosting:
      case WatchPartyState.joining:
        return Center(
          child: Column(
            mainAxisSize: MainAxisSize.min,
            children: [
              const CircularProgressIndicator(),
              const SizedBox(height: 16),
              Text(s.connecting),
            ],
          ),
        );
      case WatchPartyState.connected:
        return _buildConnectedState(theme, s);
      case WatchPartyState.error:
        return _buildErrorState(theme, s);
    }
  }

  Widget _buildIdleState(ThemeData theme, AppStrings s) {
    return SingleChildScrollView(
      padding: const EdgeInsets.all(16),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          // Name input
          Card(
            child: Padding(
              padding: const EdgeInsets.all(16),
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Text('Ваше ім\'я', style: theme.textTheme.titleMedium),
                  const SizedBox(height: 8),
                  TextField(
                    controller: _nameController,
                    decoration: const InputDecoration(
                      hintText: 'Введіть ваше ім\'я',
                      prefixIcon: Icon(Icons.person),
                    ),
                    onChanged: (value) => _service.setMyName(value),
                  ),
                ],
              ),
            ),
          ),
          const SizedBox(height: 16),

          // Info banner about service
          Container(
            padding: const EdgeInsets.all(12),
            decoration: BoxDecoration(
              color: theme.colorScheme.tertiaryContainer.withValues(alpha: 0.5),
              borderRadius: BorderRadius.circular(8),
              border: Border.all(
                color: theme.colorScheme.tertiary.withValues(alpha: 0.3),
              ),
            ),
            child: Row(
              children: [
                Icon(
                  Icons.info_outline,
                  size: 20,
                  color: theme.colorScheme.tertiary,
                ),
                const SizedBox(width: 12),
                Expanded(
                  child: Text(
                    'Спільний перегляд працює через інтернет. '
                    'Це експериментальна функція.',
                    style: theme.textTheme.bodySmall?.copyWith(
                      color: theme.colorScheme.onTertiaryContainer,
                    ),
                  ),
                ),
              ],
            ),
          ),
          const SizedBox(height: 16),

          // Host option
          Card(
            child: Padding(
              padding: const EdgeInsets.all(16),
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Row(
                    children: [
                      Icon(Icons.tv, color: theme.colorScheme.primary),
                      const SizedBox(width: 8),
                      Text(
                        'Створити кімнату',
                        style: theme.textTheme.titleMedium,
                      ),
                    ],
                  ),
                  const SizedBox(height: 8),
                  Text(
                    'Створіть кімнату та запросіть друзів для спільного перегляду',
                    style: theme.textTheme.bodyMedium?.copyWith(
                      color: Colors.grey,
                    ),
                  ),
                  if (widget.mediaTitle != null) ...[
                    const SizedBox(height: 8),
                    Container(
                      padding: const EdgeInsets.all(8),
                      decoration: BoxDecoration(
                        color: theme.colorScheme.primary.withValues(alpha: 0.1),
                        borderRadius: BorderRadius.circular(8),
                      ),
                      child: Row(
                        children: [
                          const Icon(Icons.movie, size: 20),
                          const SizedBox(width: 8),
                          Expanded(
                            child: Text(
                              widget.mediaTitle!,
                              style: theme.textTheme.bodySmall,
                              overflow: TextOverflow.ellipsis,
                            ),
                          ),
                        ],
                      ),
                    ),
                  ],
                  const SizedBox(height: 16),
                  FilledButton.icon(
                    onPressed: () async {
                      _service.setMyName(_nameController.text);
                      await _service.hostRoom(
                        mediaUrl: widget.mediaUrl,
                        mediaTitle: widget.mediaTitle,
                      );
                    },
                    icon: const Icon(Icons.add),
                    label: const Text('Створити кімнату'),
                  ),
                ],
              ),
            ),
          ),
          const SizedBox(height: 16),

          // Join option
          Card(
            child: Padding(
              padding: const EdgeInsets.all(16),
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Row(
                    children: [
                      Icon(Icons.group, color: theme.colorScheme.secondary),
                      const SizedBox(width: 8),
                      Text(
                        'Приєднатися до кімнати',
                        style: theme.textTheme.titleMedium,
                      ),
                    ],
                  ),
                  const SizedBox(height: 8),
                  Text(
                    'Введіть код кімнати для підключення',
                    style: theme.textTheme.bodyMedium?.copyWith(
                      color: Colors.grey,
                    ),
                  ),
                  const SizedBox(height: 16),
                  TextField(
                    controller: _roomCodeController,
                    decoration: const InputDecoration(
                      hintText: 'ABCD12',
                      labelText: 'Код кімнати',
                      prefixIcon: Icon(Icons.key),
                    ),
                    textCapitalization: TextCapitalization.characters,
                    maxLength: 6,
                  ),
                  const SizedBox(height: 16),
                  FilledButton.icon(
                    onPressed: () async {
                      final roomCode = _roomCodeController.text.trim();

                      if (roomCode.isEmpty || roomCode.length < 6) {
                        ScaffoldMessenger.of(context).showSnackBar(
                          const SnackBar(
                            content: Text('Введіть 6-символьний код кімнати'),
                          ),
                        );
                        return;
                      }

                      _service.setMyName(_nameController.text);
                      await _service.joinRoom(roomCode);
                    },
                    icon: const Icon(Icons.login),
                    label: const Text('Приєднатися'),
                  ),
                ],
              ),
            ),
          ),
        ],
      ),
    );
  }

  Widget _buildConnectedState(ThemeData theme, AppStrings s) {
    return LayoutBuilder(
      builder: (context, constraints) {
        final isMobile = constraints.maxWidth < 600;

        return Column(
          children: [
            // Room info bar
            _buildRoomHeader(theme, isMobile),

            // Main content
            Expanded(
              child: isMobile
                  ? DefaultTabController(
                      length: 2,
                      child: Column(
                        children: [
                          Container(
                            color: theme.colorScheme.surface,
                            child: const TabBar(
                              tabs: [
                                Tab(icon: Icon(Icons.people), text: 'Учасники'),
                                Tab(icon: Icon(Icons.chat), text: 'Чат'),
                              ],
                            ),
                          ),
                          Expanded(
                            child: TabBarView(
                              children: [
                                _buildParticipantsPanel(theme),
                                _buildChatPanel(theme),
                              ],
                            ),
                          ),
                        ],
                      ),
                    )
                  : Row(
                      children: [
                        // Participants panel
                        Container(
                          width: 250,
                          decoration: BoxDecoration(
                            border: Border(
                              right: BorderSide(color: theme.dividerColor),
                            ),
                          ),
                          child: _buildParticipantsPanel(theme),
                        ),

                        // Chat panel
                        Expanded(child: _buildChatPanel(theme)),
                      ],
                    ),
            ),
          ],
        );
      },
    );
  }

  Widget _buildRoomHeader(ThemeData theme, bool isMobile) {
    final statusBadge = Container(
      padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 4),
      decoration: BoxDecoration(
        color: _service.backendType == WatchPartyBackendType.server
            ? Colors.blue.withValues(alpha: 0.1)
            : Colors.green.withValues(alpha: 0.1),
        borderRadius: BorderRadius.circular(12),
        border: Border.all(
          color: _service.backendType == WatchPartyBackendType.server
              ? Colors.blue.withValues(alpha: 0.3)
              : Colors.green.withValues(alpha: 0.3),
        ),
      ),
      child: Row(
        mainAxisSize: MainAxisSize.min,
        children: [
          Icon(
            _service.backendType == WatchPartyBackendType.server
                ? Icons.cloud
                : Icons.hub,
            size: 14,
            color: _service.backendType == WatchPartyBackendType.server
                ? Colors.blue
                : Colors.green,
          ),
          const SizedBox(width: 4),
          Text(
            _service.backendType == WatchPartyBackendType.server
                ? 'Cloud'
                : 'P2P',
            style: theme.textTheme.labelSmall?.copyWith(
              color: _service.backendType == WatchPartyBackendType.server
                  ? Colors.blue
                  : Colors.green,
              fontWeight: FontWeight.bold,
            ),
          ),
        ],
      ),
    );

    final participantsBadge = Container(
      padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 4),
      decoration: BoxDecoration(
        color: theme.colorScheme.primary.withValues(alpha: 0.1),
        borderRadius: BorderRadius.circular(12),
      ),
      child: Row(
        mainAxisSize: MainAxisSize.min,
        children: [
          const Icon(Icons.people, size: 16),
          const SizedBox(width: 4),
          Text('${_service.participants.length}'),
        ],
      ),
    );

    final actionButtons = Row(
      mainAxisSize: MainAxisSize.min,
      children: [
        if (_service.isHost) ...[
          FilledButton.icon(
            onPressed: () {
              if (_service.room?.mediaUrl != null) {
                _enterPlayer();
              } else {
                ScaffoldMessenger.of(context).showSnackBar(
                  const SnackBar(content: Text('Спочатку оберіть медіа')),
                );
              }
            },
            icon: const Icon(Icons.play_arrow),
            label: const Text('Почати'),
          ),
          const SizedBox(width: 8),
          IconButton.filledTonal(
            icon: const Icon(Icons.copy, size: 20),
            tooltip: 'Копіювати код',
            onPressed: () {
              if (_service.room != null) {
                Clipboard.setData(ClipboardData(text: _service.room!.roomCode));
                ScaffoldMessenger.of(context).showSnackBar(
                  const SnackBar(
                    content: Text('Код скопійовано'),
                    duration: Duration(seconds: 1),
                  ),
                );
              }
            },
          ),
        ] else if (_service.room?.mediaUrl != null) ...[
          FilledButton.icon(
            onPressed: _enterPlayer,
            icon: const Icon(Icons.play_circle_outline),
            label: const Text('Приєднатися'),
          ),
        ],
      ],
    );

    if (isMobile) {
      return Container(
        width: double.infinity,
        padding: const EdgeInsets.all(12),
        color: theme.colorScheme.surfaceContainerHighest,
        child: Column(
          children: [
            Row(
              children: [
                Icon(
                  _service.isHost ? Icons.tv : Icons.group,
                  color: theme.colorScheme.primary,
                ),
                const SizedBox(width: 8),
                Expanded(
                  child: Column(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      Text(
                        _service.isHost ? 'Ви хост' : 'Підключено',
                        style: theme.textTheme.titleSmall,
                      ),
                      if (_service.room?.roomCode != null && _service.isHost)
                        Text(
                          'Код: ${_service.room!.roomCode}',
                          style: theme.textTheme.bodySmall?.copyWith(
                            color: Colors.grey,
                          ),
                        ),
                    ],
                  ),
                ),
                participantsBadge,
                const SizedBox(width: 8),
                statusBadge,
              ],
            ),
            const SizedBox(height: 12),
            SizedBox(width: double.infinity, child: actionButtons),
          ],
        ),
      );
    }

    // Desktop Layout
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 12),
      color: theme.colorScheme.surfaceContainerHighest,
      child: Row(
        children: [
          Icon(
            _service.isHost ? Icons.tv : Icons.group,
            size: 20,
            color: theme.colorScheme.primary,
          ),
          const SizedBox(width: 8),
          Expanded(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Text(
                  _service.isHost ? 'Ви хост' : 'Підключено до хоста',
                  style: theme.textTheme.titleSmall,
                ),
                if (_service.room?.roomCode != null && _service.isHost)
                  Text(
                    'Код кімнати: ${_service.room!.roomCode}',
                    style: theme.textTheme.bodySmall?.copyWith(
                      color: Colors.grey,
                    ),
                  ),
              ],
            ),
          ),
          actionButtons,
          const SizedBox(width: 16),
          participantsBadge,
          const SizedBox(width: 8),
          statusBadge,
        ],
      ),
    );
  }

  Widget _buildParticipantsPanel(ThemeData theme) {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Padding(
          padding: const EdgeInsets.all(12),
          child: Text(
            'Учасники',
            style: theme.textTheme.titleSmall?.copyWith(color: Colors.grey),
          ),
        ),
        Expanded(
          child: ListView.builder(
            padding: const EdgeInsets.symmetric(horizontal: 8),
            itemCount: _service.participants.length,
            itemBuilder: (context, index) {
              final participant = _service.participants[index];
              return ListTile(
                dense: true,
                leading: CircleAvatar(
                  radius: 16,
                  backgroundColor: participant.isHost
                      ? theme.colorScheme.primary
                      : theme.colorScheme.secondary,
                  child: Text(
                    participant.name.isNotEmpty
                        ? participant.name[0].toUpperCase()
                        : '?',
                    style: const TextStyle(color: Colors.white, fontSize: 12),
                  ),
                ),
                title: Text(participant.name, overflow: TextOverflow.ellipsis),
                subtitle: participant.isHost
                    ? Text(
                        'Хост',
                        style: TextStyle(
                          color: theme.colorScheme.primary,
                          fontSize: 11,
                        ),
                      )
                    : null,
                trailing: participant.id == _service.myId
                    ? const Icon(Icons.person, size: 16)
                    : null,
              );
            },
          ),
        ),
      ],
    );
  }

  Widget _buildChatPanel(ThemeData theme) {
    return Column(
      children: [
        // Media info if available
        if (_service.room?.mediaTitle != null)
          Container(
            width: double.infinity,
            padding: const EdgeInsets.all(12),
            color: theme.colorScheme.primary.withValues(alpha: 0.1),
            child: Row(
              children: [
                const Icon(Icons.movie, size: 20),
                const SizedBox(width: 8),
                Expanded(
                  child: Text(
                    _service.room!.mediaTitle!,
                    style: theme.textTheme.titleSmall,
                    overflow: TextOverflow.ellipsis,
                  ),
                ),
              ],
            ),
          ),

        // Chat header
        Padding(
          padding: const EdgeInsets.all(12),
          child: Row(
            children: [
              const Icon(Icons.chat, size: 20, color: Colors.grey),
              const SizedBox(width: 8),
              Text(
                'Чат',
                style: theme.textTheme.titleSmall?.copyWith(color: Colors.grey),
              ),
            ],
          ),
        ),

        // Chat messages
        Expanded(
          child: _service.chatMessages.isEmpty
              ? Center(
                  child: Column(
                    mainAxisSize: MainAxisSize.min,
                    children: [
                      Icon(
                        Icons.chat_bubble_outline,
                        size: 48,
                        color: Colors.grey.withValues(alpha: 0.5),
                      ),
                      const SizedBox(height: 8),
                      Text(
                        'Напишіть перше повідомлення!',
                        style: TextStyle(
                          color: Colors.grey.withValues(alpha: 0.7),
                        ),
                      ),
                    ],
                  ),
                )
              : ListView.builder(
                  controller: _chatScrollController,
                  padding: const EdgeInsets.symmetric(horizontal: 12),
                  itemCount: _service.chatMessages.length,
                  itemBuilder: (context, index) {
                    final message = _service.chatMessages[index];
                    final isMe = message.senderId == _service.myId;

                    return Align(
                      alignment: isMe
                          ? Alignment.centerRight
                          : Alignment.centerLeft,
                      child: Container(
                        margin: const EdgeInsets.only(bottom: 8),
                        padding: const EdgeInsets.symmetric(
                          horizontal: 12,
                          vertical: 8,
                        ),
                        constraints: BoxConstraints(
                          maxWidth: MediaQuery.of(context).size.width * 0.6,
                        ),
                        decoration: BoxDecoration(
                          color: isMe
                              ? theme.colorScheme.primary
                              : theme.colorScheme.surfaceContainerHighest,
                          borderRadius: BorderRadius.circular(16),
                        ),
                        child: Column(
                          crossAxisAlignment: CrossAxisAlignment.start,
                          children: [
                            if (!isMe)
                              Text(
                                message.senderName,
                                style: theme.textTheme.labelSmall?.copyWith(
                                  color: theme.colorScheme.primary,
                                  fontWeight: FontWeight.bold,
                                ),
                              ),
                            Text(
                              message.message,
                              style: TextStyle(
                                color: isMe ? Colors.white : null,
                              ),
                            ),
                          ],
                        ),
                      ),
                    );
                  },
                ),
        ),

        // Chat input
        Container(
          padding: const EdgeInsets.all(12),
          decoration: BoxDecoration(
            color: theme.colorScheme.surface,
            border: Border(top: BorderSide(color: theme.dividerColor)),
          ),
          child: Row(
            children: [
              Expanded(
                child: TextField(
                  controller: _chatController,
                  decoration: const InputDecoration(
                    hintText: 'Написати повідомлення...',
                    border: OutlineInputBorder(),
                    contentPadding: EdgeInsets.symmetric(
                      horizontal: 16,
                      vertical: 8,
                    ),
                  ),
                  onSubmitted: _sendMessage,
                ),
              ),
              const SizedBox(width: 8),
              IconButton.filled(
                key: const Key('chat_send_button'),
                onPressed: () => _sendMessage(_chatController.text),
                icon: const Icon(Icons.send),
              ),
            ],
          ),
        ),
      ],
    );
  }

  Widget _buildErrorState(ThemeData theme, AppStrings s) {
    return Center(
      child: Padding(
        padding: const EdgeInsets.all(32),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            Icon(Icons.error_outline, size: 64, color: theme.colorScheme.error),
            const SizedBox(height: 16),
            Text(s.connectionError, style: theme.textTheme.titleLarge),
            const SizedBox(height: 8),
            Text(
              _service.error ?? s.unknownError,
              textAlign: TextAlign.center,
              style: theme.textTheme.bodyMedium?.copyWith(color: Colors.grey),
            ),
            const SizedBox(height: 24),
            FilledButton.icon(
              onPressed: () async {
                await _service.leaveRoom();
              },
              icon: const Icon(Icons.refresh),
              label: Text(s.retry),
            ),
          ],
        ),
      ),
    );
  }

  void _sendMessage(String text) {
    if (text.trim().isEmpty) return;

    _service.sendChatMessage(text.trim());
    _chatController.clear();
  }
}
