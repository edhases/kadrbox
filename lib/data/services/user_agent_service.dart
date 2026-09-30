import 'package:http/http.dart' as http;
import 'package:shared_preferences/shared_preferences.dart';
import '../../core/utils/logger.dart';

class UserAgentService {
  static const String _uaKey = 'cached_user_agents';
  static const String _lastUpdateKey = 'ua_last_update';
  static const String _techblogUrl =
      'https://techblog.willshouse.com/2012/01/03/most-common-user-agents/';
  static const String _fallbackUrl = 'https://www.useragents.me/';

  final http.Client _client;
  final SharedPreferences? _prefs;

  UserAgentService({http.Client? client, required SharedPreferences prefs})
    : _client = client ?? http.Client(),
      _prefs = prefs;

  /// Stateless fallback instance for early init when SharedPreferences
  /// is not available yet (e.g. inside ApiClient constructor).
  UserAgentService.fallback({http.Client? client})
    : _client = client ?? http.Client(),
      _prefs = null;

  List<String> _cachedUAs = [];

  Future<void> initialize() async {
    final prefs = _prefs;
    if (prefs == null) return;
    final cached = prefs.getStringList(_uaKey);
    if (cached != null && cached.isNotEmpty) {
      _cachedUAs = cached;
      Logger.d('Loaded ${_cachedUAs.length} User-Agents from cache', tag: 'UA');
    }

    // Update if older than 24 hours
    final lastUpdate = prefs.getInt(_lastUpdateKey) ?? 0;
    final now = DateTime.now().millisecondsSinceEpoch;
    if (now - lastUpdate > 24 * 60 * 60 * 1000 || _cachedUAs.isEmpty) {
      fetchLatestUserAgents(); // Run in background
    }
  }

  List<String> get userAgents => _cachedUAs;

  // Hardcoded fallback UAs (Modern, diverse)
  static const List<String> _fallbackUAs = [
    'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/122.0.0.0 Safari/537.36',
    'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/122.0.0.0 Safari/537.36',
    'Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:123.0) Gecko/20100101 Firefox/123.0',
    'Mozilla/5.0 (Expected; Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.2 Safari/605.1.15',
    'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/122.0.0.0 Safari/537.36 Edg/122.0.0.0',
    'Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/122.0.0.0 Safari/537.36',
  ];

  String getRandomUA() {
    if (_cachedUAs.isEmpty) {
      return (_fallbackUAs.toList()..shuffle()).first;
    }
    return (_cachedUAs.toList()..shuffle()).first;
  }

  String getChromeUserAgent() {
    if (_cachedUAs.isEmpty) {
      return _fallbackUAs.firstWhere(
        (ua) => ua.contains('Chrome') && !ua.contains('Mobile'),
        orElse: () => _fallbackUAs.first,
      );
    }

    // Try to find a Desktop Chrome UA
    try {
      return _cachedUAs.firstWhere(
        (ua) => ua.contains('Chrome') && !ua.contains('Mobile'),
      );
    } catch (_) {
      // Fallback to any Chrome
      try {
        return _cachedUAs.firstWhere((ua) => ua.contains('Chrome'));
      } catch (_) {
        // Fallback to any
        return getRandomUA();
      }
    }
  }

  Future<void> fetchLatestUserAgents() async {
    // Try primary source
    bool success = await _fetchFromUrl(_techblogUrl);

    // If failed, try fallback
    if (!success) {
      Logger.d('Primary UA source failed, trying fallback...', tag: 'UA');
      // For useragents.me, we need a specific parser
      success = await _fetchFromUserAgentsMe();
    }

    if (success) {
      await _prefs?.setInt(
        _lastUpdateKey,
        DateTime.now().millisecondsSinceEpoch,
      );
    }
  }

  Future<bool> _fetchFromUserAgentsMe() async {
    try {
      Logger.d('Fetching User-Agents from $_fallbackUrl', tag: 'UA');
      final response = await _client.get(
        Uri.parse(_fallbackUrl),
        headers: {
          'User-Agent':
              'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/122.0.0.0 Safari/537.36',
          'Accept':
              'text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8',
        },
      );

      if (response.statusCode == 200) {
        final html = response.body;
        // Regex to find UAs in textareas (more reliable for useragents.me)
        final taRegex = RegExp(
          r'<textarea class="form-control ua-textarea">([^<]+)</textarea>',
        );
        final matches = taRegex.allMatches(html);

        final newUAs = matches
            .map((m) => m.group(1)!.trim())
            .where((ua) => ua.length > 50 && ua.length < 300)
            .toSet()
            .toList();

        if (newUAs.length > 5) {
          _cachedUAs = newUAs;
          await _prefs?.setStringList(_uaKey, _cachedUAs);
          Logger.d(
            'Successfully updated User-Agents: ${newUAs.length} found from $_fallbackUrl',
            tag: 'UA',
          );
          return true;
        }
      }
      Logger.w(
        'Fetch failed from $_fallbackUrl: HTTP ${response.statusCode}',
        tag: 'UA',
      );
      return false;
    } catch (e) {
      Logger.w('Error fetching from $_fallbackUrl: $e', tag: 'UA');
      return false;
    }
  }

  Future<bool> _fetchFromUrl(String url) async {
    try {
      Logger.d('Fetching User-Agents from $url', tag: 'UA');
      final response = await _client.get(
        Uri.parse(url),
        headers: {
          'User-Agent':
              'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/122.0.0.0 Safari/537.36',
          'Accept':
              'text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8',
        },
      );

      if (response.statusCode == 200) {
        final html = response.body;
        // Regex to find UAs (standard list format)
        final uaRegex = RegExp(r'Mozilla/5\.0 [^"<> \n\r\t\x27]+');
        final matches = uaRegex.allMatches(html);

        final newUAs = matches
            .map((m) => m.group(0)!.trim())
            .where((ua) => ua.length > 50 && ua.length < 300)
            .toSet()
            .toList();

        if (newUAs.length > 5) {
          _cachedUAs = newUAs;
          await _prefs?.setStringList(_uaKey, _cachedUAs);
          Logger.d(
            'Successfully updated User-Agents: ${newUAs.length} found from $url',
            tag: 'UA',
          );
          return true;
        }
      }
      Logger.w(
        'Fetch failed from $url: HTTP ${response.statusCode}',
        tag: 'UA',
      );
      return false;
    } catch (e) {
      Logger.w('Error fetching from $url: $e', tag: 'UA');
      return false;
    }
  }
}
