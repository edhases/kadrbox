import 'package:flutter/material.dart';
import 'package:flutter/foundation.dart';
import 'package:get_it/get_it.dart';
import 'package:go_router/go_router.dart';
import 'package:url_launcher/url_launcher.dart';
import 'package:flutter_cache_manager/flutter_cache_manager.dart';
import 'package:file_picker/file_picker.dart';

import '../../../core/l10n/app_strings.dart';
import '../../../data/providers/provider_registry.dart';
import '../../../data/services/settings_service.dart';
import '../../../data/services/provider_catalog_service.dart';
import '../../../core/services/version_service.dart';
import '../../../data/services/auth_service.dart';
import '../../../data/services/history_service.dart';
import '../../../data/services/favorites_service.dart';
import '../../../data/services/data_transfer_service.dart';
import '../../../data/services/update_service.dart';
import '../../theme/app_theme.dart';
import '../../widgets/custom_titlebar.dart';
import '../../widgets/settings_widgets.dart';
import 'dialogs/settings_dialogs.dart';
import 'dialogs/update_dialog.dart';

/// Settings page with full functionality and localization
class SettingsPage extends StatefulWidget {
  const SettingsPage({super.key});

  @override
  State<SettingsPage> createState() => _SettingsPageState();
}

class _SettingsPageState extends State<SettingsPage> {
  final _registry = GetIt.instance<ProviderRegistry>();
  final _settings = GetIt.instance<SettingsService>();
  final _authService = GetIt.instance<AuthService>();
  final _dataTransferService = GetIt.instance<DataTransferService>();
  final _historyService = GetIt.instance<HistoryService>();
  final _favoritesService = GetIt.instance<FavoritesService>();
  final _updateService = GetIt.instance<UpdateService>();

  AppStrings get _s => AppStrings.of(context);

  @override
  void initState() {
    super.initState();
    _settings.addListener(_onSettingsChanged);
    try {
      GetIt.instance<ProviderCatalogService>().addListener(_onSettingsChanged);
    } catch (_) {}
  }

  @override
  void dispose() {
    _settings.removeListener(_onSettingsChanged);
    try {
      GetIt.instance<ProviderCatalogService>().removeListener(_onSettingsChanged);
    } catch (_) {}
    super.dispose();
  }

  void _onSettingsChanged() {
    if (mounted) setState(() {});
  }

  Future<void> _syncNow() async {
    ScaffoldMessenger.of(
      context,
    ).showSnackBar(const SnackBar(content: Text('Синхронізація...')));

    await _historyService.syncNow();
    await _favoritesService.syncNow();

    if (mounted) {
      ScaffoldMessenger.of(context).hideCurrentSnackBar();
      ScaffoldMessenger.of(
        context,
      ).showSnackBar(const SnackBar(content: Text('Синхронізацію завершено')));
    }
  }

  // ... (skipping unchanged parts)
  void _showSyncInfo() {
    showDialog(
      context: context,
      builder: (context) => AlertDialog(
        backgroundColor: AppTheme.darkCard,
        title: const Text('Синхронізація'),
        content: const Column(
          mainAxisSize: MainAxisSize.min,
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Text('Ваші дані автоматично синхронізуються:'),
            SizedBox(height: 12),
            Row(
              children: [
                Icon(Icons.check_circle, color: Colors.green, size: 20),
                SizedBox(width: 8),
                Text('Історія переглядів'),
              ],
            ),
            SizedBox(height: 8),
            Row(
              children: [
                Icon(Icons.check_circle, color: Colors.green, size: 20),
                SizedBox(width: 8),
                Text('Обране'),
              ],
            ),
            SizedBox(height: 8),
            Row(
              children: [
                Icon(Icons.check_circle, color: Colors.green, size: 20),
                SizedBox(width: 8),
                Text('Позиція перегляду'),
              ],
            ),
          ],
        ),
        actions: [
          TextButton(
            onPressed: () => Navigator.pop(context),
            child: const Text('Зрозуміло'),
          ),
          if (_authService.isAuthenticated)
            ElevatedButton(
              onPressed: () {
                Navigator.pop(context);
                _syncNow();
              },
              style: ElevatedButton.styleFrom(
                backgroundColor: AppTheme.primaryColor,
              ),
              child: const Text('Синхронізувати зараз'),
            ),
        ],
      ),
    );
  }

  bool get _isDesktop =>
      !kIsWeb &&
      (defaultTargetPlatform == TargetPlatform.windows ||
          defaultTargetPlatform == TargetPlatform.linux ||
          defaultTargetPlatform == TargetPlatform.macOS);

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      body: Column(
        children: [
          if (_isDesktop) const CustomTitleBar(),
          _buildAppBar(),
          Expanded(
            child: ListView(
              padding: const EdgeInsets.symmetric(horizontal: 8),
              children: [
                const SizedBox(height: 8),

                // Account section
                _buildAccountSection(),

                // Providers section
                SettingsSection(
                  title: _s.contentSources,
                  icon: Icons.source,
                  children: _buildProvidersList(),
                ),

                // Playback section
                SettingsSection(
                  title: _s.playback,
                  icon: Icons.play_circle_outline,
                  children: [
                    SettingsTile(
                      icon: Icons.smart_display,
                      title: _s.player,
                      value: _settings.state.playerType.displayName,
                      onTap: () => SettingsDialogs.showPlayerTypeDialog(
                        context,
                        _settings,
                        _s,
                      ),
                    ),
                    SettingsTile(
                      icon: Icons.high_quality,
                      title: _s.defaultQuality,
                      value: _settings.state.defaultQuality.displayName,
                      onTap: () => SettingsDialogs.showQualityDialog(
                        context,
                        _settings,
                        _s,
                      ),
                    ),
                    SettingsTile(
                      icon: Icons.speed,
                      title: _s.defaultSpeed,
                      value: '${_settings.state.defaultSpeed}x',
                      onTap: () => SettingsDialogs.showSpeedDialog(
                        context,
                        _settings,
                        _s,
                      ),
                    ),
                    SettingsSwitch(
                      icon: Icons.skip_next,
                      title: _s.autoPlayNext,
                      subtitle: _s.autoPlayNextDesc,
                      value: _settings.state.autoPlayNext,
                      onChanged: (value) => _settings.setAutoPlayNext(value),
                    ),
                    SettingsSwitch(
                      icon: Icons.bookmark,
                      title: _s.rememberPosition,
                      subtitle: _s.rememberPositionDesc,
                      value: _settings.state.rememberPosition,
                      onChanged: (value) =>
                          _settings.setRememberPosition(value),
                    ),
                    SettingsSwitch(
                      icon: Icons.swipe,
                      title: _s.gestureControls,
                      subtitle: _s.gestureControlsDesc,
                      value: _settings.state.gestureControls,
                      onChanged: (value) => _settings.setGestureControls(value),
                    ),
                    SettingsSwitch(
                      icon: Icons.fast_forward,
                      title: _s.skipIntro,
                      subtitle: _s.skipIntroDesc,
                      value: _settings.state.skipIntro,
                      onChanged: (value) => _settings.setSkipIntro(value),
                    ),
                    SettingsTile(
                      icon: Icons.timer,
                      title: _s.nextEpisodeDelay,
                      value:
                          '${_settings.state.nextEpisodeDelay} ${_s.nextEpisodeDelayDesc}',
                      onTap: () => SettingsDialogs.showNextEpisodeDelayDialog(
                        context,
                        _settings,
                        _s,
                      ),
                    ),
                  ],
                ),

                // Watch Party section
                SettingsSection(
                  title: _s.watchParty,
                  icon: Icons.groups,
                  children: [
                    SettingsTile(
                      icon: Icons.person,
                      title: _s.watchPartyName,
                      value: _settings.state.watchPartyName,
                      onTap: () => _showWatchPartyNameDialog(),
                    ),
                  ],
                ),

                // Appearance section
                SettingsSection(
                  title: _s.appearance,
                  icon: Icons.palette,
                  children: [
                    SettingsTile(
                      icon: Icons.language,
                      title: _s.language,
                      value: _settings.state.locale == AppLocale.uk
                          ? 'Українська'
                          : 'English',
                      onTap: () => SettingsDialogs.showLanguageDialog(
                        context,
                        _settings,
                        _s,
                      ),
                    ),
                    SettingsTile(
                      icon: Icons.dark_mode,
                      title: _s.theme,
                      value: _settings.state.theme.displayName,
                      onTap: () => SettingsDialogs.showThemeDialog(
                        context,
                        _settings,
                        _s,
                      ),
                    ),
                    SettingsTile(
                      icon: Icons.tune,
                      title: _s.customization,
                      value: _s.customizationDesc,
                      onTap: () => context.push('/appearance'),
                    ),
                  ],
                ),

                // Data section
                SettingsSection(
                  title: _s.data,
                  icon: Icons.storage,
                  children: [
                    SettingsTile(
                      icon: Icons.history,
                      title: _s.history,
                      value: _s.historyDesc,
                      onTap: () => context.push('/history'),
                    ),
                    SettingsTile(
                      icon: Icons.favorite,
                      title: _s.favorites,
                      value: _s.favoritesDesc,
                      onTap: () => context.push('/favorites'),
                    ),
                    SettingsTile(
                      icon: Icons.bar_chart,
                      title: _s.statistics,
                      value: _s.statsDesc,
                      onTap: () => context.push('/stats'),
                    ),
                    SettingsTile(
                      icon: Icons.download_for_offline,
                      title: _s.downloads,
                      value: _s.offlineContent,
                      onTap: () => context.push('/downloads'),
                    ),
                    SettingsTile(
                      icon: Icons.folder_open,
                      title: _s.downloadFolder,
                      value: _settings.state.downloadPath.isEmpty
                          ? _s.defaultFolder
                          : _settings.state.downloadPath,
                      onTap: () => _pickDownloadFolder(),
                    ),
                    SettingsSwitch(
                      icon: Icons.wifi,
                      title: _s.onlyWifiDownload,
                      subtitle: _s.onlyWifiDownloadDesc,
                      value: _settings.state.onlyWifiDownload,
                      onChanged: (value) =>
                          _settings.setOnlyWifiDownload(value),
                    ),
                    SettingsTile(
                      icon: Icons.delete_sweep,
                      title: _s.clearCache,
                      value: _s.clearCacheDesc,
                      onTap: () => _showClearCacheDialog(),
                      destructive: true,
                    ),
                  ],
                ),

                // Sync section
                SettingsSection(
                  title: _s.sync,
                  icon: Icons.sync,
                  children: [
                    SettingsTile(
                      icon: Icons.upload,
                      title: _s.export,
                      value: _s.exportDesc,
                      onTap: () => _showExportDialog(),
                    ),
                    SettingsTile(
                      icon: Icons.download,
                      title: _s.import,
                      value: _s.importDesc,
                      onTap: () => _importData(),
                    ),
                    SettingsTile(
                      icon: Icons.share,
                      title: _s.share,
                      value: _s.shareDesc,
                      onTap: () => _shareData(),
                    ),
                  ],
                ),

                // Notifications section
                SettingsSection(
                  title: _s.notifications,
                  icon: Icons.notifications_outlined,
                  children: [
                    SettingsSwitch(
                      icon: Icons.new_releases,
                      title: _s.newEpisodeNotify,
                      subtitle: _s.newEpisodeNotifyDesc,
                      value: _settings.state.newEpisodeNotify,
                      onChanged: (value) =>
                          _settings.setNewEpisodeNotify(value),
                    ),
                    SettingsSwitch(
                      icon: Icons.system_update,
                      title: _s.updateNotify,
                      subtitle: _s.updateNotifyDesc,
                      value: _settings.state.updateNotify,
                      onChanged: (value) => _settings.setUpdateNotify(value),
                    ),
                  ],
                ),

                // Advanced section
                SettingsSection(
                  title: _s.advanced,
                  icon: Icons.settings_applications,
                  children: [
                    SettingsTile(
                      icon: Icons.devices,
                      title: _s.deviceTypeOverride,
                      value: _settings.state.deviceTypeOverride == 'auto'
                          ? _s.system
                          : _getDeviceTypeName(
                              _settings.state.deviceTypeOverride,
                            ),
                      onTap: () => SettingsDialogs.showDeviceTypeDialog(
                        context,
                        _settings,
                        _s,
                      ),
                    ),
                    SettingsSwitch(
                      icon: Icons.bug_report,
                      title: _s.debugMode,
                      subtitle: _s.debugModeDesc,
                      value: _settings.state.debugMode,
                      onChanged: (value) => _settings.setDebugMode(value),
                    ),
                    if (_isDesktop)
                      SettingsTile(
                        icon: Icons.fullscreen,
                        title: _s.fullscreenDelay,
                        value:
                            '${_settings.state.fullscreenTransitionDelay} ${_s.milliseconds}',
                        onTap: () => SettingsDialogs.showFullscreenDelayDialog(
                          context,
                          _settings,
                          _s,
                        ),
                      ),
                    SettingsTile(
                      icon: Icons.restore,
                      title: _s.resetSettings,
                      value: _s.resetSettingsDesc,
                      onTap: () => _showResetSettingsDialog(),
                      destructive: true,
                    ),
                  ],
                ),

                // About section
                SettingsSection(
                  title: _s.about,
                  icon: Icons.info_outline,
                  children: [
                    SettingsTile(
                      icon: Icons.movie_filter,
                      title: _s.appName,
                      value: '${_s.version} ${VersionService.versionName}',
                      onTap: () => _showAboutDialog(),
                    ),
                    SettingsTile(
                      icon: Icons.code,
                      title: 'GitHub',
                      value: _s.sourceCodeDesc,
                      onTap: () => _openGitHub(),
                    ),
                    SettingsTile(
                      icon: Icons.update,
                      title: _s.checkForUpdates,
                      value: VersionService.versionName,
                      onTap: () => UpdateDialog.show(context, _updateService),
                    ),
                  ],
                ),

                // Donation section
                _buildDonationSection(),

                const SizedBox(height: 32),
              ],
            ),
          ),
        ],
      ),
    );
  }

  Widget _buildDonationSection() {
    return SettingsSection(
      title: _s.supportProject,
      icon: Icons.favorite_border,
      children: [
        SettingsTile(
          icon: Icons.volunteer_activism,
          title: _s.donateButton,
          value: _s.donateDesc,
          onTap: () => _showDonationDialog(),
        ),
      ],
    );
  }

  void _showDonationDialog() {
    showDialog(
      context: context,
      builder: (context) => AlertDialog(
        backgroundColor: AppTheme.darkCard,
        title: Text(_s.supportProject),
        content: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            Container(
              padding: const EdgeInsets.all(16),
              decoration: BoxDecoration(
                color: Colors.white,
                borderRadius: BorderRadius.circular(12),
              ),
              child: Image.asset(
                'assets/images/qrcode.png',
                width: 200,
                height: 200,
              ),
            ),
            const SizedBox(height: 16),
            Text(
              _s.scanQrCode,
              textAlign: TextAlign.center,
              style: const TextStyle(fontWeight: FontWeight.bold),
            ),
            const SizedBox(height: 8),
            Text(
              _s.donateDesc,
              textAlign: TextAlign.center,
              style: TextStyle(color: AppTheme.textSecondary, fontSize: 13),
            ),
          ],
        ),
        actions: [
          TextButton(onPressed: () => context.pop(), child: Text(_s.close)),
          ElevatedButton.icon(
            onPressed: () {
              launchUrl(
                Uri.parse(
                  'https://www.paypal.com/donate/?hosted_button_id=MUGPMK7UPCYUW',
                ),
                mode: LaunchMode.externalApplication,
              );
              context.pop();
            },
            icon: const Icon(Icons.open_in_new),
            label: Text(_s.donateButton),
          ),
        ],
      ),
    );
  }

  Widget _buildAppBar() {
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 4),
      decoration: BoxDecoration(
        color: Theme.of(context).colorScheme.surface,
        border: Border(
          bottom: BorderSide(color: AppTheme.borderColor, width: 1),
        ),
      ),
      child: Row(
        children: [
          IconButton(
            icon: const Icon(Icons.arrow_back),
            onPressed: () {
              if (context.canPop()) {
                context.pop();
              } else {
                context.go('/');
              }
            },
            tooltip: _s.back,
          ),
          const SizedBox(width: 8),
          Text(
            _s.settings,
            style: const TextStyle(fontSize: 20, fontWeight: FontWeight.bold),
          ),
        ],
      ),
    );
  }

  Widget _buildAccountSection() {
    return ListenableBuilder(
      listenable: _authService,
      builder: (context, _) {
        if (_authService.isAuthenticated) {
          return SettingsSection(
            title: _s.account,
            icon: Icons.account_circle,
            children: [
              ListTile(
                leading: CircleAvatar(
                  radius: 20,
                  backgroundColor: Theme.of(
                    context,
                  ).colorScheme.primary.withValues(alpha: 0.2),
                  backgroundImage: _authService.avatarUrl != null
                      ? NetworkImage(_authService.avatarUrl!)
                      : null,
                  child: _authService.avatarUrl == null
                      ? Text(
                          _authService.displayName[0].toUpperCase(),
                          style: TextStyle(
                            color: Theme.of(context).colorScheme.primary,
                            fontWeight: FontWeight.bold,
                          ),
                        )
                      : null,
                ),
                title: Text(_authService.displayName),
                subtitle: Text(
                  _authService.userEmail ?? '',
                  style: TextStyle(color: AppTheme.textMuted, fontSize: 13),
                ),
                trailing: const Icon(
                  Icons.chevron_right,
                  color: AppTheme.textMuted,
                ),
                onTap: () => context.push('/profile'),
              ),
              SettingsTile(
                icon: Icons.sync,
                title: _s.sync,
                value: _s.syncCloud,
                onTap: () => _showSyncInfo(),
              ),
            ],
          );
        } else {
          return SettingsSection(
            title: _s.account,
            icon: Icons.account_circle,
            children: [
              Container(
                margin: const EdgeInsets.symmetric(horizontal: 8, vertical: 4),
                padding: const EdgeInsets.all(16),
                decoration: BoxDecoration(
                  color: AppTheme.darkCard,
                  borderRadius: BorderRadius.circular(12),
                ),
                child: Row(
                  children: [
                    CircleAvatar(
                      radius: 24,
                      backgroundColor: Theme.of(
                        context,
                      ).colorScheme.primary.withValues(alpha: 0.2),
                      child: Icon(
                        Icons.person,
                        size: 32,
                        color: Theme.of(context).colorScheme.primary,
                      ),
                    ),
                    const SizedBox(width: 16),
                    Expanded(
                      child: Column(
                        crossAxisAlignment: CrossAxisAlignment.start,
                        children: [
                          Text(
                            _s.signInPrompt,
                            style: const TextStyle(
                              fontWeight: FontWeight.bold,
                              fontSize: 16,
                            ),
                          ),
                          Text(
                            _s.signInDescription,
                            style: TextStyle(
                              color: Colors.white.withValues(alpha: 0.6),
                              fontSize: 12,
                            ),
                          ),
                        ],
                      ),
                    ),
                    ElevatedButton(
                      onPressed: () => context.push('/login'),
                      style: ElevatedButton.styleFrom(
                        backgroundColor: Theme.of(context).colorScheme.primary,
                      ),
                      child: Text(_s.signIn),
                    ),
                  ],
                ),
              ),
            ],
          );
        }
      },
    );
  }

  List<Widget> _buildProvidersList() {
    final providers = _registry.all;

    if (providers.isEmpty) {
      return [const EmptyProviders()];
    }

    // Separate home providers from dedicated providers
    final homeProviders = providers
        .where((p) => _registry.showsOnHome(p))
        .toList();

    final separateProviders = providers
        .where((p) => !_registry.showsOnHome(p))
        .toList();

    return [
      // Home providers section
      if (homeProviders.isNotEmpty) ...[
        Padding(
          padding: const EdgeInsets.only(left: 16, top: 8, bottom: 4),
          child: Text(
            _s.providersMain,
            style: TextStyle(
              color: AppTheme.textMuted,
              fontSize: 12,
              fontWeight: FontWeight.w500,
            ),
          ),
        ),
        ...homeProviders.map((provider) {
          final backendOff = !_registry.isBackendEnabled(provider.id);
          final isEnabled =
              _settings.isProviderEnabled(provider.id) && !backendOff;
          return ProviderTile(
            name: provider.name,
            url: provider.baseUrl,
            iconUrl: provider.iconUrl,
            isEnabled: isEnabled,
            disabledReason: backendOff ? _s.providerDisabledByServer : null,
            onChanged: backendOff
                ? null
                : (value) {
                    _settings.setProviderEnabled(provider.id, value);
                  },
          );
        }),
      ],

      // Separate providers section (HDRezka, YouTube)
      if (separateProviders.isNotEmpty) ...[
        Padding(
          padding: const EdgeInsets.only(left: 16, top: 12, bottom: 4),
          child: Text(
            _s.providersSeparate,
            style: TextStyle(
              color: AppTheme.textMuted,
              fontSize: 12,
              fontWeight: FontWeight.w500,
            ),
          ),
        ),
        Padding(
          padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 4),
          child: Text(
            _s.providersSeparateDesc,
            style: TextStyle(
              color: AppTheme.textMuted.withValues(alpha: 0.7),
              fontSize: 11,
            ),
          ),
        ),
        ...separateProviders.map((provider) {
          final backendOff = !_registry.isBackendEnabled(provider.id);
          final isEnabled =
              _settings.isProviderEnabled(provider.id) && !backendOff;
          final isSearchEnabled = _settings.isSearchEnabledForProvider(
            provider.id,
          );
          return _SeparateProviderTile(
            name: provider.name,
            url: provider.baseUrl,
            iconUrl: provider.iconUrl,
            searchLabel: _s.includeInGlobalSearch,
            fixedNote: backendOff
                ? _s.providerDisabledByServer
                : _s.fixedStreamsNote,
            isEnabled: isEnabled,
            isSearchEnabled: isSearchEnabled && !backendOff,
            showSearchToggle: !backendOff,
            hasFixedStreams: _registry.hasFixed(provider),
            onEnabledChanged: backendOff
                ? null
                : (value) {
                    _settings.setProviderEnabled(provider.id, value);
                  },
            onSearchEnabledChanged: (value) {
              _settings.setSearchEnabledForProvider(provider.id, value);
            },
          );
        }),
      ],
    ];
  }

  // ============================================================================
  // DIALOGS
  // ============================================================================

  void _showClearCacheDialog() {
    showDialog(
      context: context,
      builder: (ctx) => AlertDialog(
        backgroundColor: Theme.of(context).colorScheme.surface,
        title: Text(_s.confirmClearCache),
        content: Text(_s.clearCacheConfirmText),
        actions: [
          TextButton(
            onPressed: () => Navigator.pop(ctx),
            child: Text(_s.cancel),
          ),
          TextButton(
            onPressed: () async {
              Navigator.pop(ctx);
              await _clearCache();
              if (mounted) {
                ScaffoldMessenger.of(
                  context,
                ).showSnackBar(SnackBar(content: Text(_s.cacheCleared)));
              }
            },
            child: Text(
              _s.clearButton,
              style: TextStyle(color: AppTheme.errorColor),
            ),
          ),
        ],
      ),
    );
  }

  void _showAboutDialog() {
    showAboutDialog(
      context: context,
      applicationName: _s.appName,
      applicationVersion: VersionService.versionName,
      applicationLegalese: '© 2026 Open Source',
      applicationIcon: Container(
        width: 64,
        height: 64,
        decoration: BoxDecoration(
          color: Theme.of(context).colorScheme.primary,
          borderRadius: BorderRadius.circular(12),
        ),
        child: const Icon(Icons.movie_filter, size: 40, color: Colors.white),
      ),
    );
  }

  void _openGitHub() async {
    const url = 'https://github.com/edhases/oxide_film';
    final uri = Uri.parse(url);
    try {
      if (await canLaunchUrl(uri)) {
        await launchUrl(uri, mode: LaunchMode.externalApplication);
      } else {
        if (mounted) {
          ScaffoldMessenger.of(
            context,
          ).showSnackBar(SnackBar(content: Text(_s.openLinkError)));
        }
      }
    } catch (e) {
      if (mounted) {
        ScaffoldMessenger.of(
          context,
        ).showSnackBar(SnackBar(content: Text('${_s.error}: $e')));
      }
    }
  }

  Future<void> _clearCache() async {
    try {
      await DefaultCacheManager().emptyCache();
      PaintingBinding.instance.imageCache.clear();
      PaintingBinding.instance.imageCache.clearLiveImages();
    } catch (e) {
      debugPrint('Error clearing cache: $e');
    }
  }

  Future<void> _showExportDialog() async {
    try {
      await _dataTransferService.exportData();
      if (mounted) {
        ScaffoldMessenger.of(
          context,
        ).showSnackBar(SnackBar(content: Text('Резервну копію збережено')));
      }
    } catch (e) {
      if (mounted) {
        ScaffoldMessenger.of(
          context,
        ).showSnackBar(SnackBar(content: Text('Помилка експорту: $e')));
      }
    }
  }

  Future<void> _importData() async {
    try {
      await _dataTransferService.importData();
      if (mounted) {
        ScaffoldMessenger.of(
          context,
        ).showSnackBar(SnackBar(content: Text('Дані успішно відновлено')));
      }
    } catch (e) {
      if (mounted) {
        ScaffoldMessenger.of(
          context,
        ).showSnackBar(SnackBar(content: Text('Помилка імпорту: $e')));
      }
    }
  }

  Future<void> _shareData() async {
    try {
      final path = await _dataTransferService.shareBackup();
      if (mounted) {
        ScaffoldMessenger.of(
          context,
        ).showSnackBar(SnackBar(content: Text('${_s.backupShared}: $path')));
      }
    } catch (e) {
      if (mounted) {
        ScaffoldMessenger.of(
          context,
        ).showSnackBar(SnackBar(content: Text('${_s.error}: $e')));
      }
    }
  }

  Future<void> _pickDownloadFolder() async {
    try {
      String? result = await FilePicker.platform.getDirectoryPath(
        dialogTitle: _s.selectFolder,
      );
      if (result != null) {
        await _settings.setDownloadPath(result);
      }
    } catch (e) {
      if (mounted) {
        ScaffoldMessenger.of(
          context,
        ).showSnackBar(SnackBar(content: Text('${_s.error}: $e')));
      }
    }
  }

  void _showWatchPartyNameDialog() {
    final controller = TextEditingController(
      text: _settings.state.watchPartyName,
    );
    showDialog(
      context: context,
      builder: (ctx) => AlertDialog(
        backgroundColor: Theme.of(context).colorScheme.surface,
        title: Text(_s.watchPartyName),
        content: TextField(
          controller: controller,
          decoration: InputDecoration(
            hintText: _s.enterYourName,
            border: OutlineInputBorder(borderRadius: BorderRadius.circular(8)),
          ),
          maxLength: 20,
          autofocus: true,
        ),
        actions: [
          TextButton(
            onPressed: () => Navigator.pop(ctx),
            child: Text(_s.cancel),
          ),
          TextButton(
            onPressed: () {
              final name = controller.text.trim();
              if (name.isNotEmpty) {
                _settings.setWatchPartyName(name);
              }
              Navigator.pop(ctx);
            },
            child: Text(_s.save),
          ),
        ],
      ),
    );
  }

  String _getDeviceTypeName(String? type) {
    if (type == null || type == 'auto') return _s.system;
    switch (type) {
      case 'mobile':
        return 'Mobile';
      case 'desktop':
        return 'Desktop';
      case 'tv':
        return 'TV';
      default:
        return type;
    }
  }

  void _showResetSettingsDialog() {
    showDialog(
      context: context,
      builder: (ctx) => AlertDialog(
        backgroundColor: Theme.of(context).colorScheme.surface,
        title: Text(_s.resetSettings),
        content: Text(_s.resetSettingsConfirm),
        actions: [
          TextButton(
            onPressed: () => Navigator.pop(ctx),
            child: Text(_s.cancel),
          ),
          TextButton(
            onPressed: () async {
              Navigator.pop(ctx);
              await _settings.resetAllSettings();
              if (mounted) {
                ScaffoldMessenger.of(
                  context,
                ).showSnackBar(SnackBar(content: Text(_s.settingsReset)));
              }
            },
            child: Text(
              _s.confirm,
              style: TextStyle(color: AppTheme.errorColor),
            ),
          ),
        ],
      ),
    );
  }
}

/// Tile for separate providers (HDRezka, YouTube) with additional settings.
/// Uses the same visual language as [ProviderTile]: theme accent switches,
/// no per-provider colors, no nested cards.
class _SeparateProviderTile extends StatelessWidget {
  final String name;
  final String url;
  final String? iconUrl;
  final String searchLabel;
  final String fixedNote;
  final bool isEnabled;
  final bool isSearchEnabled;
  final bool showSearchToggle;
  final bool hasFixedStreams;
  final ValueChanged<bool>? onEnabledChanged;
  final ValueChanged<bool> onSearchEnabledChanged;

  const _SeparateProviderTile({
    required this.name,
    required this.url,
    this.iconUrl,
    required this.searchLabel,
    required this.fixedNote,
    required this.isEnabled,
    required this.isSearchEnabled,
    this.showSearchToggle = true,
    required this.hasFixedStreams,
    required this.onEnabledChanged,
    required this.onSearchEnabledChanged,
  });

  @override
  Widget build(BuildContext context) {
    final muted = Theme.of(context).textTheme.bodySmall?.color;
    return Column(
      mainAxisSize: MainAxisSize.min,
      children: [
        ProviderTile(
          name: name,
          url: url,
          iconUrl: iconUrl,
          isEnabled: isEnabled,
          disabledReason: showSearchToggle ? null : fixedNote,
          onChanged: onEnabledChanged,
        ),
        if (isEnabled && showSearchToggle)
          Padding(
            padding: const EdgeInsets.only(left: 68, right: 16, bottom: 4),
            child: Column(
              mainAxisSize: MainAxisSize.min,
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Row(
                  children: [
                    Icon(Icons.search, size: 18, color: muted),
                    const SizedBox(width: 8),
                    Expanded(
                      child: Text(
                        searchLabel,
                        style: TextStyle(fontSize: 13, color: muted),
                      ),
                    ),
                    Switch(
                      value: isSearchEnabled,
                      onChanged: onSearchEnabledChanged,
                    ),
                  ],
                ),
                if (hasFixedStreams)
                  Padding(
                    padding: const EdgeInsets.only(top: 2, bottom: 6),
                    child: Row(
                      children: [
                        Icon(
                          Icons.info_outline,
                          size: 14,
                          color: muted?.withValues(alpha: 0.7),
                        ),
                        const SizedBox(width: 6),
                        Expanded(
                          child: Text(
                            fixedNote,
                            style: TextStyle(
                              fontSize: 11,
                              color: muted?.withValues(alpha: 0.7),
                            ),
                          ),
                        ),
                      ],
                    ),
                  ),
              ],
            ),
          ),
      ],
    );
  }
}
