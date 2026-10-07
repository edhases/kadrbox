import 'package:flutter/material.dart';

/// Application locale
enum AppLocale {
  uk,
  en;

  String get displayName {
    switch (this) {
      case AppLocale.uk:
        return 'Українська';
      case AppLocale.en:
        return 'English';
    }
  }

  String get code => name;

  Locale get locale => Locale(name);

  static AppLocale fromCode(String? code) {
    return AppLocale.values.firstWhere(
      (l) => l.code == code,
      orElse: () => AppLocale.uk,
    );
  }
}

/// Localization strings
class AppStrings {
  final AppLocale locale;

  AppStrings(this.locale);

  // Factory for getting localized instance
  static AppStrings of(BuildContext context) {
    return Localizations.of<AppStrings>(context, AppStrings) ??
        AppStrings(AppLocale.uk);
  }

  // ============================================================================
  // GENERAL
  // ============================================================================

  String get appName => 'Kadrbox';
  String get loading => _t('Завантаження...', 'Loading...');
  String get error => _t('Помилка', 'Error');
  String get retry => _t('Спробувати знову', 'Try again');
  String get cancel => _t('Скасувати', 'Cancel');
  String get delete => _t('Видалити', 'Delete');
  String get clear => _t('Очистити', 'Clear');
  String get back => _t('Назад', 'Back');
  String get save => _t('Зберегти', 'Save');
  String get confirm => _t('Підтвердити', 'Confirm');
  String get close => _t('Закрити', 'Close');
  String get search => _t('Пошук', 'Search');
  String get settings => _t('Налаштування', 'Settings');
  String get language => _t('Мова', 'Language');

  // ============================================================================
  // HOME
  // ============================================================================

  String get home => _t('Головна', 'Home');
  String get popular => _t('Популярне', 'Popular');
  String get favorites => _t('Обране', 'Favorites');
  String get history => _t('Історія', 'History');
  String get noContent => _t('Немає контенту', 'No content');
  String get loadingError => _t('Помилка завантаження', 'Loading error');
  String get openLocalFile => _t('Відкрити локальне відео', 'Open Local Video');
  String get openNetworkStream => _t('Відкрити за посиланням', 'Open by Link');
  String get localVideo => _t('Локальне відео', 'Local Video');
  String get unsupportedFormat =>
      _t('Непідтримуваний формат файлу', 'Unsupported file format');
  String get selectVideoFile => _t('Виберіть відеофайл', 'Select a video file');

  // ============================================================================
  // CONTENT TYPES
  // ============================================================================

  String get movie => _t('Фільм', 'Movie');
  String get movies => _t('Фільми', 'Movies');
  String get seasonLabel => _t('Сезон', 'Season');
  String get episodeLabel => _t('Серія', 'Episode');
  String get series => _t('Серіал', 'Series');
  String get seriesPlural => _t('Серіали', 'Series');
  String get cartoon => _t('Мультфільм', 'Cartoon');
  String get cartoons => _t('Мультфільми', 'Cartoons');
  String get anime => _t('Аніме', 'Anime');
  String get animePlural => _t('Аніме', 'Anime');

  // ============================================================================
  // PLAYER
  // ============================================================================

  String get player => _t('Плеєр', 'Player');
  String get builtIn => _t('Вбудований', 'Built-in');
  String get external => _t('Зовнішній', 'External');
  String get quality => _t('Якість', 'Quality');
  String get defaultQuality => _t('Якість за замовчуванням', 'Default quality');
  String get speed => _t('Швидкість', 'Speed');
  String get voiceover => _t('Озвучка', 'Voiceover');
  String get subtitles => _t('Субтитри', 'Subtitles');
  String get autoPlayNext => _t('Автовідтворення наступного', 'Auto-play next');
  String get autoPlayNextDesc => _t(
    'Автоматично запускати наступну серію',
    'Automatically play next episode',
  );
  String get rememberPosition =>
      _t('Запам\'ятовувати позицію', 'Remember position');
  String get rememberPositionDesc =>
      _t('Продовжувати з місця зупинки', 'Continue from where you left off');
  String get playbackSettings =>
      _t('Налаштування відтворення', 'Playback settings');
  String get watch => _t('Дивитися', 'Watch');
  String get noSources => _t('Немає джерел', 'No sources');
  String resumedFrom(String time) =>
      _t('Продовжено з $time', 'Resumed from $time');

  // ============================================================================
  // DETAILS
  // ============================================================================

  String get description => _t('Опис', 'Description');
  String get actors => _t('Актори', 'Actors');
  String get seasonsAndEpisodes => _t('Сезони та серії', 'Seasons & Episodes');
  String season(int n) => _t('Сезон $n', 'Season $n');
  String episode(int n) => _t('Серія $n', 'Episode $n');
  String get availableSources => _t('Доступні джерела', 'Available sources');
  String get addToFavorites => _t('Додати до обраного', 'Add to favorites');
  String get removeFromFavorites =>
      _t('Видалити з обраного', 'Remove from favorites');
  String get addedToFavorites => _t('Додано до обраного', 'Added to favorites');
  String get removedFromFavorites =>
      _t('Видалено з обраного', 'Removed from favorites');
  String get watchParty => _t('Спільний перегляд', 'Watch party');

  // ============================================================================
  // ACCOUNT
  // ============================================================================

  String get account => _t('Акаунт', 'Account');
  String get signIn => _t('Увійти', 'Sign in');
  String get signInPrompt => _t('Увійдіть в акаунт', 'Sign in to your account');
  String get signInDescription =>
      _t('Синхронізуйте дані між пристроями', 'Sync data across devices');
  String get syncCloud =>
      _t('Історія та обране в хмарі', 'History and favorites in cloud');
  String get searchSettings => _t('Пошук налаштувань...', 'Search settings...');
  String get fullscreenDelay =>
      _t('Затримка повноекранного режиму', 'Fullscreen transition delay');
  String get milliseconds => _t('мс', 'ms');
  String get searchHint =>
      _t('Почніть вводити для пошуку', 'Start typing to search');
  String get seconds => _t('сек', 'sec');

  // ============================================================================
  // SETTINGS
  // ============================================================================

  String get contentSources => _t('Джерела контенту', 'Content sources');
  String get playback => _t('Відтворення', 'Playback');
  String get appearance => _t('Зовнішній вигляд', 'Appearance');
  String get theme => _t('Тема', 'Theme');
  String get dark => _t('Темна', 'Dark');
  String get amoled => _t('AMOLED', 'AMOLED');
  String get light => _t('Світла', 'Light');
  String get system => _t('Системна', 'System');
  String get customization => _t('Кастомізація', 'Customization');
  String get customizationDesc =>
      _t('Розмір постерів, кольори, сітка', 'Poster size, colors, grid');
  String get data => _t('Дані', 'Data');
  String get sync => _t('Синхронізація', 'Sync');
  String get export => _t('Експорт даних', 'Export data');
  String get exportDesc =>
      _t('Зберегти обране та історію', 'Save favorites and history');
  String get import => _t('Імпорт даних', 'Import data');
  String get importDesc => _t('Відновити з файлу', 'Restore from file');
  String get share => _t('Поділитись даними', 'Share data');
  String get shareDesc => _t('Надіслати копію', 'Send a copy');
  String get downloads => _t('Завантаження', 'Downloads');
  String get offlineContent => _t('Офлайн контент', 'Offline content');
  String get downloadFolder => _t('Папка для завантажень', 'Download folder');
  String get selectFolder => _t('Обрати папку', 'Select folder');
  String get defaultFolder =>
      _t('За замовчуванням (Системна)', 'Default (System)');
  String get onlyWifiDownload =>
      _t('Тільки через Wi-Fi', 'Download only over Wi-Fi');
  String get onlyWifiDownloadDesc => _t(
    'Завантажувати лише за наявності Wi-Fi підключення',
    'Only download when connected to Wi-Fi',
  );
  String get clearCache => _t('Очистити кеш', 'Clear cache');
  String get clearCacheDesc =>
      _t('Видалити тимчасові файли', 'Delete temporary files');
  String get downloaded => _t('Завантажено', 'Downloaded');
  String get inQueue => _t('В черзі', 'In Queue');
  String get clearAll => _t('Очистити все', 'Clear all');
  String get noDownloads => _t('Немає завантажень', 'No downloads');
  String get noDownloadsDesc => _t(
    'Завантажте фільми для офлайн перегляду',
    'Download movies for offline viewing',
  );
  String get noActiveDownloads =>
      _t('Немає активних завантажень', 'No active downloads');
  String get noActiveDownloadsDesc =>
      _t('Додайте контент для завантаження', 'Add content to download');
  String get fileNotFound => _t('Файл не знайдено', 'File not found');
  String get deleteDownload => _t('Видалити завантаження?', 'Delete download?');
  String get confirmDelete =>
      _t('Ви впевнені, що хочете видалити', 'Are you sure you want to delete');
  String get clearAllDownloads =>
      _t('Очистити всі завантаження?', 'Clear all downloads?');
  String get clearAllDownloadsConfirm => _t(
    'Ви впевнені, що хочете видалити всі завантажені файли? Цю дію неможливо відмінити.',
    'Are you sure you want to delete all downloaded files? This action cannot be undone.',
  );
  String get downloadsUnavailable =>
      _t('Завантаження недоступне', 'Downloads unavailable');
  String get downloadsUnavailableDesc => _t(
    'Ця функція недоступна у веб-версії',
    'This feature is not available in the web version',
  );
  String get pending => _t('Очікування...', 'Pending...');
  String get downloadingStatus => _t('Завантаження...', 'Downloading...');
  String get pausedStatus => _t('Призупинено', 'Paused');
  String get failedStatus => _t('Помилка завантаження', 'Download failed');
  String get completedStatus => _t('Завершено', 'Completed');
  String get statistics => _t('Статистика', 'Statistics');
  String get statsDesc =>
      _t('Переглянуті фільми, час, жанри', 'Watched movies, time, genres');
  String get about => _t('Про додаток', 'About');
  String get version => _t('Версія', 'Version');
  String get sourceCode => _t('Вихідний код', 'Source code');
  String get sourceCodeDesc =>
      _t('Вихідний код проєкту', 'Project source code');
  String get historyDesc => _t('Переглянути або очистити', 'View or clear');
  String get favoritesDesc => _t('Керування списком', 'Manage list');

  // Settings dialogs
  String get selectLanguage => _t('Оберіть мову', 'Select language');
  String get selectTheme => _t('Оберіть тему', 'Select theme');
  String get selectPlayer => _t('Оберіть плеєр', 'Select player');
  String get builtInPlayer =>
      _t('Вбудований media_kit плеєр', 'Built-in media_kit player');
  String get externalPlayerDesc =>
      _t('VLC, MX Player, тощо', 'VLC, MX Player, etc.');
  String get confirmClearCache => _t('Очистити кеш?', 'Clear cache?');
  String get clearCacheConfirmText => _t(
    'Це видалить всі тимчасові файли та кешовані зображення. Історія та обране залишаться.',
    'This will delete all temporary files and cached images. History and favorites will remain.',
  );
  String get cacheCleared => _t('Кеш очищено', 'Cache cleared');
  String get noProviders => _t('Немає провайдерів', 'No providers');
  String get addContentSources =>
      _t('Додайте джерела контенту', 'Add content sources');
  String get providersMain => _t('Основні провайдери', 'Main providers');
  String get providersSeparate => _t('Окремі провайдери', 'Separate providers');
  String get providersSeparateDesc => _t(
    'Мають власні розділи в каталозі та за замовчуванням вимкнені в загальному пошуку',
    'Have their own catalog sections and are excluded from global search by default',
  );
  String get includeInGlobalSearch =>
      _t('Включити в загальний пошук', 'Include in global search');
  String get fixedStreamsNote => _t(
    'Якість і дубляж обираються при запуску',
    'Quality and dubbing are picked at playback start',
  );
  String get backupShared => _t('Копію надіслано', 'Backup shared');
  String get providerDisabledByServer =>
      _t('Вимкнено сервером', 'Disabled by server');
  String exportSuccess(String path) => _t('Збережено: $path', 'Saved: $path');
  String get importSuccess =>
      _t('Дані успішно імпортовано!', 'Data imported successfully!');
  String importError(String error) =>
      _t('Помилка імпорту: $error', 'Import error: $error');
  String get willExport => _t('Буде експортовано:', 'Will export:');
  String favoritesCount(int n) =>
      _t('Обране: $n елементів', 'Favorites: $n items');
  String historyCount(int n) =>
      _t('Історія: $n записів', 'History: $n entries');
  String get settingsLabel => _t('Налаштування', 'Settings');
  String get exportWillSaveAsJson =>
      _t('Файл буде збережено у форматі JSON.', 'File will be saved as JSON.');
  String get exportButton => _t('Експортувати', 'Export');
  String get clearButton => _t('Очистити', 'Clear');
  String get openLinkError =>
      _t('Не вдалося відкрити посилання', 'Failed to open link');

  // ============================================================================
  // FILTERS
  // ============================================================================

  String get filters => _t('Фільтри', 'Filters');
  String get sort => _t('Сортування', 'Sort');
  String get sortBy => _t('Сортувати за', 'Sort by');
  String get ascending => _t('За зростанням', 'Ascending');
  String get descending => _t('За спаданням', 'Descending');
  String get title => _t('Назва', 'Title');
  String get year => _t('Рік', 'Year');
  String get rating => _t('Рейтинг', 'Rating');
  String get type => _t('Тип', 'Type');
  String get genre => _t('Жанр', 'Genre');
  String get country => _t('Країна', 'Country');
  String get clearFilters => _t('Скинути фільтри', 'Clear filters');
  String get apply => _t('Застосувати', 'Apply');
  String get noResults => _t('Нічого не знайдено', 'Nothing found');
  String get tryChangingFilters =>
      _t('Спробуйте змінити фільтри', 'Try changing filters');
  String results(int count) => _t('Результати ($count)', 'Results ($count)');

  // ============================================================================
  // WATCH PARTY
  // ============================================================================

  String get createRoom => _t('Створити кімнату', 'Create room');
  String get joinRoom => _t('Приєднатися до кімнати', 'Join room');
  String get yourName => _t('Ваше ім\'я', 'Your name');
  String get enterYourName => _t('Введіть ваше ім\'я', 'Enter your name');
  String get ipAddress => _t('IP адреса', 'IP address');
  String get port => _t('Порт', 'Port');
  String get connecting => _t('Підключення...', 'Connecting...');
  String get connected => _t('Підключено', 'Connected');
  String get youAreHost => _t('Ви хост', 'You are the host');
  String get connectedToHost => _t('Підключено до хоста', 'Connected to host');
  String get connectionCode => _t('Код підключення', 'Connection code');
  String get copyCode => _t('Копіювати код', 'Copy code');
  String get codeCopied => _t('Код скопійовано', 'Code copied');
  String get participants => _t('Учасники', 'Participants');
  String get chat => _t('Чат', 'Chat');
  String get writeFirstMessage =>
      _t('Напишіть перше повідомлення!', 'Write the first message!');
  String get sendMessage =>
      _t('Написати повідомлення...', 'Write a message...');
  String get leaveRoom => _t('Вийти з кімнати', 'Leave room');
  String get deleteAccountConfirm => _t(
    'Це видалить усі ваші дані. Продовжити?',
    'This will delete all your data. Continue?',
  );
  String get sessionExpired => _t(
    'Сесія закінчилася, будь ласка, увійдіть знову',
    'Session expired, please sign in again',
  );
  String get networkErrorLink => _t(
    'Помилка мережі при спробі авторизації',
    'Network error during auth attempt',
  );
  String get leaveRoomConfirm => _t(
    'Ви покинете спільний перегляд. Продовжити?',
    'You will leave the watch party. Continue?',
  );
  String get connectionError => _t('Помилка підключення', 'Connection error');
  String get host => _t('Хост', 'Host');
  String get start => _t('Почати', 'Start');
  String get join => _t('Приєднатися', 'Join');
  String get unknownError => _t('Невідома помилка', 'Unknown error');
  String get experimentalFeature => _t(
    'Спільний перегляд працює через інтернет. Це експериментальна функція.',
    'Watch party works over the internet. This is an experimental feature.',
  );
  String get selectMediaFirst =>
      _t('Спочатку оберіть медіа', 'Select media first');
  String get enterRoomCode =>
      _t('Введіть 6-символьний код кімнати', 'Enter 6-character room code');
  String get roomCode => _t('Код кімнати', 'Room code');

  // ============================================================================
  // NOTIFICATIONS
  // ============================================================================

  String get newEpisodes => _t('Нові серії', 'New episodes');
  String get noNewEpisodes => _t('Немає нових серій', 'No new episodes');
  String newEpisodesAvailable(int count) =>
      _t('Доступно $count нових серій', '$count new episodes available');

  // ============================================================================
  // SETTINGS - EXTENDED
  // ============================================================================

  // Watch Party settings
  String get watchPartySettings =>
      _t('Налаштування спільного перегляду', 'Watch Party settings');
  String get watchPartyName => _t('Ім\'я в Watch Party', 'Watch Party name');
  String get watchPartyNameDesc => _t(
    'Відображатиметься іншим учасникам',
    'Will be shown to other participants',
  );
  String get syncSettings => _t('Налаштування синхронізації', 'Sync settings');
  String get syncThreshold => _t('Поріг синхронізації', 'Sync threshold');
  String get syncThresholdDesc =>
      _t('Максимальна затримка перед корекцією', 'Max delay before correction');

  // Playback extended
  String get defaultSpeed => _t('Типова швидкість', 'Default speed');
  String get defaultSpeedDesc =>
      _t('Швидкість відтворення за замовчуванням', 'Default playback speed');
  String get gestureControls => _t('Керування жестами', 'Gesture controls');
  String get gestureControlsDesc => _t(
    'Свайпи для гучності та яскравості',
    'Swipe for volume and brightness',
  );
  String get skipIntro => _t('Пропуск інтро', 'Skip intro');
  String get skipIntroDesc =>
      _t('Автоматично пропускати заставку', 'Auto-skip opening credits');
  String get nextEpisodeDelay =>
      _t('Затримка наступної серії', 'Next episode delay');
  String get nextEpisodeDelayDesc =>
      _t('Секунд до автоматичного переходу', 'Seconds before auto-transition');

  // Advanced / Developer
  String get advanced => _t('Розширені', 'Advanced');
  String get developer => _t('Для розробників', 'Developer');
  String get debugMode => _t('Режим налагодження', 'Debug mode');
  String get debugModeDesc => _t(
    'Показувати логи та технічну інформацію',
    'Show logs and technical info',
  );
  String get deviceTypeOverride => _t('Тип пристрою', 'Device type');
  String get deviceTypeOverrideDesc =>
      _t('Авто-визначення або ручний вибір', 'Auto-detect or manual selection');
  String get deviceAuto => _t('Авто', 'Auto');
  String get devicePhone => _t('Телефон', 'Phone');
  String get deviceTablet => _t('Планшет', 'Tablet');
  String get deviceDesktop => _t('Десктоп', 'Desktop');
  String get deviceTV => _t('TV', 'TV');
  String get resetSettings => _t('Скинути налаштування', 'Reset settings');
  String get resetSettingsDesc => _t(
    'Повернути всі налаштування за замовчуванням',
    'Reset all settings to defaults',
  );
  String get resetSettingsConfirm => _t(
    'Ви впевнені? Всі налаштування буде скинуто.',
    'Are you sure? All settings will be reset.',
  );
  String get settingsReset => _t('Налаштування скинуто', 'Settings reset');

  // Notifications
  String get notifications => _t('Сповіщення', 'Notifications');
  String get notificationsDesc => _t(
    'Сповіщення про нові серії та оновлення',
    'Notifications about new episodes and updates',
  );
  String get newEpisodeNotify => _t('Нові серії', 'New episodes');
  String get newEpisodeNotifyDesc => _t(
    'Сповіщати про нові серії улюблених',
    'Notify about new episodes of favorites',
  );
  String get updateNotify => _t('Оновлення додатку', 'App updates');
  String get updateNotifyDesc =>
      _t('Сповіщати про нові версії', 'Notify about new versions');
  String get updateRequired => _t('Обов\'язкове оновлення', 'Update required');
  String get checkForUpdates => _t('Перевірка оновлень', 'Check for updates');
  String get updateAvailable => _t('Доступне оновлення', 'Update available');
  String get noUpdates => _t('Оновлень не знайдено', 'No updates found');
  String get latestVersion =>
      _t('Встановлена остання версія', 'Latest version installed');
  String get downloadUpdate => _t('Завантажити оновлення', 'Download update');
  String get installing => _t('Встановлення...', 'Installing...');
  String get updateError => _t('Помилка оновлення', 'Update error');
  String get whatsNew => _t('Що нового:', 'What\'s new:');
  String get supportProject => _t('Підтримати проект', 'Support project');
  String get donateDesc => _t(
    'Ваша підтримка допомагає нам розвиватися',
    'Your support helps us grow',
  );
  String get donateButton => _t('Задонатити', 'Donate');
  String get scanQrCode => _t('Відскануйте QR-код', 'Scan QR code');

  // ============================================================================
  // STATISTICS
  // ============================================================================

  String get totalWatched => _t('Всього переглянуто', 'Total watched');
  String get totalTime => _t('Загальний час', 'Total time');
  String get favoriteType => _t('Улюблений тип', 'Favorite type');
  String get topProvider => _t('Топ провайдер', 'Top provider');
  String get weeklyActivity => _t('Активність за тиждень', 'Weekly activity');
  String get byType => _t('За типом', 'By type');
  String get byProvider => _t('За провайдером', 'By provider');
  String get monthly => _t('По місяцях', 'Monthly');
  String hours(int h) => _t('$h год', '${h}h');

  // ============================================================================
  // PRIVATE HELPER
  // ============================================================================

  String _t(String uk, String en) {
    switch (locale) {
      case AppLocale.uk:
        return uk;
      case AppLocale.en:
        return en;
    }
  }
}

/// Localization delegate
///
/// Stateless by design: [load] derives the strings from the [Locale] it is
/// handed, and `WidgetsApp` re-resolves every delegate whenever
/// `MaterialApp.locale` changes. An earlier version also carried an `AppLocale`
/// field that [shouldReload] compared against — dead weight, since the field was
/// never consulted when building the strings, and a trap for anyone who memoised
/// the delegate and silently broke language switching.
class AppStringsDelegate extends LocalizationsDelegate<AppStrings> {
  const AppStringsDelegate();

  @override
  bool isSupported(Locale locale) {
    return ['uk', 'en'].contains(locale.languageCode);
  }

  @override
  Future<AppStrings> load(Locale locale) async {
    return AppStrings(AppLocale.fromCode(locale.languageCode));
  }

  /// Never reloads in place: a new locale makes `WidgetsApp` build a fresh
  /// delegate (and therefore call [load] again), which is the only path that
  /// produces strings for the new language.
  @override
  bool shouldReload(AppStringsDelegate old) => false;
}
