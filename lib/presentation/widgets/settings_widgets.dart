import 'package:flutter/material.dart';
import 'package:cached_network_image/cached_network_image.dart';
import 'package:flutter/services.dart';
import 'package:get_it/get_it.dart';
import '../../data/services/settings_service.dart';
import '../../domain/entities/ui_settings.dart';
import '../../presentation/theme/app_theme.dart';

// =============================================================================
// SETTINGS SECTION
// =============================================================================

/// A card-style section container for settings items
class SettingsSection extends StatelessWidget {
  final String title;
  final IconData icon;
  final List<Widget> children;

  const SettingsSection({
    super.key,
    required this.title,
    required this.icon,
    required this.children,
  });

  @override
  Widget build(BuildContext context) {
    return Card(
      margin: const EdgeInsets.symmetric(horizontal: 8, vertical: 6),
      color: Theme.of(context).cardColor,
      shape: RoundedRectangleBorder(
        borderRadius: BorderRadius.circular(12),
        side: BorderSide(color: Theme.of(context).dividerColor, width: 1),
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Padding(
            padding: const EdgeInsets.fromLTRB(16, 16, 16, 8),
            child: Row(
              children: [
                Icon(icon, size: 20, color: AppTheme.textMuted),
                const SizedBox(width: 8),
                Text(
                  title,
                  style: const TextStyle(
                    color: AppTheme.textMuted,
                    fontWeight: FontWeight.w600,
                    fontSize: 13,
                    letterSpacing: 0.5,
                  ),
                ),
              ],
            ),
          ),
          ...children,
          const SizedBox(height: 8),
        ],
      ),
    );
  }
}

// =============================================================================
// SETTINGS TILE
// =============================================================================

/// A list tile for navigation/action settings
class SettingsTile extends StatefulWidget {
  final IconData icon;
  final String title;
  final String value;
  final VoidCallback onTap;
  final bool destructive;

  const SettingsTile({
    super.key,
    required this.icon,
    required this.title,
    required this.value,
    required this.onTap,
    this.destructive = false,
  });

  @override
  State<SettingsTile> createState() => _SettingsTileState();
}

class _SettingsTileState extends State<SettingsTile> {
  final _settings = GetIt.instance<SettingsService>();
  bool _isFocused = false;

  UISettings get _ui => _settings.uiSettings;
  Color get _accentColor => Color(_ui.accentColor.colorValue);

  @override
  Widget build(BuildContext context) {
    return Focus(
      onFocusChange: (hasFocus) => setState(() => _isFocused = hasFocus),
      onKeyEvent: (node, event) {
        if (event is KeyDownEvent &&
            (event.logicalKey == LogicalKeyboardKey.select ||
                event.logicalKey == LogicalKeyboardKey.enter)) {
          widget.onTap();
          return KeyEventResult.handled;
        }
        return KeyEventResult.ignored;
      },
      child: AnimatedContainer(
        duration: const Duration(milliseconds: 200),
        decoration: BoxDecoration(
          color: _isFocused
              ? _accentColor.withValues(alpha: 0.1)
              : Colors.transparent,
          border: _isFocused ? Border.all(color: _accentColor, width: 2) : null,
          borderRadius: BorderRadius.circular(8),
        ),
        margin: const EdgeInsets.symmetric(horizontal: 4, vertical: 1),
        child: ListTile(
          leading: Icon(
            widget.icon,
            color: widget.destructive
                ? Theme.of(context).colorScheme.error
                : Theme.of(context).textTheme.bodySmall?.color,
          ),
          title: Text(
            widget.title,
            style: TextStyle(
              color: widget.destructive
                  ? Theme.of(context).colorScheme.error
                  : null,
            ),
          ),
          subtitle: Text(
            widget.value,
            style: TextStyle(
              color: widget.destructive
                  ? Theme.of(context).colorScheme.error.withValues(alpha: 0.7)
                  : Theme.of(context).textTheme.bodySmall?.color,
              fontSize: 13,
            ),
          ),
          trailing: Icon(
            Icons.chevron_right,
            color: Theme.of(context).textTheme.bodySmall?.color,
          ),
          onTap: widget.onTap,
        ),
      ),
    );
  }
}

// =============================================================================
// SETTINGS SWITCH
// =============================================================================

/// A switch tile for toggle settings
class SettingsSwitch extends StatefulWidget {
  final IconData icon;
  final String title;
  final String subtitle;
  final bool value;
  final ValueChanged<bool> onChanged;

  const SettingsSwitch({
    super.key,
    required this.icon,
    required this.title,
    required this.subtitle,
    required this.value,
    required this.onChanged,
  });

  @override
  State<SettingsSwitch> createState() => _SettingsSwitchState();
}

class _SettingsSwitchState extends State<SettingsSwitch> {
  final _settings = GetIt.instance<SettingsService>();
  bool _isFocused = false;

  UISettings get _ui => _settings.uiSettings;
  Color get _accentColor => Color(_ui.accentColor.colorValue);

  @override
  Widget build(BuildContext context) {
    return Focus(
      onFocusChange: (hasFocus) => setState(() => _isFocused = hasFocus),
      onKeyEvent: (node, event) {
        if (event is KeyDownEvent &&
            (event.logicalKey == LogicalKeyboardKey.select ||
                event.logicalKey == LogicalKeyboardKey.enter)) {
          widget.onChanged(!widget.value);
          return KeyEventResult.handled;
        }
        return KeyEventResult.ignored;
      },
      child: AnimatedContainer(
        duration: const Duration(milliseconds: 200),
        decoration: BoxDecoration(
          color: _isFocused
              ? _accentColor.withValues(alpha: 0.1)
              : Colors.transparent,
          border: _isFocused ? Border.all(color: _accentColor, width: 2) : null,
          borderRadius: BorderRadius.circular(8),
        ),
        margin: const EdgeInsets.symmetric(horizontal: 4, vertical: 1),
        child: SwitchListTile(
          secondary: Icon(
            widget.icon,
            color: Theme.of(context).textTheme.bodySmall?.color,
          ),
          title: Text(widget.title),
          subtitle: Text(
            widget.subtitle,
            style: TextStyle(
              color: Theme.of(context).textTheme.bodySmall?.color,
              fontSize: 13,
            ),
          ),
          value: widget.value,
          dense: true,
          onChanged: widget.onChanged,
        ),
      ),
    );
  }
}

// =============================================================================
// PROVIDER TILE
// =============================================================================

/// A tile for content provider toggle
class ProviderTile extends StatefulWidget {
  final String name;
  final String url;
  final String? iconUrl;
  final bool isEnabled;
  final ValueChanged<bool> onChanged;

  const ProviderTile({
    super.key,
    required this.name,
    required this.url,
    this.iconUrl,
    required this.isEnabled,
    required this.onChanged,
  });

  @override
  State<ProviderTile> createState() => _ProviderTileState();
}

class _ProviderTileState extends State<ProviderTile> {
  final _settings = GetIt.instance<SettingsService>();
  bool _isFocused = false;

  UISettings get _ui => _settings.uiSettings;
  Color get _accentColor => Color(_ui.accentColor.colorValue);

  @override
  Widget build(BuildContext context) {
    return Focus(
      onFocusChange: (hasFocus) => setState(() => _isFocused = hasFocus),
      onKeyEvent: (node, event) {
        if (event is KeyDownEvent &&
            (event.logicalKey == LogicalKeyboardKey.select ||
                event.logicalKey == LogicalKeyboardKey.enter)) {
          widget.onChanged(!widget.isEnabled);
          return KeyEventResult.handled;
        }
        return KeyEventResult.ignored;
      },
      child: AnimatedContainer(
        duration: const Duration(milliseconds: 200),
        decoration: BoxDecoration(
          color: _isFocused
              ? _accentColor.withValues(alpha: 0.1)
              : Colors.transparent,
          border: _isFocused ? Border.all(color: _accentColor, width: 2) : null,
          borderRadius: BorderRadius.circular(8),
        ),
        margin: const EdgeInsets.symmetric(horizontal: 4, vertical: 1),
        child: SwitchListTile(
          secondary: Container(
            width: 40,
            height: 40,
            decoration: BoxDecoration(
              color: widget.isEnabled
                  ? Theme.of(
                      context,
                    ).colorScheme.primary.withValues(alpha: 0.15)
                  : Theme.of(context).dividerColor.withValues(alpha: 0.1),
              borderRadius: BorderRadius.circular(8),
            ),
            child: widget.iconUrl != null
                ? ClipRRect(
                    borderRadius: BorderRadius.circular(8),
                    child: CachedNetworkImage(
                      imageUrl: widget.iconUrl!,
                      width: 40,
                      height: 40,
                      fit: BoxFit.cover,
                      errorWidget: (_, __, ___) => Icon(
                        Icons.movie_filter,
                        color: widget.isEnabled
                            ? Theme.of(context).colorScheme.primary
                            : Theme.of(context).textTheme.bodySmall?.color,
                      ),
                    ),
                  )
                : Icon(
                    Icons.movie_filter,
                    color: widget.isEnabled
                        ? Theme.of(context).colorScheme.primary
                        : Theme.of(context).textTheme.bodySmall?.color,
                  ),
          ),
          title: Text(widget.name),
          subtitle: Text(
            widget.url,
            style: TextStyle(
              color: Theme.of(context).textTheme.bodySmall?.color,
              fontSize: 12,
            ),
          ),
          value: widget.isEnabled,
          dense: true,
          onChanged: widget.onChanged,
        ),
      ),
    );
  }
}

// =============================================================================
// EMPTY PROVIDERS
// =============================================================================

/// Placeholder when no providers are registered
class EmptyProviders extends StatelessWidget {
  const EmptyProviders({super.key});

  @override
  Widget build(BuildContext context) {
    return Padding(
      padding: const EdgeInsets.all(16),
      child: Row(
        children: [
          Icon(Icons.warning_amber, color: Colors.orange.shade400),
          const SizedBox(width: 12),
          Expanded(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                const Text('Немає провайдерів'),
                Text(
                  'Додайте джерела контенту',
                  style: TextStyle(
                    color: Theme.of(context).textTheme.bodySmall?.color,
                    fontSize: 13,
                  ),
                ),
              ],
            ),
          ),
        ],
      ),
    );
  }
}

// =============================================================================
// SELECTION SHEET
// =============================================================================

/// A bottom sheet for selecting from a list of options
class SelectionSheet<T> extends StatelessWidget {
  final String title;
  final List<T> items;
  final T selectedItem;
  final Widget Function(T item) itemBuilder;
  final ValueChanged<T> onSelected;

  const SelectionSheet({
    super.key,
    required this.title,
    required this.items,
    required this.selectedItem,
    required this.itemBuilder,
    required this.onSelected,
  });

  @override
  Widget build(BuildContext context) {
    return SafeArea(
      child: Column(
        mainAxisSize: MainAxisSize.min,
        children: [
          // Handle
          Container(
            margin: const EdgeInsets.only(top: 12),
            width: 40,
            height: 4,
            decoration: BoxDecoration(
              color: Theme.of(context).dividerColor,
              borderRadius: BorderRadius.circular(2),
            ),
          ),
          // Title
          Padding(
            padding: const EdgeInsets.all(16),
            child: Text(
              title,
              style: const TextStyle(fontSize: 18, fontWeight: FontWeight.bold),
            ),
          ),
          // Items
          ...items.map(
            (item) => InkWell(
              onTap: () => onSelected(item),
              child: itemBuilder(item),
            ),
          ),
          const SizedBox(height: 16),
        ],
      ),
    );
  }
}

// =============================================================================
// SELECTION ITEM
// =============================================================================

/// An item in a selection sheet
class SelectionItem extends StatelessWidget {
  final IconData icon;
  final String title;
  final String? subtitle;
  final bool isSelected;

  const SelectionItem({
    super.key,
    required this.icon,
    required this.title,
    this.subtitle,
    required this.isSelected,
  });

  @override
  Widget build(BuildContext context) {
    return ListTile(
      leading: Container(
        width: 44,
        height: 44,
        decoration: BoxDecoration(
          color: isSelected
              ? Theme.of(context).colorScheme.primary.withValues(alpha: 0.15)
              : Theme.of(context).dividerColor.withValues(alpha: 0.1),
          borderRadius: BorderRadius.circular(8),
        ),
        child: Icon(
          icon,
          color: isSelected
              ? Theme.of(context).colorScheme.primary
              : Theme.of(context).textTheme.bodySmall?.color,
        ),
      ),
      title: Text(
        title,
        style: TextStyle(
          fontWeight: isSelected ? FontWeight.bold : FontWeight.normal,
          color: isSelected ? Theme.of(context).colorScheme.primary : null,
        ),
      ),
      subtitle: subtitle != null
          ? Text(
              subtitle!,
              style: TextStyle(
                color: Theme.of(context).textTheme.bodySmall?.color,
              ),
            )
          : null,
      trailing: isSelected
          ? Icon(
              Icons.check_circle,
              color: Theme.of(context).colorScheme.primary,
            )
          : null,
    );
  }
}
