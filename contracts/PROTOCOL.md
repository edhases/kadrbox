# Kadrbox catalog protocol

Version 1. The normative machine-readable shape is [openapi.yaml](./openapi.yaml);
this document explains why the shape is what it is, and carries the rules a
client must obey regardless of what a server says.

## What this protocol is for

The app ships no parsers and contacts no catalog host of ours. The user enters a
catalog URL in settings, and that server speaks the four endpoints below. We
publish the protocol; somebody else runs the server.

That inversion is the whole reason this document is careful about the client.
A server we do not control sends the app a URL and a set of headers, and the
player obeys them. Every security rule here is therefore written as a **client
obligation**, not as a request to implementers. A conforming client that skips
them is non-conforming even if it talks to a perfectly friendly server, because
it cannot tell the difference.

## Endpoints

All four are `GET`, all return JSON.

| Endpoint | Purpose |
| --- | --- |
| `/status` | Handshake: protocol version, server name, capability flags |
| `/search?q=` | Search and browse, grouped into source segments |
| `/details?id=` | One item with its voiceovers, seasons and episodes |
| `/streams?id=` | Playable locations and subtitles |

The client calls `/status` first. Everything else assumes the connection works.

## Item identity, and why there is no migration for it

`item_id` is assigned by the server and is **not globally unique**. Two catalog
servers can legitimately return the same `item_id` for different content, so
the client scopes every stored reference as `<catalog_id>:<item_id>`.

The column that holds this already exists on both sides and is already part of
every uniqueness key that matters:

```
backend  uq_user_history   UNIQUE NULLS NOT DISTINCT (user_id, media_id, provider_id, season, episode)
backend  uq_user_favorite  UNIQUE(user_id, media_id, provider_id)
client   watch_history     uniqueKeys => [{mediaId, providerId}]
```

So the scoping this protocol needs is already enforced. `catalog_id` is
carried in the existing `provider_id` / `providerId` field.

**A migration adding a separate `catalog_id` column would be actively harmful,
and this is written down so nobody attempts it again.** Every legacy row would
backfill to the same empty catalog value, the new unique key would collapse
rows that are currently distinct, and `ADD CONSTRAINT` would fail against a
populated table — a migration-time outage on the live database. It would also
desync every `ON CONFLICT` upsert target written against the current key.

What remains is only naming: the field called `provider_id` now carries a
catalog id. Renaming it would cost a migration on both sides and change no
behaviour, so it is deferred. Read `provider_id` as "which catalog this came
from" everywhere in this protocol.

## Why `segments` is an array

`/search` returns `segments` as an array of source groups, and `items` as one
flat list across all of them.

The array shape is deliberate and load-bearing. The obvious alternative — an
object keyed by segment id — breaks the moment a server wants to report a
second source: clients shipped before that change either crash on the new key
or, worse, silently render nothing. We cannot force the people running these
servers to update anything, so every response shape has to be readable by
clients that predate it. An array lets a server add a source segment tomorrow
and an old client will skip what it does not recognise and carry on.

Two consequences follow, and servers should honour both:

- An item whose `segment` names an unknown segment id is **skipped**, not fatal.
- A capability string the client does not recognise in `/status` is **ignored**,
  not fatal.

Same reason `MediaKind` is a closed enum: an unknown kind is skipped, whereas an
open string would let a server mint shapes the client has no rendering path for.

`items` is flat rather than nested per segment because the client merges, sorts
and paginates on its own. A nested shape would make global ordering impossible
without another protocol change.

## Client security rules

These are mandatory. They are part of the protocol, not an implementation note,
because the harm lands on the user: a hostile or compromised catalog server can
point the app anywhere and can ask it to send anything it holds.

### 1. `url` scheme and destination

Only `http` and `https` are accepted. `file://`, UNC paths, `ftp://` and every
other scheme are rejected before a socket is opened, and the destination host
must not be loopback, link-local (including the `169.254.169.254` cloud metadata
address), unspecified, multicast, or inside an RFC1918 / RFC4193 private range.

**Why.** The `file://` case is the sharp one: a server returning
`file:///C:/Users/name/…` would otherwise make the player read a local file and
hand its bytes to the player, from where a later request can leak them. The
private-range rule closes the same hole one level up — a server that returned
`http://192.168.1.1/` or a `metadata.google.internal` hostname would be using the
app as a request proxy into the user's own network, which is SSRF with a user's
laptop as the origin.

### 2. `headers` is an allow-list

Only `Referer`, `User-Agent` and `Origin` may be sent. Anything else is dropped
without being transmitted. `Authorization`, `Cookie` and every other client
credential are never forwarded, **even when the server asks for them by name**.

**Why.** A header map is a channel. If the server could name the key, it could
ask for `Authorization` and receive the user's token, or for `Cookie` and
receive the session — turning the catalog server into a credential phisher with
a working redirect. The allow-list is three names because those three are what
a media CDN actually needs; nothing else is worth the surface.

### 3. `headers` scope

Headers returned alongside a `url` apply to that exact URL only. They are not
carried across a redirect, and never applied to a different host.

**Why.** Redirects are the natural way around rule 2. A server returns an
allowed-looking `Referer` next to a URL on `cdn.example`, that URL 302s to an
attacker host, and if the client forwards the header on the hop it has just
handed a warm credential to the attacker. Scoping to the exact URL also means a
`Referer` for the catalog never decorates a request to a third-party CDN, which
is both the leak and a privacy problem.

### 4. Unknown `protocol_version` degrades

If `/status` reports a version the client does not know, the client warns once,
continues with the fields it understands, and drops the rest. It never panics,
never refuses to work, and never clears user state.

**Why.** We do not control the servers and cannot ship a coordinated upgrade.
A client that hard-fails on an unrecognised version hands a bored user a blank
app the moment anyone ships a newer server; a client that ignores the number
outright cannot tell the user which protocol they are on. Warn and degrade is
the only behaviour that survives a server we cannot patch.

## Versioning policy

A breaking change increments `protocol_version`.

The app always advertises, in `/status` handling, what it understands; the
client stays compatible with the previous version for as long as possible.
Servers are run by people we do not control and cannot update, so "everyone
upgrades together" is not available as a strategy — a client has to keep working
against a server that is one version behind and against one that is one version
ahead.

What counts as breaking: removing or renaming a response member, changing a
member's type, turning a required member into an optional one, or changing the
meaning of an existing enum value. What does not: adding an optional member,
adding a new value to an open-ended list (`segments`, `capabilities`), adding a
new endpoint, or adding an optional query parameter.

## Content hygiene

Every host in every example here and in `openapi.yaml` is synthetic:
`catalog.example`, `cdn.example`, provider `example-provider`, server
"Example Provider".

No real content provider is named anywhere in this directory, and there is a CI
gate (`scripts/ci/check_forbidden.sh`) that fails the build if one appears. The
reason is not squeamishness: the app is reviewed by both stores, they read the
source and the screenshots, and a real hostname in a public document is evidence
of what the app points at rather than a warning about it. That is also why the
counter-examples above describe `file://` and `169.254.169.254` abstractly —
naming a real host to say "do not use this" still puts the host in the document.

## Server implementation notes

Beyond the wire format:

- `/status` must stay cheap and unauthenticated. It is the first call after a
  URL is typed, and a slow one reads as a broken app.
- Return `empty arrays, never null`, for `segments`, `items`, `voiceovers`,
  `seasons`, `episodes`, `streams` and `subtitles`. A client that has to
  distinguish "none" from "not answered" will get it wrong on one of them.
- Emit no request headers the protocol does not define, and emit no
  `Authorization` — a catalog server that receives one has been given a
  credential it has no business holding, and the client is right to not send it.
- `next_page` is an opaque token. Clients must not parse it, so servers can move
  from offsets to cursors without a version bump.
