import 'dart:async';
import 'package:flutter/material.dart';

import '../database/app_database.dart';
import '../database/dao/history_dao.dart';

/// Watch statistics data
class WatchStats {
  final int totalWatched;
  final Duration totalWatchTime;
  final Map<String, int> genreCount;
  final Map<String, int> providerCount;
  final Map<String, int> typeCount;
  final List<MonthlyStats> monthlyStats;
  final List<DayStats> weeklyStats;

  const WatchStats({
    required this.totalWatched,
    required this.totalWatchTime,
    required this.genreCount,
    required this.providerCount,
    required this.typeCount,
    required this.monthlyStats,
    required this.weeklyStats,
  });

  factory WatchStats.empty() => const WatchStats(
    totalWatched: 0,
    totalWatchTime: Duration.zero,
    genreCount: {},
    providerCount: {},
    typeCount: {},
    monthlyStats: [],
    weeklyStats: [],
  );

  String get formattedWatchTime {
    if (totalWatchTime.inHours > 0) {
      final hours = totalWatchTime.inHours;
      final minutes = totalWatchTime.inMinutes.remainder(60);
      return '$hoursг $minutesхв';
    }
    return '${totalWatchTime.inMinutes}хв';
  }

  String get topGenre {
    if (genreCount.isEmpty) return 'Немає даних';
    final sorted = genreCount.entries.toList()
      ..sort((a, b) => b.value.compareTo(a.value));
    return sorted.first.key;
  }

  String get topProvider {
    if (providerCount.isEmpty) return 'Немає даних';
    final sorted = providerCount.entries.toList()
      ..sort((a, b) => b.value.compareTo(a.value));
    return sorted.first.key;
  }
}

/// Monthly statistics
class MonthlyStats {
  final int year;
  final int month;
  final int count;
  final Duration watchTime;

  const MonthlyStats({
    required this.year,
    required this.month,
    required this.count,
    required this.watchTime,
  });

  String get monthName {
    const months = [
      'Січень',
      'Лютий',
      'Березень',
      'Квітень',
      'Травень',
      'Червень',
      'Липень',
      'Серпень',
      'Вересень',
      'Жовтень',
      'Листопад',
      'Грудень',
    ];
    return months[month - 1];
  }

  String get label => '$monthName $year';
}

/// Daily statistics
class DayStats {
  final DateTime date;
  final int count;
  final Duration watchTime;

  const DayStats({
    required this.date,
    required this.count,
    required this.watchTime,
  });

  String get dayName {
    const days = ['Нд', 'Пн', 'Вт', 'Ср', 'Чт', 'Пт', 'Сб'];
    return days[date.weekday % 7];
  }
}

/// Statistics service for tracking watch history analytics
class StatsService extends ChangeNotifier {
  final HistoryDao _historyDao;
  WatchStats _stats = WatchStats.empty();

  WatchStats get stats => _stats;

  StatsService(AppDatabase database) : _historyDao = HistoryDao(database) {
    _loadStats();
  }

  Future<void> _loadStats() async {
    await refreshStats();
  }

  /// Refresh statistics from database
  Future<void> refreshStats() async {
    try {
      final history = await _historyDao.getAll();

      // Calculate totals
      final totalWatched = history.length;
      int totalSeconds = 0;
      final genreCount = <String, int>{};
      final providerCount = <String, int>{};
      final typeCount = <String, int>{};
      final monthlyMap = <String, MonthlyStats>{};
      final weeklyMap = <DateTime, DayStats>{};

      final now = DateTime.now();
      final weekStart = now.subtract(Duration(days: now.weekday));

      for (final item in history) {
        // Watch time - calculate progress from position/duration
        final positionMs = item.positionMs;
        final durationMs = item.durationMs;

        Duration watchedDuration;
        if (durationMs > 0) {
          // Use actual tracked duration
          watchedDuration = Duration(milliseconds: positionMs);
        } else {
          // Estimate based on type
          final isMovie = item.mediaType == 'movie';
          final estimatedDuration = isMovie
              ? const Duration(hours: 2)
              : const Duration(minutes: 45);
          // Assume 50% watched if no data
          watchedDuration = Duration(
            seconds: (0.5 * estimatedDuration.inSeconds).round(),
          );
        }
        totalSeconds += watchedDuration.inSeconds;

        // Provider stats
        providerCount[item.providerId] =
            (providerCount[item.providerId] ?? 0) + 1;

        // Type stats
        final typeKey = item.mediaType;
        typeCount[typeKey] = (typeCount[typeKey] ?? 0) + 1;

        // Monthly stats
        final monthKey = '${item.watchedAt.year}-${item.watchedAt.month}';
        final existing = monthlyMap[monthKey];
        if (existing != null) {
          monthlyMap[monthKey] = MonthlyStats(
            year: existing.year,
            month: existing.month,
            count: existing.count + 1,
            watchTime: existing.watchTime + watchedDuration,
          );
        } else {
          monthlyMap[monthKey] = MonthlyStats(
            year: item.watchedAt.year,
            month: item.watchedAt.month,
            count: 1,
            watchTime: watchedDuration,
          );
        }

        // Weekly stats (last 7 days)
        final dayDiff = now.difference(item.watchedAt).inDays;
        if (dayDiff < 7) {
          final dayKey = DateTime(
            item.watchedAt.year,
            item.watchedAt.month,
            item.watchedAt.day,
          );
          final existingDay = weeklyMap[dayKey];
          if (existingDay != null) {
            weeklyMap[dayKey] = DayStats(
              date: dayKey,
              count: existingDay.count + 1,
              watchTime: existingDay.watchTime + watchedDuration,
            );
          } else {
            weeklyMap[dayKey] = DayStats(
              date: dayKey,
              count: 1,
              watchTime: watchedDuration,
            );
          }
        }
      }

      // Sort monthly stats
      final monthlyStats = monthlyMap.values.toList()
        ..sort((a, b) {
          final yearCompare = a.year.compareTo(b.year);
          if (yearCompare != 0) return yearCompare;
          return a.month.compareTo(b.month);
        });

      // Ensure all 7 days are in weekly stats
      final weeklyStats = <DayStats>[];
      for (int i = 0; i < 7; i++) {
        final day = weekStart.add(Duration(days: i));
        final dayKey = DateTime(day.year, day.month, day.day);
        weeklyStats.add(
          weeklyMap[dayKey] ??
              DayStats(date: dayKey, count: 0, watchTime: Duration.zero),
        );
      }

      _stats = WatchStats(
        totalWatched: totalWatched,
        totalWatchTime: Duration(seconds: totalSeconds),
        genreCount: genreCount,
        providerCount: providerCount,
        typeCount: typeCount,
        monthlyStats: monthlyStats.take(12).toList(),
        weeklyStats: weeklyStats,
      );

      notifyListeners();
    } catch (e) {
      debugPrint('Failed to load stats: $e');
    }
  }

  /// Get type distribution as percentages
  Map<String, double> getTypeDistribution() {
    final total = _stats.typeCount.values.fold<int>(0, (a, b) => a + b);
    if (total == 0) return {};

    return _stats.typeCount.map(
      (key, value) => MapEntry(key, value / total * 100),
    );
  }

  /// Get provider distribution as percentages
  Map<String, double> getProviderDistribution() {
    final total = _stats.providerCount.values.fold<int>(0, (a, b) => a + b);
    if (total == 0) return {};

    return _stats.providerCount.map(
      (key, value) => MapEntry(key, value / total * 100),
    );
  }
}
