# Architecture

This document describes the standalone Bookmarks plugin. It is designed for a small Mattermost
deployment (one app node, 1 vCPU, 512 MB RAM) and uses only public plugin APIs.

## Components

```
server/
  model/        Bookmark record, compact index reference, validation, query rules
  store/        KV abstraction + indexed plugin-KV implementation
  access/       team/channel/system permission checks
  bookmarks/    bookmark use cases and audit records
  api/          REST routing, JSON decoding, error mapping
  events/       WebSocket change events (IDs only)
  testutil/     shared test API wrapper
webapp/
  src/          RHS panel, bookmark list/editor, client and local selectors
```

The plugin has no bot, no slash command and no background workers, so it has no per-node
processes beyond the plugin itself.

## Storage model

- Full record: `bookmark:<id>` (JSON). It includes the URL, title, description, tags, creator,
  source post, timestamps and a compare-and-swap version.
- Index reference: `bookmark_team:<teamID>:<shard>` and
  `bookmark_channel:<channelID>:<shard>`.
- `shard = fnv32a(bookmarkID) % 4`. Four shards keep each index value small and reduce
  compare-and-swap contention.
- References contain only fields needed by the list: ID, team/channel, URL, title, tags, creator,
  source post, timestamps and version. They omit the full description, which keeps the index
  compact.
- Concurrent updates use `CompareAndSet` retries plus in-process striped locks. Index updates use
  `KV.Update` and are skipped when only non-indexed fields change.
- A failed index update after the record write is reported; the caller can retry. Index writes are
  idempotent and version-guarded.

Search is applied after reading the index shards for the requested scope. This is bounded by the
number of bookmarks in that team/channel and avoids maintaining a search index. The maximum page
size is 100.

## Permissions

Every route resolves the actor from the `Mattermost-User-ID` header, which the Mattermost server
sets after authenticating the session. The plugin then checks `PermissionViewTeam` or
`PermissionReadChannel` on the current scope, and re-checks the source post's channel when saving
from a post. Private-channel records are filtered from lists and return 404 to non-members.

The creator or a system administrator may edit bookmark metadata and delete bookmarks. Any
member with current scope access may view them.

## Webapp

- `registerRightHandSidebarComponent` provides the panel and the link-icon channel header button.
- `registerPostDropdownMenuAction` adds **Save URL as bookmark** for public/private channel posts
  containing `http(s)` links.
- The webapp uses the host's Redux store and local selectors rather than bundling Mattermost
  redux. CSRF uses the `MMCSRF` cookie and `X-CSRF-Token` header; requests include credentials.
- WebSocket `custom_<plugin-id>_changed` and reconnect events bump a revision counter that
  refetches the visible list.
- All data is sanitized by React; URLs are rendered as links only after server-side scheme and
  credential validation.

## Performance

- No full KV scans: each list reads at most `4` shards for a team or channel.
- The list response is bounded to 100 per page; the webapp loads 30 by default.
- No outbound URL fetching: saving a bookmark never waits on a third-party site.
- The webapp bundle is small and uses Compass components already loaded by the host.

## Limitations

- Mobile clients do not load plugin webapps, so there is no mobile UI.
- No automatic import from the previous combined Tasks & Bookmarks plugin; this app has its own
  plugin ID and KV namespace.
- Bookmarks are shared by team/channel; per-user private bookmarks are not implemented.
