# Architecture

This document describes the standalone Saved Links plugin. It is designed for a small Mattermost
deployment (one app node, 1 vCPU, 512 MB RAM) and uses only public plugin APIs.

## Components

```
server/
  model/        SavedLink and folder records, compact index refs, validation, query rules
  store/        KV abstraction + indexed plugin-KV implementation
  access/       team/channel/system permission checks
  savedlinks/   saved-link and folder use cases and audit records
  api/          REST routing, JSON decoding, error mapping
  events/       WebSocket change events (IDs only)
  testutil/     shared test API wrapper
webapp/
  src/          RHS panel, inline folders/editor, saved-link list/editor, client and selectors
```

The plugin has no bot, no slash command and no background workers, so it has no per-node
processes beyond the plugin itself.

## Storage model

- Full saved-link record: `saved_link:<id>` (JSON). It includes the URL, name, tags, folder,
  position, creator, source post, timestamps and a compare-and-swap version.
- Full folder record: `folder:<id>` (JSON), with its team/channel scope, name, creator, timestamps
  and version.
- Saved-link indexes: `saved_link_team:<teamID>:<shard>` and
  `saved_link_channel:<channelID>:<shard>`.
- Folder indexes: `folder_team:<teamID>:<shard>` and `folder_channel:<channelID>:<shard>`.
- `shard = fnv32a(recordID) % 4`. Four shards keep each index value small and reduce
  compare-and-swap contention.
- Index references contain only fields needed for list views. A saved-link ref carries ID,
  team/channel, folder ID, position, URL, name, tags, creator, source post, timestamps and version.
  A folder ref carries ID, team/channel, name, creator, timestamps and version. Folder lists count
  links by reading the containing scope's saved-link refs.
- Concurrent updates use `CompareAndSet` retries plus in-process striped locks. Index updates use
  `KV.Update` and are skipped when only non-indexed fields change.
- Deleting a folder first clears its links' folder IDs (each through a normal saved-link update),
  then removes the folder. The links themselves are kept.
- A failed index update after a record write is reported; the caller can retry. Index writes are
  idempotent and version-guarded.

Search and folder filters are applied after reading the index shards for the requested scope.
This is bounded by the number of links in that team/channel and avoids maintaining a search
index. The maximum page size is 100.

Folder names are unique within their scope (case-insensitive) and a scope can hold at most 100
folders, which keeps lists and index shards bounded. Saved-link names are optional: the service
derives one from the URL when the client sends an empty name, so saving never requires a metadata
fetch.

## Ordering

Every saved link has an integer `position`. Lists sort by position ascending, then by creation time
for records that share a position. New links and links moved to another folder/scope receive the
current time as their position, which appends them to the end of their group.

`POST /saved-links/reorder` assigns positions `1000, 2000, …` to the supplied IDs. The endpoint
requires every ID to belong to the same team/channel and folder, so a reorder request cannot mix
folders. Any member with view access to that group may reorder it. The webapp only enables sorting
when one group is visible, no search filter is active, and all records fit on one page; the limits
keep the request under the 100-item cap.

## Permissions

Every route resolves the actor from the `Mattermost-User-ID` header, which the Mattermost server
sets after authenticating the session. The plugin then checks `PermissionViewTeam` or
`PermissionReadChannel` on the current scope, and re-checks the source post's channel when saving
from a post. Private-channel records are filtered from lists and return 404 to non-members.

Any member with scope access may create saved links and folders. The creator or a system
administrator may edit link metadata or folders and delete links or folders. Sorting only changes
the shared order, so any member with view access may do it.

A saved link can only reference a folder from its own scope. Moving a link to another scope drops
its folder unless the patch explicitly names a folder from the new scope.

## Webapp

- `registerRightHandSidebarComponent` provides the panel and the link-icon channel header button.
- At the root, folders are rendered as rows above ungrouped saved links. Clicking a folder opens a
  drill-in view with a back button, rename/delete actions and a link count; there is no folder
  dropdown. Search inside the root switches to a flat, folder-labelled result list; inside a
  folder it searches that folder only.
- Drag sorting uses native HTML5 drag events on a per-row handle. The handle is a button, so it can
  also be focused and moved with the arrow keys. The client sends the entire visible group to the
  reorder endpoint and refetches the list afterwards.
- The saved-link editor has an optional **Name**, a **Folder** select for the chosen scope, and no
  description field. Leaving the name empty makes the server derive it from the URL.
- `registerPostDropdownMenuAction` adds **Save link** for public/private channel posts containing
  `http(s)` links.
- The webapp uses the host's Redux store and local selectors rather than bundling Mattermost
  redux. CSRF uses the `MMCSRF` cookie and `X-CSRF-Token` header; requests include credentials.
- WebSocket `custom_<plugin-id>_changed` and reconnect events bump a revision counter that
  refetches the visible list and folders.
- All data is sanitized by React; URLs are rendered as links only after server-side scheme and
  credential validation.

## Performance

- No full KV scans: each list reads at most `4` shards for a team or channel, plus `4` shards for
  the folder list.
- Folder link counts reuse the same scope index read, so there is no per-folder query.
- The list response is bounded to 100 per page; the webapp loads up to 100 at a time so drag sorting
  can address the whole group.
- No outbound URL fetching: saving a link never waits on a third-party site.
- The webapp bundle is small and uses Compass components already loaded by the host.

## Limitations

- Mobile clients do not load plugin webapps, so there is no mobile UI.
- Saved links and folders are shared by team/channel; per-user private links and folders are not
  implemented.
- Folder deletion and saved-link updates are separate KV operations, so a link created concurrently
  with the deletion could briefly reference a removed folder. The UI omits the folder label for
  unknown folder IDs, so the link appears ungrouped.
- The plugin ID and KV key prefixes changed with the Saved Links rename. Data under
  `com.github.3tty0n.mattermost-bookmarks` is not read by this plugin; migrate or recreate it. See
  [DEPLOYMENT.md](DEPLOYMENT.md).
