import 'package:flutter/material.dart';
import '../../theme/app_theme.dart';

class ImportDataDialog extends StatelessWidget {
  final int historyCount;
  final int favoritesCount;

  const ImportDataDialog({
    super.key,
    required this.historyCount,
    required this.favoritesCount,
  });

  @override
  Widget build(BuildContext context) {
    return AlertDialog(
      backgroundColor: AppTheme.darkCard,
      shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(16)),
      title: const Row(
        children: [
          Icon(Icons.cloud_upload, color: AppTheme.primaryColor),
          SizedBox(width: 12),
          Text('Знайдено локальні дані'),
        ],
      ),
      content: Column(
        mainAxisSize: MainAxisSize.min,
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          const Text(
            'На цьому пристрої є історія переглядів та обране, які не прив\'язані до акаунту:',
            style: TextStyle(fontSize: 14, height: 1.5),
          ),
          const SizedBox(height: 16),
          _buildStatRow(Icons.history, '$historyCount записів історії'),
          const SizedBox(height: 8),
          _buildStatRow(Icons.favorite, '$favoritesCount в обраному'),
          const SizedBox(height: 16),
          const Text(
            'Бажаєте об\'єднати їх з вашим акаунтом?',
            style: TextStyle(fontSize: 14, fontWeight: FontWeight.bold),
          ),
          const SizedBox(height: 4),
          Text(
            'Якщо вибрати "Видалити", локальні дані будуть втрачені.',
            style: TextStyle(
              fontSize: 12,
              color: Colors.white.withValues(alpha: 0.6),
            ),
          ),
        ],
      ),
      actions: [
        TextButton(
          onPressed: () => Navigator.of(context).pop(false), // Delete/Clear
          style: TextButton.styleFrom(foregroundColor: AppTheme.errorColor),
          child: const Text('Видалити'),
        ),
        ElevatedButton(
          onPressed: () => Navigator.of(context).pop(true), // Merge
          style: ElevatedButton.styleFrom(
            backgroundColor: AppTheme.primaryColor,
            foregroundColor: Colors.white,
          ),
          child: const Text('Об\'єднати'),
        ),
      ],
    );
  }

  Widget _buildStatRow(IconData icon, String text) {
    return Row(
      children: [
        Icon(icon, size: 16, color: Colors.white.withValues(alpha: 0.7)),
        const SizedBox(width: 8),
        Text(
          text,
          style: TextStyle(color: Colors.white.withValues(alpha: 0.9)),
        ),
      ],
    );
  }
}
