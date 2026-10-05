/// UI customization settings
library;

/// Poster display size
enum PosterSize {
  small,
  medium,
  large;

  String get displayName {
    switch (this) {
      case PosterSize.small:
        return 'Малий';
      case PosterSize.medium:
        return 'Середній';
      case PosterSize.large:
        return 'Великий';
    }
  }

  String get key => name;

  static PosterSize fromString(String? value) {
    return PosterSize.values.firstWhere(
      (e) => e.key == value,
      orElse: () => PosterSize.medium,
    );
  }

  /// Number of columns for different screen widths
  int getColumnCount(double screenWidth) {
    if (screenWidth < 600) {
      // Mobile portrait
      switch (this) {
        case PosterSize.small:
          return 4;
        case PosterSize.medium:
          return 3;
        case PosterSize.large:
          return 2;
      }
    } else if (screenWidth < 900) {
      // Mobile landscape / Tablet portrait
      switch (this) {
        case PosterSize.small:
          return 6;
        case PosterSize.medium:
          return 4;
        case PosterSize.large:
          return 3;
      }
    } else if (screenWidth < 1200) {
      // Tablet landscape / Desktop small
      switch (this) {
        case PosterSize.small:
          return 8;
        case PosterSize.medium:
          return 6;
        case PosterSize.large:
          return 4;
      }
    } else {
      // Desktop large
      switch (this) {
        case PosterSize.small:
          return 10;
        case PosterSize.medium:
          return 7;
        case PosterSize.large:
          return 5;
      }
    }
  }

  /// Aspect ratio for poster cards
  double get aspectRatio {
    switch (this) {
      case PosterSize.small:
        return 0.6;
      case PosterSize.medium:
        return 0.65;
      case PosterSize.large:
        return 0.7;
    }
  }

  /// Card border radius
  double get borderRadius {
    switch (this) {
      case PosterSize.small:
        return 8;
      case PosterSize.medium:
        return 12;
      case PosterSize.large:
        return 16;
    }
  }
}

/// Grid spacing preset
enum GridSpacing {
  compact,
  normal,
  relaxed;

  String get displayName {
    switch (this) {
      case GridSpacing.compact:
        return 'Компактний';
      case GridSpacing.normal:
        return 'Звичайний';
      case GridSpacing.relaxed:
        return 'Вільний';
    }
  }

  String get key => name;

  static GridSpacing fromString(String? value) {
    return GridSpacing.values.firstWhere(
      (e) => e.key == value,
      orElse: () => GridSpacing.normal,
    );
  }

  double get mainAxisSpacing {
    switch (this) {
      case GridSpacing.compact:
        return 8;
      case GridSpacing.normal:
        return 12;
      case GridSpacing.relaxed:
        return 16;
    }
  }

  double get crossAxisSpacing {
    switch (this) {
      case GridSpacing.compact:
        return 8;
      case GridSpacing.normal:
        return 12;
      case GridSpacing.relaxed:
        return 16;
    }
  }

  double get padding {
    switch (this) {
      case GridSpacing.compact:
        return 8;
      case GridSpacing.normal:
        return 16;
      case GridSpacing.relaxed:
        return 24;
    }
  }
}

/// Accent color presets
enum AccentColor {
  indigo,
  purple,
  blue,
  cyan,
  teal,
  green,
  orange,
  red,
  pink;

  String get displayName {
    switch (this) {
      case AccentColor.indigo:
        return 'Індіго';
      case AccentColor.purple:
        return 'Фіолетовий';
      case AccentColor.blue:
        return 'Синій';
      case AccentColor.cyan:
        return 'Бірюзовий';
      case AccentColor.teal:
        return 'Морський';
      case AccentColor.green:
        return 'Зелений';
      case AccentColor.orange:
        return 'Помаранчевий';
      case AccentColor.red:
        return 'Червоний';
      case AccentColor.pink:
        return 'Рожевий';
    }
  }

  String get key => name;

  static AccentColor fromString(String? value) {
    return AccentColor.values.firstWhere(
      (e) => e.key == value,
      orElse: () => AccentColor.indigo,
    );
  }

  int get colorValue {
    switch (this) {
      case AccentColor.indigo:
        return 0xFF6366F1;
      case AccentColor.purple:
        return 0xFF8B5CF6;
      case AccentColor.blue:
        return 0xFF3B82F6;
      case AccentColor.cyan:
        return 0xFF06B6D4;
      case AccentColor.teal:
        return 0xFF14B8A6;
      case AccentColor.green:
        return 0xFF22C55E;
      case AccentColor.orange:
        return 0xFFF97316;
      case AccentColor.red:
        return 0xFFEF4444;
      case AccentColor.pink:
        return 0xFFEC4899;
    }
  }

  int get darkColorValue {
    switch (this) {
      case AccentColor.indigo:
        return 0xFF3730A3;
      case AccentColor.purple:
        return 0xFF5B21B6;
      case AccentColor.blue:
        return 0xFF1D4ED8;
      case AccentColor.cyan:
        return 0xFF0891B2;
      case AccentColor.teal:
        return 0xFF0D9488;
      case AccentColor.green:
        return 0xFF15803D;
      case AccentColor.orange:
        return 0xFFEA580C;
      case AccentColor.red:
        return 0xFFDC2626;
      case AccentColor.pink:
        return 0xFFDB2777;
    }
  }
}

/// Card info display style
enum CardInfoStyle {
  overlay,
  below,
  hidden;

  String get displayName {
    switch (this) {
      case CardInfoStyle.overlay:
        return 'На постері';
      case CardInfoStyle.below:
        return 'Під постером';
      case CardInfoStyle.hidden:
        return 'Приховати';
    }
  }

  String get key => name;

  static CardInfoStyle fromString(String? value) {
    return CardInfoStyle.values.firstWhere(
      (e) => e.key == value,
      orElse: () => CardInfoStyle.overlay,
    );
  }
}

/// List display style for history/favorites
enum ListStyle {
  grid,
  list,
  compact;

  String get displayName {
    switch (this) {
      case ListStyle.grid:
        return 'Сітка';
      case ListStyle.list:
        return 'Список';
      case ListStyle.compact:
        return 'Компактний';
    }
  }

  String get key => name;

  static ListStyle fromString(String? value) {
    return ListStyle.values.firstWhere(
      (e) => e.key == value,
      orElse: () => ListStyle.grid,
    );
  }
}

/// UI settings state
class UISettings {
  final PosterSize posterSize;
  final GridSpacing gridSpacing;
  final AccentColor accentColor;
  final CardInfoStyle cardInfoStyle;
  final ListStyle listStyle;
  final bool showRatings;
  final bool showYears;
  final bool animationsEnabled;
  final bool blurBackgrounds;
  final int gridColumns; // 0 for auto

  const UISettings({
    this.posterSize = PosterSize.medium,
    this.gridSpacing = GridSpacing.normal,
    this.accentColor = AccentColor.indigo,
    this.cardInfoStyle = CardInfoStyle.overlay,
    this.listStyle = ListStyle.grid,
    this.showRatings = true,
    this.showYears = true,
    this.animationsEnabled = true,
    this.blurBackgrounds = true,
    this.gridColumns = 0,
  });

  UISettings copyWith({
    PosterSize? posterSize,
    GridSpacing? gridSpacing,
    AccentColor? accentColor,
    CardInfoStyle? cardInfoStyle,
    ListStyle? listStyle,
    bool? showRatings,
    bool? showYears,
    bool? animationsEnabled,
    bool? blurBackgrounds,
    int? gridColumns,
  }) {
    return UISettings(
      posterSize: posterSize ?? this.posterSize,
      gridSpacing: gridSpacing ?? this.gridSpacing,
      accentColor: accentColor ?? this.accentColor,
      cardInfoStyle: cardInfoStyle ?? this.cardInfoStyle,
      listStyle: listStyle ?? this.listStyle,
      showRatings: showRatings ?? this.showRatings,
      showYears: showYears ?? this.showYears,
      animationsEnabled: animationsEnabled ?? this.animationsEnabled,
      blurBackgrounds: blurBackgrounds ?? this.blurBackgrounds,
      gridColumns: gridColumns ?? this.gridColumns,
    );
  }
}
