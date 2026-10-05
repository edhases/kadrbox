// GENERATED CODE - DO NOT MODIFY BY HAND

part of 'app_database.dart';

// ignore_for_file: type=lint
class $AppSettingsTable extends AppSettings
    with TableInfo<$AppSettingsTable, AppSetting> {
  @override
  final GeneratedDatabase attachedDatabase;
  final String? _alias;
  $AppSettingsTable(this.attachedDatabase, [this._alias]);
  static const VerificationMeta _idMeta = const VerificationMeta('id');
  @override
  late final GeneratedColumn<int> id = GeneratedColumn<int>(
    'id',
    aliasedName,
    false,
    hasAutoIncrement: true,
    type: DriftSqlType.int,
    requiredDuringInsert: false,
    defaultConstraints: GeneratedColumn.constraintIsAlways(
      'PRIMARY KEY AUTOINCREMENT',
    ),
  );
  static const VerificationMeta _keyMeta = const VerificationMeta('key');
  @override
  late final GeneratedColumn<String> key = GeneratedColumn<String>(
    'key',
    aliasedName,
    false,
    type: DriftSqlType.string,
    requiredDuringInsert: true,
    defaultConstraints: GeneratedColumn.constraintIsAlways('UNIQUE'),
  );
  static const VerificationMeta _valueMeta = const VerificationMeta('value');
  @override
  late final GeneratedColumn<String> value = GeneratedColumn<String>(
    'value',
    aliasedName,
    false,
    type: DriftSqlType.string,
    requiredDuringInsert: true,
  );
  static const VerificationMeta _updatedAtMeta = const VerificationMeta(
    'updatedAt',
  );
  @override
  late final GeneratedColumn<DateTime> updatedAt = GeneratedColumn<DateTime>(
    'updated_at',
    aliasedName,
    false,
    type: DriftSqlType.dateTime,
    requiredDuringInsert: false,
    defaultValue: currentDateAndTime,
  );
  @override
  List<GeneratedColumn> get $columns => [id, key, value, updatedAt];
  @override
  String get aliasedName => _alias ?? actualTableName;
  @override
  String get actualTableName => $name;
  static const String $name = 'app_settings';
  @override
  VerificationContext validateIntegrity(
    Insertable<AppSetting> instance, {
    bool isInserting = false,
  }) {
    final context = VerificationContext();
    final data = instance.toColumns(true);
    if (data.containsKey('id')) {
      context.handle(_idMeta, id.isAcceptableOrUnknown(data['id']!, _idMeta));
    }
    if (data.containsKey('key')) {
      context.handle(
        _keyMeta,
        key.isAcceptableOrUnknown(data['key']!, _keyMeta),
      );
    } else if (isInserting) {
      context.missing(_keyMeta);
    }
    if (data.containsKey('value')) {
      context.handle(
        _valueMeta,
        value.isAcceptableOrUnknown(data['value']!, _valueMeta),
      );
    } else if (isInserting) {
      context.missing(_valueMeta);
    }
    if (data.containsKey('updated_at')) {
      context.handle(
        _updatedAtMeta,
        updatedAt.isAcceptableOrUnknown(data['updated_at']!, _updatedAtMeta),
      );
    }
    return context;
  }

  @override
  Set<GeneratedColumn> get $primaryKey => {id};
  @override
  AppSetting map(Map<String, dynamic> data, {String? tablePrefix}) {
    final effectivePrefix = tablePrefix != null ? '$tablePrefix.' : '';
    return AppSetting(
      id: attachedDatabase.typeMapping.read(
        DriftSqlType.int,
        data['${effectivePrefix}id'],
      )!,
      key: attachedDatabase.typeMapping.read(
        DriftSqlType.string,
        data['${effectivePrefix}key'],
      )!,
      value: attachedDatabase.typeMapping.read(
        DriftSqlType.string,
        data['${effectivePrefix}value'],
      )!,
      updatedAt: attachedDatabase.typeMapping.read(
        DriftSqlType.dateTime,
        data['${effectivePrefix}updated_at'],
      )!,
    );
  }

  @override
  $AppSettingsTable createAlias(String alias) {
    return $AppSettingsTable(attachedDatabase, alias);
  }
}

class AppSetting extends DataClass implements Insertable<AppSetting> {
  final int id;
  final String key;
  final String value;
  final DateTime updatedAt;
  const AppSetting({
    required this.id,
    required this.key,
    required this.value,
    required this.updatedAt,
  });
  @override
  Map<String, Expression> toColumns(bool nullToAbsent) {
    final map = <String, Expression>{};
    map['id'] = Variable<int>(id);
    map['key'] = Variable<String>(key);
    map['value'] = Variable<String>(value);
    map['updated_at'] = Variable<DateTime>(updatedAt);
    return map;
  }

  AppSettingsCompanion toCompanion(bool nullToAbsent) {
    return AppSettingsCompanion(
      id: Value(id),
      key: Value(key),
      value: Value(value),
      updatedAt: Value(updatedAt),
    );
  }

  factory AppSetting.fromJson(
    Map<String, dynamic> json, {
    ValueSerializer? serializer,
  }) {
    serializer ??= driftRuntimeOptions.defaultSerializer;
    return AppSetting(
      id: serializer.fromJson<int>(json['id']),
      key: serializer.fromJson<String>(json['key']),
      value: serializer.fromJson<String>(json['value']),
      updatedAt: serializer.fromJson<DateTime>(json['updatedAt']),
    );
  }
  @override
  Map<String, dynamic> toJson({ValueSerializer? serializer}) {
    serializer ??= driftRuntimeOptions.defaultSerializer;
    return <String, dynamic>{
      'id': serializer.toJson<int>(id),
      'key': serializer.toJson<String>(key),
      'value': serializer.toJson<String>(value),
      'updatedAt': serializer.toJson<DateTime>(updatedAt),
    };
  }

  AppSetting copyWith({
    int? id,
    String? key,
    String? value,
    DateTime? updatedAt,
  }) => AppSetting(
    id: id ?? this.id,
    key: key ?? this.key,
    value: value ?? this.value,
    updatedAt: updatedAt ?? this.updatedAt,
  );
  AppSetting copyWithCompanion(AppSettingsCompanion data) {
    return AppSetting(
      id: data.id.present ? data.id.value : this.id,
      key: data.key.present ? data.key.value : this.key,
      value: data.value.present ? data.value.value : this.value,
      updatedAt: data.updatedAt.present ? data.updatedAt.value : this.updatedAt,
    );
  }

  @override
  String toString() {
    return (StringBuffer('AppSetting(')
          ..write('id: $id, ')
          ..write('key: $key, ')
          ..write('value: $value, ')
          ..write('updatedAt: $updatedAt')
          ..write(')'))
        .toString();
  }

  @override
  int get hashCode => Object.hash(id, key, value, updatedAt);
  @override
  bool operator ==(Object other) =>
      identical(this, other) ||
      (other is AppSetting &&
          other.id == this.id &&
          other.key == this.key &&
          other.value == this.value &&
          other.updatedAt == this.updatedAt);
}

class AppSettingsCompanion extends UpdateCompanion<AppSetting> {
  final Value<int> id;
  final Value<String> key;
  final Value<String> value;
  final Value<DateTime> updatedAt;
  const AppSettingsCompanion({
    this.id = const Value.absent(),
    this.key = const Value.absent(),
    this.value = const Value.absent(),
    this.updatedAt = const Value.absent(),
  });
  AppSettingsCompanion.insert({
    this.id = const Value.absent(),
    required String key,
    required String value,
    this.updatedAt = const Value.absent(),
  }) : key = Value(key),
       value = Value(value);
  static Insertable<AppSetting> custom({
    Expression<int>? id,
    Expression<String>? key,
    Expression<String>? value,
    Expression<DateTime>? updatedAt,
  }) {
    return RawValuesInsertable({
      if (id != null) 'id': id,
      if (key != null) 'key': key,
      if (value != null) 'value': value,
      if (updatedAt != null) 'updated_at': updatedAt,
    });
  }

  AppSettingsCompanion copyWith({
    Value<int>? id,
    Value<String>? key,
    Value<String>? value,
    Value<DateTime>? updatedAt,
  }) {
    return AppSettingsCompanion(
      id: id ?? this.id,
      key: key ?? this.key,
      value: value ?? this.value,
      updatedAt: updatedAt ?? this.updatedAt,
    );
  }

  @override
  Map<String, Expression> toColumns(bool nullToAbsent) {
    final map = <String, Expression>{};
    if (id.present) {
      map['id'] = Variable<int>(id.value);
    }
    if (key.present) {
      map['key'] = Variable<String>(key.value);
    }
    if (value.present) {
      map['value'] = Variable<String>(value.value);
    }
    if (updatedAt.present) {
      map['updated_at'] = Variable<DateTime>(updatedAt.value);
    }
    return map;
  }

  @override
  String toString() {
    return (StringBuffer('AppSettingsCompanion(')
          ..write('id: $id, ')
          ..write('key: $key, ')
          ..write('value: $value, ')
          ..write('updatedAt: $updatedAt')
          ..write(')'))
        .toString();
  }
}

class $EnabledProvidersTable extends EnabledProviders
    with TableInfo<$EnabledProvidersTable, EnabledProvider> {
  @override
  final GeneratedDatabase attachedDatabase;
  final String? _alias;
  $EnabledProvidersTable(this.attachedDatabase, [this._alias]);
  static const VerificationMeta _idMeta = const VerificationMeta('id');
  @override
  late final GeneratedColumn<int> id = GeneratedColumn<int>(
    'id',
    aliasedName,
    false,
    hasAutoIncrement: true,
    type: DriftSqlType.int,
    requiredDuringInsert: false,
    defaultConstraints: GeneratedColumn.constraintIsAlways(
      'PRIMARY KEY AUTOINCREMENT',
    ),
  );
  static const VerificationMeta _providerIdMeta = const VerificationMeta(
    'providerId',
  );
  @override
  late final GeneratedColumn<String> providerId = GeneratedColumn<String>(
    'provider_id',
    aliasedName,
    false,
    type: DriftSqlType.string,
    requiredDuringInsert: true,
    defaultConstraints: GeneratedColumn.constraintIsAlways('UNIQUE'),
  );
  static const VerificationMeta _isEnabledMeta = const VerificationMeta(
    'isEnabled',
  );
  @override
  late final GeneratedColumn<bool> isEnabled = GeneratedColumn<bool>(
    'is_enabled',
    aliasedName,
    false,
    type: DriftSqlType.bool,
    requiredDuringInsert: false,
    defaultConstraints: GeneratedColumn.constraintIsAlways(
      'CHECK ("is_enabled" IN (0, 1))',
    ),
    defaultValue: const Constant(true),
  );
  static const VerificationMeta _priorityMeta = const VerificationMeta(
    'priority',
  );
  @override
  late final GeneratedColumn<int> priority = GeneratedColumn<int>(
    'priority',
    aliasedName,
    false,
    type: DriftSqlType.int,
    requiredDuringInsert: false,
    defaultValue: const Constant(0),
  );
  static const VerificationMeta _updatedAtMeta = const VerificationMeta(
    'updatedAt',
  );
  @override
  late final GeneratedColumn<DateTime> updatedAt = GeneratedColumn<DateTime>(
    'updated_at',
    aliasedName,
    false,
    type: DriftSqlType.dateTime,
    requiredDuringInsert: false,
    defaultValue: currentDateAndTime,
  );
  @override
  List<GeneratedColumn> get $columns => [
    id,
    providerId,
    isEnabled,
    priority,
    updatedAt,
  ];
  @override
  String get aliasedName => _alias ?? actualTableName;
  @override
  String get actualTableName => $name;
  static const String $name = 'enabled_providers';
  @override
  VerificationContext validateIntegrity(
    Insertable<EnabledProvider> instance, {
    bool isInserting = false,
  }) {
    final context = VerificationContext();
    final data = instance.toColumns(true);
    if (data.containsKey('id')) {
      context.handle(_idMeta, id.isAcceptableOrUnknown(data['id']!, _idMeta));
    }
    if (data.containsKey('provider_id')) {
      context.handle(
        _providerIdMeta,
        providerId.isAcceptableOrUnknown(data['provider_id']!, _providerIdMeta),
      );
    } else if (isInserting) {
      context.missing(_providerIdMeta);
    }
    if (data.containsKey('is_enabled')) {
      context.handle(
        _isEnabledMeta,
        isEnabled.isAcceptableOrUnknown(data['is_enabled']!, _isEnabledMeta),
      );
    }
    if (data.containsKey('priority')) {
      context.handle(
        _priorityMeta,
        priority.isAcceptableOrUnknown(data['priority']!, _priorityMeta),
      );
    }
    if (data.containsKey('updated_at')) {
      context.handle(
        _updatedAtMeta,
        updatedAt.isAcceptableOrUnknown(data['updated_at']!, _updatedAtMeta),
      );
    }
    return context;
  }

  @override
  Set<GeneratedColumn> get $primaryKey => {id};
  @override
  EnabledProvider map(Map<String, dynamic> data, {String? tablePrefix}) {
    final effectivePrefix = tablePrefix != null ? '$tablePrefix.' : '';
    return EnabledProvider(
      id: attachedDatabase.typeMapping.read(
        DriftSqlType.int,
        data['${effectivePrefix}id'],
      )!,
      providerId: attachedDatabase.typeMapping.read(
        DriftSqlType.string,
        data['${effectivePrefix}provider_id'],
      )!,
      isEnabled: attachedDatabase.typeMapping.read(
        DriftSqlType.bool,
        data['${effectivePrefix}is_enabled'],
      )!,
      priority: attachedDatabase.typeMapping.read(
        DriftSqlType.int,
        data['${effectivePrefix}priority'],
      )!,
      updatedAt: attachedDatabase.typeMapping.read(
        DriftSqlType.dateTime,
        data['${effectivePrefix}updated_at'],
      )!,
    );
  }

  @override
  $EnabledProvidersTable createAlias(String alias) {
    return $EnabledProvidersTable(attachedDatabase, alias);
  }
}

class EnabledProvider extends DataClass implements Insertable<EnabledProvider> {
  final int id;
  final String providerId;
  final bool isEnabled;
  final int priority;
  final DateTime updatedAt;
  const EnabledProvider({
    required this.id,
    required this.providerId,
    required this.isEnabled,
    required this.priority,
    required this.updatedAt,
  });
  @override
  Map<String, Expression> toColumns(bool nullToAbsent) {
    final map = <String, Expression>{};
    map['id'] = Variable<int>(id);
    map['provider_id'] = Variable<String>(providerId);
    map['is_enabled'] = Variable<bool>(isEnabled);
    map['priority'] = Variable<int>(priority);
    map['updated_at'] = Variable<DateTime>(updatedAt);
    return map;
  }

  EnabledProvidersCompanion toCompanion(bool nullToAbsent) {
    return EnabledProvidersCompanion(
      id: Value(id),
      providerId: Value(providerId),
      isEnabled: Value(isEnabled),
      priority: Value(priority),
      updatedAt: Value(updatedAt),
    );
  }

  factory EnabledProvider.fromJson(
    Map<String, dynamic> json, {
    ValueSerializer? serializer,
  }) {
    serializer ??= driftRuntimeOptions.defaultSerializer;
    return EnabledProvider(
      id: serializer.fromJson<int>(json['id']),
      providerId: serializer.fromJson<String>(json['providerId']),
      isEnabled: serializer.fromJson<bool>(json['isEnabled']),
      priority: serializer.fromJson<int>(json['priority']),
      updatedAt: serializer.fromJson<DateTime>(json['updatedAt']),
    );
  }
  @override
  Map<String, dynamic> toJson({ValueSerializer? serializer}) {
    serializer ??= driftRuntimeOptions.defaultSerializer;
    return <String, dynamic>{
      'id': serializer.toJson<int>(id),
      'providerId': serializer.toJson<String>(providerId),
      'isEnabled': serializer.toJson<bool>(isEnabled),
      'priority': serializer.toJson<int>(priority),
      'updatedAt': serializer.toJson<DateTime>(updatedAt),
    };
  }

  EnabledProvider copyWith({
    int? id,
    String? providerId,
    bool? isEnabled,
    int? priority,
    DateTime? updatedAt,
  }) => EnabledProvider(
    id: id ?? this.id,
    providerId: providerId ?? this.providerId,
    isEnabled: isEnabled ?? this.isEnabled,
    priority: priority ?? this.priority,
    updatedAt: updatedAt ?? this.updatedAt,
  );
  EnabledProvider copyWithCompanion(EnabledProvidersCompanion data) {
    return EnabledProvider(
      id: data.id.present ? data.id.value : this.id,
      providerId: data.providerId.present
          ? data.providerId.value
          : this.providerId,
      isEnabled: data.isEnabled.present ? data.isEnabled.value : this.isEnabled,
      priority: data.priority.present ? data.priority.value : this.priority,
      updatedAt: data.updatedAt.present ? data.updatedAt.value : this.updatedAt,
    );
  }

  @override
  String toString() {
    return (StringBuffer('EnabledProvider(')
          ..write('id: $id, ')
          ..write('providerId: $providerId, ')
          ..write('isEnabled: $isEnabled, ')
          ..write('priority: $priority, ')
          ..write('updatedAt: $updatedAt')
          ..write(')'))
        .toString();
  }

  @override
  int get hashCode =>
      Object.hash(id, providerId, isEnabled, priority, updatedAt);
  @override
  bool operator ==(Object other) =>
      identical(this, other) ||
      (other is EnabledProvider &&
          other.id == this.id &&
          other.providerId == this.providerId &&
          other.isEnabled == this.isEnabled &&
          other.priority == this.priority &&
          other.updatedAt == this.updatedAt);
}

class EnabledProvidersCompanion extends UpdateCompanion<EnabledProvider> {
  final Value<int> id;
  final Value<String> providerId;
  final Value<bool> isEnabled;
  final Value<int> priority;
  final Value<DateTime> updatedAt;
  const EnabledProvidersCompanion({
    this.id = const Value.absent(),
    this.providerId = const Value.absent(),
    this.isEnabled = const Value.absent(),
    this.priority = const Value.absent(),
    this.updatedAt = const Value.absent(),
  });
  EnabledProvidersCompanion.insert({
    this.id = const Value.absent(),
    required String providerId,
    this.isEnabled = const Value.absent(),
    this.priority = const Value.absent(),
    this.updatedAt = const Value.absent(),
  }) : providerId = Value(providerId);
  static Insertable<EnabledProvider> custom({
    Expression<int>? id,
    Expression<String>? providerId,
    Expression<bool>? isEnabled,
    Expression<int>? priority,
    Expression<DateTime>? updatedAt,
  }) {
    return RawValuesInsertable({
      if (id != null) 'id': id,
      if (providerId != null) 'provider_id': providerId,
      if (isEnabled != null) 'is_enabled': isEnabled,
      if (priority != null) 'priority': priority,
      if (updatedAt != null) 'updated_at': updatedAt,
    });
  }

  EnabledProvidersCompanion copyWith({
    Value<int>? id,
    Value<String>? providerId,
    Value<bool>? isEnabled,
    Value<int>? priority,
    Value<DateTime>? updatedAt,
  }) {
    return EnabledProvidersCompanion(
      id: id ?? this.id,
      providerId: providerId ?? this.providerId,
      isEnabled: isEnabled ?? this.isEnabled,
      priority: priority ?? this.priority,
      updatedAt: updatedAt ?? this.updatedAt,
    );
  }

  @override
  Map<String, Expression> toColumns(bool nullToAbsent) {
    final map = <String, Expression>{};
    if (id.present) {
      map['id'] = Variable<int>(id.value);
    }
    if (providerId.present) {
      map['provider_id'] = Variable<String>(providerId.value);
    }
    if (isEnabled.present) {
      map['is_enabled'] = Variable<bool>(isEnabled.value);
    }
    if (priority.present) {
      map['priority'] = Variable<int>(priority.value);
    }
    if (updatedAt.present) {
      map['updated_at'] = Variable<DateTime>(updatedAt.value);
    }
    return map;
  }

  @override
  String toString() {
    return (StringBuffer('EnabledProvidersCompanion(')
          ..write('id: $id, ')
          ..write('providerId: $providerId, ')
          ..write('isEnabled: $isEnabled, ')
          ..write('priority: $priority, ')
          ..write('updatedAt: $updatedAt')
          ..write(')'))
        .toString();
  }
}

class $FavoritesTable extends Favorites
    with TableInfo<$FavoritesTable, Favorite> {
  @override
  final GeneratedDatabase attachedDatabase;
  final String? _alias;
  $FavoritesTable(this.attachedDatabase, [this._alias]);
  static const VerificationMeta _idMeta = const VerificationMeta('id');
  @override
  late final GeneratedColumn<int> id = GeneratedColumn<int>(
    'id',
    aliasedName,
    false,
    hasAutoIncrement: true,
    type: DriftSqlType.int,
    requiredDuringInsert: false,
    defaultConstraints: GeneratedColumn.constraintIsAlways(
      'PRIMARY KEY AUTOINCREMENT',
    ),
  );
  static const VerificationMeta _mediaIdMeta = const VerificationMeta(
    'mediaId',
  );
  @override
  late final GeneratedColumn<String> mediaId = GeneratedColumn<String>(
    'media_id',
    aliasedName,
    false,
    type: DriftSqlType.string,
    requiredDuringInsert: true,
  );
  static const VerificationMeta _providerIdMeta = const VerificationMeta(
    'providerId',
  );
  @override
  late final GeneratedColumn<String> providerId = GeneratedColumn<String>(
    'provider_id',
    aliasedName,
    false,
    type: DriftSqlType.string,
    requiredDuringInsert: true,
  );
  static const VerificationMeta _titleMeta = const VerificationMeta('title');
  @override
  late final GeneratedColumn<String> title = GeneratedColumn<String>(
    'title',
    aliasedName,
    false,
    type: DriftSqlType.string,
    requiredDuringInsert: true,
  );
  static const VerificationMeta _posterUrlMeta = const VerificationMeta(
    'posterUrl',
  );
  @override
  late final GeneratedColumn<String> posterUrl = GeneratedColumn<String>(
    'poster_url',
    aliasedName,
    true,
    type: DriftSqlType.string,
    requiredDuringInsert: false,
  );
  static const VerificationMeta _yearMeta = const VerificationMeta('year');
  @override
  late final GeneratedColumn<int> year = GeneratedColumn<int>(
    'year',
    aliasedName,
    true,
    type: DriftSqlType.int,
    requiredDuringInsert: false,
  );
  static const VerificationMeta _ratingMeta = const VerificationMeta('rating');
  @override
  late final GeneratedColumn<double> rating = GeneratedColumn<double>(
    'rating',
    aliasedName,
    true,
    type: DriftSqlType.double,
    requiredDuringInsert: false,
  );
  static const VerificationMeta _ratingSourceMeta = const VerificationMeta(
    'ratingSource',
  );
  @override
  late final GeneratedColumn<String> ratingSource = GeneratedColumn<String>(
    'rating_source',
    aliasedName,
    true,
    type: DriftSqlType.string,
    requiredDuringInsert: false,
  );
  static const VerificationMeta _mediaTypeMeta = const VerificationMeta(
    'mediaType',
  );
  @override
  late final GeneratedColumn<String> mediaType = GeneratedColumn<String>(
    'media_type',
    aliasedName,
    false,
    type: DriftSqlType.string,
    requiredDuringInsert: true,
  );
  static const VerificationMeta _addedAtMeta = const VerificationMeta(
    'addedAt',
  );
  @override
  late final GeneratedColumn<DateTime> addedAt = GeneratedColumn<DateTime>(
    'added_at',
    aliasedName,
    false,
    type: DriftSqlType.dateTime,
    requiredDuringInsert: false,
    defaultValue: currentDateAndTime,
  );
  @override
  List<GeneratedColumn> get $columns => [
    id,
    mediaId,
    providerId,
    title,
    posterUrl,
    year,
    rating,
    ratingSource,
    mediaType,
    addedAt,
  ];
  @override
  String get aliasedName => _alias ?? actualTableName;
  @override
  String get actualTableName => $name;
  static const String $name = 'favorites';
  @override
  VerificationContext validateIntegrity(
    Insertable<Favorite> instance, {
    bool isInserting = false,
  }) {
    final context = VerificationContext();
    final data = instance.toColumns(true);
    if (data.containsKey('id')) {
      context.handle(_idMeta, id.isAcceptableOrUnknown(data['id']!, _idMeta));
    }
    if (data.containsKey('media_id')) {
      context.handle(
        _mediaIdMeta,
        mediaId.isAcceptableOrUnknown(data['media_id']!, _mediaIdMeta),
      );
    } else if (isInserting) {
      context.missing(_mediaIdMeta);
    }
    if (data.containsKey('provider_id')) {
      context.handle(
        _providerIdMeta,
        providerId.isAcceptableOrUnknown(data['provider_id']!, _providerIdMeta),
      );
    } else if (isInserting) {
      context.missing(_providerIdMeta);
    }
    if (data.containsKey('title')) {
      context.handle(
        _titleMeta,
        title.isAcceptableOrUnknown(data['title']!, _titleMeta),
      );
    } else if (isInserting) {
      context.missing(_titleMeta);
    }
    if (data.containsKey('poster_url')) {
      context.handle(
        _posterUrlMeta,
        posterUrl.isAcceptableOrUnknown(data['poster_url']!, _posterUrlMeta),
      );
    }
    if (data.containsKey('year')) {
      context.handle(
        _yearMeta,
        year.isAcceptableOrUnknown(data['year']!, _yearMeta),
      );
    }
    if (data.containsKey('rating')) {
      context.handle(
        _ratingMeta,
        rating.isAcceptableOrUnknown(data['rating']!, _ratingMeta),
      );
    }
    if (data.containsKey('rating_source')) {
      context.handle(
        _ratingSourceMeta,
        ratingSource.isAcceptableOrUnknown(
          data['rating_source']!,
          _ratingSourceMeta,
        ),
      );
    }
    if (data.containsKey('media_type')) {
      context.handle(
        _mediaTypeMeta,
        mediaType.isAcceptableOrUnknown(data['media_type']!, _mediaTypeMeta),
      );
    } else if (isInserting) {
      context.missing(_mediaTypeMeta);
    }
    if (data.containsKey('added_at')) {
      context.handle(
        _addedAtMeta,
        addedAt.isAcceptableOrUnknown(data['added_at']!, _addedAtMeta),
      );
    }
    return context;
  }

  @override
  Set<GeneratedColumn> get $primaryKey => {id};
  @override
  List<Set<GeneratedColumn>> get uniqueKeys => [
    {mediaId, providerId},
  ];
  @override
  Favorite map(Map<String, dynamic> data, {String? tablePrefix}) {
    final effectivePrefix = tablePrefix != null ? '$tablePrefix.' : '';
    return Favorite(
      id: attachedDatabase.typeMapping.read(
        DriftSqlType.int,
        data['${effectivePrefix}id'],
      )!,
      mediaId: attachedDatabase.typeMapping.read(
        DriftSqlType.string,
        data['${effectivePrefix}media_id'],
      )!,
      providerId: attachedDatabase.typeMapping.read(
        DriftSqlType.string,
        data['${effectivePrefix}provider_id'],
      )!,
      title: attachedDatabase.typeMapping.read(
        DriftSqlType.string,
        data['${effectivePrefix}title'],
      )!,
      posterUrl: attachedDatabase.typeMapping.read(
        DriftSqlType.string,
        data['${effectivePrefix}poster_url'],
      ),
      year: attachedDatabase.typeMapping.read(
        DriftSqlType.int,
        data['${effectivePrefix}year'],
      ),
      rating: attachedDatabase.typeMapping.read(
        DriftSqlType.double,
        data['${effectivePrefix}rating'],
      ),
      ratingSource: attachedDatabase.typeMapping.read(
        DriftSqlType.string,
        data['${effectivePrefix}rating_source'],
      ),
      mediaType: attachedDatabase.typeMapping.read(
        DriftSqlType.string,
        data['${effectivePrefix}media_type'],
      )!,
      addedAt: attachedDatabase.typeMapping.read(
        DriftSqlType.dateTime,
        data['${effectivePrefix}added_at'],
      )!,
    );
  }

  @override
  $FavoritesTable createAlias(String alias) {
    return $FavoritesTable(attachedDatabase, alias);
  }
}

class Favorite extends DataClass implements Insertable<Favorite> {
  final int id;
  final String mediaId;
  final String providerId;
  final String title;
  final String? posterUrl;
  final int? year;
  final double? rating;
  final String? ratingSource;
  final String mediaType;
  final DateTime addedAt;
  const Favorite({
    required this.id,
    required this.mediaId,
    required this.providerId,
    required this.title,
    this.posterUrl,
    this.year,
    this.rating,
    this.ratingSource,
    required this.mediaType,
    required this.addedAt,
  });
  @override
  Map<String, Expression> toColumns(bool nullToAbsent) {
    final map = <String, Expression>{};
    map['id'] = Variable<int>(id);
    map['media_id'] = Variable<String>(mediaId);
    map['provider_id'] = Variable<String>(providerId);
    map['title'] = Variable<String>(title);
    if (!nullToAbsent || posterUrl != null) {
      map['poster_url'] = Variable<String>(posterUrl);
    }
    if (!nullToAbsent || year != null) {
      map['year'] = Variable<int>(year);
    }
    if (!nullToAbsent || rating != null) {
      map['rating'] = Variable<double>(rating);
    }
    if (!nullToAbsent || ratingSource != null) {
      map['rating_source'] = Variable<String>(ratingSource);
    }
    map['media_type'] = Variable<String>(mediaType);
    map['added_at'] = Variable<DateTime>(addedAt);
    return map;
  }

  FavoritesCompanion toCompanion(bool nullToAbsent) {
    return FavoritesCompanion(
      id: Value(id),
      mediaId: Value(mediaId),
      providerId: Value(providerId),
      title: Value(title),
      posterUrl: posterUrl == null && nullToAbsent
          ? const Value.absent()
          : Value(posterUrl),
      year: year == null && nullToAbsent ? const Value.absent() : Value(year),
      rating: rating == null && nullToAbsent
          ? const Value.absent()
          : Value(rating),
      ratingSource: ratingSource == null && nullToAbsent
          ? const Value.absent()
          : Value(ratingSource),
      mediaType: Value(mediaType),
      addedAt: Value(addedAt),
    );
  }

  factory Favorite.fromJson(
    Map<String, dynamic> json, {
    ValueSerializer? serializer,
  }) {
    serializer ??= driftRuntimeOptions.defaultSerializer;
    return Favorite(
      id: serializer.fromJson<int>(json['id']),
      mediaId: serializer.fromJson<String>(json['mediaId']),
      providerId: serializer.fromJson<String>(json['providerId']),
      title: serializer.fromJson<String>(json['title']),
      posterUrl: serializer.fromJson<String?>(json['posterUrl']),
      year: serializer.fromJson<int?>(json['year']),
      rating: serializer.fromJson<double?>(json['rating']),
      ratingSource: serializer.fromJson<String?>(json['ratingSource']),
      mediaType: serializer.fromJson<String>(json['mediaType']),
      addedAt: serializer.fromJson<DateTime>(json['addedAt']),
    );
  }
  @override
  Map<String, dynamic> toJson({ValueSerializer? serializer}) {
    serializer ??= driftRuntimeOptions.defaultSerializer;
    return <String, dynamic>{
      'id': serializer.toJson<int>(id),
      'mediaId': serializer.toJson<String>(mediaId),
      'providerId': serializer.toJson<String>(providerId),
      'title': serializer.toJson<String>(title),
      'posterUrl': serializer.toJson<String?>(posterUrl),
      'year': serializer.toJson<int?>(year),
      'rating': serializer.toJson<double?>(rating),
      'ratingSource': serializer.toJson<String?>(ratingSource),
      'mediaType': serializer.toJson<String>(mediaType),
      'addedAt': serializer.toJson<DateTime>(addedAt),
    };
  }

  Favorite copyWith({
    int? id,
    String? mediaId,
    String? providerId,
    String? title,
    Value<String?> posterUrl = const Value.absent(),
    Value<int?> year = const Value.absent(),
    Value<double?> rating = const Value.absent(),
    Value<String?> ratingSource = const Value.absent(),
    String? mediaType,
    DateTime? addedAt,
  }) => Favorite(
    id: id ?? this.id,
    mediaId: mediaId ?? this.mediaId,
    providerId: providerId ?? this.providerId,
    title: title ?? this.title,
    posterUrl: posterUrl.present ? posterUrl.value : this.posterUrl,
    year: year.present ? year.value : this.year,
    rating: rating.present ? rating.value : this.rating,
    ratingSource: ratingSource.present ? ratingSource.value : this.ratingSource,
    mediaType: mediaType ?? this.mediaType,
    addedAt: addedAt ?? this.addedAt,
  );
  Favorite copyWithCompanion(FavoritesCompanion data) {
    return Favorite(
      id: data.id.present ? data.id.value : this.id,
      mediaId: data.mediaId.present ? data.mediaId.value : this.mediaId,
      providerId: data.providerId.present
          ? data.providerId.value
          : this.providerId,
      title: data.title.present ? data.title.value : this.title,
      posterUrl: data.posterUrl.present ? data.posterUrl.value : this.posterUrl,
      year: data.year.present ? data.year.value : this.year,
      rating: data.rating.present ? data.rating.value : this.rating,
      ratingSource: data.ratingSource.present
          ? data.ratingSource.value
          : this.ratingSource,
      mediaType: data.mediaType.present ? data.mediaType.value : this.mediaType,
      addedAt: data.addedAt.present ? data.addedAt.value : this.addedAt,
    );
  }

  @override
  String toString() {
    return (StringBuffer('Favorite(')
          ..write('id: $id, ')
          ..write('mediaId: $mediaId, ')
          ..write('providerId: $providerId, ')
          ..write('title: $title, ')
          ..write('posterUrl: $posterUrl, ')
          ..write('year: $year, ')
          ..write('rating: $rating, ')
          ..write('ratingSource: $ratingSource, ')
          ..write('mediaType: $mediaType, ')
          ..write('addedAt: $addedAt')
          ..write(')'))
        .toString();
  }

  @override
  int get hashCode => Object.hash(
    id,
    mediaId,
    providerId,
    title,
    posterUrl,
    year,
    rating,
    ratingSource,
    mediaType,
    addedAt,
  );
  @override
  bool operator ==(Object other) =>
      identical(this, other) ||
      (other is Favorite &&
          other.id == this.id &&
          other.mediaId == this.mediaId &&
          other.providerId == this.providerId &&
          other.title == this.title &&
          other.posterUrl == this.posterUrl &&
          other.year == this.year &&
          other.rating == this.rating &&
          other.ratingSource == this.ratingSource &&
          other.mediaType == this.mediaType &&
          other.addedAt == this.addedAt);
}

class FavoritesCompanion extends UpdateCompanion<Favorite> {
  final Value<int> id;
  final Value<String> mediaId;
  final Value<String> providerId;
  final Value<String> title;
  final Value<String?> posterUrl;
  final Value<int?> year;
  final Value<double?> rating;
  final Value<String?> ratingSource;
  final Value<String> mediaType;
  final Value<DateTime> addedAt;
  const FavoritesCompanion({
    this.id = const Value.absent(),
    this.mediaId = const Value.absent(),
    this.providerId = const Value.absent(),
    this.title = const Value.absent(),
    this.posterUrl = const Value.absent(),
    this.year = const Value.absent(),
    this.rating = const Value.absent(),
    this.ratingSource = const Value.absent(),
    this.mediaType = const Value.absent(),
    this.addedAt = const Value.absent(),
  });
  FavoritesCompanion.insert({
    this.id = const Value.absent(),
    required String mediaId,
    required String providerId,
    required String title,
    this.posterUrl = const Value.absent(),
    this.year = const Value.absent(),
    this.rating = const Value.absent(),
    this.ratingSource = const Value.absent(),
    required String mediaType,
    this.addedAt = const Value.absent(),
  }) : mediaId = Value(mediaId),
       providerId = Value(providerId),
       title = Value(title),
       mediaType = Value(mediaType);
  static Insertable<Favorite> custom({
    Expression<int>? id,
    Expression<String>? mediaId,
    Expression<String>? providerId,
    Expression<String>? title,
    Expression<String>? posterUrl,
    Expression<int>? year,
    Expression<double>? rating,
    Expression<String>? ratingSource,
    Expression<String>? mediaType,
    Expression<DateTime>? addedAt,
  }) {
    return RawValuesInsertable({
      if (id != null) 'id': id,
      if (mediaId != null) 'media_id': mediaId,
      if (providerId != null) 'provider_id': providerId,
      if (title != null) 'title': title,
      if (posterUrl != null) 'poster_url': posterUrl,
      if (year != null) 'year': year,
      if (rating != null) 'rating': rating,
      if (ratingSource != null) 'rating_source': ratingSource,
      if (mediaType != null) 'media_type': mediaType,
      if (addedAt != null) 'added_at': addedAt,
    });
  }

  FavoritesCompanion copyWith({
    Value<int>? id,
    Value<String>? mediaId,
    Value<String>? providerId,
    Value<String>? title,
    Value<String?>? posterUrl,
    Value<int?>? year,
    Value<double?>? rating,
    Value<String?>? ratingSource,
    Value<String>? mediaType,
    Value<DateTime>? addedAt,
  }) {
    return FavoritesCompanion(
      id: id ?? this.id,
      mediaId: mediaId ?? this.mediaId,
      providerId: providerId ?? this.providerId,
      title: title ?? this.title,
      posterUrl: posterUrl ?? this.posterUrl,
      year: year ?? this.year,
      rating: rating ?? this.rating,
      ratingSource: ratingSource ?? this.ratingSource,
      mediaType: mediaType ?? this.mediaType,
      addedAt: addedAt ?? this.addedAt,
    );
  }

  @override
  Map<String, Expression> toColumns(bool nullToAbsent) {
    final map = <String, Expression>{};
    if (id.present) {
      map['id'] = Variable<int>(id.value);
    }
    if (mediaId.present) {
      map['media_id'] = Variable<String>(mediaId.value);
    }
    if (providerId.present) {
      map['provider_id'] = Variable<String>(providerId.value);
    }
    if (title.present) {
      map['title'] = Variable<String>(title.value);
    }
    if (posterUrl.present) {
      map['poster_url'] = Variable<String>(posterUrl.value);
    }
    if (year.present) {
      map['year'] = Variable<int>(year.value);
    }
    if (rating.present) {
      map['rating'] = Variable<double>(rating.value);
    }
    if (ratingSource.present) {
      map['rating_source'] = Variable<String>(ratingSource.value);
    }
    if (mediaType.present) {
      map['media_type'] = Variable<String>(mediaType.value);
    }
    if (addedAt.present) {
      map['added_at'] = Variable<DateTime>(addedAt.value);
    }
    return map;
  }

  @override
  String toString() {
    return (StringBuffer('FavoritesCompanion(')
          ..write('id: $id, ')
          ..write('mediaId: $mediaId, ')
          ..write('providerId: $providerId, ')
          ..write('title: $title, ')
          ..write('posterUrl: $posterUrl, ')
          ..write('year: $year, ')
          ..write('rating: $rating, ')
          ..write('ratingSource: $ratingSource, ')
          ..write('mediaType: $mediaType, ')
          ..write('addedAt: $addedAt')
          ..write(')'))
        .toString();
  }
}

class $WatchHistoryTable extends WatchHistory
    with TableInfo<$WatchHistoryTable, WatchHistoryData> {
  @override
  final GeneratedDatabase attachedDatabase;
  final String? _alias;
  $WatchHistoryTable(this.attachedDatabase, [this._alias]);
  static const VerificationMeta _idMeta = const VerificationMeta('id');
  @override
  late final GeneratedColumn<int> id = GeneratedColumn<int>(
    'id',
    aliasedName,
    false,
    hasAutoIncrement: true,
    type: DriftSqlType.int,
    requiredDuringInsert: false,
    defaultConstraints: GeneratedColumn.constraintIsAlways(
      'PRIMARY KEY AUTOINCREMENT',
    ),
  );
  static const VerificationMeta _mediaIdMeta = const VerificationMeta(
    'mediaId',
  );
  @override
  late final GeneratedColumn<String> mediaId = GeneratedColumn<String>(
    'media_id',
    aliasedName,
    false,
    type: DriftSqlType.string,
    requiredDuringInsert: true,
  );
  static const VerificationMeta _providerIdMeta = const VerificationMeta(
    'providerId',
  );
  @override
  late final GeneratedColumn<String> providerId = GeneratedColumn<String>(
    'provider_id',
    aliasedName,
    false,
    type: DriftSqlType.string,
    requiredDuringInsert: true,
  );
  static const VerificationMeta _titleMeta = const VerificationMeta('title');
  @override
  late final GeneratedColumn<String> title = GeneratedColumn<String>(
    'title',
    aliasedName,
    false,
    type: DriftSqlType.string,
    requiredDuringInsert: true,
  );
  static const VerificationMeta _posterUrlMeta = const VerificationMeta(
    'posterUrl',
  );
  @override
  late final GeneratedColumn<String> posterUrl = GeneratedColumn<String>(
    'poster_url',
    aliasedName,
    true,
    type: DriftSqlType.string,
    requiredDuringInsert: false,
  );
  static const VerificationMeta _yearMeta = const VerificationMeta('year');
  @override
  late final GeneratedColumn<int> year = GeneratedColumn<int>(
    'year',
    aliasedName,
    true,
    type: DriftSqlType.int,
    requiredDuringInsert: false,
  );
  static const VerificationMeta _ratingMeta = const VerificationMeta('rating');
  @override
  late final GeneratedColumn<double> rating = GeneratedColumn<double>(
    'rating',
    aliasedName,
    true,
    type: DriftSqlType.double,
    requiredDuringInsert: false,
  );
  static const VerificationMeta _ratingSourceMeta = const VerificationMeta(
    'ratingSource',
  );
  @override
  late final GeneratedColumn<String> ratingSource = GeneratedColumn<String>(
    'rating_source',
    aliasedName,
    true,
    type: DriftSqlType.string,
    requiredDuringInsert: false,
  );
  static const VerificationMeta _mediaTypeMeta = const VerificationMeta(
    'mediaType',
  );
  @override
  late final GeneratedColumn<String> mediaType = GeneratedColumn<String>(
    'media_type',
    aliasedName,
    false,
    type: DriftSqlType.string,
    requiredDuringInsert: true,
  );
  static const VerificationMeta _positionMsMeta = const VerificationMeta(
    'positionMs',
  );
  @override
  late final GeneratedColumn<int> positionMs = GeneratedColumn<int>(
    'position_ms',
    aliasedName,
    false,
    type: DriftSqlType.int,
    requiredDuringInsert: false,
    defaultValue: const Constant(0),
  );
  static const VerificationMeta _durationMsMeta = const VerificationMeta(
    'durationMs',
  );
  @override
  late final GeneratedColumn<int> durationMs = GeneratedColumn<int>(
    'duration_ms',
    aliasedName,
    false,
    type: DriftSqlType.int,
    requiredDuringInsert: false,
    defaultValue: const Constant(0),
  );
  static const VerificationMeta _seasonMeta = const VerificationMeta('season');
  @override
  late final GeneratedColumn<int> season = GeneratedColumn<int>(
    'season',
    aliasedName,
    true,
    type: DriftSqlType.int,
    requiredDuringInsert: false,
  );
  static const VerificationMeta _episodeMeta = const VerificationMeta(
    'episode',
  );
  @override
  late final GeneratedColumn<int> episode = GeneratedColumn<int>(
    'episode',
    aliasedName,
    true,
    type: DriftSqlType.int,
    requiredDuringInsert: false,
  );
  static const VerificationMeta _episodeTitleMeta = const VerificationMeta(
    'episodeTitle',
  );
  @override
  late final GeneratedColumn<String> episodeTitle = GeneratedColumn<String>(
    'episode_title',
    aliasedName,
    true,
    type: DriftSqlType.string,
    requiredDuringInsert: false,
  );
  static const VerificationMeta _lastStreamUrlMeta = const VerificationMeta(
    'lastStreamUrl',
  );
  @override
  late final GeneratedColumn<String> lastStreamUrl = GeneratedColumn<String>(
    'last_stream_url',
    aliasedName,
    true,
    type: DriftSqlType.string,
    requiredDuringInsert: false,
  );
  static const VerificationMeta _voiceoverMeta = const VerificationMeta(
    'voiceover',
  );
  @override
  late final GeneratedColumn<String> voiceover = GeneratedColumn<String>(
    'voiceover',
    aliasedName,
    true,
    type: DriftSqlType.string,
    requiredDuringInsert: false,
  );
  static const VerificationMeta _watchedAtMeta = const VerificationMeta(
    'watchedAt',
  );
  @override
  late final GeneratedColumn<DateTime> watchedAt = GeneratedColumn<DateTime>(
    'watched_at',
    aliasedName,
    false,
    type: DriftSqlType.dateTime,
    requiredDuringInsert: false,
    defaultValue: currentDateAndTime,
  );
  @override
  List<GeneratedColumn> get $columns => [
    id,
    mediaId,
    providerId,
    title,
    posterUrl,
    year,
    rating,
    ratingSource,
    mediaType,
    positionMs,
    durationMs,
    season,
    episode,
    episodeTitle,
    lastStreamUrl,
    voiceover,
    watchedAt,
  ];
  @override
  String get aliasedName => _alias ?? actualTableName;
  @override
  String get actualTableName => $name;
  static const String $name = 'watch_history';
  @override
  VerificationContext validateIntegrity(
    Insertable<WatchHistoryData> instance, {
    bool isInserting = false,
  }) {
    final context = VerificationContext();
    final data = instance.toColumns(true);
    if (data.containsKey('id')) {
      context.handle(_idMeta, id.isAcceptableOrUnknown(data['id']!, _idMeta));
    }
    if (data.containsKey('media_id')) {
      context.handle(
        _mediaIdMeta,
        mediaId.isAcceptableOrUnknown(data['media_id']!, _mediaIdMeta),
      );
    } else if (isInserting) {
      context.missing(_mediaIdMeta);
    }
    if (data.containsKey('provider_id')) {
      context.handle(
        _providerIdMeta,
        providerId.isAcceptableOrUnknown(data['provider_id']!, _providerIdMeta),
      );
    } else if (isInserting) {
      context.missing(_providerIdMeta);
    }
    if (data.containsKey('title')) {
      context.handle(
        _titleMeta,
        title.isAcceptableOrUnknown(data['title']!, _titleMeta),
      );
    } else if (isInserting) {
      context.missing(_titleMeta);
    }
    if (data.containsKey('poster_url')) {
      context.handle(
        _posterUrlMeta,
        posterUrl.isAcceptableOrUnknown(data['poster_url']!, _posterUrlMeta),
      );
    }
    if (data.containsKey('year')) {
      context.handle(
        _yearMeta,
        year.isAcceptableOrUnknown(data['year']!, _yearMeta),
      );
    }
    if (data.containsKey('rating')) {
      context.handle(
        _ratingMeta,
        rating.isAcceptableOrUnknown(data['rating']!, _ratingMeta),
      );
    }
    if (data.containsKey('rating_source')) {
      context.handle(
        _ratingSourceMeta,
        ratingSource.isAcceptableOrUnknown(
          data['rating_source']!,
          _ratingSourceMeta,
        ),
      );
    }
    if (data.containsKey('media_type')) {
      context.handle(
        _mediaTypeMeta,
        mediaType.isAcceptableOrUnknown(data['media_type']!, _mediaTypeMeta),
      );
    } else if (isInserting) {
      context.missing(_mediaTypeMeta);
    }
    if (data.containsKey('position_ms')) {
      context.handle(
        _positionMsMeta,
        positionMs.isAcceptableOrUnknown(data['position_ms']!, _positionMsMeta),
      );
    }
    if (data.containsKey('duration_ms')) {
      context.handle(
        _durationMsMeta,
        durationMs.isAcceptableOrUnknown(data['duration_ms']!, _durationMsMeta),
      );
    }
    if (data.containsKey('season')) {
      context.handle(
        _seasonMeta,
        season.isAcceptableOrUnknown(data['season']!, _seasonMeta),
      );
    }
    if (data.containsKey('episode')) {
      context.handle(
        _episodeMeta,
        episode.isAcceptableOrUnknown(data['episode']!, _episodeMeta),
      );
    }
    if (data.containsKey('episode_title')) {
      context.handle(
        _episodeTitleMeta,
        episodeTitle.isAcceptableOrUnknown(
          data['episode_title']!,
          _episodeTitleMeta,
        ),
      );
    }
    if (data.containsKey('last_stream_url')) {
      context.handle(
        _lastStreamUrlMeta,
        lastStreamUrl.isAcceptableOrUnknown(
          data['last_stream_url']!,
          _lastStreamUrlMeta,
        ),
      );
    }
    if (data.containsKey('voiceover')) {
      context.handle(
        _voiceoverMeta,
        voiceover.isAcceptableOrUnknown(data['voiceover']!, _voiceoverMeta),
      );
    }
    if (data.containsKey('watched_at')) {
      context.handle(
        _watchedAtMeta,
        watchedAt.isAcceptableOrUnknown(data['watched_at']!, _watchedAtMeta),
      );
    }
    return context;
  }

  @override
  Set<GeneratedColumn> get $primaryKey => {id};
  @override
  WatchHistoryData map(Map<String, dynamic> data, {String? tablePrefix}) {
    final effectivePrefix = tablePrefix != null ? '$tablePrefix.' : '';
    return WatchHistoryData(
      id: attachedDatabase.typeMapping.read(
        DriftSqlType.int,
        data['${effectivePrefix}id'],
      )!,
      mediaId: attachedDatabase.typeMapping.read(
        DriftSqlType.string,
        data['${effectivePrefix}media_id'],
      )!,
      providerId: attachedDatabase.typeMapping.read(
        DriftSqlType.string,
        data['${effectivePrefix}provider_id'],
      )!,
      title: attachedDatabase.typeMapping.read(
        DriftSqlType.string,
        data['${effectivePrefix}title'],
      )!,
      posterUrl: attachedDatabase.typeMapping.read(
        DriftSqlType.string,
        data['${effectivePrefix}poster_url'],
      ),
      year: attachedDatabase.typeMapping.read(
        DriftSqlType.int,
        data['${effectivePrefix}year'],
      ),
      rating: attachedDatabase.typeMapping.read(
        DriftSqlType.double,
        data['${effectivePrefix}rating'],
      ),
      ratingSource: attachedDatabase.typeMapping.read(
        DriftSqlType.string,
        data['${effectivePrefix}rating_source'],
      ),
      mediaType: attachedDatabase.typeMapping.read(
        DriftSqlType.string,
        data['${effectivePrefix}media_type'],
      )!,
      positionMs: attachedDatabase.typeMapping.read(
        DriftSqlType.int,
        data['${effectivePrefix}position_ms'],
      )!,
      durationMs: attachedDatabase.typeMapping.read(
        DriftSqlType.int,
        data['${effectivePrefix}duration_ms'],
      )!,
      season: attachedDatabase.typeMapping.read(
        DriftSqlType.int,
        data['${effectivePrefix}season'],
      ),
      episode: attachedDatabase.typeMapping.read(
        DriftSqlType.int,
        data['${effectivePrefix}episode'],
      ),
      episodeTitle: attachedDatabase.typeMapping.read(
        DriftSqlType.string,
        data['${effectivePrefix}episode_title'],
      ),
      lastStreamUrl: attachedDatabase.typeMapping.read(
        DriftSqlType.string,
        data['${effectivePrefix}last_stream_url'],
      ),
      voiceover: attachedDatabase.typeMapping.read(
        DriftSqlType.string,
        data['${effectivePrefix}voiceover'],
      ),
      watchedAt: attachedDatabase.typeMapping.read(
        DriftSqlType.dateTime,
        data['${effectivePrefix}watched_at'],
      )!,
    );
  }

  @override
  $WatchHistoryTable createAlias(String alias) {
    return $WatchHistoryTable(attachedDatabase, alias);
  }
}

class WatchHistoryData extends DataClass
    implements Insertable<WatchHistoryData> {
  final int id;
  final String mediaId;
  final String providerId;
  final String title;
  final String? posterUrl;
  final int? year;
  final double? rating;
  final String? ratingSource;
  final String mediaType;
  final int positionMs;
  final int durationMs;
  final int? season;
  final int? episode;
  final String? episodeTitle;
  final String? lastStreamUrl;
  final String? voiceover;
  final DateTime watchedAt;
  const WatchHistoryData({
    required this.id,
    required this.mediaId,
    required this.providerId,
    required this.title,
    this.posterUrl,
    this.year,
    this.rating,
    this.ratingSource,
    required this.mediaType,
    required this.positionMs,
    required this.durationMs,
    this.season,
    this.episode,
    this.episodeTitle,
    this.lastStreamUrl,
    this.voiceover,
    required this.watchedAt,
  });
  @override
  Map<String, Expression> toColumns(bool nullToAbsent) {
    final map = <String, Expression>{};
    map['id'] = Variable<int>(id);
    map['media_id'] = Variable<String>(mediaId);
    map['provider_id'] = Variable<String>(providerId);
    map['title'] = Variable<String>(title);
    if (!nullToAbsent || posterUrl != null) {
      map['poster_url'] = Variable<String>(posterUrl);
    }
    if (!nullToAbsent || year != null) {
      map['year'] = Variable<int>(year);
    }
    if (!nullToAbsent || rating != null) {
      map['rating'] = Variable<double>(rating);
    }
    if (!nullToAbsent || ratingSource != null) {
      map['rating_source'] = Variable<String>(ratingSource);
    }
    map['media_type'] = Variable<String>(mediaType);
    map['position_ms'] = Variable<int>(positionMs);
    map['duration_ms'] = Variable<int>(durationMs);
    if (!nullToAbsent || season != null) {
      map['season'] = Variable<int>(season);
    }
    if (!nullToAbsent || episode != null) {
      map['episode'] = Variable<int>(episode);
    }
    if (!nullToAbsent || episodeTitle != null) {
      map['episode_title'] = Variable<String>(episodeTitle);
    }
    if (!nullToAbsent || lastStreamUrl != null) {
      map['last_stream_url'] = Variable<String>(lastStreamUrl);
    }
    if (!nullToAbsent || voiceover != null) {
      map['voiceover'] = Variable<String>(voiceover);
    }
    map['watched_at'] = Variable<DateTime>(watchedAt);
    return map;
  }

  WatchHistoryCompanion toCompanion(bool nullToAbsent) {
    return WatchHistoryCompanion(
      id: Value(id),
      mediaId: Value(mediaId),
      providerId: Value(providerId),
      title: Value(title),
      posterUrl: posterUrl == null && nullToAbsent
          ? const Value.absent()
          : Value(posterUrl),
      year: year == null && nullToAbsent ? const Value.absent() : Value(year),
      rating: rating == null && nullToAbsent
          ? const Value.absent()
          : Value(rating),
      ratingSource: ratingSource == null && nullToAbsent
          ? const Value.absent()
          : Value(ratingSource),
      mediaType: Value(mediaType),
      positionMs: Value(positionMs),
      durationMs: Value(durationMs),
      season: season == null && nullToAbsent
          ? const Value.absent()
          : Value(season),
      episode: episode == null && nullToAbsent
          ? const Value.absent()
          : Value(episode),
      episodeTitle: episodeTitle == null && nullToAbsent
          ? const Value.absent()
          : Value(episodeTitle),
      lastStreamUrl: lastStreamUrl == null && nullToAbsent
          ? const Value.absent()
          : Value(lastStreamUrl),
      voiceover: voiceover == null && nullToAbsent
          ? const Value.absent()
          : Value(voiceover),
      watchedAt: Value(watchedAt),
    );
  }

  factory WatchHistoryData.fromJson(
    Map<String, dynamic> json, {
    ValueSerializer? serializer,
  }) {
    serializer ??= driftRuntimeOptions.defaultSerializer;
    return WatchHistoryData(
      id: serializer.fromJson<int>(json['id']),
      mediaId: serializer.fromJson<String>(json['mediaId']),
      providerId: serializer.fromJson<String>(json['providerId']),
      title: serializer.fromJson<String>(json['title']),
      posterUrl: serializer.fromJson<String?>(json['posterUrl']),
      year: serializer.fromJson<int?>(json['year']),
      rating: serializer.fromJson<double?>(json['rating']),
      ratingSource: serializer.fromJson<String?>(json['ratingSource']),
      mediaType: serializer.fromJson<String>(json['mediaType']),
      positionMs: serializer.fromJson<int>(json['positionMs']),
      durationMs: serializer.fromJson<int>(json['durationMs']),
      season: serializer.fromJson<int?>(json['season']),
      episode: serializer.fromJson<int?>(json['episode']),
      episodeTitle: serializer.fromJson<String?>(json['episodeTitle']),
      lastStreamUrl: serializer.fromJson<String?>(json['lastStreamUrl']),
      voiceover: serializer.fromJson<String?>(json['voiceover']),
      watchedAt: serializer.fromJson<DateTime>(json['watchedAt']),
    );
  }
  @override
  Map<String, dynamic> toJson({ValueSerializer? serializer}) {
    serializer ??= driftRuntimeOptions.defaultSerializer;
    return <String, dynamic>{
      'id': serializer.toJson<int>(id),
      'mediaId': serializer.toJson<String>(mediaId),
      'providerId': serializer.toJson<String>(providerId),
      'title': serializer.toJson<String>(title),
      'posterUrl': serializer.toJson<String?>(posterUrl),
      'year': serializer.toJson<int?>(year),
      'rating': serializer.toJson<double?>(rating),
      'ratingSource': serializer.toJson<String?>(ratingSource),
      'mediaType': serializer.toJson<String>(mediaType),
      'positionMs': serializer.toJson<int>(positionMs),
      'durationMs': serializer.toJson<int>(durationMs),
      'season': serializer.toJson<int?>(season),
      'episode': serializer.toJson<int?>(episode),
      'episodeTitle': serializer.toJson<String?>(episodeTitle),
      'lastStreamUrl': serializer.toJson<String?>(lastStreamUrl),
      'voiceover': serializer.toJson<String?>(voiceover),
      'watchedAt': serializer.toJson<DateTime>(watchedAt),
    };
  }

  WatchHistoryData copyWith({
    int? id,
    String? mediaId,
    String? providerId,
    String? title,
    Value<String?> posterUrl = const Value.absent(),
    Value<int?> year = const Value.absent(),
    Value<double?> rating = const Value.absent(),
    Value<String?> ratingSource = const Value.absent(),
    String? mediaType,
    int? positionMs,
    int? durationMs,
    Value<int?> season = const Value.absent(),
    Value<int?> episode = const Value.absent(),
    Value<String?> episodeTitle = const Value.absent(),
    Value<String?> lastStreamUrl = const Value.absent(),
    Value<String?> voiceover = const Value.absent(),
    DateTime? watchedAt,
  }) => WatchHistoryData(
    id: id ?? this.id,
    mediaId: mediaId ?? this.mediaId,
    providerId: providerId ?? this.providerId,
    title: title ?? this.title,
    posterUrl: posterUrl.present ? posterUrl.value : this.posterUrl,
    year: year.present ? year.value : this.year,
    rating: rating.present ? rating.value : this.rating,
    ratingSource: ratingSource.present ? ratingSource.value : this.ratingSource,
    mediaType: mediaType ?? this.mediaType,
    positionMs: positionMs ?? this.positionMs,
    durationMs: durationMs ?? this.durationMs,
    season: season.present ? season.value : this.season,
    episode: episode.present ? episode.value : this.episode,
    episodeTitle: episodeTitle.present ? episodeTitle.value : this.episodeTitle,
    lastStreamUrl: lastStreamUrl.present
        ? lastStreamUrl.value
        : this.lastStreamUrl,
    voiceover: voiceover.present ? voiceover.value : this.voiceover,
    watchedAt: watchedAt ?? this.watchedAt,
  );
  WatchHistoryData copyWithCompanion(WatchHistoryCompanion data) {
    return WatchHistoryData(
      id: data.id.present ? data.id.value : this.id,
      mediaId: data.mediaId.present ? data.mediaId.value : this.mediaId,
      providerId: data.providerId.present
          ? data.providerId.value
          : this.providerId,
      title: data.title.present ? data.title.value : this.title,
      posterUrl: data.posterUrl.present ? data.posterUrl.value : this.posterUrl,
      year: data.year.present ? data.year.value : this.year,
      rating: data.rating.present ? data.rating.value : this.rating,
      ratingSource: data.ratingSource.present
          ? data.ratingSource.value
          : this.ratingSource,
      mediaType: data.mediaType.present ? data.mediaType.value : this.mediaType,
      positionMs: data.positionMs.present
          ? data.positionMs.value
          : this.positionMs,
      durationMs: data.durationMs.present
          ? data.durationMs.value
          : this.durationMs,
      season: data.season.present ? data.season.value : this.season,
      episode: data.episode.present ? data.episode.value : this.episode,
      episodeTitle: data.episodeTitle.present
          ? data.episodeTitle.value
          : this.episodeTitle,
      lastStreamUrl: data.lastStreamUrl.present
          ? data.lastStreamUrl.value
          : this.lastStreamUrl,
      voiceover: data.voiceover.present ? data.voiceover.value : this.voiceover,
      watchedAt: data.watchedAt.present ? data.watchedAt.value : this.watchedAt,
    );
  }

  @override
  String toString() {
    return (StringBuffer('WatchHistoryData(')
          ..write('id: $id, ')
          ..write('mediaId: $mediaId, ')
          ..write('providerId: $providerId, ')
          ..write('title: $title, ')
          ..write('posterUrl: $posterUrl, ')
          ..write('year: $year, ')
          ..write('rating: $rating, ')
          ..write('ratingSource: $ratingSource, ')
          ..write('mediaType: $mediaType, ')
          ..write('positionMs: $positionMs, ')
          ..write('durationMs: $durationMs, ')
          ..write('season: $season, ')
          ..write('episode: $episode, ')
          ..write('episodeTitle: $episodeTitle, ')
          ..write('lastStreamUrl: $lastStreamUrl, ')
          ..write('voiceover: $voiceover, ')
          ..write('watchedAt: $watchedAt')
          ..write(')'))
        .toString();
  }

  @override
  int get hashCode => Object.hash(
    id,
    mediaId,
    providerId,
    title,
    posterUrl,
    year,
    rating,
    ratingSource,
    mediaType,
    positionMs,
    durationMs,
    season,
    episode,
    episodeTitle,
    lastStreamUrl,
    voiceover,
    watchedAt,
  );
  @override
  bool operator ==(Object other) =>
      identical(this, other) ||
      (other is WatchHistoryData &&
          other.id == this.id &&
          other.mediaId == this.mediaId &&
          other.providerId == this.providerId &&
          other.title == this.title &&
          other.posterUrl == this.posterUrl &&
          other.year == this.year &&
          other.rating == this.rating &&
          other.ratingSource == this.ratingSource &&
          other.mediaType == this.mediaType &&
          other.positionMs == this.positionMs &&
          other.durationMs == this.durationMs &&
          other.season == this.season &&
          other.episode == this.episode &&
          other.episodeTitle == this.episodeTitle &&
          other.lastStreamUrl == this.lastStreamUrl &&
          other.voiceover == this.voiceover &&
          other.watchedAt == this.watchedAt);
}

class WatchHistoryCompanion extends UpdateCompanion<WatchHistoryData> {
  final Value<int> id;
  final Value<String> mediaId;
  final Value<String> providerId;
  final Value<String> title;
  final Value<String?> posterUrl;
  final Value<int?> year;
  final Value<double?> rating;
  final Value<String?> ratingSource;
  final Value<String> mediaType;
  final Value<int> positionMs;
  final Value<int> durationMs;
  final Value<int?> season;
  final Value<int?> episode;
  final Value<String?> episodeTitle;
  final Value<String?> lastStreamUrl;
  final Value<String?> voiceover;
  final Value<DateTime> watchedAt;
  const WatchHistoryCompanion({
    this.id = const Value.absent(),
    this.mediaId = const Value.absent(),
    this.providerId = const Value.absent(),
    this.title = const Value.absent(),
    this.posterUrl = const Value.absent(),
    this.year = const Value.absent(),
    this.rating = const Value.absent(),
    this.ratingSource = const Value.absent(),
    this.mediaType = const Value.absent(),
    this.positionMs = const Value.absent(),
    this.durationMs = const Value.absent(),
    this.season = const Value.absent(),
    this.episode = const Value.absent(),
    this.episodeTitle = const Value.absent(),
    this.lastStreamUrl = const Value.absent(),
    this.voiceover = const Value.absent(),
    this.watchedAt = const Value.absent(),
  });
  WatchHistoryCompanion.insert({
    this.id = const Value.absent(),
    required String mediaId,
    required String providerId,
    required String title,
    this.posterUrl = const Value.absent(),
    this.year = const Value.absent(),
    this.rating = const Value.absent(),
    this.ratingSource = const Value.absent(),
    required String mediaType,
    this.positionMs = const Value.absent(),
    this.durationMs = const Value.absent(),
    this.season = const Value.absent(),
    this.episode = const Value.absent(),
    this.episodeTitle = const Value.absent(),
    this.lastStreamUrl = const Value.absent(),
    this.voiceover = const Value.absent(),
    this.watchedAt = const Value.absent(),
  }) : mediaId = Value(mediaId),
       providerId = Value(providerId),
       title = Value(title),
       mediaType = Value(mediaType);
  static Insertable<WatchHistoryData> custom({
    Expression<int>? id,
    Expression<String>? mediaId,
    Expression<String>? providerId,
    Expression<String>? title,
    Expression<String>? posterUrl,
    Expression<int>? year,
    Expression<double>? rating,
    Expression<String>? ratingSource,
    Expression<String>? mediaType,
    Expression<int>? positionMs,
    Expression<int>? durationMs,
    Expression<int>? season,
    Expression<int>? episode,
    Expression<String>? episodeTitle,
    Expression<String>? lastStreamUrl,
    Expression<String>? voiceover,
    Expression<DateTime>? watchedAt,
  }) {
    return RawValuesInsertable({
      if (id != null) 'id': id,
      if (mediaId != null) 'media_id': mediaId,
      if (providerId != null) 'provider_id': providerId,
      if (title != null) 'title': title,
      if (posterUrl != null) 'poster_url': posterUrl,
      if (year != null) 'year': year,
      if (rating != null) 'rating': rating,
      if (ratingSource != null) 'rating_source': ratingSource,
      if (mediaType != null) 'media_type': mediaType,
      if (positionMs != null) 'position_ms': positionMs,
      if (durationMs != null) 'duration_ms': durationMs,
      if (season != null) 'season': season,
      if (episode != null) 'episode': episode,
      if (episodeTitle != null) 'episode_title': episodeTitle,
      if (lastStreamUrl != null) 'last_stream_url': lastStreamUrl,
      if (voiceover != null) 'voiceover': voiceover,
      if (watchedAt != null) 'watched_at': watchedAt,
    });
  }

  WatchHistoryCompanion copyWith({
    Value<int>? id,
    Value<String>? mediaId,
    Value<String>? providerId,
    Value<String>? title,
    Value<String?>? posterUrl,
    Value<int?>? year,
    Value<double?>? rating,
    Value<String?>? ratingSource,
    Value<String>? mediaType,
    Value<int>? positionMs,
    Value<int>? durationMs,
    Value<int?>? season,
    Value<int?>? episode,
    Value<String?>? episodeTitle,
    Value<String?>? lastStreamUrl,
    Value<String?>? voiceover,
    Value<DateTime>? watchedAt,
  }) {
    return WatchHistoryCompanion(
      id: id ?? this.id,
      mediaId: mediaId ?? this.mediaId,
      providerId: providerId ?? this.providerId,
      title: title ?? this.title,
      posterUrl: posterUrl ?? this.posterUrl,
      year: year ?? this.year,
      rating: rating ?? this.rating,
      ratingSource: ratingSource ?? this.ratingSource,
      mediaType: mediaType ?? this.mediaType,
      positionMs: positionMs ?? this.positionMs,
      durationMs: durationMs ?? this.durationMs,
      season: season ?? this.season,
      episode: episode ?? this.episode,
      episodeTitle: episodeTitle ?? this.episodeTitle,
      lastStreamUrl: lastStreamUrl ?? this.lastStreamUrl,
      voiceover: voiceover ?? this.voiceover,
      watchedAt: watchedAt ?? this.watchedAt,
    );
  }

  @override
  Map<String, Expression> toColumns(bool nullToAbsent) {
    final map = <String, Expression>{};
    if (id.present) {
      map['id'] = Variable<int>(id.value);
    }
    if (mediaId.present) {
      map['media_id'] = Variable<String>(mediaId.value);
    }
    if (providerId.present) {
      map['provider_id'] = Variable<String>(providerId.value);
    }
    if (title.present) {
      map['title'] = Variable<String>(title.value);
    }
    if (posterUrl.present) {
      map['poster_url'] = Variable<String>(posterUrl.value);
    }
    if (year.present) {
      map['year'] = Variable<int>(year.value);
    }
    if (rating.present) {
      map['rating'] = Variable<double>(rating.value);
    }
    if (ratingSource.present) {
      map['rating_source'] = Variable<String>(ratingSource.value);
    }
    if (mediaType.present) {
      map['media_type'] = Variable<String>(mediaType.value);
    }
    if (positionMs.present) {
      map['position_ms'] = Variable<int>(positionMs.value);
    }
    if (durationMs.present) {
      map['duration_ms'] = Variable<int>(durationMs.value);
    }
    if (season.present) {
      map['season'] = Variable<int>(season.value);
    }
    if (episode.present) {
      map['episode'] = Variable<int>(episode.value);
    }
    if (episodeTitle.present) {
      map['episode_title'] = Variable<String>(episodeTitle.value);
    }
    if (lastStreamUrl.present) {
      map['last_stream_url'] = Variable<String>(lastStreamUrl.value);
    }
    if (voiceover.present) {
      map['voiceover'] = Variable<String>(voiceover.value);
    }
    if (watchedAt.present) {
      map['watched_at'] = Variable<DateTime>(watchedAt.value);
    }
    return map;
  }

  @override
  String toString() {
    return (StringBuffer('WatchHistoryCompanion(')
          ..write('id: $id, ')
          ..write('mediaId: $mediaId, ')
          ..write('providerId: $providerId, ')
          ..write('title: $title, ')
          ..write('posterUrl: $posterUrl, ')
          ..write('year: $year, ')
          ..write('rating: $rating, ')
          ..write('ratingSource: $ratingSource, ')
          ..write('mediaType: $mediaType, ')
          ..write('positionMs: $positionMs, ')
          ..write('durationMs: $durationMs, ')
          ..write('season: $season, ')
          ..write('episode: $episode, ')
          ..write('episodeTitle: $episodeTitle, ')
          ..write('lastStreamUrl: $lastStreamUrl, ')
          ..write('voiceover: $voiceover, ')
          ..write('watchedAt: $watchedAt')
          ..write(')'))
        .toString();
  }
}

class $DownloadsTable extends Downloads
    with TableInfo<$DownloadsTable, Download> {
  @override
  final GeneratedDatabase attachedDatabase;
  final String? _alias;
  $DownloadsTable(this.attachedDatabase, [this._alias]);
  static const VerificationMeta _idMeta = const VerificationMeta('id');
  @override
  late final GeneratedColumn<int> id = GeneratedColumn<int>(
    'id',
    aliasedName,
    false,
    hasAutoIncrement: true,
    type: DriftSqlType.int,
    requiredDuringInsert: false,
    defaultConstraints: GeneratedColumn.constraintIsAlways(
      'PRIMARY KEY AUTOINCREMENT',
    ),
  );
  static const VerificationMeta _mediaIdMeta = const VerificationMeta(
    'mediaId',
  );
  @override
  late final GeneratedColumn<String> mediaId = GeneratedColumn<String>(
    'media_id',
    aliasedName,
    false,
    type: DriftSqlType.string,
    requiredDuringInsert: true,
  );
  static const VerificationMeta _providerIdMeta = const VerificationMeta(
    'providerId',
  );
  @override
  late final GeneratedColumn<String> providerId = GeneratedColumn<String>(
    'provider_id',
    aliasedName,
    false,
    type: DriftSqlType.string,
    requiredDuringInsert: true,
  );
  static const VerificationMeta _titleMeta = const VerificationMeta('title');
  @override
  late final GeneratedColumn<String> title = GeneratedColumn<String>(
    'title',
    aliasedName,
    false,
    type: DriftSqlType.string,
    requiredDuringInsert: true,
  );
  static const VerificationMeta _posterUrlMeta = const VerificationMeta(
    'posterUrl',
  );
  @override
  late final GeneratedColumn<String> posterUrl = GeneratedColumn<String>(
    'poster_url',
    aliasedName,
    true,
    type: DriftSqlType.string,
    requiredDuringInsert: false,
  );
  static const VerificationMeta _yearMeta = const VerificationMeta('year');
  @override
  late final GeneratedColumn<int> year = GeneratedColumn<int>(
    'year',
    aliasedName,
    true,
    type: DriftSqlType.int,
    requiredDuringInsert: false,
  );
  static const VerificationMeta _ratingMeta = const VerificationMeta('rating');
  @override
  late final GeneratedColumn<double> rating = GeneratedColumn<double>(
    'rating',
    aliasedName,
    true,
    type: DriftSqlType.double,
    requiredDuringInsert: false,
  );
  static const VerificationMeta _ratingSourceMeta = const VerificationMeta(
    'ratingSource',
  );
  @override
  late final GeneratedColumn<String> ratingSource = GeneratedColumn<String>(
    'rating_source',
    aliasedName,
    true,
    type: DriftSqlType.string,
    requiredDuringInsert: false,
  );
  static const VerificationMeta _mediaTypeMeta = const VerificationMeta(
    'mediaType',
  );
  @override
  late final GeneratedColumn<String> mediaType = GeneratedColumn<String>(
    'media_type',
    aliasedName,
    false,
    type: DriftSqlType.string,
    requiredDuringInsert: true,
  );
  static const VerificationMeta _seasonMeta = const VerificationMeta('season');
  @override
  late final GeneratedColumn<int> season = GeneratedColumn<int>(
    'season',
    aliasedName,
    true,
    type: DriftSqlType.int,
    requiredDuringInsert: false,
  );
  static const VerificationMeta _episodeMeta = const VerificationMeta(
    'episode',
  );
  @override
  late final GeneratedColumn<int> episode = GeneratedColumn<int>(
    'episode',
    aliasedName,
    true,
    type: DriftSqlType.int,
    requiredDuringInsert: false,
  );
  static const VerificationMeta _episodeTitleMeta = const VerificationMeta(
    'episodeTitle',
  );
  @override
  late final GeneratedColumn<String> episodeTitle = GeneratedColumn<String>(
    'episode_title',
    aliasedName,
    true,
    type: DriftSqlType.string,
    requiredDuringInsert: false,
  );
  static const VerificationMeta _streamUrlMeta = const VerificationMeta(
    'streamUrl',
  );
  @override
  late final GeneratedColumn<String> streamUrl = GeneratedColumn<String>(
    'stream_url',
    aliasedName,
    false,
    type: DriftSqlType.string,
    requiredDuringInsert: true,
  );
  static const VerificationMeta _localPathMeta = const VerificationMeta(
    'localPath',
  );
  @override
  late final GeneratedColumn<String> localPath = GeneratedColumn<String>(
    'local_path',
    aliasedName,
    false,
    type: DriftSqlType.string,
    requiredDuringInsert: true,
  );
  static const VerificationMeta _qualityMeta = const VerificationMeta(
    'quality',
  );
  @override
  late final GeneratedColumn<String> quality = GeneratedColumn<String>(
    'quality',
    aliasedName,
    false,
    type: DriftSqlType.string,
    requiredDuringInsert: true,
  );
  static const VerificationMeta _voiceoverMeta = const VerificationMeta(
    'voiceover',
  );
  @override
  late final GeneratedColumn<String> voiceover = GeneratedColumn<String>(
    'voiceover',
    aliasedName,
    true,
    type: DriftSqlType.string,
    requiredDuringInsert: false,
  );
  @override
  late final GeneratedColumnWithTypeConverter<DownloadStatus, int> status =
      GeneratedColumn<int>(
        'status',
        aliasedName,
        false,
        type: DriftSqlType.int,
        requiredDuringInsert: false,
        defaultValue: const Constant(0),
      ).withConverter<DownloadStatus>($DownloadsTable.$converterstatus);
  static const VerificationMeta _progressMeta = const VerificationMeta(
    'progress',
  );
  @override
  late final GeneratedColumn<double> progress = GeneratedColumn<double>(
    'progress',
    aliasedName,
    false,
    type: DriftSqlType.double,
    requiredDuringInsert: false,
    defaultValue: const Constant(0.0),
  );
  static const VerificationMeta _fileSizeBytesMeta = const VerificationMeta(
    'fileSizeBytes',
  );
  @override
  late final GeneratedColumn<int> fileSizeBytes = GeneratedColumn<int>(
    'file_size_bytes',
    aliasedName,
    false,
    type: DriftSqlType.int,
    requiredDuringInsert: false,
    defaultValue: const Constant(0),
  );
  static const VerificationMeta _downloadedBytesMeta = const VerificationMeta(
    'downloadedBytes',
  );
  @override
  late final GeneratedColumn<int> downloadedBytes = GeneratedColumn<int>(
    'downloaded_bytes',
    aliasedName,
    false,
    type: DriftSqlType.int,
    requiredDuringInsert: false,
    defaultValue: const Constant(0),
  );
  static const VerificationMeta _headersMeta = const VerificationMeta(
    'headers',
  );
  @override
  late final GeneratedColumn<String> headers = GeneratedColumn<String>(
    'headers',
    aliasedName,
    true,
    type: DriftSqlType.string,
    requiredDuringInsert: false,
  );
  static const VerificationMeta _localPosterPathMeta = const VerificationMeta(
    'localPosterPath',
  );
  @override
  late final GeneratedColumn<String> localPosterPath = GeneratedColumn<String>(
    'local_poster_path',
    aliasedName,
    true,
    type: DriftSqlType.string,
    requiredDuringInsert: false,
  );
  static const VerificationMeta _durationMeta = const VerificationMeta(
    'duration',
  );
  @override
  late final GeneratedColumn<int> duration = GeneratedColumn<int>(
    'duration',
    aliasedName,
    true,
    type: DriftSqlType.int,
    requiredDuringInsert: false,
  );
  static const VerificationMeta _createdAtMeta = const VerificationMeta(
    'createdAt',
  );
  @override
  late final GeneratedColumn<DateTime> createdAt = GeneratedColumn<DateTime>(
    'created_at',
    aliasedName,
    false,
    type: DriftSqlType.dateTime,
    requiredDuringInsert: false,
    defaultValue: currentDateAndTime,
  );
  static const VerificationMeta _completedAtMeta = const VerificationMeta(
    'completedAt',
  );
  @override
  late final GeneratedColumn<DateTime> completedAt = GeneratedColumn<DateTime>(
    'completed_at',
    aliasedName,
    true,
    type: DriftSqlType.dateTime,
    requiredDuringInsert: false,
  );
  @override
  List<GeneratedColumn> get $columns => [
    id,
    mediaId,
    providerId,
    title,
    posterUrl,
    year,
    rating,
    ratingSource,
    mediaType,
    season,
    episode,
    episodeTitle,
    streamUrl,
    localPath,
    quality,
    voiceover,
    status,
    progress,
    fileSizeBytes,
    downloadedBytes,
    headers,
    localPosterPath,
    duration,
    createdAt,
    completedAt,
  ];
  @override
  String get aliasedName => _alias ?? actualTableName;
  @override
  String get actualTableName => $name;
  static const String $name = 'downloads';
  @override
  VerificationContext validateIntegrity(
    Insertable<Download> instance, {
    bool isInserting = false,
  }) {
    final context = VerificationContext();
    final data = instance.toColumns(true);
    if (data.containsKey('id')) {
      context.handle(_idMeta, id.isAcceptableOrUnknown(data['id']!, _idMeta));
    }
    if (data.containsKey('media_id')) {
      context.handle(
        _mediaIdMeta,
        mediaId.isAcceptableOrUnknown(data['media_id']!, _mediaIdMeta),
      );
    } else if (isInserting) {
      context.missing(_mediaIdMeta);
    }
    if (data.containsKey('provider_id')) {
      context.handle(
        _providerIdMeta,
        providerId.isAcceptableOrUnknown(data['provider_id']!, _providerIdMeta),
      );
    } else if (isInserting) {
      context.missing(_providerIdMeta);
    }
    if (data.containsKey('title')) {
      context.handle(
        _titleMeta,
        title.isAcceptableOrUnknown(data['title']!, _titleMeta),
      );
    } else if (isInserting) {
      context.missing(_titleMeta);
    }
    if (data.containsKey('poster_url')) {
      context.handle(
        _posterUrlMeta,
        posterUrl.isAcceptableOrUnknown(data['poster_url']!, _posterUrlMeta),
      );
    }
    if (data.containsKey('year')) {
      context.handle(
        _yearMeta,
        year.isAcceptableOrUnknown(data['year']!, _yearMeta),
      );
    }
    if (data.containsKey('rating')) {
      context.handle(
        _ratingMeta,
        rating.isAcceptableOrUnknown(data['rating']!, _ratingMeta),
      );
    }
    if (data.containsKey('rating_source')) {
      context.handle(
        _ratingSourceMeta,
        ratingSource.isAcceptableOrUnknown(
          data['rating_source']!,
          _ratingSourceMeta,
        ),
      );
    }
    if (data.containsKey('media_type')) {
      context.handle(
        _mediaTypeMeta,
        mediaType.isAcceptableOrUnknown(data['media_type']!, _mediaTypeMeta),
      );
    } else if (isInserting) {
      context.missing(_mediaTypeMeta);
    }
    if (data.containsKey('season')) {
      context.handle(
        _seasonMeta,
        season.isAcceptableOrUnknown(data['season']!, _seasonMeta),
      );
    }
    if (data.containsKey('episode')) {
      context.handle(
        _episodeMeta,
        episode.isAcceptableOrUnknown(data['episode']!, _episodeMeta),
      );
    }
    if (data.containsKey('episode_title')) {
      context.handle(
        _episodeTitleMeta,
        episodeTitle.isAcceptableOrUnknown(
          data['episode_title']!,
          _episodeTitleMeta,
        ),
      );
    }
    if (data.containsKey('stream_url')) {
      context.handle(
        _streamUrlMeta,
        streamUrl.isAcceptableOrUnknown(data['stream_url']!, _streamUrlMeta),
      );
    } else if (isInserting) {
      context.missing(_streamUrlMeta);
    }
    if (data.containsKey('local_path')) {
      context.handle(
        _localPathMeta,
        localPath.isAcceptableOrUnknown(data['local_path']!, _localPathMeta),
      );
    } else if (isInserting) {
      context.missing(_localPathMeta);
    }
    if (data.containsKey('quality')) {
      context.handle(
        _qualityMeta,
        quality.isAcceptableOrUnknown(data['quality']!, _qualityMeta),
      );
    } else if (isInserting) {
      context.missing(_qualityMeta);
    }
    if (data.containsKey('voiceover')) {
      context.handle(
        _voiceoverMeta,
        voiceover.isAcceptableOrUnknown(data['voiceover']!, _voiceoverMeta),
      );
    }
    if (data.containsKey('progress')) {
      context.handle(
        _progressMeta,
        progress.isAcceptableOrUnknown(data['progress']!, _progressMeta),
      );
    }
    if (data.containsKey('file_size_bytes')) {
      context.handle(
        _fileSizeBytesMeta,
        fileSizeBytes.isAcceptableOrUnknown(
          data['file_size_bytes']!,
          _fileSizeBytesMeta,
        ),
      );
    }
    if (data.containsKey('downloaded_bytes')) {
      context.handle(
        _downloadedBytesMeta,
        downloadedBytes.isAcceptableOrUnknown(
          data['downloaded_bytes']!,
          _downloadedBytesMeta,
        ),
      );
    }
    if (data.containsKey('headers')) {
      context.handle(
        _headersMeta,
        headers.isAcceptableOrUnknown(data['headers']!, _headersMeta),
      );
    }
    if (data.containsKey('local_poster_path')) {
      context.handle(
        _localPosterPathMeta,
        localPosterPath.isAcceptableOrUnknown(
          data['local_poster_path']!,
          _localPosterPathMeta,
        ),
      );
    }
    if (data.containsKey('duration')) {
      context.handle(
        _durationMeta,
        duration.isAcceptableOrUnknown(data['duration']!, _durationMeta),
      );
    }
    if (data.containsKey('created_at')) {
      context.handle(
        _createdAtMeta,
        createdAt.isAcceptableOrUnknown(data['created_at']!, _createdAtMeta),
      );
    }
    if (data.containsKey('completed_at')) {
      context.handle(
        _completedAtMeta,
        completedAt.isAcceptableOrUnknown(
          data['completed_at']!,
          _completedAtMeta,
        ),
      );
    }
    return context;
  }

  @override
  Set<GeneratedColumn> get $primaryKey => {id};
  @override
  Download map(Map<String, dynamic> data, {String? tablePrefix}) {
    final effectivePrefix = tablePrefix != null ? '$tablePrefix.' : '';
    return Download(
      id: attachedDatabase.typeMapping.read(
        DriftSqlType.int,
        data['${effectivePrefix}id'],
      )!,
      mediaId: attachedDatabase.typeMapping.read(
        DriftSqlType.string,
        data['${effectivePrefix}media_id'],
      )!,
      providerId: attachedDatabase.typeMapping.read(
        DriftSqlType.string,
        data['${effectivePrefix}provider_id'],
      )!,
      title: attachedDatabase.typeMapping.read(
        DriftSqlType.string,
        data['${effectivePrefix}title'],
      )!,
      posterUrl: attachedDatabase.typeMapping.read(
        DriftSqlType.string,
        data['${effectivePrefix}poster_url'],
      ),
      year: attachedDatabase.typeMapping.read(
        DriftSqlType.int,
        data['${effectivePrefix}year'],
      ),
      rating: attachedDatabase.typeMapping.read(
        DriftSqlType.double,
        data['${effectivePrefix}rating'],
      ),
      ratingSource: attachedDatabase.typeMapping.read(
        DriftSqlType.string,
        data['${effectivePrefix}rating_source'],
      ),
      mediaType: attachedDatabase.typeMapping.read(
        DriftSqlType.string,
        data['${effectivePrefix}media_type'],
      )!,
      season: attachedDatabase.typeMapping.read(
        DriftSqlType.int,
        data['${effectivePrefix}season'],
      ),
      episode: attachedDatabase.typeMapping.read(
        DriftSqlType.int,
        data['${effectivePrefix}episode'],
      ),
      episodeTitle: attachedDatabase.typeMapping.read(
        DriftSqlType.string,
        data['${effectivePrefix}episode_title'],
      ),
      streamUrl: attachedDatabase.typeMapping.read(
        DriftSqlType.string,
        data['${effectivePrefix}stream_url'],
      )!,
      localPath: attachedDatabase.typeMapping.read(
        DriftSqlType.string,
        data['${effectivePrefix}local_path'],
      )!,
      quality: attachedDatabase.typeMapping.read(
        DriftSqlType.string,
        data['${effectivePrefix}quality'],
      )!,
      voiceover: attachedDatabase.typeMapping.read(
        DriftSqlType.string,
        data['${effectivePrefix}voiceover'],
      ),
      status: $DownloadsTable.$converterstatus.fromSql(
        attachedDatabase.typeMapping.read(
          DriftSqlType.int,
          data['${effectivePrefix}status'],
        )!,
      ),
      progress: attachedDatabase.typeMapping.read(
        DriftSqlType.double,
        data['${effectivePrefix}progress'],
      )!,
      fileSizeBytes: attachedDatabase.typeMapping.read(
        DriftSqlType.int,
        data['${effectivePrefix}file_size_bytes'],
      )!,
      downloadedBytes: attachedDatabase.typeMapping.read(
        DriftSqlType.int,
        data['${effectivePrefix}downloaded_bytes'],
      )!,
      headers: attachedDatabase.typeMapping.read(
        DriftSqlType.string,
        data['${effectivePrefix}headers'],
      ),
      localPosterPath: attachedDatabase.typeMapping.read(
        DriftSqlType.string,
        data['${effectivePrefix}local_poster_path'],
      ),
      duration: attachedDatabase.typeMapping.read(
        DriftSqlType.int,
        data['${effectivePrefix}duration'],
      ),
      createdAt: attachedDatabase.typeMapping.read(
        DriftSqlType.dateTime,
        data['${effectivePrefix}created_at'],
      )!,
      completedAt: attachedDatabase.typeMapping.read(
        DriftSqlType.dateTime,
        data['${effectivePrefix}completed_at'],
      ),
    );
  }

  @override
  $DownloadsTable createAlias(String alias) {
    return $DownloadsTable(attachedDatabase, alias);
  }

  static TypeConverter<DownloadStatus, int> $converterstatus =
      const DownloadStatusConverter();
}

class Download extends DataClass implements Insertable<Download> {
  final int id;
  final String mediaId;
  final String providerId;
  final String title;
  final String? posterUrl;
  final int? year;
  final double? rating;
  final String? ratingSource;
  final String mediaType;
  final int? season;
  final int? episode;
  final String? episodeTitle;
  final String streamUrl;
  final String localPath;
  final String quality;
  final String? voiceover;
  final DownloadStatus status;
  final double progress;
  final int fileSizeBytes;
  final int downloadedBytes;
  final String? headers;
  final String? localPosterPath;
  final int? duration;
  final DateTime createdAt;
  final DateTime? completedAt;
  const Download({
    required this.id,
    required this.mediaId,
    required this.providerId,
    required this.title,
    this.posterUrl,
    this.year,
    this.rating,
    this.ratingSource,
    required this.mediaType,
    this.season,
    this.episode,
    this.episodeTitle,
    required this.streamUrl,
    required this.localPath,
    required this.quality,
    this.voiceover,
    required this.status,
    required this.progress,
    required this.fileSizeBytes,
    required this.downloadedBytes,
    this.headers,
    this.localPosterPath,
    this.duration,
    required this.createdAt,
    this.completedAt,
  });
  @override
  Map<String, Expression> toColumns(bool nullToAbsent) {
    final map = <String, Expression>{};
    map['id'] = Variable<int>(id);
    map['media_id'] = Variable<String>(mediaId);
    map['provider_id'] = Variable<String>(providerId);
    map['title'] = Variable<String>(title);
    if (!nullToAbsent || posterUrl != null) {
      map['poster_url'] = Variable<String>(posterUrl);
    }
    if (!nullToAbsent || year != null) {
      map['year'] = Variable<int>(year);
    }
    if (!nullToAbsent || rating != null) {
      map['rating'] = Variable<double>(rating);
    }
    if (!nullToAbsent || ratingSource != null) {
      map['rating_source'] = Variable<String>(ratingSource);
    }
    map['media_type'] = Variable<String>(mediaType);
    if (!nullToAbsent || season != null) {
      map['season'] = Variable<int>(season);
    }
    if (!nullToAbsent || episode != null) {
      map['episode'] = Variable<int>(episode);
    }
    if (!nullToAbsent || episodeTitle != null) {
      map['episode_title'] = Variable<String>(episodeTitle);
    }
    map['stream_url'] = Variable<String>(streamUrl);
    map['local_path'] = Variable<String>(localPath);
    map['quality'] = Variable<String>(quality);
    if (!nullToAbsent || voiceover != null) {
      map['voiceover'] = Variable<String>(voiceover);
    }
    {
      map['status'] = Variable<int>(
        $DownloadsTable.$converterstatus.toSql(status),
      );
    }
    map['progress'] = Variable<double>(progress);
    map['file_size_bytes'] = Variable<int>(fileSizeBytes);
    map['downloaded_bytes'] = Variable<int>(downloadedBytes);
    if (!nullToAbsent || headers != null) {
      map['headers'] = Variable<String>(headers);
    }
    if (!nullToAbsent || localPosterPath != null) {
      map['local_poster_path'] = Variable<String>(localPosterPath);
    }
    if (!nullToAbsent || duration != null) {
      map['duration'] = Variable<int>(duration);
    }
    map['created_at'] = Variable<DateTime>(createdAt);
    if (!nullToAbsent || completedAt != null) {
      map['completed_at'] = Variable<DateTime>(completedAt);
    }
    return map;
  }

  DownloadsCompanion toCompanion(bool nullToAbsent) {
    return DownloadsCompanion(
      id: Value(id),
      mediaId: Value(mediaId),
      providerId: Value(providerId),
      title: Value(title),
      posterUrl: posterUrl == null && nullToAbsent
          ? const Value.absent()
          : Value(posterUrl),
      year: year == null && nullToAbsent ? const Value.absent() : Value(year),
      rating: rating == null && nullToAbsent
          ? const Value.absent()
          : Value(rating),
      ratingSource: ratingSource == null && nullToAbsent
          ? const Value.absent()
          : Value(ratingSource),
      mediaType: Value(mediaType),
      season: season == null && nullToAbsent
          ? const Value.absent()
          : Value(season),
      episode: episode == null && nullToAbsent
          ? const Value.absent()
          : Value(episode),
      episodeTitle: episodeTitle == null && nullToAbsent
          ? const Value.absent()
          : Value(episodeTitle),
      streamUrl: Value(streamUrl),
      localPath: Value(localPath),
      quality: Value(quality),
      voiceover: voiceover == null && nullToAbsent
          ? const Value.absent()
          : Value(voiceover),
      status: Value(status),
      progress: Value(progress),
      fileSizeBytes: Value(fileSizeBytes),
      downloadedBytes: Value(downloadedBytes),
      headers: headers == null && nullToAbsent
          ? const Value.absent()
          : Value(headers),
      localPosterPath: localPosterPath == null && nullToAbsent
          ? const Value.absent()
          : Value(localPosterPath),
      duration: duration == null && nullToAbsent
          ? const Value.absent()
          : Value(duration),
      createdAt: Value(createdAt),
      completedAt: completedAt == null && nullToAbsent
          ? const Value.absent()
          : Value(completedAt),
    );
  }

  factory Download.fromJson(
    Map<String, dynamic> json, {
    ValueSerializer? serializer,
  }) {
    serializer ??= driftRuntimeOptions.defaultSerializer;
    return Download(
      id: serializer.fromJson<int>(json['id']),
      mediaId: serializer.fromJson<String>(json['mediaId']),
      providerId: serializer.fromJson<String>(json['providerId']),
      title: serializer.fromJson<String>(json['title']),
      posterUrl: serializer.fromJson<String?>(json['posterUrl']),
      year: serializer.fromJson<int?>(json['year']),
      rating: serializer.fromJson<double?>(json['rating']),
      ratingSource: serializer.fromJson<String?>(json['ratingSource']),
      mediaType: serializer.fromJson<String>(json['mediaType']),
      season: serializer.fromJson<int?>(json['season']),
      episode: serializer.fromJson<int?>(json['episode']),
      episodeTitle: serializer.fromJson<String?>(json['episodeTitle']),
      streamUrl: serializer.fromJson<String>(json['streamUrl']),
      localPath: serializer.fromJson<String>(json['localPath']),
      quality: serializer.fromJson<String>(json['quality']),
      voiceover: serializer.fromJson<String?>(json['voiceover']),
      status: serializer.fromJson<DownloadStatus>(json['status']),
      progress: serializer.fromJson<double>(json['progress']),
      fileSizeBytes: serializer.fromJson<int>(json['fileSizeBytes']),
      downloadedBytes: serializer.fromJson<int>(json['downloadedBytes']),
      headers: serializer.fromJson<String?>(json['headers']),
      localPosterPath: serializer.fromJson<String?>(json['localPosterPath']),
      duration: serializer.fromJson<int?>(json['duration']),
      createdAt: serializer.fromJson<DateTime>(json['createdAt']),
      completedAt: serializer.fromJson<DateTime?>(json['completedAt']),
    );
  }
  @override
  Map<String, dynamic> toJson({ValueSerializer? serializer}) {
    serializer ??= driftRuntimeOptions.defaultSerializer;
    return <String, dynamic>{
      'id': serializer.toJson<int>(id),
      'mediaId': serializer.toJson<String>(mediaId),
      'providerId': serializer.toJson<String>(providerId),
      'title': serializer.toJson<String>(title),
      'posterUrl': serializer.toJson<String?>(posterUrl),
      'year': serializer.toJson<int?>(year),
      'rating': serializer.toJson<double?>(rating),
      'ratingSource': serializer.toJson<String?>(ratingSource),
      'mediaType': serializer.toJson<String>(mediaType),
      'season': serializer.toJson<int?>(season),
      'episode': serializer.toJson<int?>(episode),
      'episodeTitle': serializer.toJson<String?>(episodeTitle),
      'streamUrl': serializer.toJson<String>(streamUrl),
      'localPath': serializer.toJson<String>(localPath),
      'quality': serializer.toJson<String>(quality),
      'voiceover': serializer.toJson<String?>(voiceover),
      'status': serializer.toJson<DownloadStatus>(status),
      'progress': serializer.toJson<double>(progress),
      'fileSizeBytes': serializer.toJson<int>(fileSizeBytes),
      'downloadedBytes': serializer.toJson<int>(downloadedBytes),
      'headers': serializer.toJson<String?>(headers),
      'localPosterPath': serializer.toJson<String?>(localPosterPath),
      'duration': serializer.toJson<int?>(duration),
      'createdAt': serializer.toJson<DateTime>(createdAt),
      'completedAt': serializer.toJson<DateTime?>(completedAt),
    };
  }

  Download copyWith({
    int? id,
    String? mediaId,
    String? providerId,
    String? title,
    Value<String?> posterUrl = const Value.absent(),
    Value<int?> year = const Value.absent(),
    Value<double?> rating = const Value.absent(),
    Value<String?> ratingSource = const Value.absent(),
    String? mediaType,
    Value<int?> season = const Value.absent(),
    Value<int?> episode = const Value.absent(),
    Value<String?> episodeTitle = const Value.absent(),
    String? streamUrl,
    String? localPath,
    String? quality,
    Value<String?> voiceover = const Value.absent(),
    DownloadStatus? status,
    double? progress,
    int? fileSizeBytes,
    int? downloadedBytes,
    Value<String?> headers = const Value.absent(),
    Value<String?> localPosterPath = const Value.absent(),
    Value<int?> duration = const Value.absent(),
    DateTime? createdAt,
    Value<DateTime?> completedAt = const Value.absent(),
  }) => Download(
    id: id ?? this.id,
    mediaId: mediaId ?? this.mediaId,
    providerId: providerId ?? this.providerId,
    title: title ?? this.title,
    posterUrl: posterUrl.present ? posterUrl.value : this.posterUrl,
    year: year.present ? year.value : this.year,
    rating: rating.present ? rating.value : this.rating,
    ratingSource: ratingSource.present ? ratingSource.value : this.ratingSource,
    mediaType: mediaType ?? this.mediaType,
    season: season.present ? season.value : this.season,
    episode: episode.present ? episode.value : this.episode,
    episodeTitle: episodeTitle.present ? episodeTitle.value : this.episodeTitle,
    streamUrl: streamUrl ?? this.streamUrl,
    localPath: localPath ?? this.localPath,
    quality: quality ?? this.quality,
    voiceover: voiceover.present ? voiceover.value : this.voiceover,
    status: status ?? this.status,
    progress: progress ?? this.progress,
    fileSizeBytes: fileSizeBytes ?? this.fileSizeBytes,
    downloadedBytes: downloadedBytes ?? this.downloadedBytes,
    headers: headers.present ? headers.value : this.headers,
    localPosterPath: localPosterPath.present
        ? localPosterPath.value
        : this.localPosterPath,
    duration: duration.present ? duration.value : this.duration,
    createdAt: createdAt ?? this.createdAt,
    completedAt: completedAt.present ? completedAt.value : this.completedAt,
  );
  Download copyWithCompanion(DownloadsCompanion data) {
    return Download(
      id: data.id.present ? data.id.value : this.id,
      mediaId: data.mediaId.present ? data.mediaId.value : this.mediaId,
      providerId: data.providerId.present
          ? data.providerId.value
          : this.providerId,
      title: data.title.present ? data.title.value : this.title,
      posterUrl: data.posterUrl.present ? data.posterUrl.value : this.posterUrl,
      year: data.year.present ? data.year.value : this.year,
      rating: data.rating.present ? data.rating.value : this.rating,
      ratingSource: data.ratingSource.present
          ? data.ratingSource.value
          : this.ratingSource,
      mediaType: data.mediaType.present ? data.mediaType.value : this.mediaType,
      season: data.season.present ? data.season.value : this.season,
      episode: data.episode.present ? data.episode.value : this.episode,
      episodeTitle: data.episodeTitle.present
          ? data.episodeTitle.value
          : this.episodeTitle,
      streamUrl: data.streamUrl.present ? data.streamUrl.value : this.streamUrl,
      localPath: data.localPath.present ? data.localPath.value : this.localPath,
      quality: data.quality.present ? data.quality.value : this.quality,
      voiceover: data.voiceover.present ? data.voiceover.value : this.voiceover,
      status: data.status.present ? data.status.value : this.status,
      progress: data.progress.present ? data.progress.value : this.progress,
      fileSizeBytes: data.fileSizeBytes.present
          ? data.fileSizeBytes.value
          : this.fileSizeBytes,
      downloadedBytes: data.downloadedBytes.present
          ? data.downloadedBytes.value
          : this.downloadedBytes,
      headers: data.headers.present ? data.headers.value : this.headers,
      localPosterPath: data.localPosterPath.present
          ? data.localPosterPath.value
          : this.localPosterPath,
      duration: data.duration.present ? data.duration.value : this.duration,
      createdAt: data.createdAt.present ? data.createdAt.value : this.createdAt,
      completedAt: data.completedAt.present
          ? data.completedAt.value
          : this.completedAt,
    );
  }

  @override
  String toString() {
    return (StringBuffer('Download(')
          ..write('id: $id, ')
          ..write('mediaId: $mediaId, ')
          ..write('providerId: $providerId, ')
          ..write('title: $title, ')
          ..write('posterUrl: $posterUrl, ')
          ..write('year: $year, ')
          ..write('rating: $rating, ')
          ..write('ratingSource: $ratingSource, ')
          ..write('mediaType: $mediaType, ')
          ..write('season: $season, ')
          ..write('episode: $episode, ')
          ..write('episodeTitle: $episodeTitle, ')
          ..write('streamUrl: $streamUrl, ')
          ..write('localPath: $localPath, ')
          ..write('quality: $quality, ')
          ..write('voiceover: $voiceover, ')
          ..write('status: $status, ')
          ..write('progress: $progress, ')
          ..write('fileSizeBytes: $fileSizeBytes, ')
          ..write('downloadedBytes: $downloadedBytes, ')
          ..write('headers: $headers, ')
          ..write('localPosterPath: $localPosterPath, ')
          ..write('duration: $duration, ')
          ..write('createdAt: $createdAt, ')
          ..write('completedAt: $completedAt')
          ..write(')'))
        .toString();
  }

  @override
  int get hashCode => Object.hashAll([
    id,
    mediaId,
    providerId,
    title,
    posterUrl,
    year,
    rating,
    ratingSource,
    mediaType,
    season,
    episode,
    episodeTitle,
    streamUrl,
    localPath,
    quality,
    voiceover,
    status,
    progress,
    fileSizeBytes,
    downloadedBytes,
    headers,
    localPosterPath,
    duration,
    createdAt,
    completedAt,
  ]);
  @override
  bool operator ==(Object other) =>
      identical(this, other) ||
      (other is Download &&
          other.id == this.id &&
          other.mediaId == this.mediaId &&
          other.providerId == this.providerId &&
          other.title == this.title &&
          other.posterUrl == this.posterUrl &&
          other.year == this.year &&
          other.rating == this.rating &&
          other.ratingSource == this.ratingSource &&
          other.mediaType == this.mediaType &&
          other.season == this.season &&
          other.episode == this.episode &&
          other.episodeTitle == this.episodeTitle &&
          other.streamUrl == this.streamUrl &&
          other.localPath == this.localPath &&
          other.quality == this.quality &&
          other.voiceover == this.voiceover &&
          other.status == this.status &&
          other.progress == this.progress &&
          other.fileSizeBytes == this.fileSizeBytes &&
          other.downloadedBytes == this.downloadedBytes &&
          other.headers == this.headers &&
          other.localPosterPath == this.localPosterPath &&
          other.duration == this.duration &&
          other.createdAt == this.createdAt &&
          other.completedAt == this.completedAt);
}

class DownloadsCompanion extends UpdateCompanion<Download> {
  final Value<int> id;
  final Value<String> mediaId;
  final Value<String> providerId;
  final Value<String> title;
  final Value<String?> posterUrl;
  final Value<int?> year;
  final Value<double?> rating;
  final Value<String?> ratingSource;
  final Value<String> mediaType;
  final Value<int?> season;
  final Value<int?> episode;
  final Value<String?> episodeTitle;
  final Value<String> streamUrl;
  final Value<String> localPath;
  final Value<String> quality;
  final Value<String?> voiceover;
  final Value<DownloadStatus> status;
  final Value<double> progress;
  final Value<int> fileSizeBytes;
  final Value<int> downloadedBytes;
  final Value<String?> headers;
  final Value<String?> localPosterPath;
  final Value<int?> duration;
  final Value<DateTime> createdAt;
  final Value<DateTime?> completedAt;
  const DownloadsCompanion({
    this.id = const Value.absent(),
    this.mediaId = const Value.absent(),
    this.providerId = const Value.absent(),
    this.title = const Value.absent(),
    this.posterUrl = const Value.absent(),
    this.year = const Value.absent(),
    this.rating = const Value.absent(),
    this.ratingSource = const Value.absent(),
    this.mediaType = const Value.absent(),
    this.season = const Value.absent(),
    this.episode = const Value.absent(),
    this.episodeTitle = const Value.absent(),
    this.streamUrl = const Value.absent(),
    this.localPath = const Value.absent(),
    this.quality = const Value.absent(),
    this.voiceover = const Value.absent(),
    this.status = const Value.absent(),
    this.progress = const Value.absent(),
    this.fileSizeBytes = const Value.absent(),
    this.downloadedBytes = const Value.absent(),
    this.headers = const Value.absent(),
    this.localPosterPath = const Value.absent(),
    this.duration = const Value.absent(),
    this.createdAt = const Value.absent(),
    this.completedAt = const Value.absent(),
  });
  DownloadsCompanion.insert({
    this.id = const Value.absent(),
    required String mediaId,
    required String providerId,
    required String title,
    this.posterUrl = const Value.absent(),
    this.year = const Value.absent(),
    this.rating = const Value.absent(),
    this.ratingSource = const Value.absent(),
    required String mediaType,
    this.season = const Value.absent(),
    this.episode = const Value.absent(),
    this.episodeTitle = const Value.absent(),
    required String streamUrl,
    required String localPath,
    required String quality,
    this.voiceover = const Value.absent(),
    this.status = const Value.absent(),
    this.progress = const Value.absent(),
    this.fileSizeBytes = const Value.absent(),
    this.downloadedBytes = const Value.absent(),
    this.headers = const Value.absent(),
    this.localPosterPath = const Value.absent(),
    this.duration = const Value.absent(),
    this.createdAt = const Value.absent(),
    this.completedAt = const Value.absent(),
  }) : mediaId = Value(mediaId),
       providerId = Value(providerId),
       title = Value(title),
       mediaType = Value(mediaType),
       streamUrl = Value(streamUrl),
       localPath = Value(localPath),
       quality = Value(quality);
  static Insertable<Download> custom({
    Expression<int>? id,
    Expression<String>? mediaId,
    Expression<String>? providerId,
    Expression<String>? title,
    Expression<String>? posterUrl,
    Expression<int>? year,
    Expression<double>? rating,
    Expression<String>? ratingSource,
    Expression<String>? mediaType,
    Expression<int>? season,
    Expression<int>? episode,
    Expression<String>? episodeTitle,
    Expression<String>? streamUrl,
    Expression<String>? localPath,
    Expression<String>? quality,
    Expression<String>? voiceover,
    Expression<int>? status,
    Expression<double>? progress,
    Expression<int>? fileSizeBytes,
    Expression<int>? downloadedBytes,
    Expression<String>? headers,
    Expression<String>? localPosterPath,
    Expression<int>? duration,
    Expression<DateTime>? createdAt,
    Expression<DateTime>? completedAt,
  }) {
    return RawValuesInsertable({
      if (id != null) 'id': id,
      if (mediaId != null) 'media_id': mediaId,
      if (providerId != null) 'provider_id': providerId,
      if (title != null) 'title': title,
      if (posterUrl != null) 'poster_url': posterUrl,
      if (year != null) 'year': year,
      if (rating != null) 'rating': rating,
      if (ratingSource != null) 'rating_source': ratingSource,
      if (mediaType != null) 'media_type': mediaType,
      if (season != null) 'season': season,
      if (episode != null) 'episode': episode,
      if (episodeTitle != null) 'episode_title': episodeTitle,
      if (streamUrl != null) 'stream_url': streamUrl,
      if (localPath != null) 'local_path': localPath,
      if (quality != null) 'quality': quality,
      if (voiceover != null) 'voiceover': voiceover,
      if (status != null) 'status': status,
      if (progress != null) 'progress': progress,
      if (fileSizeBytes != null) 'file_size_bytes': fileSizeBytes,
      if (downloadedBytes != null) 'downloaded_bytes': downloadedBytes,
      if (headers != null) 'headers': headers,
      if (localPosterPath != null) 'local_poster_path': localPosterPath,
      if (duration != null) 'duration': duration,
      if (createdAt != null) 'created_at': createdAt,
      if (completedAt != null) 'completed_at': completedAt,
    });
  }

  DownloadsCompanion copyWith({
    Value<int>? id,
    Value<String>? mediaId,
    Value<String>? providerId,
    Value<String>? title,
    Value<String?>? posterUrl,
    Value<int?>? year,
    Value<double?>? rating,
    Value<String?>? ratingSource,
    Value<String>? mediaType,
    Value<int?>? season,
    Value<int?>? episode,
    Value<String?>? episodeTitle,
    Value<String>? streamUrl,
    Value<String>? localPath,
    Value<String>? quality,
    Value<String?>? voiceover,
    Value<DownloadStatus>? status,
    Value<double>? progress,
    Value<int>? fileSizeBytes,
    Value<int>? downloadedBytes,
    Value<String?>? headers,
    Value<String?>? localPosterPath,
    Value<int?>? duration,
    Value<DateTime>? createdAt,
    Value<DateTime?>? completedAt,
  }) {
    return DownloadsCompanion(
      id: id ?? this.id,
      mediaId: mediaId ?? this.mediaId,
      providerId: providerId ?? this.providerId,
      title: title ?? this.title,
      posterUrl: posterUrl ?? this.posterUrl,
      year: year ?? this.year,
      rating: rating ?? this.rating,
      ratingSource: ratingSource ?? this.ratingSource,
      mediaType: mediaType ?? this.mediaType,
      season: season ?? this.season,
      episode: episode ?? this.episode,
      episodeTitle: episodeTitle ?? this.episodeTitle,
      streamUrl: streamUrl ?? this.streamUrl,
      localPath: localPath ?? this.localPath,
      quality: quality ?? this.quality,
      voiceover: voiceover ?? this.voiceover,
      status: status ?? this.status,
      progress: progress ?? this.progress,
      fileSizeBytes: fileSizeBytes ?? this.fileSizeBytes,
      downloadedBytes: downloadedBytes ?? this.downloadedBytes,
      headers: headers ?? this.headers,
      localPosterPath: localPosterPath ?? this.localPosterPath,
      duration: duration ?? this.duration,
      createdAt: createdAt ?? this.createdAt,
      completedAt: completedAt ?? this.completedAt,
    );
  }

  @override
  Map<String, Expression> toColumns(bool nullToAbsent) {
    final map = <String, Expression>{};
    if (id.present) {
      map['id'] = Variable<int>(id.value);
    }
    if (mediaId.present) {
      map['media_id'] = Variable<String>(mediaId.value);
    }
    if (providerId.present) {
      map['provider_id'] = Variable<String>(providerId.value);
    }
    if (title.present) {
      map['title'] = Variable<String>(title.value);
    }
    if (posterUrl.present) {
      map['poster_url'] = Variable<String>(posterUrl.value);
    }
    if (year.present) {
      map['year'] = Variable<int>(year.value);
    }
    if (rating.present) {
      map['rating'] = Variable<double>(rating.value);
    }
    if (ratingSource.present) {
      map['rating_source'] = Variable<String>(ratingSource.value);
    }
    if (mediaType.present) {
      map['media_type'] = Variable<String>(mediaType.value);
    }
    if (season.present) {
      map['season'] = Variable<int>(season.value);
    }
    if (episode.present) {
      map['episode'] = Variable<int>(episode.value);
    }
    if (episodeTitle.present) {
      map['episode_title'] = Variable<String>(episodeTitle.value);
    }
    if (streamUrl.present) {
      map['stream_url'] = Variable<String>(streamUrl.value);
    }
    if (localPath.present) {
      map['local_path'] = Variable<String>(localPath.value);
    }
    if (quality.present) {
      map['quality'] = Variable<String>(quality.value);
    }
    if (voiceover.present) {
      map['voiceover'] = Variable<String>(voiceover.value);
    }
    if (status.present) {
      map['status'] = Variable<int>(
        $DownloadsTable.$converterstatus.toSql(status.value),
      );
    }
    if (progress.present) {
      map['progress'] = Variable<double>(progress.value);
    }
    if (fileSizeBytes.present) {
      map['file_size_bytes'] = Variable<int>(fileSizeBytes.value);
    }
    if (downloadedBytes.present) {
      map['downloaded_bytes'] = Variable<int>(downloadedBytes.value);
    }
    if (headers.present) {
      map['headers'] = Variable<String>(headers.value);
    }
    if (localPosterPath.present) {
      map['local_poster_path'] = Variable<String>(localPosterPath.value);
    }
    if (duration.present) {
      map['duration'] = Variable<int>(duration.value);
    }
    if (createdAt.present) {
      map['created_at'] = Variable<DateTime>(createdAt.value);
    }
    if (completedAt.present) {
      map['completed_at'] = Variable<DateTime>(completedAt.value);
    }
    return map;
  }

  @override
  String toString() {
    return (StringBuffer('DownloadsCompanion(')
          ..write('id: $id, ')
          ..write('mediaId: $mediaId, ')
          ..write('providerId: $providerId, ')
          ..write('title: $title, ')
          ..write('posterUrl: $posterUrl, ')
          ..write('year: $year, ')
          ..write('rating: $rating, ')
          ..write('ratingSource: $ratingSource, ')
          ..write('mediaType: $mediaType, ')
          ..write('season: $season, ')
          ..write('episode: $episode, ')
          ..write('episodeTitle: $episodeTitle, ')
          ..write('streamUrl: $streamUrl, ')
          ..write('localPath: $localPath, ')
          ..write('quality: $quality, ')
          ..write('voiceover: $voiceover, ')
          ..write('status: $status, ')
          ..write('progress: $progress, ')
          ..write('fileSizeBytes: $fileSizeBytes, ')
          ..write('downloadedBytes: $downloadedBytes, ')
          ..write('headers: $headers, ')
          ..write('localPosterPath: $localPosterPath, ')
          ..write('duration: $duration, ')
          ..write('createdAt: $createdAt, ')
          ..write('completedAt: $completedAt')
          ..write(')'))
        .toString();
  }
}

class $SearchHistoryTableTable extends SearchHistoryTable
    with TableInfo<$SearchHistoryTableTable, SearchHistoryTableData> {
  @override
  final GeneratedDatabase attachedDatabase;
  final String? _alias;
  $SearchHistoryTableTable(this.attachedDatabase, [this._alias]);
  static const VerificationMeta _idMeta = const VerificationMeta('id');
  @override
  late final GeneratedColumn<int> id = GeneratedColumn<int>(
    'id',
    aliasedName,
    false,
    hasAutoIncrement: true,
    type: DriftSqlType.int,
    requiredDuringInsert: false,
    defaultConstraints: GeneratedColumn.constraintIsAlways(
      'PRIMARY KEY AUTOINCREMENT',
    ),
  );
  static const VerificationMeta _queryMeta = const VerificationMeta('query');
  @override
  late final GeneratedColumn<String> query = GeneratedColumn<String>(
    'query',
    aliasedName,
    false,
    type: DriftSqlType.string,
    requiredDuringInsert: true,
  );
  static const VerificationMeta _normalizedQueryMeta = const VerificationMeta(
    'normalizedQuery',
  );
  @override
  late final GeneratedColumn<String> normalizedQuery = GeneratedColumn<String>(
    'normalized_query',
    aliasedName,
    false,
    type: DriftSqlType.string,
    requiredDuringInsert: true,
  );
  static const VerificationMeta _resultCountMeta = const VerificationMeta(
    'resultCount',
  );
  @override
  late final GeneratedColumn<int> resultCount = GeneratedColumn<int>(
    'result_count',
    aliasedName,
    false,
    type: DriftSqlType.int,
    requiredDuringInsert: false,
    defaultValue: const Constant(0),
  );
  static const VerificationMeta _wasSuccessfulMeta = const VerificationMeta(
    'wasSuccessful',
  );
  @override
  late final GeneratedColumn<bool> wasSuccessful = GeneratedColumn<bool>(
    'was_successful',
    aliasedName,
    false,
    type: DriftSqlType.bool,
    requiredDuringInsert: false,
    defaultConstraints: GeneratedColumn.constraintIsAlways(
      'CHECK ("was_successful" IN (0, 1))',
    ),
    defaultValue: const Constant(true),
  );
  static const VerificationMeta _searchCountMeta = const VerificationMeta(
    'searchCount',
  );
  @override
  late final GeneratedColumn<int> searchCount = GeneratedColumn<int>(
    'search_count',
    aliasedName,
    false,
    type: DriftSqlType.int,
    requiredDuringInsert: false,
    defaultValue: const Constant(1),
  );
  static const VerificationMeta _firstSearchedAtMeta = const VerificationMeta(
    'firstSearchedAt',
  );
  @override
  late final GeneratedColumn<DateTime> firstSearchedAt =
      GeneratedColumn<DateTime>(
        'first_searched_at',
        aliasedName,
        false,
        type: DriftSqlType.dateTime,
        requiredDuringInsert: false,
        defaultValue: currentDateAndTime,
      );
  static const VerificationMeta _lastSearchedAtMeta = const VerificationMeta(
    'lastSearchedAt',
  );
  @override
  late final GeneratedColumn<DateTime> lastSearchedAt =
      GeneratedColumn<DateTime>(
        'last_searched_at',
        aliasedName,
        false,
        type: DriftSqlType.dateTime,
        requiredDuringInsert: false,
        defaultValue: currentDateAndTime,
      );
  @override
  List<GeneratedColumn> get $columns => [
    id,
    query,
    normalizedQuery,
    resultCount,
    wasSuccessful,
    searchCount,
    firstSearchedAt,
    lastSearchedAt,
  ];
  @override
  String get aliasedName => _alias ?? actualTableName;
  @override
  String get actualTableName => $name;
  static const String $name = 'search_history_table';
  @override
  VerificationContext validateIntegrity(
    Insertable<SearchHistoryTableData> instance, {
    bool isInserting = false,
  }) {
    final context = VerificationContext();
    final data = instance.toColumns(true);
    if (data.containsKey('id')) {
      context.handle(_idMeta, id.isAcceptableOrUnknown(data['id']!, _idMeta));
    }
    if (data.containsKey('query')) {
      context.handle(
        _queryMeta,
        query.isAcceptableOrUnknown(data['query']!, _queryMeta),
      );
    } else if (isInserting) {
      context.missing(_queryMeta);
    }
    if (data.containsKey('normalized_query')) {
      context.handle(
        _normalizedQueryMeta,
        normalizedQuery.isAcceptableOrUnknown(
          data['normalized_query']!,
          _normalizedQueryMeta,
        ),
      );
    } else if (isInserting) {
      context.missing(_normalizedQueryMeta);
    }
    if (data.containsKey('result_count')) {
      context.handle(
        _resultCountMeta,
        resultCount.isAcceptableOrUnknown(
          data['result_count']!,
          _resultCountMeta,
        ),
      );
    }
    if (data.containsKey('was_successful')) {
      context.handle(
        _wasSuccessfulMeta,
        wasSuccessful.isAcceptableOrUnknown(
          data['was_successful']!,
          _wasSuccessfulMeta,
        ),
      );
    }
    if (data.containsKey('search_count')) {
      context.handle(
        _searchCountMeta,
        searchCount.isAcceptableOrUnknown(
          data['search_count']!,
          _searchCountMeta,
        ),
      );
    }
    if (data.containsKey('first_searched_at')) {
      context.handle(
        _firstSearchedAtMeta,
        firstSearchedAt.isAcceptableOrUnknown(
          data['first_searched_at']!,
          _firstSearchedAtMeta,
        ),
      );
    }
    if (data.containsKey('last_searched_at')) {
      context.handle(
        _lastSearchedAtMeta,
        lastSearchedAt.isAcceptableOrUnknown(
          data['last_searched_at']!,
          _lastSearchedAtMeta,
        ),
      );
    }
    return context;
  }

  @override
  Set<GeneratedColumn> get $primaryKey => {id};
  @override
  List<Set<GeneratedColumn>> get uniqueKeys => [
    {normalizedQuery},
  ];
  @override
  SearchHistoryTableData map(Map<String, dynamic> data, {String? tablePrefix}) {
    final effectivePrefix = tablePrefix != null ? '$tablePrefix.' : '';
    return SearchHistoryTableData(
      id: attachedDatabase.typeMapping.read(
        DriftSqlType.int,
        data['${effectivePrefix}id'],
      )!,
      query: attachedDatabase.typeMapping.read(
        DriftSqlType.string,
        data['${effectivePrefix}query'],
      )!,
      normalizedQuery: attachedDatabase.typeMapping.read(
        DriftSqlType.string,
        data['${effectivePrefix}normalized_query'],
      )!,
      resultCount: attachedDatabase.typeMapping.read(
        DriftSqlType.int,
        data['${effectivePrefix}result_count'],
      )!,
      wasSuccessful: attachedDatabase.typeMapping.read(
        DriftSqlType.bool,
        data['${effectivePrefix}was_successful'],
      )!,
      searchCount: attachedDatabase.typeMapping.read(
        DriftSqlType.int,
        data['${effectivePrefix}search_count'],
      )!,
      firstSearchedAt: attachedDatabase.typeMapping.read(
        DriftSqlType.dateTime,
        data['${effectivePrefix}first_searched_at'],
      )!,
      lastSearchedAt: attachedDatabase.typeMapping.read(
        DriftSqlType.dateTime,
        data['${effectivePrefix}last_searched_at'],
      )!,
    );
  }

  @override
  $SearchHistoryTableTable createAlias(String alias) {
    return $SearchHistoryTableTable(attachedDatabase, alias);
  }
}

class SearchHistoryTableData extends DataClass
    implements Insertable<SearchHistoryTableData> {
  final int id;

  /// Original query as typed by user
  final String query;

  /// Normalized query for comparison (lowercase, trimmed)
  final String normalizedQuery;

  /// Number of results found for this query
  final int resultCount;

  /// Whether search returned any results
  final bool wasSuccessful;

  /// Number of times this query was searched
  final int searchCount;

  /// When query was first searched
  final DateTime firstSearchedAt;

  /// When query was last searched
  final DateTime lastSearchedAt;
  const SearchHistoryTableData({
    required this.id,
    required this.query,
    required this.normalizedQuery,
    required this.resultCount,
    required this.wasSuccessful,
    required this.searchCount,
    required this.firstSearchedAt,
    required this.lastSearchedAt,
  });
  @override
  Map<String, Expression> toColumns(bool nullToAbsent) {
    final map = <String, Expression>{};
    map['id'] = Variable<int>(id);
    map['query'] = Variable<String>(query);
    map['normalized_query'] = Variable<String>(normalizedQuery);
    map['result_count'] = Variable<int>(resultCount);
    map['was_successful'] = Variable<bool>(wasSuccessful);
    map['search_count'] = Variable<int>(searchCount);
    map['first_searched_at'] = Variable<DateTime>(firstSearchedAt);
    map['last_searched_at'] = Variable<DateTime>(lastSearchedAt);
    return map;
  }

  SearchHistoryTableCompanion toCompanion(bool nullToAbsent) {
    return SearchHistoryTableCompanion(
      id: Value(id),
      query: Value(query),
      normalizedQuery: Value(normalizedQuery),
      resultCount: Value(resultCount),
      wasSuccessful: Value(wasSuccessful),
      searchCount: Value(searchCount),
      firstSearchedAt: Value(firstSearchedAt),
      lastSearchedAt: Value(lastSearchedAt),
    );
  }

  factory SearchHistoryTableData.fromJson(
    Map<String, dynamic> json, {
    ValueSerializer? serializer,
  }) {
    serializer ??= driftRuntimeOptions.defaultSerializer;
    return SearchHistoryTableData(
      id: serializer.fromJson<int>(json['id']),
      query: serializer.fromJson<String>(json['query']),
      normalizedQuery: serializer.fromJson<String>(json['normalizedQuery']),
      resultCount: serializer.fromJson<int>(json['resultCount']),
      wasSuccessful: serializer.fromJson<bool>(json['wasSuccessful']),
      searchCount: serializer.fromJson<int>(json['searchCount']),
      firstSearchedAt: serializer.fromJson<DateTime>(json['firstSearchedAt']),
      lastSearchedAt: serializer.fromJson<DateTime>(json['lastSearchedAt']),
    );
  }
  @override
  Map<String, dynamic> toJson({ValueSerializer? serializer}) {
    serializer ??= driftRuntimeOptions.defaultSerializer;
    return <String, dynamic>{
      'id': serializer.toJson<int>(id),
      'query': serializer.toJson<String>(query),
      'normalizedQuery': serializer.toJson<String>(normalizedQuery),
      'resultCount': serializer.toJson<int>(resultCount),
      'wasSuccessful': serializer.toJson<bool>(wasSuccessful),
      'searchCount': serializer.toJson<int>(searchCount),
      'firstSearchedAt': serializer.toJson<DateTime>(firstSearchedAt),
      'lastSearchedAt': serializer.toJson<DateTime>(lastSearchedAt),
    };
  }

  SearchHistoryTableData copyWith({
    int? id,
    String? query,
    String? normalizedQuery,
    int? resultCount,
    bool? wasSuccessful,
    int? searchCount,
    DateTime? firstSearchedAt,
    DateTime? lastSearchedAt,
  }) => SearchHistoryTableData(
    id: id ?? this.id,
    query: query ?? this.query,
    normalizedQuery: normalizedQuery ?? this.normalizedQuery,
    resultCount: resultCount ?? this.resultCount,
    wasSuccessful: wasSuccessful ?? this.wasSuccessful,
    searchCount: searchCount ?? this.searchCount,
    firstSearchedAt: firstSearchedAt ?? this.firstSearchedAt,
    lastSearchedAt: lastSearchedAt ?? this.lastSearchedAt,
  );
  SearchHistoryTableData copyWithCompanion(SearchHistoryTableCompanion data) {
    return SearchHistoryTableData(
      id: data.id.present ? data.id.value : this.id,
      query: data.query.present ? data.query.value : this.query,
      normalizedQuery: data.normalizedQuery.present
          ? data.normalizedQuery.value
          : this.normalizedQuery,
      resultCount: data.resultCount.present
          ? data.resultCount.value
          : this.resultCount,
      wasSuccessful: data.wasSuccessful.present
          ? data.wasSuccessful.value
          : this.wasSuccessful,
      searchCount: data.searchCount.present
          ? data.searchCount.value
          : this.searchCount,
      firstSearchedAt: data.firstSearchedAt.present
          ? data.firstSearchedAt.value
          : this.firstSearchedAt,
      lastSearchedAt: data.lastSearchedAt.present
          ? data.lastSearchedAt.value
          : this.lastSearchedAt,
    );
  }

  @override
  String toString() {
    return (StringBuffer('SearchHistoryTableData(')
          ..write('id: $id, ')
          ..write('query: $query, ')
          ..write('normalizedQuery: $normalizedQuery, ')
          ..write('resultCount: $resultCount, ')
          ..write('wasSuccessful: $wasSuccessful, ')
          ..write('searchCount: $searchCount, ')
          ..write('firstSearchedAt: $firstSearchedAt, ')
          ..write('lastSearchedAt: $lastSearchedAt')
          ..write(')'))
        .toString();
  }

  @override
  int get hashCode => Object.hash(
    id,
    query,
    normalizedQuery,
    resultCount,
    wasSuccessful,
    searchCount,
    firstSearchedAt,
    lastSearchedAt,
  );
  @override
  bool operator ==(Object other) =>
      identical(this, other) ||
      (other is SearchHistoryTableData &&
          other.id == this.id &&
          other.query == this.query &&
          other.normalizedQuery == this.normalizedQuery &&
          other.resultCount == this.resultCount &&
          other.wasSuccessful == this.wasSuccessful &&
          other.searchCount == this.searchCount &&
          other.firstSearchedAt == this.firstSearchedAt &&
          other.lastSearchedAt == this.lastSearchedAt);
}

class SearchHistoryTableCompanion
    extends UpdateCompanion<SearchHistoryTableData> {
  final Value<int> id;
  final Value<String> query;
  final Value<String> normalizedQuery;
  final Value<int> resultCount;
  final Value<bool> wasSuccessful;
  final Value<int> searchCount;
  final Value<DateTime> firstSearchedAt;
  final Value<DateTime> lastSearchedAt;
  const SearchHistoryTableCompanion({
    this.id = const Value.absent(),
    this.query = const Value.absent(),
    this.normalizedQuery = const Value.absent(),
    this.resultCount = const Value.absent(),
    this.wasSuccessful = const Value.absent(),
    this.searchCount = const Value.absent(),
    this.firstSearchedAt = const Value.absent(),
    this.lastSearchedAt = const Value.absent(),
  });
  SearchHistoryTableCompanion.insert({
    this.id = const Value.absent(),
    required String query,
    required String normalizedQuery,
    this.resultCount = const Value.absent(),
    this.wasSuccessful = const Value.absent(),
    this.searchCount = const Value.absent(),
    this.firstSearchedAt = const Value.absent(),
    this.lastSearchedAt = const Value.absent(),
  }) : query = Value(query),
       normalizedQuery = Value(normalizedQuery);
  static Insertable<SearchHistoryTableData> custom({
    Expression<int>? id,
    Expression<String>? query,
    Expression<String>? normalizedQuery,
    Expression<int>? resultCount,
    Expression<bool>? wasSuccessful,
    Expression<int>? searchCount,
    Expression<DateTime>? firstSearchedAt,
    Expression<DateTime>? lastSearchedAt,
  }) {
    return RawValuesInsertable({
      if (id != null) 'id': id,
      if (query != null) 'query': query,
      if (normalizedQuery != null) 'normalized_query': normalizedQuery,
      if (resultCount != null) 'result_count': resultCount,
      if (wasSuccessful != null) 'was_successful': wasSuccessful,
      if (searchCount != null) 'search_count': searchCount,
      if (firstSearchedAt != null) 'first_searched_at': firstSearchedAt,
      if (lastSearchedAt != null) 'last_searched_at': lastSearchedAt,
    });
  }

  SearchHistoryTableCompanion copyWith({
    Value<int>? id,
    Value<String>? query,
    Value<String>? normalizedQuery,
    Value<int>? resultCount,
    Value<bool>? wasSuccessful,
    Value<int>? searchCount,
    Value<DateTime>? firstSearchedAt,
    Value<DateTime>? lastSearchedAt,
  }) {
    return SearchHistoryTableCompanion(
      id: id ?? this.id,
      query: query ?? this.query,
      normalizedQuery: normalizedQuery ?? this.normalizedQuery,
      resultCount: resultCount ?? this.resultCount,
      wasSuccessful: wasSuccessful ?? this.wasSuccessful,
      searchCount: searchCount ?? this.searchCount,
      firstSearchedAt: firstSearchedAt ?? this.firstSearchedAt,
      lastSearchedAt: lastSearchedAt ?? this.lastSearchedAt,
    );
  }

  @override
  Map<String, Expression> toColumns(bool nullToAbsent) {
    final map = <String, Expression>{};
    if (id.present) {
      map['id'] = Variable<int>(id.value);
    }
    if (query.present) {
      map['query'] = Variable<String>(query.value);
    }
    if (normalizedQuery.present) {
      map['normalized_query'] = Variable<String>(normalizedQuery.value);
    }
    if (resultCount.present) {
      map['result_count'] = Variable<int>(resultCount.value);
    }
    if (wasSuccessful.present) {
      map['was_successful'] = Variable<bool>(wasSuccessful.value);
    }
    if (searchCount.present) {
      map['search_count'] = Variable<int>(searchCount.value);
    }
    if (firstSearchedAt.present) {
      map['first_searched_at'] = Variable<DateTime>(firstSearchedAt.value);
    }
    if (lastSearchedAt.present) {
      map['last_searched_at'] = Variable<DateTime>(lastSearchedAt.value);
    }
    return map;
  }

  @override
  String toString() {
    return (StringBuffer('SearchHistoryTableCompanion(')
          ..write('id: $id, ')
          ..write('query: $query, ')
          ..write('normalizedQuery: $normalizedQuery, ')
          ..write('resultCount: $resultCount, ')
          ..write('wasSuccessful: $wasSuccessful, ')
          ..write('searchCount: $searchCount, ')
          ..write('firstSearchedAt: $firstSearchedAt, ')
          ..write('lastSearchedAt: $lastSearchedAt')
          ..write(')'))
        .toString();
  }
}

class $StoredMediaItemsTable extends StoredMediaItems
    with TableInfo<$StoredMediaItemsTable, StoredMediaItem> {
  @override
  final GeneratedDatabase attachedDatabase;
  final String? _alias;
  $StoredMediaItemsTable(this.attachedDatabase, [this._alias]);
  static const VerificationMeta _idMeta = const VerificationMeta('id');
  @override
  late final GeneratedColumn<String> id = GeneratedColumn<String>(
    'id',
    aliasedName,
    false,
    type: DriftSqlType.string,
    requiredDuringInsert: true,
  );
  static const VerificationMeta _providerIdMeta = const VerificationMeta(
    'providerId',
  );
  @override
  late final GeneratedColumn<String> providerId = GeneratedColumn<String>(
    'provider_id',
    aliasedName,
    false,
    type: DriftSqlType.string,
    requiredDuringInsert: true,
  );
  static const VerificationMeta _titleMeta = const VerificationMeta('title');
  @override
  late final GeneratedColumn<String> title = GeneratedColumn<String>(
    'title',
    aliasedName,
    false,
    type: DriftSqlType.string,
    requiredDuringInsert: true,
  );
  static const VerificationMeta _originalTitleMeta = const VerificationMeta(
    'originalTitle',
  );
  @override
  late final GeneratedColumn<String> originalTitle = GeneratedColumn<String>(
    'original_title',
    aliasedName,
    true,
    type: DriftSqlType.string,
    requiredDuringInsert: false,
  );
  static const VerificationMeta _posterUrlMeta = const VerificationMeta(
    'posterUrl',
  );
  @override
  late final GeneratedColumn<String> posterUrl = GeneratedColumn<String>(
    'poster_url',
    aliasedName,
    true,
    type: DriftSqlType.string,
    requiredDuringInsert: false,
  );
  static const VerificationMeta _yearMeta = const VerificationMeta('year');
  @override
  late final GeneratedColumn<int> year = GeneratedColumn<int>(
    'year',
    aliasedName,
    true,
    type: DriftSqlType.int,
    requiredDuringInsert: false,
  );
  static const VerificationMeta _ratingMeta = const VerificationMeta('rating');
  @override
  late final GeneratedColumn<double> rating = GeneratedColumn<double>(
    'rating',
    aliasedName,
    true,
    type: DriftSqlType.double,
    requiredDuringInsert: false,
  );
  static const VerificationMeta _mediaTypeMeta = const VerificationMeta(
    'mediaType',
  );
  @override
  late final GeneratedColumn<String> mediaType = GeneratedColumn<String>(
    'media_type',
    aliasedName,
    false,
    type: DriftSqlType.string,
    requiredDuringInsert: true,
  );
  static const VerificationMeta _descriptionMeta = const VerificationMeta(
    'description',
  );
  @override
  late final GeneratedColumn<String> description = GeneratedColumn<String>(
    'description',
    aliasedName,
    true,
    type: DriftSqlType.string,
    requiredDuringInsert: false,
  );
  static const VerificationMeta _genresMeta = const VerificationMeta('genres');
  @override
  late final GeneratedColumn<String> genres = GeneratedColumn<String>(
    'genres',
    aliasedName,
    true,
    type: DriftSqlType.string,
    requiredDuringInsert: false,
  );
  static const VerificationMeta _countryMeta = const VerificationMeta(
    'country',
  );
  @override
  late final GeneratedColumn<String> country = GeneratedColumn<String>(
    'country',
    aliasedName,
    true,
    type: DriftSqlType.string,
    requiredDuringInsert: false,
  );
  static const VerificationMeta _ratingSourceMeta = const VerificationMeta(
    'ratingSource',
  );
  @override
  late final GeneratedColumn<String> ratingSource = GeneratedColumn<String>(
    'rating_source',
    aliasedName,
    true,
    type: DriftSqlType.string,
    requiredDuringInsert: false,
  );
  static const VerificationMeta _updatedAtMeta = const VerificationMeta(
    'updatedAt',
  );
  @override
  late final GeneratedColumn<DateTime> updatedAt = GeneratedColumn<DateTime>(
    'updated_at',
    aliasedName,
    false,
    type: DriftSqlType.dateTime,
    requiredDuringInsert: false,
    defaultValue: currentDateAndTime,
  );
  @override
  List<GeneratedColumn> get $columns => [
    id,
    providerId,
    title,
    originalTitle,
    posterUrl,
    year,
    rating,
    mediaType,
    description,
    genres,
    country,
    ratingSource,
    updatedAt,
  ];
  @override
  String get aliasedName => _alias ?? actualTableName;
  @override
  String get actualTableName => $name;
  static const String $name = 'stored_media_items';
  @override
  VerificationContext validateIntegrity(
    Insertable<StoredMediaItem> instance, {
    bool isInserting = false,
  }) {
    final context = VerificationContext();
    final data = instance.toColumns(true);
    if (data.containsKey('id')) {
      context.handle(_idMeta, id.isAcceptableOrUnknown(data['id']!, _idMeta));
    } else if (isInserting) {
      context.missing(_idMeta);
    }
    if (data.containsKey('provider_id')) {
      context.handle(
        _providerIdMeta,
        providerId.isAcceptableOrUnknown(data['provider_id']!, _providerIdMeta),
      );
    } else if (isInserting) {
      context.missing(_providerIdMeta);
    }
    if (data.containsKey('title')) {
      context.handle(
        _titleMeta,
        title.isAcceptableOrUnknown(data['title']!, _titleMeta),
      );
    } else if (isInserting) {
      context.missing(_titleMeta);
    }
    if (data.containsKey('original_title')) {
      context.handle(
        _originalTitleMeta,
        originalTitle.isAcceptableOrUnknown(
          data['original_title']!,
          _originalTitleMeta,
        ),
      );
    }
    if (data.containsKey('poster_url')) {
      context.handle(
        _posterUrlMeta,
        posterUrl.isAcceptableOrUnknown(data['poster_url']!, _posterUrlMeta),
      );
    }
    if (data.containsKey('year')) {
      context.handle(
        _yearMeta,
        year.isAcceptableOrUnknown(data['year']!, _yearMeta),
      );
    }
    if (data.containsKey('rating')) {
      context.handle(
        _ratingMeta,
        rating.isAcceptableOrUnknown(data['rating']!, _ratingMeta),
      );
    }
    if (data.containsKey('media_type')) {
      context.handle(
        _mediaTypeMeta,
        mediaType.isAcceptableOrUnknown(data['media_type']!, _mediaTypeMeta),
      );
    } else if (isInserting) {
      context.missing(_mediaTypeMeta);
    }
    if (data.containsKey('description')) {
      context.handle(
        _descriptionMeta,
        description.isAcceptableOrUnknown(
          data['description']!,
          _descriptionMeta,
        ),
      );
    }
    if (data.containsKey('genres')) {
      context.handle(
        _genresMeta,
        genres.isAcceptableOrUnknown(data['genres']!, _genresMeta),
      );
    }
    if (data.containsKey('country')) {
      context.handle(
        _countryMeta,
        country.isAcceptableOrUnknown(data['country']!, _countryMeta),
      );
    }
    if (data.containsKey('rating_source')) {
      context.handle(
        _ratingSourceMeta,
        ratingSource.isAcceptableOrUnknown(
          data['rating_source']!,
          _ratingSourceMeta,
        ),
      );
    }
    if (data.containsKey('updated_at')) {
      context.handle(
        _updatedAtMeta,
        updatedAt.isAcceptableOrUnknown(data['updated_at']!, _updatedAtMeta),
      );
    }
    return context;
  }

  @override
  Set<GeneratedColumn> get $primaryKey => {providerId, id};
  @override
  StoredMediaItem map(Map<String, dynamic> data, {String? tablePrefix}) {
    final effectivePrefix = tablePrefix != null ? '$tablePrefix.' : '';
    return StoredMediaItem(
      id: attachedDatabase.typeMapping.read(
        DriftSqlType.string,
        data['${effectivePrefix}id'],
      )!,
      providerId: attachedDatabase.typeMapping.read(
        DriftSqlType.string,
        data['${effectivePrefix}provider_id'],
      )!,
      title: attachedDatabase.typeMapping.read(
        DriftSqlType.string,
        data['${effectivePrefix}title'],
      )!,
      originalTitle: attachedDatabase.typeMapping.read(
        DriftSqlType.string,
        data['${effectivePrefix}original_title'],
      ),
      posterUrl: attachedDatabase.typeMapping.read(
        DriftSqlType.string,
        data['${effectivePrefix}poster_url'],
      ),
      year: attachedDatabase.typeMapping.read(
        DriftSqlType.int,
        data['${effectivePrefix}year'],
      ),
      rating: attachedDatabase.typeMapping.read(
        DriftSqlType.double,
        data['${effectivePrefix}rating'],
      ),
      mediaType: attachedDatabase.typeMapping.read(
        DriftSqlType.string,
        data['${effectivePrefix}media_type'],
      )!,
      description: attachedDatabase.typeMapping.read(
        DriftSqlType.string,
        data['${effectivePrefix}description'],
      ),
      genres: attachedDatabase.typeMapping.read(
        DriftSqlType.string,
        data['${effectivePrefix}genres'],
      ),
      country: attachedDatabase.typeMapping.read(
        DriftSqlType.string,
        data['${effectivePrefix}country'],
      ),
      ratingSource: attachedDatabase.typeMapping.read(
        DriftSqlType.string,
        data['${effectivePrefix}rating_source'],
      ),
      updatedAt: attachedDatabase.typeMapping.read(
        DriftSqlType.dateTime,
        data['${effectivePrefix}updated_at'],
      )!,
    );
  }

  @override
  $StoredMediaItemsTable createAlias(String alias) {
    return $StoredMediaItemsTable(attachedDatabase, alias);
  }
}

class StoredMediaItem extends DataClass implements Insertable<StoredMediaItem> {
  final String id;
  final String providerId;
  final String title;
  final String? originalTitle;
  final String? posterUrl;
  final int? year;
  final double? rating;
  final String mediaType;
  final String? description;
  final String? genres;
  final String? country;
  final String? ratingSource;
  final DateTime updatedAt;
  const StoredMediaItem({
    required this.id,
    required this.providerId,
    required this.title,
    this.originalTitle,
    this.posterUrl,
    this.year,
    this.rating,
    required this.mediaType,
    this.description,
    this.genres,
    this.country,
    this.ratingSource,
    required this.updatedAt,
  });
  @override
  Map<String, Expression> toColumns(bool nullToAbsent) {
    final map = <String, Expression>{};
    map['id'] = Variable<String>(id);
    map['provider_id'] = Variable<String>(providerId);
    map['title'] = Variable<String>(title);
    if (!nullToAbsent || originalTitle != null) {
      map['original_title'] = Variable<String>(originalTitle);
    }
    if (!nullToAbsent || posterUrl != null) {
      map['poster_url'] = Variable<String>(posterUrl);
    }
    if (!nullToAbsent || year != null) {
      map['year'] = Variable<int>(year);
    }
    if (!nullToAbsent || rating != null) {
      map['rating'] = Variable<double>(rating);
    }
    map['media_type'] = Variable<String>(mediaType);
    if (!nullToAbsent || description != null) {
      map['description'] = Variable<String>(description);
    }
    if (!nullToAbsent || genres != null) {
      map['genres'] = Variable<String>(genres);
    }
    if (!nullToAbsent || country != null) {
      map['country'] = Variable<String>(country);
    }
    if (!nullToAbsent || ratingSource != null) {
      map['rating_source'] = Variable<String>(ratingSource);
    }
    map['updated_at'] = Variable<DateTime>(updatedAt);
    return map;
  }

  StoredMediaItemsCompanion toCompanion(bool nullToAbsent) {
    return StoredMediaItemsCompanion(
      id: Value(id),
      providerId: Value(providerId),
      title: Value(title),
      originalTitle: originalTitle == null && nullToAbsent
          ? const Value.absent()
          : Value(originalTitle),
      posterUrl: posterUrl == null && nullToAbsent
          ? const Value.absent()
          : Value(posterUrl),
      year: year == null && nullToAbsent ? const Value.absent() : Value(year),
      rating: rating == null && nullToAbsent
          ? const Value.absent()
          : Value(rating),
      mediaType: Value(mediaType),
      description: description == null && nullToAbsent
          ? const Value.absent()
          : Value(description),
      genres: genres == null && nullToAbsent
          ? const Value.absent()
          : Value(genres),
      country: country == null && nullToAbsent
          ? const Value.absent()
          : Value(country),
      ratingSource: ratingSource == null && nullToAbsent
          ? const Value.absent()
          : Value(ratingSource),
      updatedAt: Value(updatedAt),
    );
  }

  factory StoredMediaItem.fromJson(
    Map<String, dynamic> json, {
    ValueSerializer? serializer,
  }) {
    serializer ??= driftRuntimeOptions.defaultSerializer;
    return StoredMediaItem(
      id: serializer.fromJson<String>(json['id']),
      providerId: serializer.fromJson<String>(json['providerId']),
      title: serializer.fromJson<String>(json['title']),
      originalTitle: serializer.fromJson<String?>(json['originalTitle']),
      posterUrl: serializer.fromJson<String?>(json['posterUrl']),
      year: serializer.fromJson<int?>(json['year']),
      rating: serializer.fromJson<double?>(json['rating']),
      mediaType: serializer.fromJson<String>(json['mediaType']),
      description: serializer.fromJson<String?>(json['description']),
      genres: serializer.fromJson<String?>(json['genres']),
      country: serializer.fromJson<String?>(json['country']),
      ratingSource: serializer.fromJson<String?>(json['ratingSource']),
      updatedAt: serializer.fromJson<DateTime>(json['updatedAt']),
    );
  }
  @override
  Map<String, dynamic> toJson({ValueSerializer? serializer}) {
    serializer ??= driftRuntimeOptions.defaultSerializer;
    return <String, dynamic>{
      'id': serializer.toJson<String>(id),
      'providerId': serializer.toJson<String>(providerId),
      'title': serializer.toJson<String>(title),
      'originalTitle': serializer.toJson<String?>(originalTitle),
      'posterUrl': serializer.toJson<String?>(posterUrl),
      'year': serializer.toJson<int?>(year),
      'rating': serializer.toJson<double?>(rating),
      'mediaType': serializer.toJson<String>(mediaType),
      'description': serializer.toJson<String?>(description),
      'genres': serializer.toJson<String?>(genres),
      'country': serializer.toJson<String?>(country),
      'ratingSource': serializer.toJson<String?>(ratingSource),
      'updatedAt': serializer.toJson<DateTime>(updatedAt),
    };
  }

  StoredMediaItem copyWith({
    String? id,
    String? providerId,
    String? title,
    Value<String?> originalTitle = const Value.absent(),
    Value<String?> posterUrl = const Value.absent(),
    Value<int?> year = const Value.absent(),
    Value<double?> rating = const Value.absent(),
    String? mediaType,
    Value<String?> description = const Value.absent(),
    Value<String?> genres = const Value.absent(),
    Value<String?> country = const Value.absent(),
    Value<String?> ratingSource = const Value.absent(),
    DateTime? updatedAt,
  }) => StoredMediaItem(
    id: id ?? this.id,
    providerId: providerId ?? this.providerId,
    title: title ?? this.title,
    originalTitle: originalTitle.present
        ? originalTitle.value
        : this.originalTitle,
    posterUrl: posterUrl.present ? posterUrl.value : this.posterUrl,
    year: year.present ? year.value : this.year,
    rating: rating.present ? rating.value : this.rating,
    mediaType: mediaType ?? this.mediaType,
    description: description.present ? description.value : this.description,
    genres: genres.present ? genres.value : this.genres,
    country: country.present ? country.value : this.country,
    ratingSource: ratingSource.present ? ratingSource.value : this.ratingSource,
    updatedAt: updatedAt ?? this.updatedAt,
  );
  StoredMediaItem copyWithCompanion(StoredMediaItemsCompanion data) {
    return StoredMediaItem(
      id: data.id.present ? data.id.value : this.id,
      providerId: data.providerId.present
          ? data.providerId.value
          : this.providerId,
      title: data.title.present ? data.title.value : this.title,
      originalTitle: data.originalTitle.present
          ? data.originalTitle.value
          : this.originalTitle,
      posterUrl: data.posterUrl.present ? data.posterUrl.value : this.posterUrl,
      year: data.year.present ? data.year.value : this.year,
      rating: data.rating.present ? data.rating.value : this.rating,
      mediaType: data.mediaType.present ? data.mediaType.value : this.mediaType,
      description: data.description.present
          ? data.description.value
          : this.description,
      genres: data.genres.present ? data.genres.value : this.genres,
      country: data.country.present ? data.country.value : this.country,
      ratingSource: data.ratingSource.present
          ? data.ratingSource.value
          : this.ratingSource,
      updatedAt: data.updatedAt.present ? data.updatedAt.value : this.updatedAt,
    );
  }

  @override
  String toString() {
    return (StringBuffer('StoredMediaItem(')
          ..write('id: $id, ')
          ..write('providerId: $providerId, ')
          ..write('title: $title, ')
          ..write('originalTitle: $originalTitle, ')
          ..write('posterUrl: $posterUrl, ')
          ..write('year: $year, ')
          ..write('rating: $rating, ')
          ..write('mediaType: $mediaType, ')
          ..write('description: $description, ')
          ..write('genres: $genres, ')
          ..write('country: $country, ')
          ..write('ratingSource: $ratingSource, ')
          ..write('updatedAt: $updatedAt')
          ..write(')'))
        .toString();
  }

  @override
  int get hashCode => Object.hash(
    id,
    providerId,
    title,
    originalTitle,
    posterUrl,
    year,
    rating,
    mediaType,
    description,
    genres,
    country,
    ratingSource,
    updatedAt,
  );
  @override
  bool operator ==(Object other) =>
      identical(this, other) ||
      (other is StoredMediaItem &&
          other.id == this.id &&
          other.providerId == this.providerId &&
          other.title == this.title &&
          other.originalTitle == this.originalTitle &&
          other.posterUrl == this.posterUrl &&
          other.year == this.year &&
          other.rating == this.rating &&
          other.mediaType == this.mediaType &&
          other.description == this.description &&
          other.genres == this.genres &&
          other.country == this.country &&
          other.ratingSource == this.ratingSource &&
          other.updatedAt == this.updatedAt);
}

class StoredMediaItemsCompanion extends UpdateCompanion<StoredMediaItem> {
  final Value<String> id;
  final Value<String> providerId;
  final Value<String> title;
  final Value<String?> originalTitle;
  final Value<String?> posterUrl;
  final Value<int?> year;
  final Value<double?> rating;
  final Value<String> mediaType;
  final Value<String?> description;
  final Value<String?> genres;
  final Value<String?> country;
  final Value<String?> ratingSource;
  final Value<DateTime> updatedAt;
  final Value<int> rowid;
  const StoredMediaItemsCompanion({
    this.id = const Value.absent(),
    this.providerId = const Value.absent(),
    this.title = const Value.absent(),
    this.originalTitle = const Value.absent(),
    this.posterUrl = const Value.absent(),
    this.year = const Value.absent(),
    this.rating = const Value.absent(),
    this.mediaType = const Value.absent(),
    this.description = const Value.absent(),
    this.genres = const Value.absent(),
    this.country = const Value.absent(),
    this.ratingSource = const Value.absent(),
    this.updatedAt = const Value.absent(),
    this.rowid = const Value.absent(),
  });
  StoredMediaItemsCompanion.insert({
    required String id,
    required String providerId,
    required String title,
    this.originalTitle = const Value.absent(),
    this.posterUrl = const Value.absent(),
    this.year = const Value.absent(),
    this.rating = const Value.absent(),
    required String mediaType,
    this.description = const Value.absent(),
    this.genres = const Value.absent(),
    this.country = const Value.absent(),
    this.ratingSource = const Value.absent(),
    this.updatedAt = const Value.absent(),
    this.rowid = const Value.absent(),
  }) : id = Value(id),
       providerId = Value(providerId),
       title = Value(title),
       mediaType = Value(mediaType);
  static Insertable<StoredMediaItem> custom({
    Expression<String>? id,
    Expression<String>? providerId,
    Expression<String>? title,
    Expression<String>? originalTitle,
    Expression<String>? posterUrl,
    Expression<int>? year,
    Expression<double>? rating,
    Expression<String>? mediaType,
    Expression<String>? description,
    Expression<String>? genres,
    Expression<String>? country,
    Expression<String>? ratingSource,
    Expression<DateTime>? updatedAt,
    Expression<int>? rowid,
  }) {
    return RawValuesInsertable({
      if (id != null) 'id': id,
      if (providerId != null) 'provider_id': providerId,
      if (title != null) 'title': title,
      if (originalTitle != null) 'original_title': originalTitle,
      if (posterUrl != null) 'poster_url': posterUrl,
      if (year != null) 'year': year,
      if (rating != null) 'rating': rating,
      if (mediaType != null) 'media_type': mediaType,
      if (description != null) 'description': description,
      if (genres != null) 'genres': genres,
      if (country != null) 'country': country,
      if (ratingSource != null) 'rating_source': ratingSource,
      if (updatedAt != null) 'updated_at': updatedAt,
      if (rowid != null) 'rowid': rowid,
    });
  }

  StoredMediaItemsCompanion copyWith({
    Value<String>? id,
    Value<String>? providerId,
    Value<String>? title,
    Value<String?>? originalTitle,
    Value<String?>? posterUrl,
    Value<int?>? year,
    Value<double?>? rating,
    Value<String>? mediaType,
    Value<String?>? description,
    Value<String?>? genres,
    Value<String?>? country,
    Value<String?>? ratingSource,
    Value<DateTime>? updatedAt,
    Value<int>? rowid,
  }) {
    return StoredMediaItemsCompanion(
      id: id ?? this.id,
      providerId: providerId ?? this.providerId,
      title: title ?? this.title,
      originalTitle: originalTitle ?? this.originalTitle,
      posterUrl: posterUrl ?? this.posterUrl,
      year: year ?? this.year,
      rating: rating ?? this.rating,
      mediaType: mediaType ?? this.mediaType,
      description: description ?? this.description,
      genres: genres ?? this.genres,
      country: country ?? this.country,
      ratingSource: ratingSource ?? this.ratingSource,
      updatedAt: updatedAt ?? this.updatedAt,
      rowid: rowid ?? this.rowid,
    );
  }

  @override
  Map<String, Expression> toColumns(bool nullToAbsent) {
    final map = <String, Expression>{};
    if (id.present) {
      map['id'] = Variable<String>(id.value);
    }
    if (providerId.present) {
      map['provider_id'] = Variable<String>(providerId.value);
    }
    if (title.present) {
      map['title'] = Variable<String>(title.value);
    }
    if (originalTitle.present) {
      map['original_title'] = Variable<String>(originalTitle.value);
    }
    if (posterUrl.present) {
      map['poster_url'] = Variable<String>(posterUrl.value);
    }
    if (year.present) {
      map['year'] = Variable<int>(year.value);
    }
    if (rating.present) {
      map['rating'] = Variable<double>(rating.value);
    }
    if (mediaType.present) {
      map['media_type'] = Variable<String>(mediaType.value);
    }
    if (description.present) {
      map['description'] = Variable<String>(description.value);
    }
    if (genres.present) {
      map['genres'] = Variable<String>(genres.value);
    }
    if (country.present) {
      map['country'] = Variable<String>(country.value);
    }
    if (ratingSource.present) {
      map['rating_source'] = Variable<String>(ratingSource.value);
    }
    if (updatedAt.present) {
      map['updated_at'] = Variable<DateTime>(updatedAt.value);
    }
    if (rowid.present) {
      map['rowid'] = Variable<int>(rowid.value);
    }
    return map;
  }

  @override
  String toString() {
    return (StringBuffer('StoredMediaItemsCompanion(')
          ..write('id: $id, ')
          ..write('providerId: $providerId, ')
          ..write('title: $title, ')
          ..write('originalTitle: $originalTitle, ')
          ..write('posterUrl: $posterUrl, ')
          ..write('year: $year, ')
          ..write('rating: $rating, ')
          ..write('mediaType: $mediaType, ')
          ..write('description: $description, ')
          ..write('genres: $genres, ')
          ..write('country: $country, ')
          ..write('ratingSource: $ratingSource, ')
          ..write('updatedAt: $updatedAt, ')
          ..write('rowid: $rowid')
          ..write(')'))
        .toString();
  }
}

abstract class _$AppDatabase extends GeneratedDatabase {
  _$AppDatabase(QueryExecutor e) : super(e);
  $AppDatabaseManager get managers => $AppDatabaseManager(this);
  late final $AppSettingsTable appSettings = $AppSettingsTable(this);
  late final $EnabledProvidersTable enabledProviders = $EnabledProvidersTable(
    this,
  );
  late final $FavoritesTable favorites = $FavoritesTable(this);
  late final $WatchHistoryTable watchHistory = $WatchHistoryTable(this);
  late final $DownloadsTable downloads = $DownloadsTable(this);
  late final $SearchHistoryTableTable searchHistoryTable =
      $SearchHistoryTableTable(this);
  late final $StoredMediaItemsTable storedMediaItems = $StoredMediaItemsTable(
    this,
  );
  @override
  Iterable<TableInfo<Table, Object?>> get allTables =>
      allSchemaEntities.whereType<TableInfo<Table, Object?>>();
  @override
  List<DatabaseSchemaEntity> get allSchemaEntities => [
    appSettings,
    enabledProviders,
    favorites,
    watchHistory,
    downloads,
    searchHistoryTable,
    storedMediaItems,
  ];
}

typedef $$AppSettingsTableCreateCompanionBuilder =
    AppSettingsCompanion Function({
      Value<int> id,
      required String key,
      required String value,
      Value<DateTime> updatedAt,
    });
typedef $$AppSettingsTableUpdateCompanionBuilder =
    AppSettingsCompanion Function({
      Value<int> id,
      Value<String> key,
      Value<String> value,
      Value<DateTime> updatedAt,
    });

class $$AppSettingsTableFilterComposer
    extends Composer<_$AppDatabase, $AppSettingsTable> {
  $$AppSettingsTableFilterComposer({
    required super.$db,
    required super.$table,
    super.joinBuilder,
    super.$addJoinBuilderToRootComposer,
    super.$removeJoinBuilderFromRootComposer,
  });
  ColumnFilters<int> get id => $composableBuilder(
    column: $table.id,
    builder: (column) => ColumnFilters(column),
  );

  ColumnFilters<String> get key => $composableBuilder(
    column: $table.key,
    builder: (column) => ColumnFilters(column),
  );

  ColumnFilters<String> get value => $composableBuilder(
    column: $table.value,
    builder: (column) => ColumnFilters(column),
  );

  ColumnFilters<DateTime> get updatedAt => $composableBuilder(
    column: $table.updatedAt,
    builder: (column) => ColumnFilters(column),
  );
}

class $$AppSettingsTableOrderingComposer
    extends Composer<_$AppDatabase, $AppSettingsTable> {
  $$AppSettingsTableOrderingComposer({
    required super.$db,
    required super.$table,
    super.joinBuilder,
    super.$addJoinBuilderToRootComposer,
    super.$removeJoinBuilderFromRootComposer,
  });
  ColumnOrderings<int> get id => $composableBuilder(
    column: $table.id,
    builder: (column) => ColumnOrderings(column),
  );

  ColumnOrderings<String> get key => $composableBuilder(
    column: $table.key,
    builder: (column) => ColumnOrderings(column),
  );

  ColumnOrderings<String> get value => $composableBuilder(
    column: $table.value,
    builder: (column) => ColumnOrderings(column),
  );

  ColumnOrderings<DateTime> get updatedAt => $composableBuilder(
    column: $table.updatedAt,
    builder: (column) => ColumnOrderings(column),
  );
}

class $$AppSettingsTableAnnotationComposer
    extends Composer<_$AppDatabase, $AppSettingsTable> {
  $$AppSettingsTableAnnotationComposer({
    required super.$db,
    required super.$table,
    super.joinBuilder,
    super.$addJoinBuilderToRootComposer,
    super.$removeJoinBuilderFromRootComposer,
  });
  GeneratedColumn<int> get id =>
      $composableBuilder(column: $table.id, builder: (column) => column);

  GeneratedColumn<String> get key =>
      $composableBuilder(column: $table.key, builder: (column) => column);

  GeneratedColumn<String> get value =>
      $composableBuilder(column: $table.value, builder: (column) => column);

  GeneratedColumn<DateTime> get updatedAt =>
      $composableBuilder(column: $table.updatedAt, builder: (column) => column);
}

class $$AppSettingsTableTableManager
    extends
        RootTableManager<
          _$AppDatabase,
          $AppSettingsTable,
          AppSetting,
          $$AppSettingsTableFilterComposer,
          $$AppSettingsTableOrderingComposer,
          $$AppSettingsTableAnnotationComposer,
          $$AppSettingsTableCreateCompanionBuilder,
          $$AppSettingsTableUpdateCompanionBuilder,
          (
            AppSetting,
            BaseReferences<_$AppDatabase, $AppSettingsTable, AppSetting>,
          ),
          AppSetting,
          PrefetchHooks Function()
        > {
  $$AppSettingsTableTableManager(_$AppDatabase db, $AppSettingsTable table)
    : super(
        TableManagerState(
          db: db,
          table: table,
          createFilteringComposer: () =>
              $$AppSettingsTableFilterComposer($db: db, $table: table),
          createOrderingComposer: () =>
              $$AppSettingsTableOrderingComposer($db: db, $table: table),
          createComputedFieldComposer: () =>
              $$AppSettingsTableAnnotationComposer($db: db, $table: table),
          updateCompanionCallback:
              ({
                Value<int> id = const Value.absent(),
                Value<String> key = const Value.absent(),
                Value<String> value = const Value.absent(),
                Value<DateTime> updatedAt = const Value.absent(),
              }) => AppSettingsCompanion(
                id: id,
                key: key,
                value: value,
                updatedAt: updatedAt,
              ),
          createCompanionCallback:
              ({
                Value<int> id = const Value.absent(),
                required String key,
                required String value,
                Value<DateTime> updatedAt = const Value.absent(),
              }) => AppSettingsCompanion.insert(
                id: id,
                key: key,
                value: value,
                updatedAt: updatedAt,
              ),
          withReferenceMapper: (p0) => p0
              .map((e) => (e.readTable(table), BaseReferences(db, table, e)))
              .toList(),
          prefetchHooksCallback: null,
        ),
      );
}

typedef $$AppSettingsTableProcessedTableManager =
    ProcessedTableManager<
      _$AppDatabase,
      $AppSettingsTable,
      AppSetting,
      $$AppSettingsTableFilterComposer,
      $$AppSettingsTableOrderingComposer,
      $$AppSettingsTableAnnotationComposer,
      $$AppSettingsTableCreateCompanionBuilder,
      $$AppSettingsTableUpdateCompanionBuilder,
      (
        AppSetting,
        BaseReferences<_$AppDatabase, $AppSettingsTable, AppSetting>,
      ),
      AppSetting,
      PrefetchHooks Function()
    >;
typedef $$EnabledProvidersTableCreateCompanionBuilder =
    EnabledProvidersCompanion Function({
      Value<int> id,
      required String providerId,
      Value<bool> isEnabled,
      Value<int> priority,
      Value<DateTime> updatedAt,
    });
typedef $$EnabledProvidersTableUpdateCompanionBuilder =
    EnabledProvidersCompanion Function({
      Value<int> id,
      Value<String> providerId,
      Value<bool> isEnabled,
      Value<int> priority,
      Value<DateTime> updatedAt,
    });

class $$EnabledProvidersTableFilterComposer
    extends Composer<_$AppDatabase, $EnabledProvidersTable> {
  $$EnabledProvidersTableFilterComposer({
    required super.$db,
    required super.$table,
    super.joinBuilder,
    super.$addJoinBuilderToRootComposer,
    super.$removeJoinBuilderFromRootComposer,
  });
  ColumnFilters<int> get id => $composableBuilder(
    column: $table.id,
    builder: (column) => ColumnFilters(column),
  );

  ColumnFilters<String> get providerId => $composableBuilder(
    column: $table.providerId,
    builder: (column) => ColumnFilters(column),
  );

  ColumnFilters<bool> get isEnabled => $composableBuilder(
    column: $table.isEnabled,
    builder: (column) => ColumnFilters(column),
  );

  ColumnFilters<int> get priority => $composableBuilder(
    column: $table.priority,
    builder: (column) => ColumnFilters(column),
  );

  ColumnFilters<DateTime> get updatedAt => $composableBuilder(
    column: $table.updatedAt,
    builder: (column) => ColumnFilters(column),
  );
}

class $$EnabledProvidersTableOrderingComposer
    extends Composer<_$AppDatabase, $EnabledProvidersTable> {
  $$EnabledProvidersTableOrderingComposer({
    required super.$db,
    required super.$table,
    super.joinBuilder,
    super.$addJoinBuilderToRootComposer,
    super.$removeJoinBuilderFromRootComposer,
  });
  ColumnOrderings<int> get id => $composableBuilder(
    column: $table.id,
    builder: (column) => ColumnOrderings(column),
  );

  ColumnOrderings<String> get providerId => $composableBuilder(
    column: $table.providerId,
    builder: (column) => ColumnOrderings(column),
  );

  ColumnOrderings<bool> get isEnabled => $composableBuilder(
    column: $table.isEnabled,
    builder: (column) => ColumnOrderings(column),
  );

  ColumnOrderings<int> get priority => $composableBuilder(
    column: $table.priority,
    builder: (column) => ColumnOrderings(column),
  );

  ColumnOrderings<DateTime> get updatedAt => $composableBuilder(
    column: $table.updatedAt,
    builder: (column) => ColumnOrderings(column),
  );
}

class $$EnabledProvidersTableAnnotationComposer
    extends Composer<_$AppDatabase, $EnabledProvidersTable> {
  $$EnabledProvidersTableAnnotationComposer({
    required super.$db,
    required super.$table,
    super.joinBuilder,
    super.$addJoinBuilderToRootComposer,
    super.$removeJoinBuilderFromRootComposer,
  });
  GeneratedColumn<int> get id =>
      $composableBuilder(column: $table.id, builder: (column) => column);

  GeneratedColumn<String> get providerId => $composableBuilder(
    column: $table.providerId,
    builder: (column) => column,
  );

  GeneratedColumn<bool> get isEnabled =>
      $composableBuilder(column: $table.isEnabled, builder: (column) => column);

  GeneratedColumn<int> get priority =>
      $composableBuilder(column: $table.priority, builder: (column) => column);

  GeneratedColumn<DateTime> get updatedAt =>
      $composableBuilder(column: $table.updatedAt, builder: (column) => column);
}

class $$EnabledProvidersTableTableManager
    extends
        RootTableManager<
          _$AppDatabase,
          $EnabledProvidersTable,
          EnabledProvider,
          $$EnabledProvidersTableFilterComposer,
          $$EnabledProvidersTableOrderingComposer,
          $$EnabledProvidersTableAnnotationComposer,
          $$EnabledProvidersTableCreateCompanionBuilder,
          $$EnabledProvidersTableUpdateCompanionBuilder,
          (
            EnabledProvider,
            BaseReferences<
              _$AppDatabase,
              $EnabledProvidersTable,
              EnabledProvider
            >,
          ),
          EnabledProvider,
          PrefetchHooks Function()
        > {
  $$EnabledProvidersTableTableManager(
    _$AppDatabase db,
    $EnabledProvidersTable table,
  ) : super(
        TableManagerState(
          db: db,
          table: table,
          createFilteringComposer: () =>
              $$EnabledProvidersTableFilterComposer($db: db, $table: table),
          createOrderingComposer: () =>
              $$EnabledProvidersTableOrderingComposer($db: db, $table: table),
          createComputedFieldComposer: () =>
              $$EnabledProvidersTableAnnotationComposer($db: db, $table: table),
          updateCompanionCallback:
              ({
                Value<int> id = const Value.absent(),
                Value<String> providerId = const Value.absent(),
                Value<bool> isEnabled = const Value.absent(),
                Value<int> priority = const Value.absent(),
                Value<DateTime> updatedAt = const Value.absent(),
              }) => EnabledProvidersCompanion(
                id: id,
                providerId: providerId,
                isEnabled: isEnabled,
                priority: priority,
                updatedAt: updatedAt,
              ),
          createCompanionCallback:
              ({
                Value<int> id = const Value.absent(),
                required String providerId,
                Value<bool> isEnabled = const Value.absent(),
                Value<int> priority = const Value.absent(),
                Value<DateTime> updatedAt = const Value.absent(),
              }) => EnabledProvidersCompanion.insert(
                id: id,
                providerId: providerId,
                isEnabled: isEnabled,
                priority: priority,
                updatedAt: updatedAt,
              ),
          withReferenceMapper: (p0) => p0
              .map((e) => (e.readTable(table), BaseReferences(db, table, e)))
              .toList(),
          prefetchHooksCallback: null,
        ),
      );
}

typedef $$EnabledProvidersTableProcessedTableManager =
    ProcessedTableManager<
      _$AppDatabase,
      $EnabledProvidersTable,
      EnabledProvider,
      $$EnabledProvidersTableFilterComposer,
      $$EnabledProvidersTableOrderingComposer,
      $$EnabledProvidersTableAnnotationComposer,
      $$EnabledProvidersTableCreateCompanionBuilder,
      $$EnabledProvidersTableUpdateCompanionBuilder,
      (
        EnabledProvider,
        BaseReferences<_$AppDatabase, $EnabledProvidersTable, EnabledProvider>,
      ),
      EnabledProvider,
      PrefetchHooks Function()
    >;
typedef $$FavoritesTableCreateCompanionBuilder =
    FavoritesCompanion Function({
      Value<int> id,
      required String mediaId,
      required String providerId,
      required String title,
      Value<String?> posterUrl,
      Value<int?> year,
      Value<double?> rating,
      Value<String?> ratingSource,
      required String mediaType,
      Value<DateTime> addedAt,
    });
typedef $$FavoritesTableUpdateCompanionBuilder =
    FavoritesCompanion Function({
      Value<int> id,
      Value<String> mediaId,
      Value<String> providerId,
      Value<String> title,
      Value<String?> posterUrl,
      Value<int?> year,
      Value<double?> rating,
      Value<String?> ratingSource,
      Value<String> mediaType,
      Value<DateTime> addedAt,
    });

class $$FavoritesTableFilterComposer
    extends Composer<_$AppDatabase, $FavoritesTable> {
  $$FavoritesTableFilterComposer({
    required super.$db,
    required super.$table,
    super.joinBuilder,
    super.$addJoinBuilderToRootComposer,
    super.$removeJoinBuilderFromRootComposer,
  });
  ColumnFilters<int> get id => $composableBuilder(
    column: $table.id,
    builder: (column) => ColumnFilters(column),
  );

  ColumnFilters<String> get mediaId => $composableBuilder(
    column: $table.mediaId,
    builder: (column) => ColumnFilters(column),
  );

  ColumnFilters<String> get providerId => $composableBuilder(
    column: $table.providerId,
    builder: (column) => ColumnFilters(column),
  );

  ColumnFilters<String> get title => $composableBuilder(
    column: $table.title,
    builder: (column) => ColumnFilters(column),
  );

  ColumnFilters<String> get posterUrl => $composableBuilder(
    column: $table.posterUrl,
    builder: (column) => ColumnFilters(column),
  );

  ColumnFilters<int> get year => $composableBuilder(
    column: $table.year,
    builder: (column) => ColumnFilters(column),
  );

  ColumnFilters<double> get rating => $composableBuilder(
    column: $table.rating,
    builder: (column) => ColumnFilters(column),
  );

  ColumnFilters<String> get ratingSource => $composableBuilder(
    column: $table.ratingSource,
    builder: (column) => ColumnFilters(column),
  );

  ColumnFilters<String> get mediaType => $composableBuilder(
    column: $table.mediaType,
    builder: (column) => ColumnFilters(column),
  );

  ColumnFilters<DateTime> get addedAt => $composableBuilder(
    column: $table.addedAt,
    builder: (column) => ColumnFilters(column),
  );
}

class $$FavoritesTableOrderingComposer
    extends Composer<_$AppDatabase, $FavoritesTable> {
  $$FavoritesTableOrderingComposer({
    required super.$db,
    required super.$table,
    super.joinBuilder,
    super.$addJoinBuilderToRootComposer,
    super.$removeJoinBuilderFromRootComposer,
  });
  ColumnOrderings<int> get id => $composableBuilder(
    column: $table.id,
    builder: (column) => ColumnOrderings(column),
  );

  ColumnOrderings<String> get mediaId => $composableBuilder(
    column: $table.mediaId,
    builder: (column) => ColumnOrderings(column),
  );

  ColumnOrderings<String> get providerId => $composableBuilder(
    column: $table.providerId,
    builder: (column) => ColumnOrderings(column),
  );

  ColumnOrderings<String> get title => $composableBuilder(
    column: $table.title,
    builder: (column) => ColumnOrderings(column),
  );

  ColumnOrderings<String> get posterUrl => $composableBuilder(
    column: $table.posterUrl,
    builder: (column) => ColumnOrderings(column),
  );

  ColumnOrderings<int> get year => $composableBuilder(
    column: $table.year,
    builder: (column) => ColumnOrderings(column),
  );

  ColumnOrderings<double> get rating => $composableBuilder(
    column: $table.rating,
    builder: (column) => ColumnOrderings(column),
  );

  ColumnOrderings<String> get ratingSource => $composableBuilder(
    column: $table.ratingSource,
    builder: (column) => ColumnOrderings(column),
  );

  ColumnOrderings<String> get mediaType => $composableBuilder(
    column: $table.mediaType,
    builder: (column) => ColumnOrderings(column),
  );

  ColumnOrderings<DateTime> get addedAt => $composableBuilder(
    column: $table.addedAt,
    builder: (column) => ColumnOrderings(column),
  );
}

class $$FavoritesTableAnnotationComposer
    extends Composer<_$AppDatabase, $FavoritesTable> {
  $$FavoritesTableAnnotationComposer({
    required super.$db,
    required super.$table,
    super.joinBuilder,
    super.$addJoinBuilderToRootComposer,
    super.$removeJoinBuilderFromRootComposer,
  });
  GeneratedColumn<int> get id =>
      $composableBuilder(column: $table.id, builder: (column) => column);

  GeneratedColumn<String> get mediaId =>
      $composableBuilder(column: $table.mediaId, builder: (column) => column);

  GeneratedColumn<String> get providerId => $composableBuilder(
    column: $table.providerId,
    builder: (column) => column,
  );

  GeneratedColumn<String> get title =>
      $composableBuilder(column: $table.title, builder: (column) => column);

  GeneratedColumn<String> get posterUrl =>
      $composableBuilder(column: $table.posterUrl, builder: (column) => column);

  GeneratedColumn<int> get year =>
      $composableBuilder(column: $table.year, builder: (column) => column);

  GeneratedColumn<double> get rating =>
      $composableBuilder(column: $table.rating, builder: (column) => column);

  GeneratedColumn<String> get ratingSource => $composableBuilder(
    column: $table.ratingSource,
    builder: (column) => column,
  );

  GeneratedColumn<String> get mediaType =>
      $composableBuilder(column: $table.mediaType, builder: (column) => column);

  GeneratedColumn<DateTime> get addedAt =>
      $composableBuilder(column: $table.addedAt, builder: (column) => column);
}

class $$FavoritesTableTableManager
    extends
        RootTableManager<
          _$AppDatabase,
          $FavoritesTable,
          Favorite,
          $$FavoritesTableFilterComposer,
          $$FavoritesTableOrderingComposer,
          $$FavoritesTableAnnotationComposer,
          $$FavoritesTableCreateCompanionBuilder,
          $$FavoritesTableUpdateCompanionBuilder,
          (Favorite, BaseReferences<_$AppDatabase, $FavoritesTable, Favorite>),
          Favorite,
          PrefetchHooks Function()
        > {
  $$FavoritesTableTableManager(_$AppDatabase db, $FavoritesTable table)
    : super(
        TableManagerState(
          db: db,
          table: table,
          createFilteringComposer: () =>
              $$FavoritesTableFilterComposer($db: db, $table: table),
          createOrderingComposer: () =>
              $$FavoritesTableOrderingComposer($db: db, $table: table),
          createComputedFieldComposer: () =>
              $$FavoritesTableAnnotationComposer($db: db, $table: table),
          updateCompanionCallback:
              ({
                Value<int> id = const Value.absent(),
                Value<String> mediaId = const Value.absent(),
                Value<String> providerId = const Value.absent(),
                Value<String> title = const Value.absent(),
                Value<String?> posterUrl = const Value.absent(),
                Value<int?> year = const Value.absent(),
                Value<double?> rating = const Value.absent(),
                Value<String?> ratingSource = const Value.absent(),
                Value<String> mediaType = const Value.absent(),
                Value<DateTime> addedAt = const Value.absent(),
              }) => FavoritesCompanion(
                id: id,
                mediaId: mediaId,
                providerId: providerId,
                title: title,
                posterUrl: posterUrl,
                year: year,
                rating: rating,
                ratingSource: ratingSource,
                mediaType: mediaType,
                addedAt: addedAt,
              ),
          createCompanionCallback:
              ({
                Value<int> id = const Value.absent(),
                required String mediaId,
                required String providerId,
                required String title,
                Value<String?> posterUrl = const Value.absent(),
                Value<int?> year = const Value.absent(),
                Value<double?> rating = const Value.absent(),
                Value<String?> ratingSource = const Value.absent(),
                required String mediaType,
                Value<DateTime> addedAt = const Value.absent(),
              }) => FavoritesCompanion.insert(
                id: id,
                mediaId: mediaId,
                providerId: providerId,
                title: title,
                posterUrl: posterUrl,
                year: year,
                rating: rating,
                ratingSource: ratingSource,
                mediaType: mediaType,
                addedAt: addedAt,
              ),
          withReferenceMapper: (p0) => p0
              .map((e) => (e.readTable(table), BaseReferences(db, table, e)))
              .toList(),
          prefetchHooksCallback: null,
        ),
      );
}

typedef $$FavoritesTableProcessedTableManager =
    ProcessedTableManager<
      _$AppDatabase,
      $FavoritesTable,
      Favorite,
      $$FavoritesTableFilterComposer,
      $$FavoritesTableOrderingComposer,
      $$FavoritesTableAnnotationComposer,
      $$FavoritesTableCreateCompanionBuilder,
      $$FavoritesTableUpdateCompanionBuilder,
      (Favorite, BaseReferences<_$AppDatabase, $FavoritesTable, Favorite>),
      Favorite,
      PrefetchHooks Function()
    >;
typedef $$WatchHistoryTableCreateCompanionBuilder =
    WatchHistoryCompanion Function({
      Value<int> id,
      required String mediaId,
      required String providerId,
      required String title,
      Value<String?> posterUrl,
      Value<int?> year,
      Value<double?> rating,
      Value<String?> ratingSource,
      required String mediaType,
      Value<int> positionMs,
      Value<int> durationMs,
      Value<int?> season,
      Value<int?> episode,
      Value<String?> episodeTitle,
      Value<String?> lastStreamUrl,
      Value<String?> voiceover,
      Value<DateTime> watchedAt,
    });
typedef $$WatchHistoryTableUpdateCompanionBuilder =
    WatchHistoryCompanion Function({
      Value<int> id,
      Value<String> mediaId,
      Value<String> providerId,
      Value<String> title,
      Value<String?> posterUrl,
      Value<int?> year,
      Value<double?> rating,
      Value<String?> ratingSource,
      Value<String> mediaType,
      Value<int> positionMs,
      Value<int> durationMs,
      Value<int?> season,
      Value<int?> episode,
      Value<String?> episodeTitle,
      Value<String?> lastStreamUrl,
      Value<String?> voiceover,
      Value<DateTime> watchedAt,
    });

class $$WatchHistoryTableFilterComposer
    extends Composer<_$AppDatabase, $WatchHistoryTable> {
  $$WatchHistoryTableFilterComposer({
    required super.$db,
    required super.$table,
    super.joinBuilder,
    super.$addJoinBuilderToRootComposer,
    super.$removeJoinBuilderFromRootComposer,
  });
  ColumnFilters<int> get id => $composableBuilder(
    column: $table.id,
    builder: (column) => ColumnFilters(column),
  );

  ColumnFilters<String> get mediaId => $composableBuilder(
    column: $table.mediaId,
    builder: (column) => ColumnFilters(column),
  );

  ColumnFilters<String> get providerId => $composableBuilder(
    column: $table.providerId,
    builder: (column) => ColumnFilters(column),
  );

  ColumnFilters<String> get title => $composableBuilder(
    column: $table.title,
    builder: (column) => ColumnFilters(column),
  );

  ColumnFilters<String> get posterUrl => $composableBuilder(
    column: $table.posterUrl,
    builder: (column) => ColumnFilters(column),
  );

  ColumnFilters<int> get year => $composableBuilder(
    column: $table.year,
    builder: (column) => ColumnFilters(column),
  );

  ColumnFilters<double> get rating => $composableBuilder(
    column: $table.rating,
    builder: (column) => ColumnFilters(column),
  );

  ColumnFilters<String> get ratingSource => $composableBuilder(
    column: $table.ratingSource,
    builder: (column) => ColumnFilters(column),
  );

  ColumnFilters<String> get mediaType => $composableBuilder(
    column: $table.mediaType,
    builder: (column) => ColumnFilters(column),
  );

  ColumnFilters<int> get positionMs => $composableBuilder(
    column: $table.positionMs,
    builder: (column) => ColumnFilters(column),
  );

  ColumnFilters<int> get durationMs => $composableBuilder(
    column: $table.durationMs,
    builder: (column) => ColumnFilters(column),
  );

  ColumnFilters<int> get season => $composableBuilder(
    column: $table.season,
    builder: (column) => ColumnFilters(column),
  );

  ColumnFilters<int> get episode => $composableBuilder(
    column: $table.episode,
    builder: (column) => ColumnFilters(column),
  );

  ColumnFilters<String> get episodeTitle => $composableBuilder(
    column: $table.episodeTitle,
    builder: (column) => ColumnFilters(column),
  );

  ColumnFilters<String> get lastStreamUrl => $composableBuilder(
    column: $table.lastStreamUrl,
    builder: (column) => ColumnFilters(column),
  );

  ColumnFilters<String> get voiceover => $composableBuilder(
    column: $table.voiceover,
    builder: (column) => ColumnFilters(column),
  );

  ColumnFilters<DateTime> get watchedAt => $composableBuilder(
    column: $table.watchedAt,
    builder: (column) => ColumnFilters(column),
  );
}

class $$WatchHistoryTableOrderingComposer
    extends Composer<_$AppDatabase, $WatchHistoryTable> {
  $$WatchHistoryTableOrderingComposer({
    required super.$db,
    required super.$table,
    super.joinBuilder,
    super.$addJoinBuilderToRootComposer,
    super.$removeJoinBuilderFromRootComposer,
  });
  ColumnOrderings<int> get id => $composableBuilder(
    column: $table.id,
    builder: (column) => ColumnOrderings(column),
  );

  ColumnOrderings<String> get mediaId => $composableBuilder(
    column: $table.mediaId,
    builder: (column) => ColumnOrderings(column),
  );

  ColumnOrderings<String> get providerId => $composableBuilder(
    column: $table.providerId,
    builder: (column) => ColumnOrderings(column),
  );

  ColumnOrderings<String> get title => $composableBuilder(
    column: $table.title,
    builder: (column) => ColumnOrderings(column),
  );

  ColumnOrderings<String> get posterUrl => $composableBuilder(
    column: $table.posterUrl,
    builder: (column) => ColumnOrderings(column),
  );

  ColumnOrderings<int> get year => $composableBuilder(
    column: $table.year,
    builder: (column) => ColumnOrderings(column),
  );

  ColumnOrderings<double> get rating => $composableBuilder(
    column: $table.rating,
    builder: (column) => ColumnOrderings(column),
  );

  ColumnOrderings<String> get ratingSource => $composableBuilder(
    column: $table.ratingSource,
    builder: (column) => ColumnOrderings(column),
  );

  ColumnOrderings<String> get mediaType => $composableBuilder(
    column: $table.mediaType,
    builder: (column) => ColumnOrderings(column),
  );

  ColumnOrderings<int> get positionMs => $composableBuilder(
    column: $table.positionMs,
    builder: (column) => ColumnOrderings(column),
  );

  ColumnOrderings<int> get durationMs => $composableBuilder(
    column: $table.durationMs,
    builder: (column) => ColumnOrderings(column),
  );

  ColumnOrderings<int> get season => $composableBuilder(
    column: $table.season,
    builder: (column) => ColumnOrderings(column),
  );

  ColumnOrderings<int> get episode => $composableBuilder(
    column: $table.episode,
    builder: (column) => ColumnOrderings(column),
  );

  ColumnOrderings<String> get episodeTitle => $composableBuilder(
    column: $table.episodeTitle,
    builder: (column) => ColumnOrderings(column),
  );

  ColumnOrderings<String> get lastStreamUrl => $composableBuilder(
    column: $table.lastStreamUrl,
    builder: (column) => ColumnOrderings(column),
  );

  ColumnOrderings<String> get voiceover => $composableBuilder(
    column: $table.voiceover,
    builder: (column) => ColumnOrderings(column),
  );

  ColumnOrderings<DateTime> get watchedAt => $composableBuilder(
    column: $table.watchedAt,
    builder: (column) => ColumnOrderings(column),
  );
}

class $$WatchHistoryTableAnnotationComposer
    extends Composer<_$AppDatabase, $WatchHistoryTable> {
  $$WatchHistoryTableAnnotationComposer({
    required super.$db,
    required super.$table,
    super.joinBuilder,
    super.$addJoinBuilderToRootComposer,
    super.$removeJoinBuilderFromRootComposer,
  });
  GeneratedColumn<int> get id =>
      $composableBuilder(column: $table.id, builder: (column) => column);

  GeneratedColumn<String> get mediaId =>
      $composableBuilder(column: $table.mediaId, builder: (column) => column);

  GeneratedColumn<String> get providerId => $composableBuilder(
    column: $table.providerId,
    builder: (column) => column,
  );

  GeneratedColumn<String> get title =>
      $composableBuilder(column: $table.title, builder: (column) => column);

  GeneratedColumn<String> get posterUrl =>
      $composableBuilder(column: $table.posterUrl, builder: (column) => column);

  GeneratedColumn<int> get year =>
      $composableBuilder(column: $table.year, builder: (column) => column);

  GeneratedColumn<double> get rating =>
      $composableBuilder(column: $table.rating, builder: (column) => column);

  GeneratedColumn<String> get ratingSource => $composableBuilder(
    column: $table.ratingSource,
    builder: (column) => column,
  );

  GeneratedColumn<String> get mediaType =>
      $composableBuilder(column: $table.mediaType, builder: (column) => column);

  GeneratedColumn<int> get positionMs => $composableBuilder(
    column: $table.positionMs,
    builder: (column) => column,
  );

  GeneratedColumn<int> get durationMs => $composableBuilder(
    column: $table.durationMs,
    builder: (column) => column,
  );

  GeneratedColumn<int> get season =>
      $composableBuilder(column: $table.season, builder: (column) => column);

  GeneratedColumn<int> get episode =>
      $composableBuilder(column: $table.episode, builder: (column) => column);

  GeneratedColumn<String> get episodeTitle => $composableBuilder(
    column: $table.episodeTitle,
    builder: (column) => column,
  );

  GeneratedColumn<String> get lastStreamUrl => $composableBuilder(
    column: $table.lastStreamUrl,
    builder: (column) => column,
  );

  GeneratedColumn<String> get voiceover =>
      $composableBuilder(column: $table.voiceover, builder: (column) => column);

  GeneratedColumn<DateTime> get watchedAt =>
      $composableBuilder(column: $table.watchedAt, builder: (column) => column);
}

class $$WatchHistoryTableTableManager
    extends
        RootTableManager<
          _$AppDatabase,
          $WatchHistoryTable,
          WatchHistoryData,
          $$WatchHistoryTableFilterComposer,
          $$WatchHistoryTableOrderingComposer,
          $$WatchHistoryTableAnnotationComposer,
          $$WatchHistoryTableCreateCompanionBuilder,
          $$WatchHistoryTableUpdateCompanionBuilder,
          (
            WatchHistoryData,
            BaseReferences<_$AppDatabase, $WatchHistoryTable, WatchHistoryData>,
          ),
          WatchHistoryData,
          PrefetchHooks Function()
        > {
  $$WatchHistoryTableTableManager(_$AppDatabase db, $WatchHistoryTable table)
    : super(
        TableManagerState(
          db: db,
          table: table,
          createFilteringComposer: () =>
              $$WatchHistoryTableFilterComposer($db: db, $table: table),
          createOrderingComposer: () =>
              $$WatchHistoryTableOrderingComposer($db: db, $table: table),
          createComputedFieldComposer: () =>
              $$WatchHistoryTableAnnotationComposer($db: db, $table: table),
          updateCompanionCallback:
              ({
                Value<int> id = const Value.absent(),
                Value<String> mediaId = const Value.absent(),
                Value<String> providerId = const Value.absent(),
                Value<String> title = const Value.absent(),
                Value<String?> posterUrl = const Value.absent(),
                Value<int?> year = const Value.absent(),
                Value<double?> rating = const Value.absent(),
                Value<String?> ratingSource = const Value.absent(),
                Value<String> mediaType = const Value.absent(),
                Value<int> positionMs = const Value.absent(),
                Value<int> durationMs = const Value.absent(),
                Value<int?> season = const Value.absent(),
                Value<int?> episode = const Value.absent(),
                Value<String?> episodeTitle = const Value.absent(),
                Value<String?> lastStreamUrl = const Value.absent(),
                Value<String?> voiceover = const Value.absent(),
                Value<DateTime> watchedAt = const Value.absent(),
              }) => WatchHistoryCompanion(
                id: id,
                mediaId: mediaId,
                providerId: providerId,
                title: title,
                posterUrl: posterUrl,
                year: year,
                rating: rating,
                ratingSource: ratingSource,
                mediaType: mediaType,
                positionMs: positionMs,
                durationMs: durationMs,
                season: season,
                episode: episode,
                episodeTitle: episodeTitle,
                lastStreamUrl: lastStreamUrl,
                voiceover: voiceover,
                watchedAt: watchedAt,
              ),
          createCompanionCallback:
              ({
                Value<int> id = const Value.absent(),
                required String mediaId,
                required String providerId,
                required String title,
                Value<String?> posterUrl = const Value.absent(),
                Value<int?> year = const Value.absent(),
                Value<double?> rating = const Value.absent(),
                Value<String?> ratingSource = const Value.absent(),
                required String mediaType,
                Value<int> positionMs = const Value.absent(),
                Value<int> durationMs = const Value.absent(),
                Value<int?> season = const Value.absent(),
                Value<int?> episode = const Value.absent(),
                Value<String?> episodeTitle = const Value.absent(),
                Value<String?> lastStreamUrl = const Value.absent(),
                Value<String?> voiceover = const Value.absent(),
                Value<DateTime> watchedAt = const Value.absent(),
              }) => WatchHistoryCompanion.insert(
                id: id,
                mediaId: mediaId,
                providerId: providerId,
                title: title,
                posterUrl: posterUrl,
                year: year,
                rating: rating,
                ratingSource: ratingSource,
                mediaType: mediaType,
                positionMs: positionMs,
                durationMs: durationMs,
                season: season,
                episode: episode,
                episodeTitle: episodeTitle,
                lastStreamUrl: lastStreamUrl,
                voiceover: voiceover,
                watchedAt: watchedAt,
              ),
          withReferenceMapper: (p0) => p0
              .map((e) => (e.readTable(table), BaseReferences(db, table, e)))
              .toList(),
          prefetchHooksCallback: null,
        ),
      );
}

typedef $$WatchHistoryTableProcessedTableManager =
    ProcessedTableManager<
      _$AppDatabase,
      $WatchHistoryTable,
      WatchHistoryData,
      $$WatchHistoryTableFilterComposer,
      $$WatchHistoryTableOrderingComposer,
      $$WatchHistoryTableAnnotationComposer,
      $$WatchHistoryTableCreateCompanionBuilder,
      $$WatchHistoryTableUpdateCompanionBuilder,
      (
        WatchHistoryData,
        BaseReferences<_$AppDatabase, $WatchHistoryTable, WatchHistoryData>,
      ),
      WatchHistoryData,
      PrefetchHooks Function()
    >;
typedef $$DownloadsTableCreateCompanionBuilder =
    DownloadsCompanion Function({
      Value<int> id,
      required String mediaId,
      required String providerId,
      required String title,
      Value<String?> posterUrl,
      Value<int?> year,
      Value<double?> rating,
      Value<String?> ratingSource,
      required String mediaType,
      Value<int?> season,
      Value<int?> episode,
      Value<String?> episodeTitle,
      required String streamUrl,
      required String localPath,
      required String quality,
      Value<String?> voiceover,
      Value<DownloadStatus> status,
      Value<double> progress,
      Value<int> fileSizeBytes,
      Value<int> downloadedBytes,
      Value<String?> headers,
      Value<String?> localPosterPath,
      Value<int?> duration,
      Value<DateTime> createdAt,
      Value<DateTime?> completedAt,
    });
typedef $$DownloadsTableUpdateCompanionBuilder =
    DownloadsCompanion Function({
      Value<int> id,
      Value<String> mediaId,
      Value<String> providerId,
      Value<String> title,
      Value<String?> posterUrl,
      Value<int?> year,
      Value<double?> rating,
      Value<String?> ratingSource,
      Value<String> mediaType,
      Value<int?> season,
      Value<int?> episode,
      Value<String?> episodeTitle,
      Value<String> streamUrl,
      Value<String> localPath,
      Value<String> quality,
      Value<String?> voiceover,
      Value<DownloadStatus> status,
      Value<double> progress,
      Value<int> fileSizeBytes,
      Value<int> downloadedBytes,
      Value<String?> headers,
      Value<String?> localPosterPath,
      Value<int?> duration,
      Value<DateTime> createdAt,
      Value<DateTime?> completedAt,
    });

class $$DownloadsTableFilterComposer
    extends Composer<_$AppDatabase, $DownloadsTable> {
  $$DownloadsTableFilterComposer({
    required super.$db,
    required super.$table,
    super.joinBuilder,
    super.$addJoinBuilderToRootComposer,
    super.$removeJoinBuilderFromRootComposer,
  });
  ColumnFilters<int> get id => $composableBuilder(
    column: $table.id,
    builder: (column) => ColumnFilters(column),
  );

  ColumnFilters<String> get mediaId => $composableBuilder(
    column: $table.mediaId,
    builder: (column) => ColumnFilters(column),
  );

  ColumnFilters<String> get providerId => $composableBuilder(
    column: $table.providerId,
    builder: (column) => ColumnFilters(column),
  );

  ColumnFilters<String> get title => $composableBuilder(
    column: $table.title,
    builder: (column) => ColumnFilters(column),
  );

  ColumnFilters<String> get posterUrl => $composableBuilder(
    column: $table.posterUrl,
    builder: (column) => ColumnFilters(column),
  );

  ColumnFilters<int> get year => $composableBuilder(
    column: $table.year,
    builder: (column) => ColumnFilters(column),
  );

  ColumnFilters<double> get rating => $composableBuilder(
    column: $table.rating,
    builder: (column) => ColumnFilters(column),
  );

  ColumnFilters<String> get ratingSource => $composableBuilder(
    column: $table.ratingSource,
    builder: (column) => ColumnFilters(column),
  );

  ColumnFilters<String> get mediaType => $composableBuilder(
    column: $table.mediaType,
    builder: (column) => ColumnFilters(column),
  );

  ColumnFilters<int> get season => $composableBuilder(
    column: $table.season,
    builder: (column) => ColumnFilters(column),
  );

  ColumnFilters<int> get episode => $composableBuilder(
    column: $table.episode,
    builder: (column) => ColumnFilters(column),
  );

  ColumnFilters<String> get episodeTitle => $composableBuilder(
    column: $table.episodeTitle,
    builder: (column) => ColumnFilters(column),
  );

  ColumnFilters<String> get streamUrl => $composableBuilder(
    column: $table.streamUrl,
    builder: (column) => ColumnFilters(column),
  );

  ColumnFilters<String> get localPath => $composableBuilder(
    column: $table.localPath,
    builder: (column) => ColumnFilters(column),
  );

  ColumnFilters<String> get quality => $composableBuilder(
    column: $table.quality,
    builder: (column) => ColumnFilters(column),
  );

  ColumnFilters<String> get voiceover => $composableBuilder(
    column: $table.voiceover,
    builder: (column) => ColumnFilters(column),
  );

  ColumnWithTypeConverterFilters<DownloadStatus, DownloadStatus, int>
  get status => $composableBuilder(
    column: $table.status,
    builder: (column) => ColumnWithTypeConverterFilters(column),
  );

  ColumnFilters<double> get progress => $composableBuilder(
    column: $table.progress,
    builder: (column) => ColumnFilters(column),
  );

  ColumnFilters<int> get fileSizeBytes => $composableBuilder(
    column: $table.fileSizeBytes,
    builder: (column) => ColumnFilters(column),
  );

  ColumnFilters<int> get downloadedBytes => $composableBuilder(
    column: $table.downloadedBytes,
    builder: (column) => ColumnFilters(column),
  );

  ColumnFilters<String> get headers => $composableBuilder(
    column: $table.headers,
    builder: (column) => ColumnFilters(column),
  );

  ColumnFilters<String> get localPosterPath => $composableBuilder(
    column: $table.localPosterPath,
    builder: (column) => ColumnFilters(column),
  );

  ColumnFilters<int> get duration => $composableBuilder(
    column: $table.duration,
    builder: (column) => ColumnFilters(column),
  );

  ColumnFilters<DateTime> get createdAt => $composableBuilder(
    column: $table.createdAt,
    builder: (column) => ColumnFilters(column),
  );

  ColumnFilters<DateTime> get completedAt => $composableBuilder(
    column: $table.completedAt,
    builder: (column) => ColumnFilters(column),
  );
}

class $$DownloadsTableOrderingComposer
    extends Composer<_$AppDatabase, $DownloadsTable> {
  $$DownloadsTableOrderingComposer({
    required super.$db,
    required super.$table,
    super.joinBuilder,
    super.$addJoinBuilderToRootComposer,
    super.$removeJoinBuilderFromRootComposer,
  });
  ColumnOrderings<int> get id => $composableBuilder(
    column: $table.id,
    builder: (column) => ColumnOrderings(column),
  );

  ColumnOrderings<String> get mediaId => $composableBuilder(
    column: $table.mediaId,
    builder: (column) => ColumnOrderings(column),
  );

  ColumnOrderings<String> get providerId => $composableBuilder(
    column: $table.providerId,
    builder: (column) => ColumnOrderings(column),
  );

  ColumnOrderings<String> get title => $composableBuilder(
    column: $table.title,
    builder: (column) => ColumnOrderings(column),
  );

  ColumnOrderings<String> get posterUrl => $composableBuilder(
    column: $table.posterUrl,
    builder: (column) => ColumnOrderings(column),
  );

  ColumnOrderings<int> get year => $composableBuilder(
    column: $table.year,
    builder: (column) => ColumnOrderings(column),
  );

  ColumnOrderings<double> get rating => $composableBuilder(
    column: $table.rating,
    builder: (column) => ColumnOrderings(column),
  );

  ColumnOrderings<String> get ratingSource => $composableBuilder(
    column: $table.ratingSource,
    builder: (column) => ColumnOrderings(column),
  );

  ColumnOrderings<String> get mediaType => $composableBuilder(
    column: $table.mediaType,
    builder: (column) => ColumnOrderings(column),
  );

  ColumnOrderings<int> get season => $composableBuilder(
    column: $table.season,
    builder: (column) => ColumnOrderings(column),
  );

  ColumnOrderings<int> get episode => $composableBuilder(
    column: $table.episode,
    builder: (column) => ColumnOrderings(column),
  );

  ColumnOrderings<String> get episodeTitle => $composableBuilder(
    column: $table.episodeTitle,
    builder: (column) => ColumnOrderings(column),
  );

  ColumnOrderings<String> get streamUrl => $composableBuilder(
    column: $table.streamUrl,
    builder: (column) => ColumnOrderings(column),
  );

  ColumnOrderings<String> get localPath => $composableBuilder(
    column: $table.localPath,
    builder: (column) => ColumnOrderings(column),
  );

  ColumnOrderings<String> get quality => $composableBuilder(
    column: $table.quality,
    builder: (column) => ColumnOrderings(column),
  );

  ColumnOrderings<String> get voiceover => $composableBuilder(
    column: $table.voiceover,
    builder: (column) => ColumnOrderings(column),
  );

  ColumnOrderings<int> get status => $composableBuilder(
    column: $table.status,
    builder: (column) => ColumnOrderings(column),
  );

  ColumnOrderings<double> get progress => $composableBuilder(
    column: $table.progress,
    builder: (column) => ColumnOrderings(column),
  );

  ColumnOrderings<int> get fileSizeBytes => $composableBuilder(
    column: $table.fileSizeBytes,
    builder: (column) => ColumnOrderings(column),
  );

  ColumnOrderings<int> get downloadedBytes => $composableBuilder(
    column: $table.downloadedBytes,
    builder: (column) => ColumnOrderings(column),
  );

  ColumnOrderings<String> get headers => $composableBuilder(
    column: $table.headers,
    builder: (column) => ColumnOrderings(column),
  );

  ColumnOrderings<String> get localPosterPath => $composableBuilder(
    column: $table.localPosterPath,
    builder: (column) => ColumnOrderings(column),
  );

  ColumnOrderings<int> get duration => $composableBuilder(
    column: $table.duration,
    builder: (column) => ColumnOrderings(column),
  );

  ColumnOrderings<DateTime> get createdAt => $composableBuilder(
    column: $table.createdAt,
    builder: (column) => ColumnOrderings(column),
  );

  ColumnOrderings<DateTime> get completedAt => $composableBuilder(
    column: $table.completedAt,
    builder: (column) => ColumnOrderings(column),
  );
}

class $$DownloadsTableAnnotationComposer
    extends Composer<_$AppDatabase, $DownloadsTable> {
  $$DownloadsTableAnnotationComposer({
    required super.$db,
    required super.$table,
    super.joinBuilder,
    super.$addJoinBuilderToRootComposer,
    super.$removeJoinBuilderFromRootComposer,
  });
  GeneratedColumn<int> get id =>
      $composableBuilder(column: $table.id, builder: (column) => column);

  GeneratedColumn<String> get mediaId =>
      $composableBuilder(column: $table.mediaId, builder: (column) => column);

  GeneratedColumn<String> get providerId => $composableBuilder(
    column: $table.providerId,
    builder: (column) => column,
  );

  GeneratedColumn<String> get title =>
      $composableBuilder(column: $table.title, builder: (column) => column);

  GeneratedColumn<String> get posterUrl =>
      $composableBuilder(column: $table.posterUrl, builder: (column) => column);

  GeneratedColumn<int> get year =>
      $composableBuilder(column: $table.year, builder: (column) => column);

  GeneratedColumn<double> get rating =>
      $composableBuilder(column: $table.rating, builder: (column) => column);

  GeneratedColumn<String> get ratingSource => $composableBuilder(
    column: $table.ratingSource,
    builder: (column) => column,
  );

  GeneratedColumn<String> get mediaType =>
      $composableBuilder(column: $table.mediaType, builder: (column) => column);

  GeneratedColumn<int> get season =>
      $composableBuilder(column: $table.season, builder: (column) => column);

  GeneratedColumn<int> get episode =>
      $composableBuilder(column: $table.episode, builder: (column) => column);

  GeneratedColumn<String> get episodeTitle => $composableBuilder(
    column: $table.episodeTitle,
    builder: (column) => column,
  );

  GeneratedColumn<String> get streamUrl =>
      $composableBuilder(column: $table.streamUrl, builder: (column) => column);

  GeneratedColumn<String> get localPath =>
      $composableBuilder(column: $table.localPath, builder: (column) => column);

  GeneratedColumn<String> get quality =>
      $composableBuilder(column: $table.quality, builder: (column) => column);

  GeneratedColumn<String> get voiceover =>
      $composableBuilder(column: $table.voiceover, builder: (column) => column);

  GeneratedColumnWithTypeConverter<DownloadStatus, int> get status =>
      $composableBuilder(column: $table.status, builder: (column) => column);

  GeneratedColumn<double> get progress =>
      $composableBuilder(column: $table.progress, builder: (column) => column);

  GeneratedColumn<int> get fileSizeBytes => $composableBuilder(
    column: $table.fileSizeBytes,
    builder: (column) => column,
  );

  GeneratedColumn<int> get downloadedBytes => $composableBuilder(
    column: $table.downloadedBytes,
    builder: (column) => column,
  );

  GeneratedColumn<String> get headers =>
      $composableBuilder(column: $table.headers, builder: (column) => column);

  GeneratedColumn<String> get localPosterPath => $composableBuilder(
    column: $table.localPosterPath,
    builder: (column) => column,
  );

  GeneratedColumn<int> get duration =>
      $composableBuilder(column: $table.duration, builder: (column) => column);

  GeneratedColumn<DateTime> get createdAt =>
      $composableBuilder(column: $table.createdAt, builder: (column) => column);

  GeneratedColumn<DateTime> get completedAt => $composableBuilder(
    column: $table.completedAt,
    builder: (column) => column,
  );
}

class $$DownloadsTableTableManager
    extends
        RootTableManager<
          _$AppDatabase,
          $DownloadsTable,
          Download,
          $$DownloadsTableFilterComposer,
          $$DownloadsTableOrderingComposer,
          $$DownloadsTableAnnotationComposer,
          $$DownloadsTableCreateCompanionBuilder,
          $$DownloadsTableUpdateCompanionBuilder,
          (Download, BaseReferences<_$AppDatabase, $DownloadsTable, Download>),
          Download,
          PrefetchHooks Function()
        > {
  $$DownloadsTableTableManager(_$AppDatabase db, $DownloadsTable table)
    : super(
        TableManagerState(
          db: db,
          table: table,
          createFilteringComposer: () =>
              $$DownloadsTableFilterComposer($db: db, $table: table),
          createOrderingComposer: () =>
              $$DownloadsTableOrderingComposer($db: db, $table: table),
          createComputedFieldComposer: () =>
              $$DownloadsTableAnnotationComposer($db: db, $table: table),
          updateCompanionCallback:
              ({
                Value<int> id = const Value.absent(),
                Value<String> mediaId = const Value.absent(),
                Value<String> providerId = const Value.absent(),
                Value<String> title = const Value.absent(),
                Value<String?> posterUrl = const Value.absent(),
                Value<int?> year = const Value.absent(),
                Value<double?> rating = const Value.absent(),
                Value<String?> ratingSource = const Value.absent(),
                Value<String> mediaType = const Value.absent(),
                Value<int?> season = const Value.absent(),
                Value<int?> episode = const Value.absent(),
                Value<String?> episodeTitle = const Value.absent(),
                Value<String> streamUrl = const Value.absent(),
                Value<String> localPath = const Value.absent(),
                Value<String> quality = const Value.absent(),
                Value<String?> voiceover = const Value.absent(),
                Value<DownloadStatus> status = const Value.absent(),
                Value<double> progress = const Value.absent(),
                Value<int> fileSizeBytes = const Value.absent(),
                Value<int> downloadedBytes = const Value.absent(),
                Value<String?> headers = const Value.absent(),
                Value<String?> localPosterPath = const Value.absent(),
                Value<int?> duration = const Value.absent(),
                Value<DateTime> createdAt = const Value.absent(),
                Value<DateTime?> completedAt = const Value.absent(),
              }) => DownloadsCompanion(
                id: id,
                mediaId: mediaId,
                providerId: providerId,
                title: title,
                posterUrl: posterUrl,
                year: year,
                rating: rating,
                ratingSource: ratingSource,
                mediaType: mediaType,
                season: season,
                episode: episode,
                episodeTitle: episodeTitle,
                streamUrl: streamUrl,
                localPath: localPath,
                quality: quality,
                voiceover: voiceover,
                status: status,
                progress: progress,
                fileSizeBytes: fileSizeBytes,
                downloadedBytes: downloadedBytes,
                headers: headers,
                localPosterPath: localPosterPath,
                duration: duration,
                createdAt: createdAt,
                completedAt: completedAt,
              ),
          createCompanionCallback:
              ({
                Value<int> id = const Value.absent(),
                required String mediaId,
                required String providerId,
                required String title,
                Value<String?> posterUrl = const Value.absent(),
                Value<int?> year = const Value.absent(),
                Value<double?> rating = const Value.absent(),
                Value<String?> ratingSource = const Value.absent(),
                required String mediaType,
                Value<int?> season = const Value.absent(),
                Value<int?> episode = const Value.absent(),
                Value<String?> episodeTitle = const Value.absent(),
                required String streamUrl,
                required String localPath,
                required String quality,
                Value<String?> voiceover = const Value.absent(),
                Value<DownloadStatus> status = const Value.absent(),
                Value<double> progress = const Value.absent(),
                Value<int> fileSizeBytes = const Value.absent(),
                Value<int> downloadedBytes = const Value.absent(),
                Value<String?> headers = const Value.absent(),
                Value<String?> localPosterPath = const Value.absent(),
                Value<int?> duration = const Value.absent(),
                Value<DateTime> createdAt = const Value.absent(),
                Value<DateTime?> completedAt = const Value.absent(),
              }) => DownloadsCompanion.insert(
                id: id,
                mediaId: mediaId,
                providerId: providerId,
                title: title,
                posterUrl: posterUrl,
                year: year,
                rating: rating,
                ratingSource: ratingSource,
                mediaType: mediaType,
                season: season,
                episode: episode,
                episodeTitle: episodeTitle,
                streamUrl: streamUrl,
                localPath: localPath,
                quality: quality,
                voiceover: voiceover,
                status: status,
                progress: progress,
                fileSizeBytes: fileSizeBytes,
                downloadedBytes: downloadedBytes,
                headers: headers,
                localPosterPath: localPosterPath,
                duration: duration,
                createdAt: createdAt,
                completedAt: completedAt,
              ),
          withReferenceMapper: (p0) => p0
              .map((e) => (e.readTable(table), BaseReferences(db, table, e)))
              .toList(),
          prefetchHooksCallback: null,
        ),
      );
}

typedef $$DownloadsTableProcessedTableManager =
    ProcessedTableManager<
      _$AppDatabase,
      $DownloadsTable,
      Download,
      $$DownloadsTableFilterComposer,
      $$DownloadsTableOrderingComposer,
      $$DownloadsTableAnnotationComposer,
      $$DownloadsTableCreateCompanionBuilder,
      $$DownloadsTableUpdateCompanionBuilder,
      (Download, BaseReferences<_$AppDatabase, $DownloadsTable, Download>),
      Download,
      PrefetchHooks Function()
    >;
typedef $$SearchHistoryTableTableCreateCompanionBuilder =
    SearchHistoryTableCompanion Function({
      Value<int> id,
      required String query,
      required String normalizedQuery,
      Value<int> resultCount,
      Value<bool> wasSuccessful,
      Value<int> searchCount,
      Value<DateTime> firstSearchedAt,
      Value<DateTime> lastSearchedAt,
    });
typedef $$SearchHistoryTableTableUpdateCompanionBuilder =
    SearchHistoryTableCompanion Function({
      Value<int> id,
      Value<String> query,
      Value<String> normalizedQuery,
      Value<int> resultCount,
      Value<bool> wasSuccessful,
      Value<int> searchCount,
      Value<DateTime> firstSearchedAt,
      Value<DateTime> lastSearchedAt,
    });

class $$SearchHistoryTableTableFilterComposer
    extends Composer<_$AppDatabase, $SearchHistoryTableTable> {
  $$SearchHistoryTableTableFilterComposer({
    required super.$db,
    required super.$table,
    super.joinBuilder,
    super.$addJoinBuilderToRootComposer,
    super.$removeJoinBuilderFromRootComposer,
  });
  ColumnFilters<int> get id => $composableBuilder(
    column: $table.id,
    builder: (column) => ColumnFilters(column),
  );

  ColumnFilters<String> get query => $composableBuilder(
    column: $table.query,
    builder: (column) => ColumnFilters(column),
  );

  ColumnFilters<String> get normalizedQuery => $composableBuilder(
    column: $table.normalizedQuery,
    builder: (column) => ColumnFilters(column),
  );

  ColumnFilters<int> get resultCount => $composableBuilder(
    column: $table.resultCount,
    builder: (column) => ColumnFilters(column),
  );

  ColumnFilters<bool> get wasSuccessful => $composableBuilder(
    column: $table.wasSuccessful,
    builder: (column) => ColumnFilters(column),
  );

  ColumnFilters<int> get searchCount => $composableBuilder(
    column: $table.searchCount,
    builder: (column) => ColumnFilters(column),
  );

  ColumnFilters<DateTime> get firstSearchedAt => $composableBuilder(
    column: $table.firstSearchedAt,
    builder: (column) => ColumnFilters(column),
  );

  ColumnFilters<DateTime> get lastSearchedAt => $composableBuilder(
    column: $table.lastSearchedAt,
    builder: (column) => ColumnFilters(column),
  );
}

class $$SearchHistoryTableTableOrderingComposer
    extends Composer<_$AppDatabase, $SearchHistoryTableTable> {
  $$SearchHistoryTableTableOrderingComposer({
    required super.$db,
    required super.$table,
    super.joinBuilder,
    super.$addJoinBuilderToRootComposer,
    super.$removeJoinBuilderFromRootComposer,
  });
  ColumnOrderings<int> get id => $composableBuilder(
    column: $table.id,
    builder: (column) => ColumnOrderings(column),
  );

  ColumnOrderings<String> get query => $composableBuilder(
    column: $table.query,
    builder: (column) => ColumnOrderings(column),
  );

  ColumnOrderings<String> get normalizedQuery => $composableBuilder(
    column: $table.normalizedQuery,
    builder: (column) => ColumnOrderings(column),
  );

  ColumnOrderings<int> get resultCount => $composableBuilder(
    column: $table.resultCount,
    builder: (column) => ColumnOrderings(column),
  );

  ColumnOrderings<bool> get wasSuccessful => $composableBuilder(
    column: $table.wasSuccessful,
    builder: (column) => ColumnOrderings(column),
  );

  ColumnOrderings<int> get searchCount => $composableBuilder(
    column: $table.searchCount,
    builder: (column) => ColumnOrderings(column),
  );

  ColumnOrderings<DateTime> get firstSearchedAt => $composableBuilder(
    column: $table.firstSearchedAt,
    builder: (column) => ColumnOrderings(column),
  );

  ColumnOrderings<DateTime> get lastSearchedAt => $composableBuilder(
    column: $table.lastSearchedAt,
    builder: (column) => ColumnOrderings(column),
  );
}

class $$SearchHistoryTableTableAnnotationComposer
    extends Composer<_$AppDatabase, $SearchHistoryTableTable> {
  $$SearchHistoryTableTableAnnotationComposer({
    required super.$db,
    required super.$table,
    super.joinBuilder,
    super.$addJoinBuilderToRootComposer,
    super.$removeJoinBuilderFromRootComposer,
  });
  GeneratedColumn<int> get id =>
      $composableBuilder(column: $table.id, builder: (column) => column);

  GeneratedColumn<String> get query =>
      $composableBuilder(column: $table.query, builder: (column) => column);

  GeneratedColumn<String> get normalizedQuery => $composableBuilder(
    column: $table.normalizedQuery,
    builder: (column) => column,
  );

  GeneratedColumn<int> get resultCount => $composableBuilder(
    column: $table.resultCount,
    builder: (column) => column,
  );

  GeneratedColumn<bool> get wasSuccessful => $composableBuilder(
    column: $table.wasSuccessful,
    builder: (column) => column,
  );

  GeneratedColumn<int> get searchCount => $composableBuilder(
    column: $table.searchCount,
    builder: (column) => column,
  );

  GeneratedColumn<DateTime> get firstSearchedAt => $composableBuilder(
    column: $table.firstSearchedAt,
    builder: (column) => column,
  );

  GeneratedColumn<DateTime> get lastSearchedAt => $composableBuilder(
    column: $table.lastSearchedAt,
    builder: (column) => column,
  );
}

class $$SearchHistoryTableTableTableManager
    extends
        RootTableManager<
          _$AppDatabase,
          $SearchHistoryTableTable,
          SearchHistoryTableData,
          $$SearchHistoryTableTableFilterComposer,
          $$SearchHistoryTableTableOrderingComposer,
          $$SearchHistoryTableTableAnnotationComposer,
          $$SearchHistoryTableTableCreateCompanionBuilder,
          $$SearchHistoryTableTableUpdateCompanionBuilder,
          (
            SearchHistoryTableData,
            BaseReferences<
              _$AppDatabase,
              $SearchHistoryTableTable,
              SearchHistoryTableData
            >,
          ),
          SearchHistoryTableData,
          PrefetchHooks Function()
        > {
  $$SearchHistoryTableTableTableManager(
    _$AppDatabase db,
    $SearchHistoryTableTable table,
  ) : super(
        TableManagerState(
          db: db,
          table: table,
          createFilteringComposer: () =>
              $$SearchHistoryTableTableFilterComposer($db: db, $table: table),
          createOrderingComposer: () =>
              $$SearchHistoryTableTableOrderingComposer($db: db, $table: table),
          createComputedFieldComposer: () =>
              $$SearchHistoryTableTableAnnotationComposer(
                $db: db,
                $table: table,
              ),
          updateCompanionCallback:
              ({
                Value<int> id = const Value.absent(),
                Value<String> query = const Value.absent(),
                Value<String> normalizedQuery = const Value.absent(),
                Value<int> resultCount = const Value.absent(),
                Value<bool> wasSuccessful = const Value.absent(),
                Value<int> searchCount = const Value.absent(),
                Value<DateTime> firstSearchedAt = const Value.absent(),
                Value<DateTime> lastSearchedAt = const Value.absent(),
              }) => SearchHistoryTableCompanion(
                id: id,
                query: query,
                normalizedQuery: normalizedQuery,
                resultCount: resultCount,
                wasSuccessful: wasSuccessful,
                searchCount: searchCount,
                firstSearchedAt: firstSearchedAt,
                lastSearchedAt: lastSearchedAt,
              ),
          createCompanionCallback:
              ({
                Value<int> id = const Value.absent(),
                required String query,
                required String normalizedQuery,
                Value<int> resultCount = const Value.absent(),
                Value<bool> wasSuccessful = const Value.absent(),
                Value<int> searchCount = const Value.absent(),
                Value<DateTime> firstSearchedAt = const Value.absent(),
                Value<DateTime> lastSearchedAt = const Value.absent(),
              }) => SearchHistoryTableCompanion.insert(
                id: id,
                query: query,
                normalizedQuery: normalizedQuery,
                resultCount: resultCount,
                wasSuccessful: wasSuccessful,
                searchCount: searchCount,
                firstSearchedAt: firstSearchedAt,
                lastSearchedAt: lastSearchedAt,
              ),
          withReferenceMapper: (p0) => p0
              .map((e) => (e.readTable(table), BaseReferences(db, table, e)))
              .toList(),
          prefetchHooksCallback: null,
        ),
      );
}

typedef $$SearchHistoryTableTableProcessedTableManager =
    ProcessedTableManager<
      _$AppDatabase,
      $SearchHistoryTableTable,
      SearchHistoryTableData,
      $$SearchHistoryTableTableFilterComposer,
      $$SearchHistoryTableTableOrderingComposer,
      $$SearchHistoryTableTableAnnotationComposer,
      $$SearchHistoryTableTableCreateCompanionBuilder,
      $$SearchHistoryTableTableUpdateCompanionBuilder,
      (
        SearchHistoryTableData,
        BaseReferences<
          _$AppDatabase,
          $SearchHistoryTableTable,
          SearchHistoryTableData
        >,
      ),
      SearchHistoryTableData,
      PrefetchHooks Function()
    >;
typedef $$StoredMediaItemsTableCreateCompanionBuilder =
    StoredMediaItemsCompanion Function({
      required String id,
      required String providerId,
      required String title,
      Value<String?> originalTitle,
      Value<String?> posterUrl,
      Value<int?> year,
      Value<double?> rating,
      required String mediaType,
      Value<String?> description,
      Value<String?> genres,
      Value<String?> country,
      Value<String?> ratingSource,
      Value<DateTime> updatedAt,
      Value<int> rowid,
    });
typedef $$StoredMediaItemsTableUpdateCompanionBuilder =
    StoredMediaItemsCompanion Function({
      Value<String> id,
      Value<String> providerId,
      Value<String> title,
      Value<String?> originalTitle,
      Value<String?> posterUrl,
      Value<int?> year,
      Value<double?> rating,
      Value<String> mediaType,
      Value<String?> description,
      Value<String?> genres,
      Value<String?> country,
      Value<String?> ratingSource,
      Value<DateTime> updatedAt,
      Value<int> rowid,
    });

class $$StoredMediaItemsTableFilterComposer
    extends Composer<_$AppDatabase, $StoredMediaItemsTable> {
  $$StoredMediaItemsTableFilterComposer({
    required super.$db,
    required super.$table,
    super.joinBuilder,
    super.$addJoinBuilderToRootComposer,
    super.$removeJoinBuilderFromRootComposer,
  });
  ColumnFilters<String> get id => $composableBuilder(
    column: $table.id,
    builder: (column) => ColumnFilters(column),
  );

  ColumnFilters<String> get providerId => $composableBuilder(
    column: $table.providerId,
    builder: (column) => ColumnFilters(column),
  );

  ColumnFilters<String> get title => $composableBuilder(
    column: $table.title,
    builder: (column) => ColumnFilters(column),
  );

  ColumnFilters<String> get originalTitle => $composableBuilder(
    column: $table.originalTitle,
    builder: (column) => ColumnFilters(column),
  );

  ColumnFilters<String> get posterUrl => $composableBuilder(
    column: $table.posterUrl,
    builder: (column) => ColumnFilters(column),
  );

  ColumnFilters<int> get year => $composableBuilder(
    column: $table.year,
    builder: (column) => ColumnFilters(column),
  );

  ColumnFilters<double> get rating => $composableBuilder(
    column: $table.rating,
    builder: (column) => ColumnFilters(column),
  );

  ColumnFilters<String> get mediaType => $composableBuilder(
    column: $table.mediaType,
    builder: (column) => ColumnFilters(column),
  );

  ColumnFilters<String> get description => $composableBuilder(
    column: $table.description,
    builder: (column) => ColumnFilters(column),
  );

  ColumnFilters<String> get genres => $composableBuilder(
    column: $table.genres,
    builder: (column) => ColumnFilters(column),
  );

  ColumnFilters<String> get country => $composableBuilder(
    column: $table.country,
    builder: (column) => ColumnFilters(column),
  );

  ColumnFilters<String> get ratingSource => $composableBuilder(
    column: $table.ratingSource,
    builder: (column) => ColumnFilters(column),
  );

  ColumnFilters<DateTime> get updatedAt => $composableBuilder(
    column: $table.updatedAt,
    builder: (column) => ColumnFilters(column),
  );
}

class $$StoredMediaItemsTableOrderingComposer
    extends Composer<_$AppDatabase, $StoredMediaItemsTable> {
  $$StoredMediaItemsTableOrderingComposer({
    required super.$db,
    required super.$table,
    super.joinBuilder,
    super.$addJoinBuilderToRootComposer,
    super.$removeJoinBuilderFromRootComposer,
  });
  ColumnOrderings<String> get id => $composableBuilder(
    column: $table.id,
    builder: (column) => ColumnOrderings(column),
  );

  ColumnOrderings<String> get providerId => $composableBuilder(
    column: $table.providerId,
    builder: (column) => ColumnOrderings(column),
  );

  ColumnOrderings<String> get title => $composableBuilder(
    column: $table.title,
    builder: (column) => ColumnOrderings(column),
  );

  ColumnOrderings<String> get originalTitle => $composableBuilder(
    column: $table.originalTitle,
    builder: (column) => ColumnOrderings(column),
  );

  ColumnOrderings<String> get posterUrl => $composableBuilder(
    column: $table.posterUrl,
    builder: (column) => ColumnOrderings(column),
  );

  ColumnOrderings<int> get year => $composableBuilder(
    column: $table.year,
    builder: (column) => ColumnOrderings(column),
  );

  ColumnOrderings<double> get rating => $composableBuilder(
    column: $table.rating,
    builder: (column) => ColumnOrderings(column),
  );

  ColumnOrderings<String> get mediaType => $composableBuilder(
    column: $table.mediaType,
    builder: (column) => ColumnOrderings(column),
  );

  ColumnOrderings<String> get description => $composableBuilder(
    column: $table.description,
    builder: (column) => ColumnOrderings(column),
  );

  ColumnOrderings<String> get genres => $composableBuilder(
    column: $table.genres,
    builder: (column) => ColumnOrderings(column),
  );

  ColumnOrderings<String> get country => $composableBuilder(
    column: $table.country,
    builder: (column) => ColumnOrderings(column),
  );

  ColumnOrderings<String> get ratingSource => $composableBuilder(
    column: $table.ratingSource,
    builder: (column) => ColumnOrderings(column),
  );

  ColumnOrderings<DateTime> get updatedAt => $composableBuilder(
    column: $table.updatedAt,
    builder: (column) => ColumnOrderings(column),
  );
}

class $$StoredMediaItemsTableAnnotationComposer
    extends Composer<_$AppDatabase, $StoredMediaItemsTable> {
  $$StoredMediaItemsTableAnnotationComposer({
    required super.$db,
    required super.$table,
    super.joinBuilder,
    super.$addJoinBuilderToRootComposer,
    super.$removeJoinBuilderFromRootComposer,
  });
  GeneratedColumn<String> get id =>
      $composableBuilder(column: $table.id, builder: (column) => column);

  GeneratedColumn<String> get providerId => $composableBuilder(
    column: $table.providerId,
    builder: (column) => column,
  );

  GeneratedColumn<String> get title =>
      $composableBuilder(column: $table.title, builder: (column) => column);

  GeneratedColumn<String> get originalTitle => $composableBuilder(
    column: $table.originalTitle,
    builder: (column) => column,
  );

  GeneratedColumn<String> get posterUrl =>
      $composableBuilder(column: $table.posterUrl, builder: (column) => column);

  GeneratedColumn<int> get year =>
      $composableBuilder(column: $table.year, builder: (column) => column);

  GeneratedColumn<double> get rating =>
      $composableBuilder(column: $table.rating, builder: (column) => column);

  GeneratedColumn<String> get mediaType =>
      $composableBuilder(column: $table.mediaType, builder: (column) => column);

  GeneratedColumn<String> get description => $composableBuilder(
    column: $table.description,
    builder: (column) => column,
  );

  GeneratedColumn<String> get genres =>
      $composableBuilder(column: $table.genres, builder: (column) => column);

  GeneratedColumn<String> get country =>
      $composableBuilder(column: $table.country, builder: (column) => column);

  GeneratedColumn<String> get ratingSource => $composableBuilder(
    column: $table.ratingSource,
    builder: (column) => column,
  );

  GeneratedColumn<DateTime> get updatedAt =>
      $composableBuilder(column: $table.updatedAt, builder: (column) => column);
}

class $$StoredMediaItemsTableTableManager
    extends
        RootTableManager<
          _$AppDatabase,
          $StoredMediaItemsTable,
          StoredMediaItem,
          $$StoredMediaItemsTableFilterComposer,
          $$StoredMediaItemsTableOrderingComposer,
          $$StoredMediaItemsTableAnnotationComposer,
          $$StoredMediaItemsTableCreateCompanionBuilder,
          $$StoredMediaItemsTableUpdateCompanionBuilder,
          (
            StoredMediaItem,
            BaseReferences<
              _$AppDatabase,
              $StoredMediaItemsTable,
              StoredMediaItem
            >,
          ),
          StoredMediaItem,
          PrefetchHooks Function()
        > {
  $$StoredMediaItemsTableTableManager(
    _$AppDatabase db,
    $StoredMediaItemsTable table,
  ) : super(
        TableManagerState(
          db: db,
          table: table,
          createFilteringComposer: () =>
              $$StoredMediaItemsTableFilterComposer($db: db, $table: table),
          createOrderingComposer: () =>
              $$StoredMediaItemsTableOrderingComposer($db: db, $table: table),
          createComputedFieldComposer: () =>
              $$StoredMediaItemsTableAnnotationComposer($db: db, $table: table),
          updateCompanionCallback:
              ({
                Value<String> id = const Value.absent(),
                Value<String> providerId = const Value.absent(),
                Value<String> title = const Value.absent(),
                Value<String?> originalTitle = const Value.absent(),
                Value<String?> posterUrl = const Value.absent(),
                Value<int?> year = const Value.absent(),
                Value<double?> rating = const Value.absent(),
                Value<String> mediaType = const Value.absent(),
                Value<String?> description = const Value.absent(),
                Value<String?> genres = const Value.absent(),
                Value<String?> country = const Value.absent(),
                Value<String?> ratingSource = const Value.absent(),
                Value<DateTime> updatedAt = const Value.absent(),
                Value<int> rowid = const Value.absent(),
              }) => StoredMediaItemsCompanion(
                id: id,
                providerId: providerId,
                title: title,
                originalTitle: originalTitle,
                posterUrl: posterUrl,
                year: year,
                rating: rating,
                mediaType: mediaType,
                description: description,
                genres: genres,
                country: country,
                ratingSource: ratingSource,
                updatedAt: updatedAt,
                rowid: rowid,
              ),
          createCompanionCallback:
              ({
                required String id,
                required String providerId,
                required String title,
                Value<String?> originalTitle = const Value.absent(),
                Value<String?> posterUrl = const Value.absent(),
                Value<int?> year = const Value.absent(),
                Value<double?> rating = const Value.absent(),
                required String mediaType,
                Value<String?> description = const Value.absent(),
                Value<String?> genres = const Value.absent(),
                Value<String?> country = const Value.absent(),
                Value<String?> ratingSource = const Value.absent(),
                Value<DateTime> updatedAt = const Value.absent(),
                Value<int> rowid = const Value.absent(),
              }) => StoredMediaItemsCompanion.insert(
                id: id,
                providerId: providerId,
                title: title,
                originalTitle: originalTitle,
                posterUrl: posterUrl,
                year: year,
                rating: rating,
                mediaType: mediaType,
                description: description,
                genres: genres,
                country: country,
                ratingSource: ratingSource,
                updatedAt: updatedAt,
                rowid: rowid,
              ),
          withReferenceMapper: (p0) => p0
              .map((e) => (e.readTable(table), BaseReferences(db, table, e)))
              .toList(),
          prefetchHooksCallback: null,
        ),
      );
}

typedef $$StoredMediaItemsTableProcessedTableManager =
    ProcessedTableManager<
      _$AppDatabase,
      $StoredMediaItemsTable,
      StoredMediaItem,
      $$StoredMediaItemsTableFilterComposer,
      $$StoredMediaItemsTableOrderingComposer,
      $$StoredMediaItemsTableAnnotationComposer,
      $$StoredMediaItemsTableCreateCompanionBuilder,
      $$StoredMediaItemsTableUpdateCompanionBuilder,
      (
        StoredMediaItem,
        BaseReferences<_$AppDatabase, $StoredMediaItemsTable, StoredMediaItem>,
      ),
      StoredMediaItem,
      PrefetchHooks Function()
    >;

class $AppDatabaseManager {
  final _$AppDatabase _db;
  $AppDatabaseManager(this._db);
  $$AppSettingsTableTableManager get appSettings =>
      $$AppSettingsTableTableManager(_db, _db.appSettings);
  $$EnabledProvidersTableTableManager get enabledProviders =>
      $$EnabledProvidersTableTableManager(_db, _db.enabledProviders);
  $$FavoritesTableTableManager get favorites =>
      $$FavoritesTableTableManager(_db, _db.favorites);
  $$WatchHistoryTableTableManager get watchHistory =>
      $$WatchHistoryTableTableManager(_db, _db.watchHistory);
  $$DownloadsTableTableManager get downloads =>
      $$DownloadsTableTableManager(_db, _db.downloads);
  $$SearchHistoryTableTableTableManager get searchHistoryTable =>
      $$SearchHistoryTableTableTableManager(_db, _db.searchHistoryTable);
  $$StoredMediaItemsTableTableManager get storedMediaItems =>
      $$StoredMediaItemsTableTableManager(_db, _db.storedMediaItems);
}
