import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:get_it/get_it.dart';
import 'package:oxide_film/data/services/watch_party_service.dart';
import 'package:oxide_film/presentation/pages/watch_party/watch_party_page.dart';
import 'package:flutter_localizations/flutter_localizations.dart';
import 'package:oxide_film/core/l10n/app_strings.dart';

// Manual Mock for Service
class MockWatchPartyService extends ChangeNotifier
    implements WatchPartyService {
  @override
  WatchPartyState state = WatchPartyState.idle;

  @override
  String myName = 'Test User';

  @override
  List<WatchPartyParticipant> participants = [];

  @override
  List<ChatMessage> chatMessages = [];

  @override
  WatchPartyBackendType backendType = WatchPartyBackendType.server;

  @override
  WatchPartyRoom? room;

  @override
  bool isHost = false;

  @override
  bool isPlaying = false; // Stub

  @override
  String get myId => 'test_id';

  @override
  String? currentRoomCode;

  @override
  String? error;

  @override
  void setMyName(String name) {
    myName = name;
    notifyListeners();
  }

  @override
  Future<bool> hostRoom({String? mediaUrl, String? mediaTitle}) async {
    isHost = true;
    state = WatchPartyState.connected;
    room = WatchPartyRoom(
      id: 'HOST12',
      hostId: 'me',
      hostName: myName,
      mediaTitle: mediaTitle,
      mediaUrl: mediaUrl,
    );
    notifyListeners();
    return true;
  }

  @override
  Future<bool> joinRoom(String roomCode) async {
    isHost = false;
    currentRoomCode = roomCode;
    state = WatchPartyState.connected;
    participants = [
      WatchPartyParticipant(
        id: 'other',
        name: 'Other',
        isHost: true,
        joinedAt: DateTime.now(),
      ),
      WatchPartyParticipant(
        id: 'me',
        name: myName,
        isHost: false,
        joinedAt: DateTime.now(),
      ),
    ];
    notifyListeners();
    return true;
  }

  @override
  Future<void> leaveRoom() async {
    state = WatchPartyState.idle;
    notifyListeners();
  }

  @override
  void sendChatMessage(String message) {
    chatMessages.add(
      ChatMessage(
        id: '1',
        senderId: 'me',
        senderName: myName,
        message: message,
        timestamp: DateTime.now(),
      ),
    );
    notifyListeners();
  }

  // Stubs for other members to avoid unimplemented errors
  @override
  dynamic noSuchMethod(Invocation invocation) => super.noSuchMethod(invocation);
}

void main() {
  late MockWatchPartyService mockService;

  Widget createWidget() {
    return const MaterialApp(
      localizationsDelegates: [
        AppStringsDelegate(),
        GlobalMaterialLocalizations.delegate,
        GlobalWidgetsLocalizations.delegate,
        GlobalCupertinoLocalizations.delegate,
      ],
      supportedLocales: [Locale('uk'), Locale('en')],
      locale: Locale('uk'),
      home: WatchPartyPage(
        mediaTitle: 'Test Movie',
        mediaUrl: 'http://test.com',
      ),
    );
  }

  setUp(() async {
    await GetIt.I.reset();
    mockService = MockWatchPartyService();
    GetIt.I.registerSingleton<WatchPartyService>(mockService);

    // Reset window size to default (phone-like for consistency).
    // tester.view is the supported replacement for the deprecated
    // binding.window / *TestValue accessors.
  });

  testWidgets('WatchPartyPage shows idle state initially', (
    WidgetTester tester,
  ) async {
    // Add cleanup for window size
    addTearDown(tester.view.resetPhysicalSize);
    addTearDown(tester.view.resetDevicePixelRatio);

    await tester.pumpWidget(createWidget());
    await tester.pumpAndSettle();

    // Verify state
    print('DEBUG: Service state: ${mockService.state}');

    // Check if any widgets are rendered
    final anyText = find.byType(Text);
    if (anyText.evaluate().isEmpty) {
      fail('No Text widgets found! Page likely failed to build.');
    }

    final allTextStrings = anyText
        .evaluate()
        .map((e) => (e.widget as Text).data)
        .toList();
    print('DEBUG: All visible text: $allTextStrings');

    // Check for title (English or Ukrainian)
    final titleFound =
        find.text('Спільний перегляд').evaluate().isNotEmpty ||
        find.text('Watch party').evaluate().isNotEmpty;

    expect(
      titleFound,
      isTrue,
      reason: 'Title not found. Visible text: $allTextStrings',
    );

    // Check for "Your Name" field label
    final nameLabelFound =
        find.text('Ваше ім\'я').evaluate().isNotEmpty ||
        find.text('Your Name').evaluate().isNotEmpty;
    expect(nameLabelFound, isTrue, reason: 'Name input label not found');

    // Check for Host button text
    final hostButtonFound =
        find.text('Створити кімнату').evaluate().isNotEmpty ||
        find.text('Create Room').evaluate().isNotEmpty;
    expect(hostButtonFound, isTrue, reason: 'Host button not found');
  });

  testWidgets('Inputting name and hosting changes state', (
    WidgetTester tester,
  ) async {
    await tester.pumpWidget(createWidget());
    await tester.pumpAndSettle();

    // Debug print for TextFields
    final textFields = find.byType(TextField);
    print('DEBUG: Found ${textFields.evaluate().length} TextFields');

    // Enter name - find by type if specific text not found
    final nameField = find
        .byType(TextField)
        .at(0); // Assuming first field is name
    await tester.enterText(nameField, 'MyName');
    await tester.pump();

    // Click host button
    final hostButton = find.byIcon(Icons.add);
    await tester.ensureVisible(hostButton);
    await tester.pumpAndSettle();

    await tester.tap(hostButton);

    await tester.pump(); // Start async
    await tester.pump(); // Finish async

    expect(mockService.state, WatchPartyState.connected);
    // Flexible expectation for success message
    final successTextFound =
        find.text('Ви хост').evaluate().isNotEmpty ||
        find.text('You are host').evaluate().isNotEmpty;

    expect(successTextFound, isTrue, reason: 'Host status text not found');
  });

  testWidgets('Inputting code and joining changes state', (
    WidgetTester tester,
  ) async {
    await tester.pumpWidget(createWidget());
    await tester.pumpAndSettle();

    // Enter code - second text field
    final codeField = find.byType(TextField).at(1);
    await tester.ensureVisible(codeField);
    await tester.enterText(codeField, 'ABCDEF');
    await tester.pump();

    // Click join button
    final joinButton = find.byIcon(Icons.login); // Icon in button
    await tester.ensureVisible(joinButton);
    await tester.pumpAndSettle();

    await tester.tap(joinButton);

    await tester.pump(); // Start async
    await tester.pump(); // Finish async

    expect(mockService.state, WatchPartyState.connected);
    // Check for connected status
    final connectedTextFound =
        find.text('Підключено').evaluate().isNotEmpty ||
        find.text('Підключено до хоста').evaluate().isNotEmpty ||
        find.text('Connected').evaluate().isNotEmpty;
    expect(
      connectedTextFound,
      isTrue,
      reason: 'Connected status text not found',
    );

    expect(find.text('Other'), findsOneWidget);
  });

  testWidgets('Sending chat message works', (WidgetTester tester) async {
    // Start in connected state
    await mockService.joinRoom('CODE'); // Wait for it
    await tester.pumpWidget(createWidget());
    await tester.pumpAndSettle();

    // Switch to Chat tab
    // Might need to find Tab by icon if text fails
    final chatTab = find.text('Чат').evaluate().isNotEmpty
        ? find.text('Чат')
        : find.byIcon(Icons.chat).first; // Icon in TabBar

    await tester.tap(chatTab);
    await tester.pumpAndSettle();

    // Enter message
    // On mobile layout, chat input is at bottom
    final chatInput = find.byType(TextField).last;
    await tester.enterText(chatInput, 'Hello Chat');

    await tester.tap(find.byKey(const Key('chat_send_button')));
    await tester.pump();

    expect(mockService.chatMessages.length, 1);
    expect(mockService.chatMessages.first.message, 'Hello Chat');
  });
}
