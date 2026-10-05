import 'package:equatable/equatable.dart';

/// Base failure class for domain layer error handling
abstract class Failure extends Equatable {
  final String message;
  final String? code;

  const Failure({required this.message, this.code});

  @override
  List<Object?> get props => [message, code];
}

/// Network-related failures
class NetworkFailure extends Failure {
  const NetworkFailure({super.message = 'Network error occurred', super.code});
}

/// Server-related failures
class ServerFailure extends Failure {
  final int? statusCode;

  const ServerFailure({
    super.message = 'Server error occurred',
    super.code,
    this.statusCode,
  });

  @override
  List<Object?> get props => [message, code, statusCode];
}

/// Parsing/format failures
class ParsingFailure extends Failure {
  const ParsingFailure({super.message = 'Failed to parse data', super.code});
}

/// Cache-related failures
class CacheFailure extends Failure {
  const CacheFailure({super.message = 'Cache error occurred', super.code});
}

/// Provider not found failure
class ProviderNotFoundFailure extends Failure {
  const ProviderNotFoundFailure({
    super.message = 'Content provider not found',
    super.code,
  });
}

/// No stream sources available
class NoStreamFailure extends Failure {
  const NoStreamFailure({
    super.message = 'No streaming sources available',
    super.code,
  });
}
