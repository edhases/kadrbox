import 'package:flutter/material.dart';
import 'package:flutter/foundation.dart';
import 'package:get_it/get_it.dart';

import '../../../data/services/stats_service.dart';
import '../../../data/services/settings_service.dart';
import '../../theme/app_theme.dart';
import '../../widgets/custom_titlebar.dart';

/// Statistics page showing watch history analytics
class StatsPage extends StatefulWidget {
  const StatsPage({super.key});

  @override
  State<StatsPage> createState() => _StatsPageState();
}

class _StatsPageState extends State<StatsPage> {
  final _statsService = GetIt.instance<StatsService>();
  final _settings = GetIt.instance<SettingsService>();

  bool get _isDesktop =>
      !kIsWeb &&
      (defaultTargetPlatform == TargetPlatform.windows ||
          defaultTargetPlatform == TargetPlatform.linux ||
          defaultTargetPlatform == TargetPlatform.macOS);

  Color get _accentColor => Color(_settings.uiSettings.accentColor.colorValue);

  @override
  void initState() {
    super.initState();
    _statsService.addListener(_onStatsChanged);
    _statsService.refreshStats();
  }

  @override
  void dispose() {
    _statsService.removeListener(_onStatsChanged);
    super.dispose();
  }

  void _onStatsChanged() {
    if (mounted) setState(() {});
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      body: Column(
        children: [
          if (_isDesktop) const CustomTitleBar(),
          _buildAppBar(),
          Expanded(
            child: RefreshIndicator(
              onRefresh: _statsService.refreshStats,
              child: ListView(
                padding: const EdgeInsets.all(16),
                children: [
                  // Summary cards
                  _buildSummaryCards(),

                  const SizedBox(height: 24),

                  // Weekly activity
                  _buildSectionTitle('Активність за тиждень'),
                  const SizedBox(height: 12),
                  _buildWeeklyChart(),

                  const SizedBox(height: 24),

                  // Content type distribution
                  _buildSectionTitle('Типи контенту'),
                  const SizedBox(height: 12),
                  _buildTypeDistribution(),

                  const SizedBox(height: 24),

                  // Provider distribution
                  _buildSectionTitle('Джерела'),
                  const SizedBox(height: 12),
                  _buildProviderDistribution(),

                  const SizedBox(height: 24),

                  // Monthly stats
                  if (_statsService.stats.monthlyStats.isNotEmpty) ...[
                    _buildSectionTitle('Помісячна статистика'),
                    const SizedBox(height: 12),
                    _buildMonthlyStats(),
                  ],

                  const SizedBox(height: 32),
                ],
              ),
            ),
          ),
        ],
      ),
    );
  }

  Widget _buildAppBar() {
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 8),
      child: Row(
        children: [
          IconButton(
            icon: const Icon(Icons.arrow_back),
            onPressed: () => Navigator.of(context).pop(),
          ),
          const SizedBox(width: 8),
          const Text(
            'Статистика',
            style: TextStyle(fontSize: 20, fontWeight: FontWeight.w600),
          ),
          const Spacer(),
          IconButton(
            icon: const Icon(Icons.refresh),
            onPressed: () => _statsService.refreshStats(),
            tooltip: 'Оновити',
          ),
        ],
      ),
    );
  }

  Widget _buildSectionTitle(String title) {
    return Text(
      title,
      style: const TextStyle(
        fontSize: 18,
        fontWeight: FontWeight.w600,
        color: AppTheme.textPrimary,
      ),
    );
  }

  Widget _buildSummaryCards() {
    final stats = _statsService.stats;

    return Wrap(
      spacing: 12,
      runSpacing: 12,
      children: [
        _SummaryCard(
          icon: Icons.visibility,
          title: 'Переглянуто',
          value: stats.totalWatched.toString(),
          color: _accentColor,
        ),
        _SummaryCard(
          icon: Icons.schedule,
          title: 'Загальний час',
          value: stats.formattedWatchTime,
          color: Colors.green,
        ),
        _SummaryCard(
          icon: Icons.category,
          title: 'Улюблений тип',
          value: _getTypeDisplayName(
            stats.typeCount.entries
                .fold<MapEntry<String, int>?>(
                  null,
                  (max, e) => max == null || e.value > max.value ? e : max,
                )
                ?.key,
          ),
          color: Colors.orange,
        ),
        _SummaryCard(
          icon: Icons.source,
          title: 'Топ джерело',
          value: stats.topProvider,
          color: Colors.purple,
        ),
      ],
    );
  }

  String _getTypeDisplayName(String? type) {
    if (type == null) return 'Немає даних';
    switch (type) {
      case 'movie':
        return 'Фільми';
      case 'series':
        return 'Серіали';
      case 'cartoon':
        return 'Мультфільми';
      case 'anime':
        return 'Аніме';
      default:
        return type;
    }
  }

  Widget _buildWeeklyChart() {
    final weeklyStats = _statsService.stats.weeklyStats;
    if (weeklyStats.isEmpty) {
      return _buildEmptyState('Немає даних за тиждень');
    }

    final maxCount = weeklyStats.fold<int>(
      1,
      (max, s) => s.count > max ? s.count : max,
    );

    return Container(
      padding: const EdgeInsets.all(16),
      decoration: BoxDecoration(
        color: AppTheme.darkCard,
        borderRadius: BorderRadius.circular(12),
        border: Border.all(color: AppTheme.borderColor),
      ),
      child: Column(
        children: [
          SizedBox(
            height: 120,
            child: Row(
              crossAxisAlignment: CrossAxisAlignment.end,
              children: weeklyStats.map((day) {
                final height = day.count / maxCount;
                final isToday = _isToday(day.date);

                return Expanded(
                  child: Padding(
                    padding: const EdgeInsets.symmetric(horizontal: 2),
                    child: Column(
                      mainAxisAlignment: MainAxisAlignment.end,
                      mainAxisSize: MainAxisSize.min,
                      children: [
                        if (day.count > 0)
                          Text(
                            '${day.count}',
                            style: TextStyle(
                              fontSize: 10,
                              color: isToday
                                  ? _accentColor
                                  : AppTheme.textMuted,
                            ),
                          ),
                        const SizedBox(height: 2),
                        Flexible(
                          child: AnimatedContainer(
                            duration: const Duration(milliseconds: 300),
                            constraints: BoxConstraints(
                              maxHeight: height * 70,
                              minHeight: day.count > 0 ? 4 : 0,
                            ),
                            decoration: BoxDecoration(
                              color: isToday
                                  ? _accentColor
                                  : _accentColor.withValues(alpha: 0.5),
                              borderRadius: BorderRadius.circular(4),
                            ),
                          ),
                        ),
                        const SizedBox(height: 4),
                        Text(
                          day.dayName,
                          style: TextStyle(
                            fontSize: 11,
                            fontWeight: isToday
                                ? FontWeight.bold
                                : FontWeight.normal,
                            color: isToday
                                ? _accentColor
                                : AppTheme.textSecondary,
                          ),
                        ),
                      ],
                    ),
                  ),
                );
              }).toList(),
            ),
          ),
        ],
      ),
    );
  }

  bool _isToday(DateTime date) {
    final now = DateTime.now();
    return date.year == now.year &&
        date.month == now.month &&
        date.day == now.day;
  }

  Widget _buildTypeDistribution() {
    final distribution = _statsService.getTypeDistribution();
    if (distribution.isEmpty) {
      return _buildEmptyState('Немає даних про типи контенту');
    }

    final colors = {
      'movie': Colors.blue,
      'series': Colors.green,
      'cartoon': Colors.orange,
      'anime': Colors.pink,
    };

    final icons = {
      'movie': Icons.movie,
      'series': Icons.tv,
      'cartoon': Icons.animation,
      'anime': Icons.auto_awesome,
    };

    return Container(
      padding: const EdgeInsets.all(16),
      decoration: BoxDecoration(
        color: AppTheme.darkCard,
        borderRadius: BorderRadius.circular(12),
        border: Border.all(color: AppTheme.borderColor),
      ),
      child: Column(
        children: distribution.entries.map((entry) {
          final color = colors[entry.key] ?? AppTheme.textMuted;
          final icon = icons[entry.key] ?? Icons.category;

          return Padding(
            padding: const EdgeInsets.symmetric(vertical: 6),
            child: Row(
              children: [
                Icon(icon, color: color, size: 20),
                const SizedBox(width: 12),
                Expanded(
                  child: Text(
                    _getTypeDisplayName(entry.key),
                    style: const TextStyle(color: AppTheme.textPrimary),
                  ),
                ),
                SizedBox(
                  width: 150,
                  child: ClipRRect(
                    borderRadius: BorderRadius.circular(4),
                    child: LinearProgressIndicator(
                      value: entry.value / 100,
                      backgroundColor: AppTheme.darkSurface,
                      valueColor: AlwaysStoppedAnimation(color),
                      minHeight: 8,
                    ),
                  ),
                ),
                const SizedBox(width: 12),
                SizedBox(
                  width: 50,
                  child: Text(
                    '${entry.value.toStringAsFixed(1)}%',
                    textAlign: TextAlign.right,
                    style: TextStyle(color: color, fontWeight: FontWeight.w600),
                  ),
                ),
              ],
            ),
          );
        }).toList(),
      ),
    );
  }

  Widget _buildProviderDistribution() {
    final distribution = _statsService.getProviderDistribution();
    if (distribution.isEmpty) {
      return _buildEmptyState('Немає даних про джерела');
    }

    return Container(
      padding: const EdgeInsets.all(16),
      decoration: BoxDecoration(
        color: AppTheme.darkCard,
        borderRadius: BorderRadius.circular(12),
        border: Border.all(color: AppTheme.borderColor),
      ),
      child: Column(
        children: distribution.entries.take(5).map((entry) {
          return Padding(
            padding: const EdgeInsets.symmetric(vertical: 6),
            child: Row(
              children: [
                const Icon(
                  Icons.source,
                  color: AppTheme.textSecondary,
                  size: 20,
                ),
                const SizedBox(width: 12),
                Expanded(
                  child: Text(
                    entry.key,
                    style: const TextStyle(color: AppTheme.textPrimary),
                  ),
                ),
                SizedBox(
                  width: 150,
                  child: ClipRRect(
                    borderRadius: BorderRadius.circular(4),
                    child: LinearProgressIndicator(
                      value: entry.value / 100,
                      backgroundColor: AppTheme.darkSurface,
                      valueColor: AlwaysStoppedAnimation(_accentColor),
                      minHeight: 8,
                    ),
                  ),
                ),
                const SizedBox(width: 12),
                SizedBox(
                  width: 50,
                  child: Text(
                    '${entry.value.toStringAsFixed(1)}%',
                    textAlign: TextAlign.right,
                    style: TextStyle(
                      color: _accentColor,
                      fontWeight: FontWeight.w600,
                    ),
                  ),
                ),
              ],
            ),
          );
        }).toList(),
      ),
    );
  }

  Widget _buildMonthlyStats() {
    final monthlyStats = _statsService.stats.monthlyStats;
    if (monthlyStats.isEmpty) {
      return _buildEmptyState('Немає помісячних даних');
    }

    return Container(
      padding: const EdgeInsets.all(16),
      decoration: BoxDecoration(
        color: AppTheme.darkCard,
        borderRadius: BorderRadius.circular(12),
        border: Border.all(color: AppTheme.borderColor),
      ),
      child: Column(
        children: monthlyStats.reversed.take(6).map((month) {
          return Padding(
            padding: const EdgeInsets.symmetric(vertical: 8),
            child: Row(
              children: [
                SizedBox(
                  width: 100,
                  child: Text(
                    month.label,
                    style: const TextStyle(color: AppTheme.textSecondary),
                  ),
                ),
                const SizedBox(width: 16),
                Expanded(
                  child: Row(
                    children: [
                      Container(
                        padding: const EdgeInsets.symmetric(
                          horizontal: 8,
                          vertical: 4,
                        ),
                        decoration: BoxDecoration(
                          color: _accentColor.withValues(alpha: 0.2),
                          borderRadius: BorderRadius.circular(4),
                        ),
                        child: Text(
                          '${month.count} переглядів',
                          style: TextStyle(
                            color: _accentColor,
                            fontSize: 12,
                            fontWeight: FontWeight.w500,
                          ),
                        ),
                      ),
                      const SizedBox(width: 8),
                      Icon(Icons.schedule, size: 14, color: AppTheme.textMuted),
                      const SizedBox(width: 4),
                      Text(
                        _formatDuration(month.watchTime),
                        style: const TextStyle(
                          color: AppTheme.textMuted,
                          fontSize: 12,
                        ),
                      ),
                    ],
                  ),
                ),
              ],
            ),
          );
        }).toList(),
      ),
    );
  }

  String _formatDuration(Duration duration) {
    if (duration.inHours > 0) {
      return '${duration.inHours}г ${duration.inMinutes.remainder(60)}хв';
    }
    return '${duration.inMinutes}хв';
  }

  Widget _buildEmptyState(String message) {
    return Container(
      padding: const EdgeInsets.all(24),
      decoration: BoxDecoration(
        color: AppTheme.darkCard,
        borderRadius: BorderRadius.circular(12),
        border: Border.all(color: AppTheme.borderColor),
      ),
      child: Center(
        child: Column(
          children: [
            Icon(Icons.bar_chart, size: 48, color: AppTheme.textMuted),
            const SizedBox(height: 12),
            Text(message, style: const TextStyle(color: AppTheme.textMuted)),
          ],
        ),
      ),
    );
  }
}

class _SummaryCard extends StatelessWidget {
  final IconData icon;
  final String title;
  final String value;
  final Color color;

  const _SummaryCard({
    required this.icon,
    required this.title,
    required this.value,
    required this.color,
  });

  @override
  Widget build(BuildContext context) {
    return Container(
      width: 160,
      padding: const EdgeInsets.all(16),
      decoration: BoxDecoration(
        color: color.withValues(alpha: 0.1),
        borderRadius: BorderRadius.circular(12),
        border: Border.all(color: color.withValues(alpha: 0.3)),
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Row(
            children: [
              Icon(icon, color: color, size: 20),
              const Spacer(),
            ],
          ),
          const SizedBox(height: 12),
          Text(
            value,
            style: TextStyle(
              fontSize: 24,
              fontWeight: FontWeight.bold,
              color: color,
            ),
          ),
          const SizedBox(height: 4),
          Text(
            title,
            style: const TextStyle(fontSize: 12, color: AppTheme.textSecondary),
          ),
        ],
      ),
    );
  }
}
