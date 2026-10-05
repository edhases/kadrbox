import 'dart:convert';
import 'dart:io';

import 'package:dio/dio.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:kadrbox/core/config/app_config.dart';
import 'package:kadrbox/core/network/api_client.dart';
import 'package:kadrbox/data/services/kadrbox_server_service.dart';
import 'package:shared_preferences/shared_preferences.dart';

/// The watch-party socket is ticket-authenticated, and the client used to skip
/// that step entirely: it opened `?room=&user_id=&user_name=` with no ticket and
/// the hub answered 401, so every server-hosted room silently fell back to the
/// peer transport.
///
/// These tests run against a real loopback server and a real WebSocket upgrade.
/// The whole failure was a disagreement between two processes about a URL, and
/// a mocked HTTP client would have agreed with whatever the client wrote.
void main() {
  late HttpServer server;
  late KadrboxServerService service;
  late ApiClient apiClient;
  late String originalBaseUrl;

  /// What the stub server saw, so the tests assert on the wire format rather
  /// than on the client's own idea of it.
  final seen = <String, dynamic>{};

  setUp(() async {
    originalBaseUrl = AppConfig.serverBaseUrl;
    SharedPreferences.setMockInitialValues({});
    final prefs = await SharedPreferences.getInstance();

    seen.clear();
    server = await HttpServer.bind(InternetAddress.loopbackIPv4, 0);
    AppConfig.serverBaseUrl = 'http://127.0.0.1:${server.port}';

    server.listen((HttpRequest req) async {
      final key = '${req.method} ${req.uri.path}';
      seen[key] = {
        'query': req.uri.queryParameters,
        'headers': req.headers,
      };

      if (key == 'POST /api/v1/auth/login') {
        req.response.headers.contentType = ContentType.json;
        req.response.write(
          jsonEncode({
            'access_token': 'jwt_access_123',
            'refresh_token': 'jwt_refresh_456',
            'user': {
              'id': 'u100',
              'email': 'user@example.com',
              'username': 'Valik',
            },
          }),
        );
        await req.response.close();
        return;
      }

      if (key == 'POST /api/v1/watch-party/tickets') {
        // Mirrors the server: the route sits behind AuthMiddleware.
        final auth =
            req.headers.value(HttpHeaders.authorizationHeader) ?? '';
        if (!auth.startsWith('Bearer ')) {
          req.response.statusCode = HttpStatus.unauthorized;
          req.response.write(jsonEncode({'error': 'authentication required'}));
          await req.response.close();
          return;
        }

        final decoded = jsonDecode(await utf8.decodeStream(req));
        seen['ticket-body'] = decoded;

        req.response.headers.contentType = ContentType.json;
        req.response.write(
          jsonEncode({
            'ticket': 'signed-ticket-for-${decoded['roomCode']}',
            'roomCode': decoded['roomCode'],
            'expiresAt': DateTime.now()
                .add(const Duration(minutes: 5))
                .toUtc()
                .toIso8601String(),
          }),
        );
        await req.response.close();
        return;
      }

      // The WebSocket upgrade. This mirrors the hub: no ticket, no upgrade.
      final ticket = req.uri.queryParameters['ticket'];
      if (ticket == null || ticket.isEmpty) {
        req.response.statusCode = HttpStatus.unauthorized;
        req.response.write(jsonEncode({'error': 'missing_ticket'}));
        await req.response.close();
        return;
      }
      if (req.uri.queryParameters['room'] != 'ROOM1') {
        req.response.statusCode = HttpStatus.forbidden;
        req.response.write(jsonEncode({'error': 'wrong_room'}));
        await req.response.close();
        return;
      }

      final socket = await WebSocketTransformer.upgrade(req);
      socket.add('joined');
    });

    final dio = Dio(
      BaseOptions(
        connectTimeout: const Duration(seconds: 5),
        receiveTimeout: const Duration(seconds: 5),
      ),
    );
    apiClient = ApiClient(prefs: prefs, dio: dio);
    service = KadrboxServerService(prefs, apiClient);
  });

  tearDown(() async {
    AppConfig.serverBaseUrl = originalBaseUrl;
    await server.close(force: true);
  });

  Future<void> signIn() => service.signIn('user@example.com', 'secret');

  group('issuing a watch-party ticket', () {
    test('refuses to ask for one while signed out', () async {
      await expectLater(
        service.issueWatchPartyTicket(roomCode: 'ROOM1', isHost: true),
        throwsA(isA<StateError>()),
      );
      // The point of the guard: an anonymous room would be indistinguishable
      // from an unreviewed one, so the request must not leave the device.
      expect(seen.containsKey('POST /api/v1/watch-party/tickets'), isFalse);
    });

    test('sends the room, the host flag and the display name', () async {
      await signIn();

      final ticket = await service.issueWatchPartyTicket(
        roomCode: 'ROOM1',
        isHost: true,
        userName: 'Вася',
      );

      expect(ticket, 'signed-ticket-for-ROOM1');
      expect(seen['ticket-body'], {
        'roomCode': 'ROOM1',
        'isHost': true,
        'userName': 'Вася',
      });
    });

    test('omits the display name when there is none', () async {
      await signIn();

      await service.issueWatchPartyTicket(roomCode: 'ROOM1', isHost: false);

      final body = seen['ticket-body'] as Map<String, dynamic>;
      expect(body.containsKey('userName'), isFalse);
      expect(body['isHost'], isFalse);
    });

    test('surfaces a malformed response instead of connecting anyway', () async {
      // A ticket that came back empty would produce a socket URL with an empty
      // `?ticket=`, which fails as 401 at handshake -- far from the cause.
      await server.close(force: true);
      server = await HttpServer.bind(InternetAddress.loopbackIPv4, 0);
      AppConfig.serverBaseUrl = 'http://127.0.0.1:${server.port}';
      server.listen((HttpRequest req) async {
        if (req.uri.path == '/api/v1/auth/login') {
          req.response.headers.contentType = ContentType.json;
          req.response.write(
            jsonEncode({
              'access_token': 'jwt_access_123',
              'refresh_token': 'jwt_refresh_456',
              'user': {'id': 'u100', 'email': 'user@example.com'},
            }),
          );
        } else {
          // A ticket-shaped response with no ticket in it.
          req.response.headers.contentType = ContentType.json;
          req.response.write(jsonEncode({'roomCode': 'ROOM1'}));
        }
        await req.response.close();
      });

      await signIn();
      await expectLater(
        service.issueWatchPartyTicket(roomCode: 'ROOM1', isHost: false),
        throwsA(isA<FormatException>()),
      );
    });
  });

  group('connecting with a ticket', () {
    test('upgrades and reads a frame back', () async {
      await signIn();
      final ticket = await service.issueWatchPartyTicket(
        roomCode: 'ROOM1',
        isHost: false,
        userName: 'Вася',
      );

      final ws = await service.connectWatchParty(
        roomCode: 'ROOM1',
        ticket: ticket,
      );
      addTearDown(ws.close);

      expect(await ws.first, 'joined');
    });

    test('puts the ticket and the room on the wire, and nothing else',
        () async {
      await signIn();
      final ticket = await service.issueWatchPartyTicket(
        roomCode: 'ROOM1',
        isHost: false,
      );

      final ws = await service.connectWatchParty(
        roomCode: 'ROOM1',
        ticket: ticket,
      );
      addTearDown(ws.close);
      await ws.first;

      final query = Map<String, String>.from(
        (seen['GET /api/v1/ws/watch-party']! as Map)['query'] as Map,
      );
      expect(query['room'], 'ROOM1');
      expect(query['ticket'], ticket);
      // Identity comes from the verified ticket claims. A client-supplied
      // user_id would be dead weight at best and, on a peer-facing endpoint, an
      // invitation to stop trusting the ticket at all.
      expect(query.containsKey('user_id'), isFalse);
      expect(query.containsKey('user_name'), isFalse);
    });

    test('fails the handshake when the ticket is missing', () async {
      // What the client used to do. The stub answers exactly as the hub does.
      await signIn();

      await expectLater(
        WebSocket.connect(
          '${AppConfig.serverWsUrl.replaceFirst('wss', 'ws')}'
          '?room=ROOM1&user_id=1791236235734&user_name=User',
        ),
        throwsA(isA<WebSocketException>()),
      );
    });
  });
}