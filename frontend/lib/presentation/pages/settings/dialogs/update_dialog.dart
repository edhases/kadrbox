import 'package:flutter/material.dart';
import 'package:go_router/go_router.dart';
import '../../../../core/l10n/app_strings.dart';
import '../../../../data/services/update_service.dart';
import '../../../../domain/entities/update_info.dart';
import '../../../theme/app_theme.dart';

class UpdateDialog extends StatefulWidget {
  final UpdateService updateService;

  const UpdateDialog({super.key, required this.updateService});

  static Future<void> show(BuildContext context, UpdateService updateService) {
    return showDialog(
      context: context,
      barrierDismissible: false,
      builder: (context) => UpdateDialog(updateService: updateService),
    );
  }

  @override
  State<UpdateDialog> createState() => _UpdateDialogState();
}

class _UpdateDialogState extends State<UpdateDialog> {
  UpdateCheckResult _status = UpdateCheckResult.upToDate;
  UpdateInfo? _updateInfo;
  bool _isChecking = true;
  bool _isDownloading = false;
  double _progress = 0;
  String? _error;

  @override
  void initState() {
    super.initState();
    _checkUpdate();
  }

  Future<void> _checkUpdate() async {
    setState(() {
      _isChecking = true;
      _error = null;
    });

    try {
      final (result, info) = await widget.updateService.checkForUpdate(
        force: true,
      );
      if (mounted) {
        setState(() {
          _status = result;
          _updateInfo = info;
          _isChecking = false;
        });
      }
    } catch (e) {
      if (mounted) {
        setState(() {
          _status = UpdateCheckResult.error;
          _isChecking = false;
          _error = e.toString();
        });
      }
    }
  }

  Future<void> _startUpdate() async {
    if (_updateInfo == null) return;

    setState(() {
      _isDownloading = true;
      _progress = 0;
    });

    try {
      final file = await widget.updateService.downloadUpdate(
        _updateInfo!,
        onProgress: (received, total) {
          if (total > 0 && mounted) {
            setState(() {
              _progress = received / total;
            });
          }
        },
      );

      if (file != null && mounted) {
        setState(() {
          _isDownloading = false;
        });
        await widget.updateService.installUpdate(file);
        if (mounted) context.pop();
      } else if (mounted) {
        setState(() {
          _isDownloading = false;
          _error = 'Failed to download update';
        });
      }
    } catch (e) {
      if (mounted) {
        setState(() {
          _isDownloading = false;
          _error = e.toString();
        });
      }
    }
  }

  @override
  Widget build(BuildContext context) {
    final s = AppStrings.of(context);
    final theme = Theme.of(context);

    return AlertDialog(
      backgroundColor: AppTheme.surfaceColor,
      shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(16)),
      title: Text(
        _isChecking
            ? s.checkForUpdates
            : (_updateInfo != null ? s.updateAvailable : s.checkForUpdates),
        style: const TextStyle(fontWeight: FontWeight.bold),
      ),
      content: SizedBox(
        width: 400,
        child: Column(
          mainAxisSize: MainAxisSize.min,
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            if (_isChecking) ...[
              const Center(
                child: Padding(
                  padding: EdgeInsets.symmetric(vertical: 20),
                  child: CircularProgressIndicator(),
                ),
              ),
              Center(child: Text(s.loading)),
            ] else if (_error != null) ...[
              Text(s.updateError, style: const TextStyle(color: Colors.red)),
              const SizedBox(height: 8),
              Text(
                _error!,
                style: TextStyle(
                  color: Colors.white.withValues(alpha: 0.6),
                  fontSize: 12,
                ),
              ),
            ] else if (_isDownloading) ...[
              Text(s.downloadingStatus),
              const SizedBox(height: 16),
              LinearProgressIndicator(
                value: _progress > 0 ? _progress : null,
                backgroundColor: theme.colorScheme.primary.withValues(
                  alpha: 0.1,
                ),
                borderRadius: BorderRadius.circular(4),
              ),
              const SizedBox(height: 8),
              Center(
                child: Text(
                  '${(_progress * 100).toStringAsFixed(1)}%',
                  style: const TextStyle(
                    fontSize: 12,
                    fontWeight: FontWeight.bold,
                  ),
                ),
              ),
            ] else if (_updateInfo != null) ...[
              Row(
                children: [
                  const Icon(Icons.info_outline, size: 16, color: Colors.blue),
                  const SizedBox(width: 8),
                  Text(
                    '${_updateInfo!.forCurrentPlatform?.versionName ?? ""} (${_updateInfo!.forCurrentPlatform?.versionCode ?? ""})',
                    style: const TextStyle(fontWeight: FontWeight.bold),
                  ),
                ],
              ),
              const SizedBox(height: 16),
              if (_updateInfo!.forCurrentPlatform?.releaseNotes.isNotEmpty ??
                  false) ...[
                Text(
                  s.whatsNew,
                  style: const TextStyle(
                    fontWeight: FontWeight.bold,
                    fontSize: 14,
                  ),
                ),
                const SizedBox(height: 4),
                Container(
                  constraints: const BoxConstraints(maxHeight: 200),
                  padding: const EdgeInsets.all(12),
                  decoration: BoxDecoration(
                    color: Colors.white.withValues(alpha: 0.05),
                    borderRadius: BorderRadius.circular(8),
                  ),
                  child: SingleChildScrollView(
                    child: Text(
                      _updateInfo!.forCurrentPlatform?.releaseNotes ?? "",
                      style: TextStyle(
                        color: Colors.white.withValues(alpha: 0.8),
                        fontSize: 13,
                      ),
                    ),
                  ),
                ),
              ],
            ] else ...[
              Row(
                children: [
                  const Icon(Icons.check_circle, color: Colors.green),
                  const SizedBox(width: 8),
                  Text(s.latestVersion),
                ],
              ),
            ],
          ],
        ),
      ),
      actions: _isChecking || _isDownloading
          ? []
          : [
              TextButton(onPressed: () => context.pop(), child: Text(s.close)),
              if (_updateInfo != null)
                ElevatedButton(
                  onPressed: _startUpdate,
                  style: ElevatedButton.styleFrom(
                    backgroundColor: theme.colorScheme.primary,
                    foregroundColor: Colors.white,
                  ),
                  child: Text(s.downloadUpdate),
                ),
              if (_status == UpdateCheckResult.upToDate ||
                  _status == UpdateCheckResult.error)
                TextButton(onPressed: _checkUpdate, child: Text(s.retry)),
            ],
    );
  }
}
