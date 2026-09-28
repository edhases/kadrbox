import 'dart:io';
import 'package:flutter/foundation.dart';
import 'package:http/http.dart' as http;
import 'package:url_launcher/url_launcher.dart';
import '../../core/utils/logger.dart';
import 'oxide_server_service.dart';
import 'pocketbase_service.dart';

/// Authentication service supporting Oxide Go Server (Chi/PostgreSQL) and PocketBase fallback
///
/// Note: Most cloud sync functionality (watch history, favorites) has been
/// moved to HistoryService and FavoritesService for offline-first approach.
/// This service now only handles authentication.
class AuthService extends ChangeNotifier {
  static const _tag = 'AuthService';

  final OxideServerService _server;
  final PocketBaseService _pb;

  // Cached state
  bool _isLoading = false;
  String? _error;

  // Getters
  bool get isAuthenticated => _server.isAuthenticated || _pb.isAuthenticated;
  String? get userId => _server.userId ?? _pb.userId;
  String? get userEmail => _server.userEmail ?? _pb.userEmail;
  String? get userName => _server.userName ?? _pb.userName;
  bool get isLoading => _isLoading;
  bool get isGuest => !isAuthenticated;
  bool get isVerified => _server.isAuthenticated ? _server.isVerified : (_pb.pb.authStore.record?.data['verified'] == true);
  String? get error => _error;
  List<String> _linkedProviders = [];
  List<String> get linkedProviders => _linkedProviders;

  /// Get current user info (for cloud sync)
  ({String id, String email, String? name})? get currentUser {
    if (!isAuthenticated || userId == null || userEmail == null) return null;
    return (id: userId!, email: userEmail!, name: userName);
  }

  /// Get user profile data (for UI pages)
  Map<String, dynamic>? get profile {
    if (!isAuthenticated) return null;
    if (_server.isAuthenticated) {
      return {
        'display_name': _server.userName ?? displayName,
        'bio': _server.bio ?? '',
        'avatar': _server.avatar ?? '',
        'created_at': _server.user?['created_at'] ?? '',
        'watched_count': 0,
        'favorites_count': 0,
      };
    }
    final record = _pb.pb.authStore.record;
    if (record == null) return null;

    return {
      'display_name': record.data['name'] ?? userName ?? '',
      'bio': record.data['bio'] ?? '',
      'avatar': record.data['avatar'] ?? '',
      'created_at': record.created,
      // These will be computed if needed (from actual data)
      'watched_count': 0,
      'favorites_count': 0,
    };
  }

  /// Get avatar URL from backend
  String? get avatarUrl {
    if (!isAuthenticated) return null;
    if (_server.isAuthenticated && _server.avatar != null && _server.avatar!.isNotEmpty) {
      return _server.avatar;
    }
    final record = _pb.pb.authStore.record;
    if (record == null || record.data['avatar'] == null) return null;

    // PocketBase avatar URL format
    return '${_pb.pb.baseUrl}/api/files/${record.collectionName}/${record.id}/${record.data['avatar']}';
  }

  String get displayName => userName ?? userEmail?.split('@').first ?? 'Гість';

  AuthService(this._server, this._pb) {
    if (isAuthenticated) {
      Logger.i('User already authenticated: $userEmail', tag: _tag);
    }
  }

  /// Sign up with email and password
  Future<void> signUp({
    required String email,
    required String password,
    String? displayName,
  }) async {
    Logger.i('Attempting sign up for: $email', tag: _tag);
    _setLoading(true);
    _error = null;

    try {
      await _server.signUp(
        email: email,
        password: password,
        name: displayName ?? email.split('@').first,
      );
      Logger.i('Sign up successful for: $email', tag: _tag);
      notifyListeners();
    } catch (e) {
      try {
        await _pb.signUp(
          email: email,
          password: password,
          name: displayName ?? email.split('@').first,
        );
        Logger.i('Fallback PB Sign up successful for: $email', tag: _tag);
        notifyListeners();
        return;
      } catch (_) {}
      Logger.e('Sign up failed', tag: _tag, error: e);
      _error = _translateError(e.toString());
      rethrow;
    } finally {
      _setLoading(false);
    }
  }

  /// Sign in with email and password
  Future<void> signIn({required String email, required String password}) async {
    Logger.i('Attempting sign in for: $email', tag: _tag);
    _setLoading(true);
    _error = null;

    try {
      await _server.signIn(email, password);
      Logger.i('Sign in successful for: $email', tag: _tag);
      notifyListeners();
    } catch (e) {
      try {
        await _pb.signIn(email, password);
        Logger.i('Fallback PB Sign in successful for: $email', tag: _tag);
        notifyListeners();
        return;
      } catch (_) {}
      Logger.e('Sign in failed', tag: _tag, error: e);
      _error = _translateError(e.toString());
      rethrow;
    } finally {
      _setLoading(false);
    }
  }

  /// Sign in with social provider (Google, Discord, etc.)
  Future<void> socialSignIn(String provider) async {
    Logger.i('Attempting social sign in for provider: $provider', tag: _tag);
    _setLoading(true);
    _error = null;

    try {
      if (provider == 'discord' || provider == 'telegram') {
        await _server.signInWithOAuthLoopback(provider);
      } else {
        await _pb.authWithOAuth2(provider, (url) async {
          // Launch the authentication URL in an external browser
          if (!await launchUrl(url, mode: LaunchMode.externalApplication)) {
            throw Exception('Не вдалося відкрити вікно авторизації');
          }
        });
      }
      Logger.i('Social sign in successful: $userEmail', tag: _tag);
      notifyListeners();
    } catch (e) {
      Logger.e('Social sign in failed', tag: _tag, error: e);
      _error = _translateError(e.toString());
      rethrow;
    } finally {
      _setLoading(false);
    }
  }

  /// Sign in with Google ID token directly (Oxide Server)
  Future<void> signInWithGoogle(String idToken) async {
    Logger.i('Attempting Google sign in via Oxide Server', tag: _tag);
    _setLoading(true);
    _error = null;

    try {
      await _server.signInWithGoogle(idToken);
      Logger.i('Google sign in successful: $userEmail', tag: _tag);
      notifyListeners();
    } catch (e) {
      Logger.e('Google sign in failed', tag: _tag, error: e);
      _error = _translateError(e.toString());
      rethrow;
    } finally {
      _setLoading(false);
    }
  }

  /// Link social provider to existing account
  Future<void> linkSocialAccount(String provider) async {
    if (!isAuthenticated) throw Exception('Потрібно авторизуватися');
    Logger.i('Attempting to link provider: $provider', tag: _tag);
    _setLoading(true);
    _error = null;

    try {
      await _pb.authWithOAuth2(provider, (url) async {
        if (!await launchUrl(url, mode: LaunchMode.externalApplication)) {
          throw Exception('Не вдалося відкрити вікно авторизації');
        }
      });
      Logger.i('Provider $provider linked successfully', tag: _tag);
      await fetchLinkedProviders();
    } catch (e) {
      Logger.e('Linking failed', tag: _tag, error: e);
      _error = _translateError(e.toString());
      rethrow;
    } finally {
      _setLoading(false);
    }
  }

  /// Unlink social provider
  Future<void> unlinkSocialAccount(String provider) async {
    if (!isAuthenticated) throw Exception('Потрібно авторизуватися');
    Logger.i('Attempting to unlink provider: $provider', tag: _tag);
    _setLoading(true);
    _error = null;

    try {
      await _pb.unlinkExternalAuth(provider);
      Logger.i('Provider $provider unlinked successfully', tag: _tag);
      await fetchLinkedProviders();
    } catch (e) {
      Logger.e('Unlinking failed', tag: _tag, error: e);
      _error = _translateError(e.toString());
      rethrow;
    } finally {
      _setLoading(false);
    }
  }

  /// Fetch list of linked providers
  Future<void> fetchLinkedProviders() async {
    if (!isAuthenticated) return;
    try {
      final auths = await _pb.listExternalAuths();
      _linkedProviders = auths.map((e) => e['provider'] as String).toList();
      notifyListeners();
    } catch (e) {
      Logger.e('Failed to fetch linked providers', tag: _tag, error: e);
    }
  }

  /// Sign out
  Future<void> signOut() async {
    Logger.i('Signing out user: $userEmail', tag: _tag);
    try {
      await _server.signOut();
      if (_pb.isAuthenticated) {
        await _pb.signOut();
      }
      notifyListeners();
      Logger.i('Sign out successful', tag: _tag);
    } catch (e) {
      Logger.e('Sign out failed', tag: _tag, error: e);
      rethrow;
    }
  }

  /// Update user profile
  Future<void> updateProfile({String? displayName, String? bio}) async {
    if (!isAuthenticated || userId == null) {
      throw Exception('Not authenticated');
    }

    Logger.i('Updating profile for: $userEmail', tag: _tag);
    _setLoading(true);
    _error = null;

    try {
      if (_server.isAuthenticated) {
        await _server.updateProfile(name: displayName, bio: bio);
      }
      if (_pb.isAuthenticated && _pb.userId != null) {
        final updateData = <String, dynamic>{};
        if (displayName != null) updateData['name'] = displayName;
        if (bio != null) updateData['bio'] = bio;

        await _pb.pb.collection('users').update(_pb.userId!, body: updateData);
      }
      Logger.i('Profile updated successfully', tag: _tag);
      notifyListeners();
    } catch (e) {
      Logger.e('Profile update failed', tag: _tag, error: e);
      _error = _translateError(e.toString());
      rethrow;
    } finally {
      _setLoading(false);
    }
  }

  /// Request password reset email
  Future<void> resetPassword(String email) async {
    Logger.i('Requesting password reset for: $email', tag: _tag);
    _setLoading(true);
    _error = null;

    try {
      if (_server.isAuthenticated || _server.userEmail != null) {
        await _server.requestPasswordReset(email);
        Logger.i('Password reset email requested via Oxide Server: $email', tag: _tag);
        return;
      }
      await _server.requestPasswordReset(email);
      Logger.i('Password reset email sent to: $email', tag: _tag);
    } catch (e) {
      try {
        await _pb.pb.collection('users').requestPasswordReset(email);
        Logger.i('Fallback PB password reset sent for: $email', tag: _tag);
        return;
      } catch (_) {}
      Logger.e('Password reset failed', tag: _tag, error: e);
      _error = _translateError(e.toString());
      rethrow;
    } finally {
      _setLoading(false);
    }
  }

  /// Request email verification
  Future<void> requestVerification(String email) async {
    Logger.i('Requesting email verification for: $email', tag: _tag);
    _setLoading(true);
    _error = null;

    try {
      await _server.resendVerification(email);
      Logger.i('Verification email sent via Oxide Server: $email', tag: _tag);
    } catch (e) {
      try {
        await _pb.pb.collection('users').requestVerification(email);
        Logger.i('Fallback PB verification email sent: $email', tag: _tag);
        return;
      } catch (_) {}
      Logger.e('Email verification request failed', tag: _tag, error: e);
      _error = _translateError(e.toString());
      rethrow;
    } finally {
      _setLoading(false);
    }
  }

  /// Verify email with token from letter
  Future<void> verifyEmail(String token) async {
    Logger.i('Verifying email token', tag: _tag);
    _setLoading(true);
    _error = null;

    try {
      await _server.verifyEmail(token);
      notifyListeners();
    } catch (e) {
      Logger.e('Email verification failed', tag: _tag, error: e);
      _error = _translateError(e.toString());
      rethrow;
    } finally {
      _setLoading(false);
    }
  }

  /// Refresh authentication token
  Future<bool> refreshAuth() async {
    if (_server.isAuthenticated) {
      final ok = await _server.refreshAuth();
      if (ok) return true;
    }
    if (_pb.isAuthenticated) {
      return await _pb.refreshAuth();
    }
    return false;
  }

  /// Update user avatar
  Future<void> updateAvatar(String filePath) async {
    if (!isAuthenticated || userId == null) {
      throw Exception('Not authenticated');
    }

    Logger.i('Updating avatar for: $userEmail', tag: _tag);
    _setLoading(true);
    _error = null;

    try {
      if (_server.isAuthenticated) {
        await _server.uploadAvatar(filePath);
      } else if (_pb.isAuthenticated && _pb.userId != null) {
        await _pb.pb
            .collection('users')
            .update(
              userId!,
              files: [
                http.MultipartFile.fromBytes(
                  'avatar',
                  await File(filePath).readAsBytes(),
                  filename: 'avatar.jpg',
                ),
              ],
            );
      }
      Logger.i('Avatar updated successfully', tag: _tag);
      notifyListeners();
    } catch (e) {
      Logger.e('Avatar update failed', tag: _tag, error: e);
      _error = _translateError(e.toString());
      rethrow;
    } finally {
      _setLoading(false);
    }
  }

  /// Change password
  Future<void> changePassword(
    String oldPassword,
    String newPassword,
    String newPasswordConfirm,
  ) async {
    if (!isAuthenticated || userId == null) {
      throw Exception('Not authenticated');
    }

    Logger.i('Changing password for: $userEmail', tag: _tag);
    _setLoading(true);
    _error = null;

    try {
      if (_server.isAuthenticated) {
        await _server.changePassword(oldPassword, newPassword);
      }
      if (_pb.isAuthenticated && _pb.userId != null) {
        await _pb.pb
            .collection('users')
            .update(
              userId!,
              body: {
                'oldPassword': oldPassword,
                'password': newPassword,
                'passwordConfirm': newPasswordConfirm,
              },
            );
      }
      Logger.i('Password changed successfully', tag: _tag);
    } catch (e) {
      Logger.e('Password change failed', tag: _tag, error: e);
      _error = _translateError(e.toString());
      rethrow;
    } finally {
      _setLoading(false);
    }
  }

  /// Delete account
  Future<void> deleteAccount() async {
    if (!isAuthenticated || userId == null) {
      throw Exception('Not authenticated');
    }

    Logger.i('Deleting account: $userEmail', tag: _tag);
    _setLoading(true);
    _error = null;

    try {
      if (_server.isAuthenticated) {
        await _server.deleteAccount();
      }
      if (_pb.isAuthenticated && _pb.userId != null) {
        await _pb.pb.collection('users').delete(userId!);
      }
      Logger.i('Account deleted successfully', tag: _tag);
      await signOut();
    } catch (e) {
      Logger.e('Account deletion failed', tag: _tag, error: e);
      _error = _translateError(e.toString());
      rethrow;
    } finally {
      _setLoading(false);
    }
  }

  // ============================================================================
  // HELPERS
  // ============================================================================

  void _setLoading(bool value) {
    _isLoading = value;
    notifyListeners();
  }

  String _translateError(String message) {
    // Translate common PocketBase auth errors to Ukrainian
    final lowerMsg = message.toLowerCase();

    if (lowerMsg.contains('invalid login credentials') ||
        lowerMsg.contains('invalid username or password')) {
      return 'Невірний email або пароль';
    }
    if (lowerMsg.contains('email not confirmed') ||
        lowerMsg.contains('not verified')) {
      return 'Електронна пошта не підтверджена. Перевірте свою скриньку.';
    }
    if (lowerMsg.contains('failed to send password reset')) {
      return 'Не вдалося надіслати лист для скидання пароля. Перевірте налаштування пошти (SMTP).';
    }
    if (lowerMsg.contains('identity already exists')) {
      return 'Цей email уже використовується іншим акаунтом.';
    }
    if (lowerMsg.contains('missing or invalid') ||
        lowerMsg.contains('validation failed')) {
      return 'Помилка валідації. Перевірте введені дані.';
    }
    if (lowerMsg.contains('already exists') ||
        lowerMsg.contains('user already registered')) {
      return 'Користувач з таким email вже існує';
    }
    if (lowerMsg.contains('password') && lowerMsg.contains('at least')) {
      return 'Пароль має бути щонайменше 6 символів';
    }
    if (lowerMsg.contains('invalid email')) {
      return 'Невірний формат email';
    }
    if (lowerMsg.contains('rate limit') || lowerMsg.contains('too many')) {
      return 'Забагато спроб. Спробуйте пізніше';
    }

    return message;
  }
}
