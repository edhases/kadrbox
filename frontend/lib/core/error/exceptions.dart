/// Base exception class for data layer
class AppException implements Exception {
  final String message;
  final String? code;

  const AppException({required this.message, this.code});

  @override
  String toString() => 'AppException: $message (code: $code)';
}

/// Network exceptions (no internet, timeout, etc.)
class NetworkException extends AppException {
  const NetworkException({super.message = 'Network error', super.code});
}

/// Server exceptions (4xx, 5xx responses)
class ServerException extends AppException {
  final int statusCode;

  const ServerException({
    super.message = 'Server error',
    super.code,
    required this.statusCode,
  });
}

/// Parsing exceptions
class ParsingException extends AppException {
  const ParsingException({super.message = 'Parsing error', super.code});
}

/// Cache exceptions
class CacheException extends AppException {
  const CacheException({super.message = 'Cache error', super.code});
}
