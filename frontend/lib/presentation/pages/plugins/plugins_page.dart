import 'package:flutter/material.dart';
import 'package:get_it/get_it.dart';
import 'package:go_router/go_router.dart';

import '../../theme/app_theme.dart';
import '../../../data/models/provider_catalog.dart';
import '../../../data/services/catalog_client.dart';
import '../../../data/providers/provider_registry.dart';
import '../../../data/providers/server_backed_provider.dart';
import '../../../data/services/settings_service.dart';
import '../../widgets/custom_titlebar.dart';

/// Page for managing content sources / plugins (Lampa / Kodi-like external provider URLs)
class PluginsPage extends StatefulWidget {
  const PluginsPage({super.key});

  @override
  State<PluginsPage> createState() => _PluginsPageState();
}

class _PluginsPageState extends State<PluginsPage> {
  final _registry = GetIt.instance<ProviderRegistry>();
  final _settings = GetIt.instance<SettingsService>();
  final _urlController = TextEditingController();
  bool _isConnecting = false;
  String? _connectError;

  @override
  void dispose() {
    _urlController.dispose();
    super.dispose();
  }

  Future<void> _addSource() async {
    final url = _urlController.text.trim();
    if (url.isEmpty) return;

    setState(() {
      _isConnecting = true;
      _connectError = null;
    });

    CatalogClient? probe;
    try {
      // A catalog may legitimately be on loopback or a private address -- that
      // is what a self-hosted server on the user's own machine looks like. The
      // peer-supplied rules in stream_url.dart are stricter on purpose and must
      // not be reused here.
      final uri = Uri.tryParse(url);
      if (uri == null || (!uri.isScheme('http') && !uri.isScheme('https'))) {
        throw const CatalogException(
          'Введіть коректну URL адресу (http або https)',
        );
      }

      // Actually contact the server before registering anything. The previous
      // version checked the scheme, registered, and reported success without a
      // single request -- so a source that did not exist looked added.
      probe = CatalogClient(baseUrl: url);
      final status = await probe.handshake();

      // Identified by what the server calls itself, not by its host. Deriving
      // the id from the host collided for two catalogues on one host, and made
      // every stored item lose its scope the moment the host changed.
      final sourceId = status.catalogId;
      if (_registry.getById(sourceId) != null) {
        throw const CatalogException('Цей сервер каталогу вже підключено');
      }

      final entry = ProviderCatalogEntry(
        id: sourceId,
        name: status.app.isEmpty ? uri.host : status.app,
        baseUrl: url,
        showOnHome: true,
        hasFixedStreams: false,
        contentTypes: ['movie', 'series', 'cartoon', 'anime'],
        searchEnabledDefault: true,
        enabled: true,
        healthy: true,
      );

      _registry.register(ServerBackedProvider(entry));
      await _settings.setProviderEnabled(sourceId, true);

      _urlController.clear();
      if (!mounted) return;

      final warning = status.compatibilityWarning;
      ScaffoldMessenger.of(context).showSnackBar(
        SnackBar(
          content: Text(warning ?? 'Каталог «${entry.name}» підключено'),
          backgroundColor: warning == null ? Colors.green : Colors.orange,
          duration: Duration(seconds: warning == null ? 2 : 5),
        ),
      );
    } on CatalogException catch (e) {
      if (mounted) setState(() => _connectError = e.message);
    } catch (e) {
      if (mounted) setState(() => _connectError = 'Помилка: $e');
    } finally {
      // The client is kept alive by the registered provider; this probe copy is
      // only for the handshake.
      probe?.dispose();
      if (mounted) {
        setState(() {
          _isConnecting = false;
        });
      }
    }
  }

  void _removeSource(String id) {
    setState(() {
      _registry.unregister(id);
      _settings.setProviderEnabled(id, false);
    });
  }

  @override
  Widget build(BuildContext context) {
    final providers = _registry.all;

    return Scaffold(
      body: Column(
        children: [
          const CustomTitleBar(),
          AppBar(
            title: const Text('Джерела та плагіни'),
            leading: IconButton(
              icon: const Icon(Icons.arrow_back),
              onPressed: () => context.go('/'),
            ),
          ),
          Expanded(
            child: ListView(
              padding: const EdgeInsets.symmetric(horizontal: 20, vertical: 16),
              children: [
                _buildAddSourceCard(),
                const SizedBox(height: 24),
                Text(
                  'Підключені джерела (${providers.length})',
                  style: Theme.of(context).textTheme.titleMedium?.copyWith(
                    fontWeight: FontWeight.bold,
                  ),
                ),
                const SizedBox(height: 12),
                if (providers.isEmpty)
                  _buildEmptyState()
                else
                  ...providers.map((p) => _buildProviderCard(p)),
              ],
            ),
          ),
        ],
      ),
    );
  }

  Widget _buildAddSourceCard() {
    return Card(
      shape: RoundedRectangleBorder(
        borderRadius: BorderRadius.circular(16),
        side: BorderSide(color: AppTheme.darkBorder),
      ),
      child: Padding(
        padding: const EdgeInsets.all(20),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Row(
              children: [
                Container(
                  padding: const EdgeInsets.all(10),
                  decoration: BoxDecoration(
                    color: AppTheme.primaryColor.withValues(alpha: 0.15),
                    borderRadius: BorderRadius.circular(10),
                  ),
                  child: const Icon(
                    Icons.add_link,
                    color: AppTheme.primaryColor,
                  ),
                ),
                const SizedBox(width: 14),
                Expanded(
                  child: Column(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      const Text(
                        'Підключити стороннє джерело',
                        style: TextStyle(
                          fontSize: 16,
                          fontWeight: FontWeight.bold,
                        ),
                      ),
                      const SizedBox(height: 2),
                      Text(
                        'Введіть URL вашого сервера каталогу або сумісного плагіна (наприклад, Lampa-сумісний або Kadrbox backend)',
                        style: TextStyle(
                          fontSize: 12,
                          color: Theme.of(context).textTheme.bodySmall?.color,
                        ),
                      ),
                    ],
                  ),
                ),
              ],
            ),
            const SizedBox(height: 16),
            TextField(
              controller: _urlController,
              decoration: InputDecoration(
                hintText: 'https://myserver.example.com/api',
                prefixIcon: const Icon(Icons.language),
                errorText: _connectError,
                border: OutlineInputBorder(
                  borderRadius: BorderRadius.circular(12),
                ),
              ),
              onSubmitted: (_) => _addSource(),
            ),
            const SizedBox(height: 14),
            Align(
              alignment: Alignment.centerRight,
              child: FilledButton.icon(
                onPressed: _isConnecting ? null : _addSource,
                icon: _isConnecting
                    ? const SizedBox(
                        width: 16,
                        height: 16,
                        child: CircularProgressIndicator(
                          strokeWidth: 2,
                          color: Colors.white,
                        ),
                      )
                    : const Icon(Icons.add),
                label: const Text('Підключити'),
              ),
            ),
          ],
        ),
      ),
    );
  }

  Widget _buildEmptyState() {
    return Container(
      padding: const EdgeInsets.symmetric(vertical: 40, horizontal: 20),
      decoration: BoxDecoration(
        color: AppTheme.darkCard,
        borderRadius: BorderRadius.circular(16),
        border: Border.all(color: AppTheme.darkBorder),
      ),
      child: Column(
        mainAxisAlignment: MainAxisAlignment.center,
        children: [
          const Icon(
            Icons.extension_off_outlined,
            size: 56,
            color: Colors.grey,
          ),
          const SizedBox(height: 16),
          const Text(
            'Немає підключених джерел контенту',
            style: TextStyle(fontSize: 16, fontWeight: FontWeight.w600),
          ),
          const SizedBox(height: 8),
          Text(
            'Kadrbox працює як чистий медіаплеєр. Щоб дивитися онлайн-каталоги, додайте URL-адресу власного сервера або плагіна вище.',
            textAlign: TextAlign.center,
            style: TextStyle(
              fontSize: 13,
              color: Theme.of(context).textTheme.bodySmall?.color,
            ),
          ),
        ],
      ),
    );
  }

  Widget _buildProviderCard(dynamic provider) {
    final isEnabled = _settings.isProviderEnabled(provider.id);

    return Card(
      margin: const EdgeInsets.only(bottom: 10),
      shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(12)),
      child: ListTile(
        leading: CircleAvatar(
          backgroundColor: AppTheme.primaryColor.withValues(alpha: 0.15),
          child: const Icon(Icons.dns, color: AppTheme.primaryColor),
        ),
        title: Text(
          provider.name,
          style: const TextStyle(fontWeight: FontWeight.w600),
        ),
        subtitle: Text(provider.baseUrl, style: const TextStyle(fontSize: 12)),
        trailing: Row(
          mainAxisSize: MainAxisSize.min,
          children: [
            Switch(
              value: isEnabled,
              onChanged: (val) {
                setState(() {
                  _settings.setProviderEnabled(provider.id, val);
                });
              },
            ),
            IconButton(
              icon: const Icon(Icons.delete_outline, color: Colors.redAccent),
              tooltip: 'Видалити джерело',
              onPressed: () => _removeSource(provider.id),
            ),
          ],
        ),
      ),
    );
  }
}
