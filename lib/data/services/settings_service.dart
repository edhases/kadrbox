import 'dart:async';
import 'package:flutter/material.dart';
import '../../core/l10n/app_strings.dart';
import '../../core/utils/logger.dart';
import '../../domain/entities/ui_settings.dart';
import '../database/app_database.dart';
import '../database/dao/settings_dao.dart';

/// Application theme mode
enum AppThemeMode {
  dark,
  amoled,
  light,
  system;

  String get displayName {
    switch (this) {
      case AppThemeMode.dark:
        return 'Темна';
      case AppThemeMode.amoled:
        return 'AMOLED';
      case AppThemeMode.light:
        return 'Світла';
      case AppThemeMode.system:
        return 'Системна';
    }
  }

  String get key => name;

  static AppThemeMode fromString(String? value) {
    return AppThemeMode.values.firstWhere(
      (e) => e.key == value,
      orElse: () => AppThemeMode.dark,
    );
  }
}

/// Default quality preference
enum DefaultQuality {
  auto,
  q480p,
  q720p,
  q1080p,
  q1440p;

  String get displayName {
    switch (this) {
      case DefaultQuality.auto:
        return 'Авто';
      case DefaultQuality.q480p:
        return '480p';
      case DefaultQuality.q720p:
        return '720p';
      case DefaultQuality.q1080p:
        return '1080p';
      case DefaultQuality.q1440p:
        return '1440p+';
    }
  }

  String get key => name;

  static DefaultQuality fromString(String? value) {
    return DefaultQuality.values.firstWhere(
      (e) => e.key == value,
      orElse: () => DefaultQuality.auto,
    );
  }
}

/// Player type preference
enum PlayerType {
  internal,
  external;

  String get displayName {
    switch (this) {
      case PlayerType.internal:
        return 'Вбудований';
      case PlayerType.external:
        return 'Зовнішній';
    }
  }

  String get key => name;

  static PlayerType fromString(String? value) {
    return PlayerType.values.firstWhere(
      (e) => e.key == value,
      orElse: () => PlayerType.internal,
    );
  }
}

/// Current settings state
class SettingsState {
  final AppThemeMode theme;
  final DefaultQuality defaultQuality;
  final PlayerType playerType;
  final bool autoPlayNext;
  final bool rememberPosition;
  final String subtitleLanguage;
  final String watchPartyName;
  final Map<String, bool> providerStates;
  final UISettings uiSettings;
  final AppLocale locale;
  // External API keys
  final String? tmdbApiKey;
  final String? traktAccessToken;
  // New extended settings
  final double defaultSpeed;
  final bool gestureControls;
  final bool skipIntro;
  final int nextEpisodeDelay; // seconds
  final bool debugMode;
  final String deviceTypeOverride; // 'auto', 'phone', 'tablet', 'desktop', 'tv'
  final bool newEpisodeNotify;
  final bool updateNotify;
  final int fullscreenTransitionDelay; // milliseconds
  final String downloadPath;
  final bool onlyWifiDownload;

  const SettingsState({
    this.theme = AppThemeMode.dark,
    this.defaultQuality = DefaultQuality.auto,
    this.playerType = PlayerType.internal,
    this.autoPlayNext = true,
    this.rememberPosition = true,
    this.subtitleLanguage = 'uk',
    this.watchPartyName = 'User',
    this.providerStates = const {},
    this.uiSettings = const UISettings(),
    this.locale = AppLocale.uk,
    this.tmdbApiKey,
    this.traktAccessToken,
    // New defaults
    this.defaultSpeed = 1.0,
    this.gestureControls = true,
    this.skipIntro = false,
    this.nextEpisodeDelay = 5,
    this.debugMode = false,
    this.deviceTypeOverride = 'auto',
    this.newEpisodeNotify = true,
    this.updateNotify = true,
    this.fullscreenTransitionDelay = 200,
    this.downloadPath = '',
    this.onlyWifiDownload = true,
  });

  SettingsState copyWith({
    AppThemeMode? theme,
    DefaultQuality? defaultQuality,
    PlayerType? playerType,
    bool? autoPlayNext,
    bool? rememberPosition,
    String? subtitleLanguage,
    String? watchPartyName,
    Map<String, bool>? providerStates,
    UISettings? uiSettings,
    AppLocale? locale,
    String? tmdbApiKey,
    String? traktAccessToken,
    double? defaultSpeed,
    bool? gestureControls,
    bool? skipIntro,
    int? nextEpisodeDelay,
    bool? debugMode,
    String? deviceTypeOverride,
    bool? newEpisodeNotify,
    bool? updateNotify,
    int? fullscreenTransitionDelay,
    String? downloadPath,
    bool? onlyWifiDownload,
  }) {
    return SettingsState(
      theme: theme ?? this.theme,
      defaultQuality: defaultQuality ?? this.defaultQuality,
      playerType: playerType ?? this.playerType,
      autoPlayNext: autoPlayNext ?? this.autoPlayNext,
      rememberPosition: rememberPosition ?? this.rememberPosition,
      subtitleLanguage: subtitleLanguage ?? this.subtitleLanguage,
      watchPartyName: watchPartyName ?? this.watchPartyName,
      providerStates: providerStates ?? this.providerStates,
      uiSettings: uiSettings ?? this.uiSettings,
      locale: locale ?? this.locale,
      tmdbApiKey: tmdbApiKey ?? this.tmdbApiKey,
      traktAccessToken: traktAccessToken ?? this.traktAccessToken,
      defaultSpeed: defaultSpeed ?? this.defaultSpeed,
      gestureControls: gestureControls ?? this.gestureControls,
      skipIntro: skipIntro ?? this.skipIntro,
      nextEpisodeDelay: nextEpisodeDelay ?? this.nextEpisodeDelay,
      debugMode: debugMode ?? this.debugMode,
      deviceTypeOverride: deviceTypeOverride ?? this.deviceTypeOverride,
      newEpisodeNotify: newEpisodeNotify ?? this.newEpisodeNotify,
      updateNotify: updateNotify ?? this.updateNotify,
      fullscreenTransitionDelay:
          fullscreenTransitionDelay ?? this.fullscreenTransitionDelay,
      downloadPath: downloadPath ?? this.downloadPath,
      onlyWifiDownload: onlyWifiDownload ?? this.onlyWifiDownload,
    );
  }

  ThemeMode get flutterThemeMode {
    switch (theme) {
      case AppThemeMode.dark:
      case AppThemeMode.amoled:
        return ThemeMode.dark;
      case AppThemeMode.light:
        return ThemeMode.light;
      case AppThemeMode.system:
        return ThemeMode.system;
    }
  }
}

/// Settings service - manages app settings with persistence
class SettingsService extends ChangeNotifier {
  final SettingsDao _settingsDao;

  SettingsState _state = const SettingsState();
  SettingsState get state => _state;

  // Stream subscriptions
  StreamSubscription<Map<String, String>>? _settingsSubscription;
  StreamSubscription<Map<String, bool>>? _providersSubscription;

  SettingsService(AppDatabase database) : _settingsDao = SettingsDao(database) {
    _init();
  }

  Future<void> _init() async {
    // Load initial settings
    await _loadSettings();

    // Subscribe to changes
    _settingsSubscription = _settingsDao.watchAllSettings().listen((settings) {
      _updateStateFromSettings(settings);
    });

    _providersSubscription = _settingsDao.watchProviderStates().listen((
      providers,
    ) {
      _state = _state.copyWith(providerStates: providers);
      notifyListeners();
    });
  }

  Future<void> _loadSettings() async {
    Logger.d('Loading settings from database...', tag: 'Settings');
    final settings = await _settingsDao.getAllSettings();
    Logger.d('Loaded ${settings.length} settings', tag: 'Settings');
    _updateStateFromSettings(settings);

    final providerStates = await _settingsDao.getEnabledProviders();
    final providers = {for (var p in providerStates) p.providerId: p.isEnabled};
    Logger.d('Loaded ${providers.length} provider states', tag: 'Settings');
    _state = _state.copyWith(providerStates: providers);
    notifyListeners();
  }

  void _updateStateFromSettings(Map<String, String> settings) {
    _state = SettingsState(
      theme: AppThemeMode.fromString(settings['theme']),
      defaultQuality: DefaultQuality.fromString(settings['default_quality']),
      playerType: PlayerType.fromString(settings['player_type']),
      autoPlayNext: settings['auto_play_next'] != 'false',
      rememberPosition: settings['remember_position'] != 'false',
      subtitleLanguage: settings['subtitle_language'] ?? 'uk',
      watchPartyName: settings['watch_party_name'] ?? 'User',
      providerStates: _state.providerStates,
      locale: AppLocale.fromCode(settings['locale']),
      uiSettings: UISettings(
        posterSize: PosterSize.fromString(settings['poster_size']),
        gridSpacing: GridSpacing.fromString(settings['grid_spacing']),
        accentColor: AccentColor.fromString(settings['accent_color']),
        cardInfoStyle: CardInfoStyle.fromString(settings['card_info_style']),
        listStyle: ListStyle.fromString(settings['list_style']),
        showRatings: settings['show_ratings'] != 'false',
        showYears: settings['show_years'] != 'false',
        animationsEnabled: settings['animations_enabled'] != 'false',
        blurBackgrounds: settings['blur_backgrounds'] != 'false',
        gridColumns: int.tryParse(settings['grid_columns'] ?? '') ?? 0,
      ),
      // New extended settings
      defaultSpeed: double.tryParse(settings['default_speed'] ?? '') ?? 1.0,
      gestureControls: settings['gesture_controls'] != 'false',
      skipIntro: settings['skip_intro'] == 'true',
      nextEpisodeDelay: int.tryParse(settings['next_episode_delay'] ?? '') ?? 5,
      debugMode: settings['debug_mode'] == 'true',
      deviceTypeOverride: settings['device_type_override'] ?? 'auto',
      newEpisodeNotify: settings['new_episode_notify'] != 'false',
      updateNotify: settings['update_notify'] != 'false',
      fullscreenTransitionDelay:
          int.tryParse(settings['fullscreen_transition_delay'] ?? '') ?? 200,
      downloadPath: settings['download_path'] ?? '',
      onlyWifiDownload: settings['only_wifi_download'] != 'false',
    );
    notifyListeners();
  }

  // ============================================================================
  // SETTERS
  // ============================================================================

  /// Set locale/language
  Future<void> setLocale(AppLocale locale) async {
    await _settingsDao.setSetting('locale', locale.code);
  }

  /// Set theme mode
  Future<void> setTheme(AppThemeMode theme) async {
    Logger.d('setTheme: ${theme.key}', tag: 'Settings');
    await _settingsDao.setSetting('theme', theme.key);
  }

  /// Set default quality
  Future<void> setDefaultQuality(DefaultQuality quality) async {
    await _settingsDao.setSetting('default_quality', quality.key);
  }

  /// Set player type
  Future<void> setPlayerType(PlayerType type) async {
    await _settingsDao.setSetting('player_type', type.key);
  }

  /// Set auto play next
  Future<void> setAutoPlayNext(bool value) async {
    await _settingsDao.setSetting('auto_play_next', value.toString());
  }

  /// Set remember position
  Future<void> setRememberPosition(bool value) async {
    await _settingsDao.setSetting('remember_position', value.toString());
  }

  /// Set subtitle language
  Future<void> setSubtitleLanguage(String language) async {
    await _settingsDao.setSetting('subtitle_language', language);
  }

  /// Set watch party name
  Future<void> setWatchPartyName(String name) async {
    await _settingsDao.setSetting('watch_party_name', name);
  }

  /// Set provider enabled state
  Future<void> setProviderEnabled(String providerId, bool isEnabled) async {
    Logger.d('setProviderEnabled: $providerId = $isEnabled', tag: 'Settings');
    await _settingsDao.setProviderEnabled(providerId, isEnabled);
  }

  /// Check if provider is enabled
  bool isProviderEnabled(String providerId) {
    return _state.providerStates[providerId] ?? true;
  }

  // ============================================================================
  // UI SETTINGS
  // ============================================================================

  /// Get UI settings
  UISettings get uiSettings => _state.uiSettings;

  /// Set poster size
  Future<void> setPosterSize(PosterSize size) async {
    await _settingsDao.setSetting('poster_size', size.key);
  }

  /// Set grid spacing
  Future<void> setGridSpacing(GridSpacing spacing) async {
    await _settingsDao.setSetting('grid_spacing', spacing.key);
  }

  /// Set accent color
  Future<void> setAccentColor(AccentColor color) async {
    await _settingsDao.setSetting('accent_color', color.key);
  }

  /// Set card info style
  Future<void> setCardInfoStyle(CardInfoStyle style) async {
    await _settingsDao.setSetting('card_info_style', style.key);
  }

  /// Set list style
  Future<void> setListStyle(ListStyle style) async {
    await _settingsDao.setSetting('list_style', style.key);
  }

  /// Set show ratings
  Future<void> setShowRatings(bool value) async {
    await _settingsDao.setSetting('show_ratings', value.toString());
  }

  /// Set show years
  Future<void> setShowYears(bool value) async {
    await _settingsDao.setSetting('show_years', value.toString());
  }

  /// Set animations enabled
  Future<void> setAnimationsEnabled(bool value) async {
    await _settingsDao.setSetting('animations_enabled', value.toString());
  }

  /// Set blur backgrounds
  Future<void> setBlurBackgrounds(bool value) async {
    await _settingsDao.setSetting('blur_backgrounds', value.toString());
  }

  /// Set grid columns
  Future<void> setGridColumns(int columns) async {
    await _settingsDao.setSetting('grid_columns', columns.toString());
  }

  // ============================================================================
  // EXTENDED SETTINGS
  // ============================================================================

  /// Set default playback speed
  Future<void> setDefaultSpeed(double speed) async {
    await _settingsDao.setSetting('default_speed', speed.toString());
  }

  /// Set gesture controls enabled
  Future<void> setGestureControls(bool value) async {
    await _settingsDao.setSetting('gesture_controls', value.toString());
  }

  /// Set skip intro
  Future<void> setSkipIntro(bool value) async {
    await _settingsDao.setSetting('skip_intro', value.toString());
  }

  /// Set next episode delay
  Future<void> setNextEpisodeDelay(int seconds) async {
    await _settingsDao.setSetting('next_episode_delay', seconds.toString());
  }

  /// Set debug mode
  Future<void> setDebugMode(bool value) async {
    await _settingsDao.setSetting('debug_mode', value.toString());
  }

  /// Set device type override
  Future<void> setDeviceTypeOverride(String type) async {
    await _settingsDao.setSetting('device_type_override', type);
  }

  /// Set new episode notifications
  Future<void> setNewEpisodeNotify(bool value) async {
    await _settingsDao.setSetting('new_episode_notify', value.toString());
  }

  /// Set update notifications
  Future<void> setUpdateNotify(bool value) async {
    await _settingsDao.setSetting('update_notify', value.toString());
  }

  /// Set fullscreen transition delay
  Future<void> setFullscreenTransitionDelay(int milliseconds) async {
    await _settingsDao.setSetting(
      'fullscreen_transition_delay',
      milliseconds.toString(),
    );
  }

  /// Set download path
  Future<void> setDownloadPath(String path) async {
    await _settingsDao.setSetting('download_path', path);
  }

  /// Set only wifi download
  Future<void> setOnlyWifiDownload(bool value) async {
    await _settingsDao.setSetting('only_wifi_download', value.toString());
  }

  // ============================================================================
  // SEPARATE PROVIDER SEARCH SETTINGS
  // ============================================================================

  /// Check if search is enabled for a separate provider (default: false)
  /// Separate providers are excluded from global search by default
  bool isSearchEnabledForProvider(String providerId) {
    final key = 'search_enabled_$providerId';
    // For separate providers, default is false (not included in global search)
    return _state.providerStates[key] ?? false;
  }

  /// Enable/disable a provider in global search
  Future<void> setSearchEnabledForProvider(
    String providerId,
    bool value,
  ) async {
    final key = 'search_enabled_$providerId';
    await _settingsDao.setProviderEnabled(key, value);
    final newStates = Map<String, bool>.from(_state.providerStates);
    newStates[key] = value;
    _state = _state.copyWith(providerStates: newStates);
    notifyListeners();
  }

  /// Reset all settings to defaults
  Future<void> resetAllSettings() async {
    await _settingsDao.clearAllSettings();
    _state = const SettingsState();
    notifyListeners();
  }

  @override
  void dispose() {
    _settingsSubscription?.cancel();
    _providersSubscription?.cancel();
    super.dispose();
  }
}
