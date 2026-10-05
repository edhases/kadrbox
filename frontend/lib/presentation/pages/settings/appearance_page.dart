import 'package:flutter/material.dart';
import 'package:flutter/foundation.dart';
import 'package:get_it/get_it.dart';

import '../../../data/services/settings_service.dart';
import '../../../domain/entities/ui_settings.dart';
import '../../widgets/custom_titlebar.dart';

/// Appearance customization page
class AppearancePage extends StatefulWidget {
  const AppearancePage({super.key});

  @override
  State<AppearancePage> createState() => _AppearancePageState();
}

class _AppearancePageState extends State<AppearancePage> {
  final _settings = GetIt.instance<SettingsService>();

  bool get _isDesktop =>
      !kIsWeb &&
      (defaultTargetPlatform == TargetPlatform.windows ||
          defaultTargetPlatform == TargetPlatform.linux ||
          defaultTargetPlatform == TargetPlatform.macOS);

  @override
  void initState() {
    super.initState();
    _settings.addListener(_onSettingsChanged);
  }

  @override
  void dispose() {
    _settings.removeListener(_onSettingsChanged);
    super.dispose();
  }

  void _onSettingsChanged() {
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
            child: ListView(
              padding: const EdgeInsets.all(16),
              children: [
                // Preview section
                _buildPreviewCard(),

                const SizedBox(height: 24),

                // Theme section
                _buildSectionTitle('Тема оформлення'),
                const SizedBox(height: 12),
                _buildThemeSelector(),

                const SizedBox(height: 24),

                // Poster size section
                _buildSectionTitle('Розмір постерів'),
                const SizedBox(height: 12),
                _buildPosterSizeSelector(),

                const SizedBox(height: 24),

                // Grid columns section
                _buildSectionTitle('Кількість колонок'),
                const SizedBox(height: 4),
                Text(
                  'Залиште 0 для автоматичного підбору за розміром постерів',
                  style: TextStyle(
                    fontSize: 12,
                    color: Theme.of(context).textTheme.bodySmall?.color,
                  ),
                ),
                const SizedBox(height: 12),
                _buildGridColumnsSelector(),

                const SizedBox(height: 24),

                // Grid spacing section
                _buildSectionTitle('Відступи сітки'),
                const SizedBox(height: 12),
                _buildGridSpacingSelector(),

                const SizedBox(height: 24),

                // Accent color section
                _buildSectionTitle('Акцентний колір'),
                const SizedBox(height: 12),
                _buildAccentColorSelector(),

                const SizedBox(height: 24),

                // Card info style section
                _buildSectionTitle('Інформація на картці'),
                const SizedBox(height: 12),
                _buildCardInfoStyleSelector(),

                const SizedBox(height: 24),

                // List style section
                _buildSectionTitle('Стиль списку'),
                const SizedBox(height: 12),
                _buildListStyleSelector(),

                const SizedBox(height: 24),

                // Display options
                _buildSectionTitle('Параметри відображення'),
                const SizedBox(height: 12),
                _buildDisplayOptions(),

                const SizedBox(height: 32),
              ],
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
            'Зовнішній вигляд',
            style: TextStyle(fontSize: 20, fontWeight: FontWeight.w600),
          ),
        ],
      ),
    );
  }

  Widget _buildSectionTitle(String title) {
    return Text(
      title,
      style: TextStyle(
        fontSize: 16,
        fontWeight: FontWeight.w600,
        color: Theme.of(context).textTheme.titleLarge?.color,
      ),
    );
  }

  Widget _buildPreviewCard() {
    final ui = _settings.uiSettings;
    final accentColor = Color(ui.accentColor.colorValue);

    return Container(
      padding: const EdgeInsets.all(16),
      decoration: BoxDecoration(
        color: Theme.of(context).cardColor,
        borderRadius: BorderRadius.circular(16),
        border: Border.all(color: Theme.of(context).dividerColor),
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Row(
            children: [
              Icon(Icons.visibility, color: accentColor, size: 20),
              const SizedBox(width: 8),
              const Text(
                'Попередній перегляд',
                style: TextStyle(
                  fontWeight: FontWeight.w600,
                  color: null, // Uses default text color
                ),
              ),
            ],
          ),
          const SizedBox(height: 16),
          LayoutBuilder(
            builder: (context, constraints) {
              final screenWidth = MediaQuery.of(context).size.width;
              final columnCount = ui.gridColumns > 0
                  ? ui.gridColumns
                  : ui.posterSize.getColumnCount(screenWidth);
              final spacing = ui.gridSpacing.crossAxisSpacing;

              final previewColumnCount = columnCount > 6
                  ? 6
                  : columnCount; // Limit preview columns

              // Calculate item width exactly as GridView would
              // Available width for items = total width - (total spacing)
              // Note: The LayoutBuilder is inside a card with padding 16*2=32 + screen padding 16*2=32?
              // No, we should use constraints.maxWidth which is the width INSIDE the preview card.
              // BUT getColumnCount uses SCREEN width logic.

              final itemWidth =
                  (constraints.maxWidth - (columnCount - 1) * spacing) /
                  columnCount;
              final itemHeight = itemWidth / ui.posterSize.aspectRatio;

              return SizedBox(
                height: itemHeight,
                child: Row(
                  crossAxisAlignment: CrossAxisAlignment.stretch,
                  children: List.generate(previewColumnCount, (index) {
                    return Expanded(
                      child: Padding(
                        padding: EdgeInsets.only(
                          left: index == 0 ? 0 : spacing / 2,
                          right: index == previewColumnCount - 1
                              ? 0
                              : spacing / 2,
                        ),
                        child: _buildPreviewPoster(index, accentColor),
                      ),
                    );
                  }),
                ),
              );
            },
          ),
        ],
      ),
    );
  }

  Widget _buildPreviewPoster(int index, Color accentColor) {
    final ui = _settings.uiSettings;
    final titles = [
      'Фільм 1',
      'Серіал 2',
      'Аніме 3',
      'Мульт 4',
      'Док 5',
      'Шоу 6',
      'Фільм 7',
      'Серіал 8',
      'Аніме 9',
      'Мульт 10',
    ];
    final years = [
      '2024',
      '2023',
      '2024',
      '2022',
      '2023',
      '2024',
      '2023',
      '2022',
      '2024',
      '2023',
    ];
    final ratings = [
      '8.5',
      '7.2',
      '9.1',
      '8.0',
      '7.5',
      '6.8',
      '7.9',
      '8.3',
      '9.0',
      '7.7',
    ];

    final dataIndex = index % titles.length;

    return Container(
      decoration: BoxDecoration(
        borderRadius: BorderRadius.circular(ui.posterSize.borderRadius),
        gradient: LinearGradient(
          begin: Alignment.topCenter,
          end: Alignment.bottomCenter,
          colors: [
            accentColor.withValues(alpha: 0.3),
            accentColor.withValues(alpha: 0.6),
          ],
        ),
      ),
      child: Stack(
        children: [
          // Rating badge
          if (ui.showRatings)
            Positioned(
              top: 4,
              right: 4,
              child: Container(
                padding: const EdgeInsets.symmetric(horizontal: 4, vertical: 2),
                decoration: BoxDecoration(
                  color: Colors.black54,
                  borderRadius: BorderRadius.circular(4),
                ),
                child: Row(
                  mainAxisSize: MainAxisSize.min,
                  children: [
                    const Icon(Icons.star, size: 8, color: Colors.amber),
                    const SizedBox(width: 2),
                    Text(
                      ratings[dataIndex],
                      style: const TextStyle(
                        fontSize: 8,
                        fontWeight: FontWeight.bold,
                      ),
                    ),
                  ],
                ),
              ),
            ),

          // Info overlay
          if (ui.cardInfoStyle == CardInfoStyle.overlay)
            Positioned(
              left: 0,
              right: 0,
              bottom: 0,
              child: Container(
                padding: const EdgeInsets.all(6),
                decoration: BoxDecoration(
                  gradient: LinearGradient(
                    begin: Alignment.topCenter,
                    end: Alignment.bottomCenter,
                    colors: [
                      Colors.transparent,
                      Colors.black.withValues(alpha: 0.8),
                    ],
                  ),
                  borderRadius: BorderRadius.only(
                    bottomLeft: Radius.circular(ui.posterSize.borderRadius),
                    bottomRight: Radius.circular(ui.posterSize.borderRadius),
                  ),
                ),
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  mainAxisSize: MainAxisSize.min,
                  children: [
                    Text(
                      titles[dataIndex],
                      style: const TextStyle(
                        fontSize: 10,
                        fontWeight: FontWeight.w600,
                      ),
                      maxLines: 1,
                      overflow: TextOverflow.ellipsis,
                    ),
                    if (ui.showYears)
                      Text(
                        years[dataIndex],
                        style: TextStyle(
                          fontSize: 9,
                          color: Theme.of(context).textTheme.bodySmall?.color,
                        ),
                      ),
                  ],
                ),
              ),
            ),
        ],
      ),
    );
  }

  Widget _buildThemeSelector() {
    final currentTheme = _settings.state.theme;

    return Row(
      children: AppThemeMode.values.map((mode) {
        final isSelected = mode == currentTheme;
        return Expanded(
          child: Padding(
            padding: EdgeInsets.only(
              right: mode != AppThemeMode.values.last ? 8 : 0,
            ),
            child: _OptionButton(
              label: mode.displayName,
              icon: _getThemeIcon(mode),
              isSelected: isSelected,
              onTap: () => _settings.setTheme(mode),
            ),
          ),
        );
      }).toList(),
    );
  }

  IconData _getThemeIcon(AppThemeMode theme) {
    switch (theme) {
      case AppThemeMode.dark:
        return Icons.dark_mode;
      case AppThemeMode.amoled:
        return Icons.smartphone;
      case AppThemeMode.light:
        return Icons.light_mode;
      case AppThemeMode.system:
        return Icons.settings_brightness;
    }
  }

  Widget _buildPosterSizeSelector() {
    final currentSize = _settings.uiSettings.posterSize;

    return Row(
      children: PosterSize.values.map((size) {
        final isSelected = size == currentSize;
        return Expanded(
          child: Padding(
            padding: EdgeInsets.only(
              right: size != PosterSize.values.last ? 8 : 0,
            ),
            child: _OptionButton(
              label: size.displayName,
              icon: _getSizeIcon(size),
              isSelected: isSelected,
              onTap: () => _settings.setPosterSize(size),
            ),
          ),
        );
      }).toList(),
    );
  }

  IconData _getSizeIcon(PosterSize size) {
    switch (size) {
      case PosterSize.small:
        return Icons.grid_view;
      case PosterSize.medium:
        return Icons.view_module;
      case PosterSize.large:
        return Icons.view_agenda;
    }
  }

  Widget _buildGridColumnsSelector() {
    final currentColumns = _settings.uiSettings.gridColumns;
    final options = [0, 2, 3, 4, 5, 6];

    return Row(
      children: options.map((cols) {
        final isSelected = cols == currentColumns;
        return Expanded(
          child: Padding(
            padding: EdgeInsets.only(right: cols != options.last ? 8 : 0),
            child: _OptionButton(
              label: cols == 0 ? 'Авто' : cols.toString(),
              icon: cols == 0 ? Icons.auto_awesome : Icons.grid_on,
              isSelected: isSelected,
              onTap: () => _settings.setGridColumns(cols),
            ),
          ),
        );
      }).toList(),
    );
  }

  Widget _buildGridSpacingSelector() {
    final currentSpacing = _settings.uiSettings.gridSpacing;

    return Row(
      children: GridSpacing.values.map((spacing) {
        final isSelected = spacing == currentSpacing;
        return Expanded(
          child: Padding(
            padding: EdgeInsets.only(
              right: spacing != GridSpacing.values.last ? 8 : 0,
            ),
            child: _OptionButton(
              label: spacing.displayName,
              icon: _getSpacingIcon(spacing),
              isSelected: isSelected,
              onTap: () => _settings.setGridSpacing(spacing),
            ),
          ),
        );
      }).toList(),
    );
  }

  IconData _getSpacingIcon(GridSpacing spacing) {
    switch (spacing) {
      case GridSpacing.compact:
        return Icons.density_small;
      case GridSpacing.normal:
        return Icons.density_medium;
      case GridSpacing.relaxed:
        return Icons.density_large;
    }
  }

  Widget _buildAccentColorSelector() {
    return Wrap(
      spacing: 12,
      runSpacing: 12,
      children: AccentColor.values.map((color) {
        final isSelected = color == _settings.uiSettings.accentColor;
        return _ColorButton(
          color: Color(color.colorValue),
          isSelected: isSelected,
          onTap: () => _settings.setAccentColor(color),
          tooltip: color.displayName,
        );
      }).toList(),
    );
  }

  Widget _buildCardInfoStyleSelector() {
    final currentStyle = _settings.uiSettings.cardInfoStyle;

    return Row(
      children: CardInfoStyle.values.map((style) {
        final isSelected = style == currentStyle;
        return Expanded(
          child: Padding(
            padding: EdgeInsets.only(
              right: style != CardInfoStyle.values.last ? 8 : 0,
            ),
            child: _OptionButton(
              label: style.displayName,
              icon: _getStyleIcon(style),
              isSelected: isSelected,
              onTap: () => _settings.setCardInfoStyle(style),
            ),
          ),
        );
      }).toList(),
    );
  }

  IconData _getStyleIcon(CardInfoStyle style) {
    switch (style) {
      case CardInfoStyle.overlay:
        return Icons.layers;
      case CardInfoStyle.below:
        return Icons.view_list;
      case CardInfoStyle.hidden:
        return Icons.visibility_off;
    }
  }

  Widget _buildListStyleSelector() {
    final currentStyle = _settings.uiSettings.listStyle;

    return Row(
      children: ListStyle.values.map((style) {
        final isSelected = style == currentStyle;
        return Expanded(
          child: Padding(
            padding: EdgeInsets.only(
              right: style != ListStyle.values.last ? 8 : 0,
            ),
            child: _OptionButton(
              label: style.displayName,
              icon: _getListStyleIcon(style),
              isSelected: isSelected,
              onTap: () => _settings.setListStyle(style),
            ),
          ),
        );
      }).toList(),
    );
  }

  IconData _getListStyleIcon(ListStyle style) {
    switch (style) {
      case ListStyle.grid:
        return Icons.grid_view;
      case ListStyle.list:
        return Icons.view_list;
      case ListStyle.compact:
        return Icons.view_headline;
    }
  }

  Widget _buildDisplayOptions() {
    final ui = _settings.uiSettings;

    return Container(
      decoration: BoxDecoration(
        color: Theme.of(context).cardColor,
        borderRadius: BorderRadius.circular(12),
        border: Border.all(color: Theme.of(context).dividerColor),
      ),
      child: Column(
        children: [
          _SwitchTile(
            icon: Icons.star,
            title: 'Показувати рейтинг',
            subtitle: 'Відображати бейдж з рейтингом',
            value: ui.showRatings,
            onChanged: (v) => _settings.setShowRatings(v),
          ),
          const Divider(height: 1, indent: 56),
          _SwitchTile(
            icon: Icons.calendar_today,
            title: 'Показувати рік',
            subtitle: 'Відображати рік випуску',
            value: ui.showYears,
            onChanged: (v) => _settings.setShowYears(v),
          ),
          const Divider(height: 1, indent: 56),
          _SwitchTile(
            icon: Icons.animation,
            title: 'Анімації',
            subtitle: 'Увімкнути анімації інтерфейсу',
            value: ui.animationsEnabled,
            onChanged: (v) => _settings.setAnimationsEnabled(v),
          ),
          const Divider(height: 1, indent: 56),
          _SwitchTile(
            icon: Icons.blur_on,
            title: 'Розмиття фону',
            subtitle: 'Ефект скла на панелях',
            value: ui.blurBackgrounds,
            onChanged: (v) => _settings.setBlurBackgrounds(v),
          ),
        ],
      ),
    );
  }
}

class _OptionButton extends StatelessWidget {
  final String label;
  final IconData icon;
  final bool isSelected;
  final VoidCallback onTap;

  const _OptionButton({
    required this.label,
    required this.icon,
    required this.isSelected,
    required this.onTap,
  });

  @override
  Widget build(BuildContext context) {
    final settings = GetIt.instance<SettingsService>();
    final accentColor = Color(settings.uiSettings.accentColor.colorValue);

    return Material(
      color: isSelected
          ? accentColor.withValues(alpha: 0.2)
          : Theme.of(context).cardColor,
      borderRadius: BorderRadius.circular(12),
      child: InkWell(
        onTap: onTap,
        borderRadius: BorderRadius.circular(12),
        child: Container(
          padding: const EdgeInsets.symmetric(vertical: 16),
          decoration: BoxDecoration(
            borderRadius: BorderRadius.circular(12),
            border: Border.all(
              color: isSelected ? accentColor : Theme.of(context).dividerColor,
              width: isSelected ? 2 : 1,
            ),
          ),
          child: Column(
            mainAxisSize: MainAxisSize.min,
            children: [
              Icon(
                icon,
                color: isSelected
                    ? accentColor
                    : Theme.of(context).textTheme.bodySmall?.color,
                size: 28,
              ),
              const SizedBox(height: 8),
              Text(
                label,
                style: TextStyle(
                  fontSize: 13,
                  fontWeight: isSelected ? FontWeight.w600 : FontWeight.normal,
                  color: isSelected
                      ? accentColor
                      : Theme.of(context).textTheme.bodyMedium?.color,
                ),
              ),
            ],
          ),
        ),
      ),
    );
  }
}

class _ColorButton extends StatelessWidget {
  final Color color;
  final bool isSelected;
  final VoidCallback onTap;
  final String tooltip;

  const _ColorButton({
    required this.color,
    required this.isSelected,
    required this.onTap,
    required this.tooltip,
  });

  @override
  Widget build(BuildContext context) {
    return Tooltip(
      message: tooltip,
      child: GestureDetector(
        onTap: onTap,
        child: AnimatedContainer(
          duration: const Duration(milliseconds: 200),
          width: 44,
          height: 44,
          decoration: BoxDecoration(
            color: color,
            shape: BoxShape.circle,
            border: Border.all(
              color: isSelected ? Colors.white : Colors.transparent,
              width: 3,
            ),
            boxShadow: isSelected
                ? [
                    BoxShadow(
                      color: color.withValues(alpha: 0.6),
                      blurRadius: 12,
                      spreadRadius: 2,
                    ),
                  ]
                : null,
          ),
          child: isSelected
              ? const Icon(Icons.check, color: Colors.white, size: 20)
              : null,
        ),
      ),
    );
  }
}

class _SwitchTile extends StatelessWidget {
  final IconData icon;
  final String title;
  final String subtitle;
  final bool value;
  final ValueChanged<bool> onChanged;

  const _SwitchTile({
    required this.icon,
    required this.title,
    required this.subtitle,
    required this.value,
    required this.onChanged,
  });

  @override
  Widget build(BuildContext context) {
    final settings = GetIt.instance<SettingsService>();
    final accentColor = Color(settings.uiSettings.accentColor.colorValue);

    return ListTile(
      leading: Icon(icon, color: Theme.of(context).textTheme.bodySmall?.color),
      title: Text(
        title,
        style: TextStyle(color: Theme.of(context).textTheme.bodyMedium?.color),
      ),
      subtitle: Text(
        subtitle,
        style: TextStyle(
          fontSize: 12,
          color: Theme.of(context).textTheme.bodySmall?.color,
        ),
      ),
      trailing: Switch(
        value: value,
        onChanged: onChanged,
        activeThumbColor: accentColor,
      ),
    );
  }
}
