import 'dart:math';

/// Application configuration constants
class AppConfig {
  AppConfig._();

  static const String appName = 'Oxide Film';
  static const String appVersion = '1.0.0';

  // Oxide Go Server backend (high-performance lightweight Go + PostgreSQL + Redis via Cloudflare Tunnel)
  static const String defaultServerUrl = 'https://film.oxideteam.pp.ua';
  static String serverBaseUrl = defaultServerUrl;
  static String get serverApiUrl => '$serverBaseUrl/api/v1';
  static String get serverWsUrl {
    final uri = Uri.parse(serverBaseUrl);
    final scheme = uri.scheme == 'https' ? 'wss' : 'ws';
    final port = uri.hasPort ? ':${uri.port}' : '';
    return '$scheme://${uri.host}$port/api/v1/ws/watch-party';
  }

  // PocketBase legacy fallback URL
  static const String backendUrl =
      'https://oxide.skystreamua.space'; // Production server
  static String get adminUrl => '$backendUrl/_/';

  // TMDB API (user should set their own key)
  static const String tmdbBaseUrl = 'https://api.themoviedb.org/3';
  static const String tmdbImageBaseUrl = 'https://image.tmdb.org/t/p';
  static String? tmdbApiKey;

  // User-Agent pool for web scraping
  // IMPORTANT: One UA is selected at app startup and kept for the entire session
  // to avoid triggering anti-bot systems that detect UA rotation
  static const List<String> _userAgents = [
    'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36',
    'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/121.0.0.0 Safari/537.36',
    'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/122.0.0.0 Safari/537.36',
    'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36',
    'Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:121.0) Gecko/20100101 Firefox/121.0',
    'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.2 Safari/605.1.15',
  ];

  /// Session-persistent User-Agent (randomly selected once at app startup)
  /// This prevents anti-bot systems from detecting UA rotation within a session
  static final String userAgent =
      _userAgents[Random().nextInt(_userAgents.length)];

  // Timeouts
  static const Duration connectTimeout = Duration(seconds: 30);
  static const Duration receiveTimeout = Duration(seconds: 30);

  // Cache settings
  static const Duration cacheMaxAge = Duration(hours: 24);
}
