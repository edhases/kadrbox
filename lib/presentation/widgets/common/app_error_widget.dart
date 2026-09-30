import 'package:flutter/material.dart';
import 'package:flutter/services.dart';

import '../../theme/app_theme.dart';

/// Unified error display widget for consistent error UI across the app.
///
/// Features:
/// - Customizable icon, title, and message
/// - Optional retry button
/// - Optional back button
/// - Alternative actions (e.g., try different stream)
/// - Optional collapsible "Деталі" diagnostic block ([details]) with a
///   «Копіювати діагностику» action
class AppErrorWidget extends StatefulWidget {
  final String? title;
  final String? message;
  final VoidCallback? onRetry;
  final VoidCallback? onBack;
  final List<Widget>? alternativeActions;
  final IconData icon;
  final Color? iconColor;

  /// Monospace diagnostic text shown inside the collapsed disclosure.
  final String? details;

  /// Invoked by the «Копіювати діагностику» button.
  final VoidCallback? onCopyDetails;

  /// Label of the copy button.
  final String copyLabel;

  const AppErrorWidget({
    super.key,
    this.title,
    this.message,
    this.onRetry,
    this.onBack,
    this.alternativeActions,
    this.icon = Icons.error_outline,
    this.iconColor,
    this.details,
    this.onCopyDetails,
    this.copyLabel = 'Копіювати діагностику',
  });

  /// Creates an error widget for network/loading failures
  factory AppErrorWidget.loading({
    String? message,
    VoidCallback? onRetry,
    VoidCallback? onBack,
  }) {
    return AppErrorWidget(
      title: 'Помилка завантаження',
      message: message,
      onRetry: onRetry,
      onBack: onBack,
      icon: Icons.cloud_off,
    );
  }

  /// Creates an error widget for playback failures.
  ///
  /// [details] should carry the full diagnostic block and [onCopyDetails]
  /// should copy it to the clipboard.
  factory AppErrorWidget.playback({
    String? title,
    String? message,
    VoidCallback? onRetry,
    VoidCallback? onBack,
    List<Widget>? alternativeActions,
    String? details,
    VoidCallback? onCopyDetails,
  }) {
    return AppErrorWidget(
      title: title ?? 'Помилка відтворення',
      message: message ?? 'Не вдалося завантажити відео',
      onRetry: onRetry,
      onBack: onBack,
      alternativeActions: alternativeActions,
      details: details,
      onCopyDetails: onCopyDetails,
      icon: Icons.videocam_off,
      iconColor: Colors.red,
    );
  }

  /// Creates a generic error widget
  factory AppErrorWidget.generic({String? message, VoidCallback? onRetry}) {
    return AppErrorWidget(
      title: 'Щось пішло не так',
      message: message,
      onRetry: onRetry,
      icon: Icons.warning_amber_rounded,
      iconColor: Colors.orange,
    );
  }

  @override
  State<AppErrorWidget> createState() => _AppErrorWidgetState();
}

class _AppErrorWidgetState extends State<AppErrorWidget> {
  bool _detailsOpen = false;

  Future<void> _copyDetails(BuildContext context) async {
    final details = widget.details;
    if (details == null || details.isEmpty) return;
    await Clipboard.setData(ClipboardData(text: details));
    if (!context.mounted) return;
    // onCopyDetails lets the owner also fire its own logging/snackbar.
    widget.onCopyDetails?.call();
    ScaffoldMessenger.of(context).showSnackBar(
      const SnackBar(
        content: Text('Діагностику скопійовано'),
        duration: Duration(seconds: 2),
      ),
    );
  }

  @override
  Widget build(BuildContext context) {
    return SingleChildScrollView(
      child: Center(
        child: Padding(
          padding: const EdgeInsets.all(32),
          child: Column(
            mainAxisSize: MainAxisSize.min,
            children: [
              // Icon
              Icon(
                widget.icon,
                size: 64,
                color: widget.iconColor ?? AppTheme.errorColor,
              ),
              const SizedBox(height: 16),

              // Title
              if (widget.title != null)
                Text(
                  widget.title!,
                  style: Theme.of(context).textTheme.headlineSmall?.copyWith(
                    fontWeight: FontWeight.bold,
                  ),
                  textAlign: TextAlign.center,
                ),

              // Message
              if (widget.message != null) ...[
                const SizedBox(height: 8),
                Text(
                  widget.message!,
                  style: Theme.of(context).textTheme.bodyMedium?.copyWith(
                    color: AppTheme.textSecondary,
                  ),
                  textAlign: TextAlign.center,
                ),
              ],

              // Action buttons
              if (widget.onRetry != null || widget.onBack != null) ...[
                const SizedBox(height: 24),
                Row(
                  mainAxisSize: MainAxisSize.min,
                  children: [
                    if (widget.onRetry != null)
                      ElevatedButton.icon(
                        onPressed: widget.onRetry,
                        icon: const Icon(Icons.refresh),
                        label: const Text('Спробувати знову'),
                        style: ElevatedButton.styleFrom(
                          backgroundColor: AppTheme.primaryColor,
                          foregroundColor: Colors.white,
                        ),
                      ),
                    if (widget.onRetry != null && widget.onBack != null)
                      const SizedBox(width: 12),
                    if (widget.onBack != null)
                      OutlinedButton.icon(
                        onPressed: widget.onBack,
                        icon: const Icon(Icons.arrow_back),
                        label: const Text('Назад'),
                        style: OutlinedButton.styleFrom(
                          foregroundColor: AppTheme.textPrimary,
                          side: const BorderSide(color: AppTheme.borderColor),
                        ),
                      ),
                  ],
                ),
              ],

              // Alternative actions
              if (widget.alternativeActions != null &&
                  widget.alternativeActions!.isNotEmpty) ...[
                const SizedBox(height: 24),
                Text(
                  'Або спробуйте:',
                  style: TextStyle(color: AppTheme.textMuted, fontSize: 12),
                ),
                const SizedBox(height: 8),
                Wrap(
                  spacing: 8,
                  runSpacing: 8,
                  alignment: WrapAlignment.center,
                  children: widget.alternativeActions!,
                ),
              ],

              // Collapsed diagnostics disclosure
              if (widget.details != null && widget.details!.isNotEmpty) ...[
                const SizedBox(height: 24),
                Align(
                  alignment: Alignment.center,
                  child: TextButton.icon(
                    onPressed: () =>
                        setState(() => _detailsOpen = !_detailsOpen),
                    icon: Icon(
                      _detailsOpen ? Icons.expand_less : Icons.expand_more,
                      size: 18,
                    ),
                    label: Text(_detailsOpen ? 'Деталі' : 'Показати деталі'),
                    style: TextButton.styleFrom(
                      foregroundColor: AppTheme.textSecondary,
                    ),
                  ),
                ),
                if (_detailsOpen) ...[
                  const SizedBox(height: 8),
                  ConstrainedBox(
                    constraints: const BoxConstraints(maxWidth: 640),
                    child: Container(
                      width: double.infinity,
                      padding: const EdgeInsets.all(12),
                      decoration: BoxDecoration(
                        color: Colors.black.withValues(alpha: 0.6),
                        borderRadius: BorderRadius.circular(8),
                        border: Border.all(color: AppTheme.borderColor),
                      ),
                      child: SelectableText(
                        widget.details!,
                        style: const TextStyle(
                          fontFamily: 'monospace',
                          fontFamilyFallback: ['Consolas', 'Courier New'],
                          fontSize: 11,
                          height: 1.4,
                          color: AppTheme.textSecondary,
                        ),
                      ),
                    ),
                  ),
                  const SizedBox(height: 8),
                  OutlinedButton.icon(
                    onPressed: () => _copyDetails(context),
                    icon: const Icon(Icons.copy_all, size: 18),
                    label: Text(widget.copyLabel),
                    style: OutlinedButton.styleFrom(
                      foregroundColor: AppTheme.textPrimary,
                      side: const BorderSide(color: AppTheme.borderColor),
                    ),
                  ),
                ],
              ],
            ],
          ),
        ),
      ),
    );
  }
}
