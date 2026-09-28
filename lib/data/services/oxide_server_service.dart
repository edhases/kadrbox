import 'dart:async';
import 'dart:convert';
import 'dart:io';
import 'package:shared_preferences/shared_preferences.dart';
import '../../core/config/app_config.dart';
import '../../core/network/api_client.dart';
import '../../core/utils/logger.dart';

/// Service for communication with Oxide Go Backend
/// (Go Chi + PostgreSQL 16 + Redis Pub/Sub WebSocket)
class OxideServerService {
  static const _tag = 'OxideServerService';
  static const _tokenKey = 'oxide_jwt_access_token';
  static const _refreshKey = 'oxide_jwt_refresh_token';
  static const _userKey = 'oxide_user_json';

  final SharedPreferences _prefs;
  final ApiClient _apiClient;

  String? _accessToken;
  String? _refreshToken;
  Map<String, dynamic>? _user;

  OxideServerService(this._prefs, this._apiClient) {
    _loadState();
  }

  void _loadState() {
    _accessToken = _prefs.getString(_tokenKey);
    _refreshToken = _prefs.getString(_refreshKey);
    final userJson = _prefs.getString(_userKey);
    if (userJson != null) {
      try {
        _user = jsonDecode(userJson) as Map<String, dynamic>;
      } catch (e) {
        Logger.w('Failed to parse cached user JSON: $e', tag: _tag);
      }
    }

    if (_accessToken != null && _accessToken!.isNotEmpty) {
      _apiClient.setAuthToken(_accessToken);
      Logger.i('Restored auth session for: $userEmail', tag: _tag);
    }
  }

  // Quick access properties
  bool get isAuthenticated => _accessToken != null && _accessToken!.isNotEmpty;
  String? get userId => _user?['id'];
  String? get userEmail => _user?['email'];
  String? get userName => _user?['name'];
  String? get avatar => _user?['avatar'];
  String? get bio => _user?['bio'];
  Map<String, dynamic>? get user => _user;
  String? get accessToken => _accessToken;

  // ===========================================================================
  // Authentication & Profile
  // ===========================================================================

  /// Sign in with email and password
  Future<void> signIn(String email, String password) async {
    final url = '${AppConfig.serverApiUrl}/auth/login';
    try {
      final res = await _apiClient.post(url, data: {
        'email': email.trim(),
        'password': password,
      });

      if (res is Map) {
        await _saveAuthData(res);
        Logger.i('User signed in successfully: $email', tag: _tag);
      } else {
        throw Exception('Неочікувана відповідь від сервера');
      }
    } catch (e) {
      Logger.e('Sign in failed for $email', tag: _tag, error: e);
      rethrow;
    }
  }

  /// Sign up with email, password, and display name
  Future<void> signUp({
    required String email,
    required String password,
    required String name,
  }) async {
    final url = '${AppConfig.serverApiUrl}/auth/register';
    try {
      final res = await _apiClient.post(url, data: {
        'email': email.trim(),
        'password': password,
        'name': name.trim(),
      });

      if (res is Map) {
        await _saveAuthData(res);
        Logger.i('User registered successfully: $email', tag: _tag);
      } else {
        throw Exception('Неочікувана відповідь від сервера');
      }
    } catch (e) {
      Logger.e('Sign up failed for $email', tag: _tag, error: e);
      rethrow;
    }
  }

  /// Sign out and clear stored tokens
  Future<void> signOut() async {
    _accessToken = null;
    _refreshToken = null;
    _user = null;
    _apiClient.setAuthToken(null);
    await _prefs.remove(_tokenKey);
    await _prefs.remove(_refreshKey);
    await _prefs.remove(_userKey);
    Logger.i('User signed out and tokens cleared', tag: _tag);
  }

  /// Refresh auth access token
  Future<bool> refreshAuth() async {
    if (_refreshToken == null || _refreshToken!.isEmpty) return false;

    final url = '${AppConfig.serverApiUrl}/auth/refresh';
    try {
      final res = await _apiClient.post(url, data: {
        'refresh_token': _refreshToken,
      });

      if (res is Map && res['token'] != null) {
        _accessToken = res['token'] as String;
        await _prefs.setString(_tokenKey, _accessToken!);
        if (res['refresh_token'] != null) {
          _refreshToken = res['refresh_token'] as String;
          await _prefs.setString(_refreshKey, _refreshToken!);
        }
        _apiClient.setAuthToken(_accessToken);
        Logger.d('Auth token refreshed successfully', tag: _tag);
        return true;
      }
      return false;
    } catch (e) {
      Logger.w('Failed to refresh token: $e', tag: _tag);
      return false;
    }
  }

  /// Get current user profile from server
  Future<Map<String, dynamic>?> fetchMe() async {
    if (!isAuthenticated) return null;
    final url = '${AppConfig.serverApiUrl}/auth/me';
    try {
      final res = await _apiClient.getJson(url);
      if (res.isNotEmpty) {
        _user = res;
        await _prefs.setString(_userKey, jsonEncode(_user));
        return _user;
      }
    } catch (e) {
      Logger.w('Failed to fetch user profile: $e', tag: _tag);
    }
    return _user;
  }

  /// Update user profile
  Future<void> updateProfile({String? name, String? bio, String? avatar}) async {
    if (!isAuthenticated) throw Exception('Потрібно авторизуватися');
    final url = '${AppConfig.serverApiUrl}/auth/profile';

    final body = <String, dynamic>{};
    if (name != null) body['name'] = name;
    if (bio != null) body['bio'] = bio;
    if (avatar != null) body['avatar'] = avatar;

    final res = await _apiClient.put(url, data: body);
    if (res is Map) {
      _user = Map<String, dynamic>.from(res);
      await _prefs.setString(_userKey, jsonEncode(_user));
      Logger.i('Profile updated', tag: _tag);
    }
  }

  Future<void> _saveAuthData(Map<dynamic, dynamic> res) async {
    _accessToken = res['token'] as String?;
    _refreshToken = res['refresh_token'] as String?;

    if (res['user'] != null && res['user'] is Map) {
      _user = Map<String, dynamic>.from(res['user'] as Map);
      await _prefs.setString(_userKey, jsonEncode(_user));
    }

    if (_accessToken != null) {
      await _prefs.setString(_tokenKey, _accessToken!);
      _apiClient.setAuthToken(_accessToken);
    }
    if (_refreshToken != null) {
      await _prefs.setString(_refreshKey, _refreshToken!);
    }
  }

  // ===========================================================================
  // Cloud Sync: Watch History
  // ===========================================================================

  /// Get watch history from server
  Future<List<Map<String, dynamic>>> getHistory({int limit = 100, int offset = 0}) async {
    if (!isAuthenticated) return [];
    final url = '${AppConfig.serverApiUrl}/sync/history';
    try {
      final list = await _apiClient.getJsonList(url, queryParameters: {
        'limit': limit,
        'offset': offset,
      });
      return list.map((e) => Map<String, dynamic>.from(e as Map)).toList();
    } catch (e) {
      Logger.w('Failed to get history from server: $e', tag: _tag);
      return [];
    }
  }

  /// Save progress / update watch history item on server
  Future<void> saveHistoryProgress({
    required String mediaId,
    required String providerId,
    required String title,
    String? posterUrl,
    int? year,
    required String mediaType,
    required int positionMs,
    required int durationMs,
    int? season,
    int? episode,
    String? episodeTitle,
    String? lastStreamUrl,
    String? voiceover,
    DateTime? watchedAt,
  }) async {
    if (!isAuthenticated) return;
    final url = '${AppConfig.serverApiUrl}/sync/history';
    final payload = {
      'mediaId': mediaId,
      'providerId': providerId,
      'title': title,
      'posterUrl': posterUrl,
      'year': year,
      'mediaType': mediaType,
      'positionMs': positionMs,
      'durationMs': durationMs,
      'season': season,
      'episode': episode,
      'episodeTitle': episodeTitle,
      'lastStreamUrl': lastStreamUrl,
      'voiceover': voiceover,
      'watchedAt': (watchedAt ?? DateTime.now()).toUtc().toIso8601String(),
    };

    try {
      await _apiClient.post(url, data: payload);
    } catch (e) {
      Logger.w('Failed to sync history item to server: $e', tag: _tag);
    }
  }

  /// Get continue watching items from server
  Future<List<Map<String, dynamic>>> getContinueWatching({int limit = 20}) async {
    if (!isAuthenticated) return [];
    final url = '${AppConfig.serverApiUrl}/sync/continue-watching';
    try {
      final list = await _apiClient.getJsonList(url, queryParameters: {
        'limit': limit,
      });
      return list.map((e) => Map<String, dynamic>.from(e as Map)).toList();
    } catch (e) {
      Logger.w('Failed to get continue-watching from server: $e', tag: _tag);
      return [];
    }
  }

  // ===========================================================================
  // Cloud Sync: Favorites
  // ===========================================================================

  /// Get favorites from server
  Future<List<Map<String, dynamic>>> getFavorites({int limit = 200, int offset = 0}) async {
    if (!isAuthenticated) return [];
    final url = '${AppConfig.serverApiUrl}/sync/favorites';
    try {
      final list = await _apiClient.getJsonList(url, queryParameters: {
        'limit': limit,
        'offset': offset,
      });
      return list.map((e) => Map<String, dynamic>.from(e as Map)).toList();
    } catch (e) {
      Logger.w('Failed to get favorites from server: $e', tag: _tag);
      return [];
    }
  }

  /// Toggle favorite on server
  Future<bool> toggleFavorite({
    required String mediaId,
    required String providerId,
    required String title,
    String? posterUrl,
    int? year,
    required String mediaType,
    double? rating,
    String? ratingSource,
  }) async {
    if (!isAuthenticated) return false;
    final url = '${AppConfig.serverApiUrl}/sync/favorites/toggle';
    try {
      final res = await _apiClient.post(url, data: {
        'mediaId': mediaId,
        'providerId': providerId,
        'title': title,
        'posterUrl': posterUrl,
        'year': year,
        'mediaType': mediaType,
        'rating': rating,
        'ratingSource': ratingSource,
      });
      if (res is Map && res['is_favorite'] != null) {
        return res['is_favorite'] as bool;
      }
    } catch (e) {
      Logger.w('Failed to toggle favorite on server: $e', tag: _tag);
    }
    return false;
  }

  // ===========================================================================
  // Watch Party: WebSocket
  // ===========================================================================

  /// Connect to Watch Party room via WebSocket
  Future<WebSocket> connectWatchParty({
    required String roomCode,
    required String userId,
    required String userName,
  }) async {
    final baseUrl = AppConfig.serverWsUrl;
    final uri = Uri.parse(baseUrl).replace(queryParameters: {
      'room': roomCode,
      'user_id': userId,
      'user_name': userName,
    });

    Logger.i('Connecting to Watch Party WebSocket: $uri', tag: _tag);
    final ws = await WebSocket.connect(uri.toString());
    Logger.i('Connected to Watch Party WebSocket', tag: _tag);
    return ws;
  }
}
