import 'dart:io';

import 'package:flutter/material.dart';
import 'package:get_it/get_it.dart';
import 'package:go_router/go_router.dart';

import 'package:image_picker/image_picker.dart';

import '../../../core/utils/logger.dart';
import '../../../data/services/auth_service.dart';
import '../../../data/services/history_service.dart';
import '../../../data/services/favorites_service.dart';
import '../../theme/app_theme.dart';

/// Profile page for viewing and editing user profile
class ProfilePage extends StatefulWidget {
  const ProfilePage({super.key});

  @override
  State<ProfilePage> createState() => _ProfilePageState();
}

class _ProfilePageState extends State<ProfilePage> {
  final _authService = GetIt.instance<AuthService>();
  final _historyService = GetIt.instance<HistoryService>();
  final _favoritesService = GetIt.instance<FavoritesService>();
  final _nameController = TextEditingController();
  final _bioController = TextEditingController();
  final _picker = ImagePicker();

  bool _isEditing = false;
  bool _isLoading = false;

  @override
  void initState() {
    super.initState();
    _loadProfileData();
    _authService.fetchLinkedProviders();
  }

  void _loadProfileData() {
    final profile = _authService.profile;
    _nameController.text = profile?['display_name'] ?? _authService.displayName;
    _bioController.text = profile?['bio'] ?? '';
  }

  @override
  void dispose() {
    _nameController.dispose();
    _bioController.dispose();
    super.dispose();
  }

  Future<void> _saveProfile() async {
    setState(() => _isLoading = true);

    try {
      await _authService.updateProfile(
        displayName: _nameController.text.trim(),
        bio: _bioController.text.trim().isEmpty
            ? null
            : _bioController.text.trim(),
      );

      if (mounted) {
        setState(() => _isEditing = false);
        ScaffoldMessenger.of(
          context,
        ).showSnackBar(const SnackBar(content: Text('Профіль оновлено')));
      }
    } catch (e) {
      if (mounted) {
        ScaffoldMessenger.of(
          context,
        ).showSnackBar(SnackBar(content: Text('Помилка: $e')));
      }
    } finally {
      if (mounted) {
        setState(() => _isLoading = false);
      }
    }
  }

  Future<void> _signOut() async {
    final confirmed = await showDialog<bool>(
      context: context,
      builder: (context) => AlertDialog(
        backgroundColor: AppTheme.darkCard,
        title: const Text('Вийти з акаунту?'),
        content: const Text(
          'Ви впевнені, що хочете вийти? Локальні дані залишаться на пристрої.',
        ),
        actions: [
          TextButton(
            onPressed: () => Navigator.pop(context, false),
            child: const Text('Скасувати'),
          ),
          ElevatedButton(
            onPressed: () => Navigator.pop(context, true),
            style: ElevatedButton.styleFrom(backgroundColor: Colors.red),
            child: const Text('Вийти'),
          ),
        ],
      ),
    );

    if (confirmed == true) {
      await _authService.signOut();
      if (mounted) {
        context.go('/');
      }
    }
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

  Future<void> _pickImage() async {
    try {
      // Add source selection dialog if needed, for now defaults to gallery
      final XFile? image = await _picker.pickImage(
        source: ImageSource.gallery,
        maxWidth: 512,
        maxHeight: 512,
        imageQuality: 70,
      );

      if (image != null) {
        Logger.i('Image picked: ${image.path}');
        setState(() => _isLoading = true);

        // Verify file exists before attempting upload
        final file = File(image.path);
        if (!await file.exists()) {
          throw Exception('Файл не знайдено за шляхом: ${image.path}');
        }

        await _authService.updateAvatar(image.path);
        if (mounted) {
          ScaffoldMessenger.of(
            context,
          ).showSnackBar(const SnackBar(content: Text('Аватар оновлено')));
        }
      } else {
        Logger.i('Image picking cancelled');
      }
    } catch (e, stack) {
      Logger.e('Error picking/uploading image', error: e, stackTrace: stack);
      if (mounted) {
        ScaffoldMessenger.of(context).showSnackBar(
          SnackBar(
            content: Text('Помилка: $e'),
            duration: const Duration(seconds: 5),
            action: SnackBarAction(label: 'OK', onPressed: () {}),
          ),
        );
      }
    } finally {
      if (mounted) {
        setState(() => _isLoading = false);
      }
    }
  }

  Future<void> _showChangePasswordDialog() async {
    final oldPassController = TextEditingController();
    final newPassController = TextEditingController();
    final confirmPassController = TextEditingController();

    await showDialog(
      context: context,
      builder: (context) => AlertDialog(
        backgroundColor: AppTheme.darkCard,
        title: const Text('Змінити пароль'),
        content: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            TextField(
              controller: oldPassController,
              obscureText: true,
              decoration: const InputDecoration(labelText: 'Старий пароль'),
            ),
            const SizedBox(height: 8),
            TextField(
              controller: newPassController,
              obscureText: true,
              decoration: const InputDecoration(labelText: 'Новий пароль'),
            ),
            const SizedBox(height: 8),
            TextField(
              controller: confirmPassController,
              obscureText: true,
              decoration: const InputDecoration(
                labelText: 'Підтвердіть пароль',
              ),
            ),
          ],
        ),
        actions: [
          TextButton(
            onPressed: () => Navigator.pop(context),
            child: const Text('Скасувати'),
          ),
          ElevatedButton(
            onPressed: () async {
              if (newPassController.text != confirmPassController.text) {
                ScaffoldMessenger.of(context).showSnackBar(
                  const SnackBar(content: Text('Паролі не співпадають')),
                );
                return;
              }
              try {
                Navigator.pop(context); // Close dialog first
                setState(() => _isLoading = true);

                await _authService.changePassword(
                  oldPassController.text,
                  newPassController.text,
                  confirmPassController.text,
                );

                // context.mounted, not this State's `mounted`: this runs inside
                // the dialog's builder, so the State's flag says nothing about
                // whether the element owning `context` is still mounted.
                if (context.mounted) {
                  ScaffoldMessenger.of(context).showSnackBar(
                    const SnackBar(content: Text('Пароль успішно змінено')),
                  );
                }
              } catch (e) {
                if (context.mounted) {
                  ScaffoldMessenger.of(context).showSnackBar(
                    SnackBar(
                      content: Text('Помилка: ${_authService.error ?? e}'),
                    ),
                  );
                }
              } finally {
                if (mounted) setState(() => _isLoading = false);
              }
            },
            child: const Text('Змінити'),
          ),
        ],
      ),
    );
  }

  Future<void> _deleteAccount() async {
    final confirmed = await showDialog<bool>(
      context: context,
      builder: (context) => AlertDialog(
        backgroundColor: AppTheme.darkCard,
        title: const Text('Видалити акаунт?'),
        content: const Text(
          'Ця дія незворотня! Всі ваші дані на сервері будуть видалені. Локальні дані залишаться.',
          style: TextStyle(color: Colors.red),
        ),
        actions: [
          TextButton(
            onPressed: () => Navigator.pop(context, false),
            child: const Text('Скасувати'),
          ),
          ElevatedButton(
            onPressed: () => Navigator.pop(context, true),
            style: ElevatedButton.styleFrom(backgroundColor: Colors.red),
            child: const Text('Видалити назавжди'),
          ),
        ],
      ),
    );

    if (confirmed == true) {
      setState(() => _isLoading = true);
      try {
        await _authService.deleteAccount();
        if (mounted) {
          context.go('/');
        }
      } catch (e) {
        if (mounted) {
          final errText = _authService.error ?? e.toString();
          ScaffoldMessenger.of(context).showSnackBar(
            SnackBar(content: Text('Помилка видалення: $errText')),
          );
        }
      } finally {
        if (mounted) {
          setState(() => _isLoading = false);
        }
      }
    }
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      backgroundColor: AppTheme.backgroundColor,
      appBar: AppBar(
        backgroundColor: Colors.transparent,
        elevation: 0,
        title: const Text('Профіль'),
        actions: [
          if (_authService.isAuthenticated && !_isEditing) ...[
            IconButton(
              icon: const Icon(Icons.sync),
              onPressed: _syncNow,
              tooltip: 'Синхронізувати',
            ),
            IconButton(
              icon: const Icon(Icons.edit),
              onPressed: () => setState(() => _isEditing = true),
              tooltip: 'Редагувати',
            ),
          ],
          if (_isEditing)
            IconButton(
              icon: const Icon(Icons.close),
              onPressed: () {
                setState(() => _isEditing = false);
                _loadProfileData(); // Reset
              },
              tooltip: 'Скасувати',
            ),
        ],
      ),
      body: ListenableBuilder(
        listenable: _authService,
        builder: (context, _) {
          if (!_authService.isAuthenticated) {
            return _buildNotLoggedIn();
          }
          return _buildProfile();
        },
      ),
    );
  }

  Widget _buildNotLoggedIn() {
    return Center(
      child: Padding(
        padding: const EdgeInsets.all(32),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            Icon(
              Icons.account_circle_outlined,
              size: 80,
              color: Colors.white.withValues(alpha: 0.5),
            ),
            const SizedBox(height: 16),
            const Text(
              'Ви не увійшли',
              style: TextStyle(fontSize: 20, fontWeight: FontWeight.bold),
            ),
            const SizedBox(height: 8),
            Text(
              'Увійдіть, щоб синхронізувати історію переглядів та обране між пристроями',
              style: TextStyle(color: Colors.white.withValues(alpha: 0.7)),
              textAlign: TextAlign.center,
            ),
            const SizedBox(height: 24),
            SizedBox(
              width: 200,
              child: ElevatedButton(
                onPressed: () => context.go('/login'),
                style: ElevatedButton.styleFrom(
                  backgroundColor: AppTheme.primaryColor,
                  padding: const EdgeInsets.symmetric(vertical: 12),
                ),
                child: const Text('Увійти'),
              ),
            ),
            const SizedBox(height: 12),
            TextButton(
              onPressed: () => context.go('/register'),
              child: const Text('Створити акаунт'),
            ),
          ],
        ),
      ),
    );
  }

  /// Orange banner for accounts with unverified email.
  Widget _buildVerifyBanner() {
    return Container(
      margin: const EdgeInsets.only(bottom: 16),
      padding: const EdgeInsets.all(16),
      decoration: BoxDecoration(
        color: Colors.orange.withValues(alpha: 0.12),
        borderRadius: BorderRadius.circular(12),
        border: Border.all(color: Colors.orange.withValues(alpha: 0.4)),
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          const Row(
            children: [
              Icon(Icons.mark_email_unread_outlined, color: Colors.orange),
              SizedBox(width: 8),
              Expanded(
                child: Text(
                  'Підтвердіть пошту, інакше акаунт не активний',
                  style: TextStyle(fontWeight: FontWeight.bold),
                ),
              ),
            ],
          ),
          const SizedBox(height: 12),
          Row(
            children: [
              Expanded(
                child: OutlinedButton(
                  onPressed: _isLoading
                      ? null
                      : () => context.go(
                          '/verify-email?email=${Uri.encodeQueryComponent(_authService.userEmail ?? '')}',
                        ),
                  child: const Text('Підтвердити'),
                ),
              ),
              const SizedBox(width: 8),
              IconButton(
                onPressed: _isLoading
                    ? null
                    : () async {
                        setState(() => _isLoading = true);
                        try {
                          await _authService.refreshProfile();
                        } finally {
                          if (mounted) setState(() => _isLoading = false);
                        }
                      },
                icon: const Icon(Icons.refresh),
                tooltip: 'Оновити статус',
              ),
            ],
          ),
        ],
      ),
    );
  }

  Widget _buildProfile() {
    final profile = _authService.profile;

    return SingleChildScrollView(
      padding: const EdgeInsets.all(16),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          if (!_authService.isVerified) _buildVerifyBanner(),
          // ── Compact profile card ─────────────────────────────────────────
          Container(
            padding: const EdgeInsets.all(16),
            decoration: BoxDecoration(
              color: AppTheme.darkCard,
              borderRadius: BorderRadius.circular(16),
              border: Border.all(
                color: Colors.white.withValues(alpha: 0.08),
                width: 1,
              ),
            ),
            child: Row(
              children: [
                // Avatar
                GestureDetector(
                  onTap: _isEditing ? _pickImage : null,
                  child: Stack(
                    children: [
                      CircleAvatar(
                        radius: 36,
                        backgroundColor: AppTheme.primaryColor.withValues(
                          alpha: 0.15,
                        ),
                        backgroundImage: _authService.avatarUrl != null
                            ? NetworkImage(_authService.avatarUrl!)
                            : null,
                        child: _authService.avatarUrl == null
                            ? Text(
                                _authService.displayName[0].toUpperCase(),
                                style: const TextStyle(
                                  fontSize: 28,
                                  fontWeight: FontWeight.bold,
                                  color: AppTheme.primaryColor,
                                ),
                              )
                            : null,
                      ),
                      if (_isEditing)
                        Positioned(
                          right: 0,
                          bottom: 0,
                          child: Container(
                            padding: const EdgeInsets.all(3),
                            decoration: const BoxDecoration(
                              color: AppTheme.primaryColor,
                              shape: BoxShape.circle,
                            ),
                            child: const Icon(
                              Icons.camera_alt,
                              color: Colors.white,
                              size: 14,
                            ),
                          ),
                        ),
                    ],
                  ),
                ),
                const SizedBox(width: 16),
                // Name, email, stats in one compact block
                Expanded(
                  child: Column(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      if (_isEditing)
                        TextField(
                          controller: _nameController,
                          decoration: InputDecoration(
                            labelText: "Ім'я",
                            filled: true,
                            fillColor: AppTheme.darkBackground,
                            border: OutlineInputBorder(
                              borderRadius: BorderRadius.circular(8),
                            ),
                            isDense: true,
                            contentPadding: const EdgeInsets.symmetric(
                              horizontal: 12,
                              vertical: 10,
                            ),
                          ),
                        )
                      else
                        Text(
                          _authService.displayName,
                          style: const TextStyle(
                            fontSize: 18,
                            fontWeight: FontWeight.w600,
                          ),
                        ),
                      const SizedBox(height: 4),
                      Text(
                        _authService.userEmail ?? '',
                        style: const TextStyle(
                          fontSize: 13,
                          color: Color(0xFF8E8E93),
                        ),
                      ),
                      const SizedBox(height: 12),
                      // Stats row inline
                      ListenableBuilder(
                        listenable: Listenable.merge([
                          _historyService,
                          _favoritesService,
                        ]),
                        builder: (context, _) => Row(
                          children: [
                            FutureBuilder<int>(
                              future: _historyService.count,
                              initialData: 0,
                              builder: (context, snapshot) =>
                                  _buildInlineStatItem(
                                    snapshot.data?.toString() ?? '0',
                                    'Переглянуто',
                                  ),
                            ),
                            const SizedBox(width: 20),
                            FutureBuilder<int>(
                              future: _favoritesService.count,
                              initialData: 0,
                              builder: (context, snapshot) =>
                                  _buildInlineStatItem(
                                    snapshot.data?.toString() ?? '0',
                                    'Обране',
                                  ),
                            ),
                            const SizedBox(width: 20),
                            _buildInlineStatItem(
                              _formatDate(profile?['created_at']),
                              'З нами з',
                            ),
                          ],
                        ),
                      ),
                    ],
                  ),
                ),
              ],
            ),
          ),

          // ── Bio ────────────────────────────────────────────────────────
          if (_isEditing) ...[
            const SizedBox(height: 16),
            TextField(
              controller: _bioController,
              maxLines: 3,
              decoration: InputDecoration(
                labelText: 'Про себе',
                hintText: 'Напишіть кілька слів про себе...',
                filled: true,
                fillColor: AppTheme.darkCard,
                border: OutlineInputBorder(
                  borderRadius: BorderRadius.circular(12),
                ),
              ),
            ),
          ] else if (profile?['bio'] != null && profile!['bio'].isNotEmpty) ...[
            const SizedBox(height: 16),
            Container(
              width: double.infinity,
              padding: const EdgeInsets.all(14),
              decoration: BoxDecoration(
                color: AppTheme.darkCard,
                borderRadius: BorderRadius.circular(12),
                border: Border.all(color: Colors.white.withValues(alpha: 0.08)),
              ),
              child: Text(
                profile['bio'],
                style: TextStyle(color: Colors.white.withValues(alpha: 0.7)),
              ),
            ),
          ],

          // ── Save button ───────────────────────────────────────────────
          if (_isEditing) ...[
            const SizedBox(height: 20),
            SizedBox(
              width: double.infinity,
              child: ElevatedButton(
                onPressed: _isLoading ? null : _saveProfile,
                style: ElevatedButton.styleFrom(
                  backgroundColor: AppTheme.primaryColor,
                  padding: const EdgeInsets.symmetric(vertical: 14),
                ),
                child: _isLoading
                    ? const SizedBox(
                        height: 20,
                        width: 20,
                        child: CircularProgressIndicator(
                          strokeWidth: 2,
                          color: Colors.white,
                        ),
                      )
                    : const Text('Зберегти'),
              ),
            ),
            const SizedBox(height: 12),
            Row(
              children: [
                Expanded(
                  child: OutlinedButton.icon(
                    onPressed: _showChangePasswordDialog,
                    icon: const Icon(Icons.lock_reset),
                    label: const Text('Змінити пароль'),
                  ),
                ),
                const SizedBox(width: 12),
                Expanded(
                  child: OutlinedButton.icon(
                    onPressed: _deleteAccount,
                    icon: const Icon(Icons.delete_forever, color: Colors.red),
                    label: const Text(
                      'Видалити акаунт',
                      style: TextStyle(color: Colors.red),
                    ),
                    style: OutlinedButton.styleFrom(
                      side: const BorderSide(color: Colors.red),
                    ),
                  ),
                ),
              ],
            ),
          ] else ...[
            // ── Linked accounts ────────────────────────────────────
            const SizedBox(height: 24),
            _buildLinkedAccounts(),

            // ── Sign out — fixed width, left-aligned ──────────────────
            const SizedBox(height: 24),
            OutlinedButton.icon(
              onPressed: _signOut,
              icon: const Icon(Icons.logout, color: Colors.red, size: 18),
              label: const Text(
                'Вийти з акаунту',
                style: TextStyle(color: Colors.red),
              ),
              style: OutlinedButton.styleFrom(
                side: const BorderSide(color: Colors.red),
                padding: const EdgeInsets.symmetric(
                  horizontal: 20,
                  vertical: 12,
                ),
                minimumSize: Size.zero,
                tapTargetSize: MaterialTapTargetSize.shrinkWrap,
              ),
            ),
          ],

          const SizedBox(height: 32),
        ],
      ),
    );
  }

  Widget _buildInlineStatItem(String value, String label) {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Text(
          value,
          style: const TextStyle(fontSize: 15, fontWeight: FontWeight.bold),
        ),
        Text(
          label,
          style: const TextStyle(fontSize: 11, color: Color(0xFF8E8E93)),
        ),
      ],
    );
  }

  Widget _buildLinkedAccounts() {
    final linked = _authService.linkedProviders;
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        const Text(
          'Підключені соцмережі',
          style: TextStyle(fontSize: 18, fontWeight: FontWeight.bold),
        ),
        const SizedBox(height: 12),
        _buildProviderTile(
          'google',
          'Google',
          Icons.login,
          linked.contains('google'),
        ),
        _buildProviderTile(
          'discord',
          'Discord',
          Icons.chat,
          linked.contains('discord'),
        ),
        _buildProviderTile(
          'telegram',
          'Telegram',
          Icons.send,
          linked.contains('telegram'),
        ),
      ],
    );
  }

  Widget _buildProviderTile(
    String id,
    String name,
    IconData icon,
    bool isLinked,
  ) {
    return Card(
      color: AppTheme.darkCard,
      margin: const EdgeInsets.only(bottom: 8),
      child: ListTile(
        leading: Icon(icon, color: isLinked ? Colors.green : Colors.grey),
        title: Text(name),
        trailing: isLinked
            ? TextButton(
                onPressed: () => _handleUnlink(id),
                child: const Text(
                  'Відключити',
                  style: TextStyle(color: Colors.red),
                ),
              )
            : TextButton(
                onPressed: () => _handleLink(id),
                child: const Text('Підключити'),
              ),
      ),
    );
  }

  Future<void> _handleLink(String provider) async {
    setState(() => _isLoading = true);
    try {
      await _authService.linkSocialAccount(provider);
      if (mounted) {
        ScaffoldMessenger.of(
          context,
        ).showSnackBar(SnackBar(content: Text('Акаунт $provider підключено')));
      }
    } catch (e) {
      if (mounted) {
        ScaffoldMessenger.of(
          context,
        ).showSnackBar(SnackBar(content: Text('Помилка підключення: $e')));
      }
    } finally {
      if (mounted) setState(() => _isLoading = false);
    }
  }

  Future<void> _handleUnlink(String provider) async {
    final confirmed = await showDialog<bool>(
      context: context,
      builder: (context) => AlertDialog(
        backgroundColor: AppTheme.darkCard,
        title: Text('Відключити $provider?'),
        content: const Text('Ви більше не зможете входити через цей акаунт.'),
        actions: [
          TextButton(
            onPressed: () => Navigator.pop(context, false),
            child: const Text('Скасувати'),
          ),
          ElevatedButton(
            onPressed: () => Navigator.pop(context, true),
            style: ElevatedButton.styleFrom(backgroundColor: Colors.red),
            child: const Text('Відключити'),
          ),
        ],
      ),
    );

    if (confirmed == true) {
      setState(() => _isLoading = true);
      try {
        await _authService.unlinkSocialAccount(provider);
        if (mounted) {
          ScaffoldMessenger.of(context).showSnackBar(
            SnackBar(content: Text('Акаунт $provider відключено')),
          );
        }
      } catch (e) {
        if (mounted) {
          ScaffoldMessenger.of(
            context,
          ).showSnackBar(SnackBar(content: Text('Помилка відключення: $e')));
        }
      } finally {
        if (mounted) setState(() => _isLoading = false);
      }
    }
  }

  String _formatDate(String? isoDate) {
    if (isoDate == null) return '-';
    try {
      final date = DateTime.parse(isoDate);
      return '${date.day}.${date.month}.${date.year}';
    } catch (_) {
      return '-';
    }
  }
}
