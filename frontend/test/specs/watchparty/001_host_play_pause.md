Title: Host play/pause command is broadcasted and clients update playback state

Given:
- A WatchPartyService connected to a fake backend
- Participants: Host (isHost=true) and Client (isHost=false) both connected
- Clients have Mock PlayerControllers or spies to observe play/pause calls

When:
- Host triggers play action (or sends play message via service)

Mocks/Helpers:
- Fake backend that can broadcast messages to connected clients
- Mock PlayerController for client to receive play/pause commands

Expectations:
- Clients receive broadcast message and their PlayerControllers call play()
- WatchPartyService internal state reflects playback isPlaying=true
- Logs or metrics show successful broadcast; no unhandled exceptions

Notes:
- Repeat for pause action and ensure toggles are consistent even if several hosts send commands rapidly
