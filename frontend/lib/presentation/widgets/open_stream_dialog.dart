import 'package:flutter/material.dart';

import '../../core/utils/stream_url.dart';

/// What the user asked the network-stream dialog to do with the URL.
enum StreamDialogAction {
  /// Play it locally.
  play,

  /// Seed a watch-party room with it.
  watchParty,
}

/// The "open a network stream" dialog.
///
/// It is a [StatefulWidget] that owns its own [TextEditingController] on
/// purpose. The first version built the controller in the caller and disposed
/// it as soon as `showDialog` returned. That is too early: `Navigator.pop`
/// completes the dialog's future while the route is still animating out, so the
/// [TextField] was still listening to an already-disposed controller. Flutter
/// then threw "A TextEditingController was used after being disposed" from
/// inside the exit transition's `didUpdateWidget`, which tore the element tree
/// apart -- `_dependents.isEmpty` on an [InheritedElement], then "Tried to
/// build dirty widget in the wrong build scope" naming a `FilledButton`, and a
/// red debug screen over a stream that was in fact playing.
///
/// Disposing in this widget's `dispose()` ties the lifetime of the controller
/// to the lifetime of the element that actually listens to it, so the race
/// cannot happen.
///
/// [validateMediaUrl] rejects non-HTTP schemes, embedded credentials and
/// loopback/private hosts, because a watch-party URL is peer-supplied and gets
/// handed to other people's players.
class OpenStreamDialog extends StatefulWidget {
  const OpenStreamDialog({super.key});

  @override
  State<OpenStreamDialog> createState() => _OpenStreamDialogState();
}

class _OpenStreamDialogState extends State<OpenStreamDialog> {
  final _controller = TextEditingController();
  String? _error;

  @override
  void dispose() {
    _controller.dispose();
    super.dispose();
  }

  void _submit(StreamDialogAction action) {
    final problem = validateMediaUrl(_controller.text);
    if (problem != null) {
      setState(() => _error = problem);
      return;
    }
    Navigator.pop(
      context,
      (action: action, url: _controller.text.trim()),
    );
  }

  @override
  Widget build(BuildContext context) {
    return AlertDialog(
      title: const Text('Відкрити мережевий потік'),
      content: TextField(
        controller: _controller,
        decoration: InputDecoration(
          hintText: 'https://example.com/stream.m3u8',
          labelText: 'URL відео або HLS потоку',
          prefixIcon: const Icon(Icons.link),
          errorText: _error,
        ),
        autofocus: true,
        onChanged: (_) {
          // Clear the complaint as soon as the user starts fixing it.
          if (_error != null) setState(() => _error = null);
        },
        onSubmitted: (_) => _submit(StreamDialogAction.play),
      ),
      actions: [
        TextButton(
          onPressed: () => Navigator.pop(context),
          child: const Text('Скасувати'),
        ),
        OutlinedButton.icon(
          onPressed: () => _submit(StreamDialogAction.watchParty),
          icon: const Icon(Icons.groups, size: 20),
          label: const Text('Спільний перегляд'),
        ),
        FilledButton(
          onPressed: () => _submit(StreamDialogAction.play),
          child: const Text('Відтворити'),
        ),
      ],
    );
  }
}