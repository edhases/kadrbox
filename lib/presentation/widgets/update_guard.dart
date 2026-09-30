import 'package:flutter/material.dart';
import 'package:get_it/get_it.dart';
import 'package:permission_handler/permission_handler.dart';
import 'dart:io';

import '../../data/services/update_service.dart';
import '../../data/services/settings_service.dart';
import '../../core/l10n/app_strings.dart';
import '../theme/app_theme.dart';
import '../pages/settings/dialogs/update_dialog.dart';

class UpdateGuard extends StatefulWidget {
  final Widget child;

  const UpdateGuard({super.key, required this.child});

  @override
  State<UpdateGuard> createState() => _UpdateGuardState();
}

class _UpdateGuardState extends State<UpdateGuard> {
  final _updateService = GetIt.instance<UpdateService>();
  final _settings = GetIt.instance<SettingsService>();
  bool _mustUpdate = false;

  @override
  void initState() {
    super.initState();
    _initializeApp();
  }

  Future<void> _initializeApp() async {
    // 1. Request Android permissions if needed
    if (Platform.isAndroid) {
      try {
        await [Permission.storage, Permission.notification].request();
      } catch (_) {}
    }

    // 2. Check for updates in background (non-blocking)
    if (_settings.state.updateNotify) {
      try {
        final (result, _) = await _updateService
            .checkForUpdate()
            .timeout(const Duration(seconds: 5));
        if (mounted) {
          if (result == UpdateCheckResult.forcedUpdate) {
            setState(() {
              _mustUpdate = true;
            });
          } else if (result == UpdateCheckResult.updateAvailable) {
            _showOptionalUpdateDialog();
          }
        }
      } catch (_) {
        // Network timeout or error - app continues to function normally
      }
    }
  }

  void _showOptionalUpdateDialog() {
    final s = AppStrings.of(context);
    showDialog(
      context: context,
      builder: (context) => AlertDialog(
        backgroundColor: AppTheme.surfaceColor,
        title: Text(s.updateAvailable),
        content: Text(s.updateNotifyDesc),
        actions: [
          TextButton(
            onPressed: () => Navigator.pop(context),
            child: Text(s.cancel),
          ),
          ElevatedButton(
            onPressed: () {
              Navigator.pop(context);
              UpdateDialog.show(context, _updateService);
            },
            child: Text(s.downloadUpdate),
          ),
        ],
      ),
    );
  }

  @override
  Widget build(BuildContext context) {
    if (_mustUpdate) {
      return _buildForcedUpdateScreen();
    }

    return widget.child;
  }

  Widget _buildForcedUpdateScreen() {
    final s = AppStrings.of(context);
    return Scaffold(
      backgroundColor: AppTheme.backgroundColor,
      body: Center(
        child: Padding(
          padding: const EdgeInsets.all(30),
          child: Column(
            mainAxisAlignment: MainAxisAlignment.center,
            children: [
              const Icon(
                Icons.system_update_alt,
                size: 80,
                color: Colors.orange,
              ),
              const SizedBox(height: 30),
              Text(
                s.updateRequired,
                style: const TextStyle(
                  fontSize: 24,
                  fontWeight: FontWeight.bold,
                  color: Colors.white,
                ),
                textAlign: TextAlign.center,
              ),
              const SizedBox(height: 16),
              const Text(
                'This version is no longer supported. Please update to continue using Oxide Film.',
                style: TextStyle(color: Colors.white70, fontSize: 16),
                textAlign: TextAlign.center,
              ),
              const SizedBox(height: 40),
              SizedBox(
                width: double.infinity,
                child: ElevatedButton(
                  onPressed: () => UpdateDialog.show(context, _updateService),
                  style: ElevatedButton.styleFrom(
                    backgroundColor: Theme.of(context).colorScheme.primary,
                    foregroundColor: Colors.white,
                    padding: const EdgeInsets.symmetric(vertical: 16),
                    shape: RoundedRectangleBorder(
                      borderRadius: BorderRadius.circular(12),
                    ),
                  ),
                  child: Text(s.downloadUpdate),
                ),
              ),
            ],
          ),
        ),
      ),
    );
  }
}
