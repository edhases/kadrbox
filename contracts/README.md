# contracts/

The protocol the Kadrbox app speaks to a catalog server.

The app ships no parsers and calls no catalog host of ours. The user points it at
a server of their choosing; that server implements this protocol. So the protocol
is the deliverable — nothing here tells anyone how to scrape anything.

## Contents

| Path | What it is |
| --- | --- |
| [openapi.yaml](./openapi.yaml) | Normative machine-readable schema for the four endpoints |
| [PROTOCOL.md](./PROTOCOL.md) | Design rationale, the mandatory client security rules, versioning policy |
| `fixtures/` | Sample payloads, for implementers and for the stub server |
| `conformance/` | The checks a server or client runs to claim conformance |
| `stub/` | A minimal server that speaks the protocol, for development |

`fixtures/`, `conformance/` and `stub/` are maintained separately.

## Endpoints at a glance

| Endpoint | Purpose |
| --- | --- |
| `GET /status` | `protocol_version`, server `app` name, `capabilities` |
| `GET /search?q=` | `segments[]`, flat `items[]`, paging |
| `GET /details?id=` | One item plus `voiceovers`, `seasons`, `episodes` |
| `GET /streams?id=` | `streams[]` and `subtitles[]` |

All `GET`, all JSON. Call `/status` first.

## If you are writing a server

Start with [PROTOCOL.md](./PROTOCOL.md), then implement against
[openapi.yaml](./openapi.yaml). The two things most worth reading before you
start, because clients break on them:

- **`segments` is an array**, always, even with one source. Clients in the field
  predate any second segment you add and cannot be updated.
- **Never ask a client to send you credentials.** `headers` are limited to
  `Referer`, `User-Agent` and `Origin`; a client that forwards `Authorization`
  or `Cookie` is non-conforming, and if yours receives one, stop storing it.

## If you are writing or reviewing a client

The security rules in
[PROTOCOL.md § Client security rules](./PROTOCOL.md#client-security-rules) are
mandatory and are part of the protocol, not implementation advice. Short version:
`http`/`https` only and no private-range destinations; `headers` limited to three
names and never a credential; headers scoped to the exact `url` and not carried
across redirects; unknown `protocol_version` warns and degrades.

## House rules for this directory

- Every host in every example is synthetic — `catalog.example`, `cdn.example`,
  `example-provider`, "Example Provider".
- No real content provider is named here, not even as a counter-example. This
  directory is public and store reviewers read it; a real hostname is evidence.
- `scripts/ci/check_forbidden.sh` scans `contracts/` and fails the build on any
  hit. It is not a review habit, it is the test.
