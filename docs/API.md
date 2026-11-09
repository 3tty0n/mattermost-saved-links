# HTTP API

All routes are below:

```
<SiteURL>/plugins/com.github.3tty0n.mattermost-saved-links/api/v1
```

Authentication is the normal Mattermost session cookie. The server adds `Mattermost-User-ID`
after authenticating; the plugin rejects requests without it. Non-GET requests also require the
`X-CSRF-Token` header (the webapp reads `MMCSRF`).

Errors are JSON `{"error":"..."}`. Validation errors return 400, missing or inaccessible records
404, permission failures 403, compare-and-swap conflicts 409, and unexpected failures 500. All
responses set `Cache-Control: no-store` because names and URLs are private data.

## List saved links

`GET /saved-links?scope=team|channel|all&team_id=...&channel_id=...&folder_id=...&q=...&tag=...&page=0&per_page=30`

- `scope=team` requires `team_id`.
- `scope=channel` requires `channel_id`.
- `scope=all` returns every saved link in teams the user can access.
- `folder_id` limits the result to one folder. Use `folder_id=none` for links that are not in any
  folder. Omit it to include every folder.
- `q` searches name, URL and tags; `tag` matches a tag case-insensitively.
- `per_page` defaults to 30 and is capped at 100.

Results are ordered by `position` ascending, then by creation time for records that share a
position (legacy records default to position 0).

Response:

```json
{
  "items": [
    {
      "id": "26characterid",
      "teamId": "teamid",
      "channelId": "channelid",
      "folderId": "folderid",
      "position": 1000,
      "url": "https://example.com/docs",
      "name": "Docs",
      "tags": ["work"],
      "creatorId": "userid",
      "sourcePostId": "postid",
      "createdAt": 1700000000000,
      "updatedAt": 1700000000000
    }
  ],
  "total": 1,
  "page": 0,
  "perPage": 30,
  "hasMore": false
}
```

## Create saved link

`POST /saved-links`

```json
{
  "teamId": "teamid",
  "channelId": "channelid",
  "folderId": "optionalfolderid",
  "url": "https://example.com/docs",
  "name": "Optional name",
  "tags": ["work", "reference"],
  "sourcePostId": "optionalpostid"
}
```

`teamId` or `channelId` selects the scope; when `channelId` is present the server derives the
team and rejects a mismatched `teamId`. Only `http`/`https` URLs up to 2048 bytes are accepted;
URLs with credentials are rejected. Names are limited to 200 characters and tags to 20 entries of
50 characters each. A source post must be in an accessible public/private channel.

`name` is optional. When it is empty the server derives a readable name from the URL (the last
path segment, or the host when the path is empty). A saved link can therefore be created with a URL
alone. `folderId` is optional; a folder must belong to the saved link's resolved team/channel.
The new link gets a position after the existing links in its folder.

Returns 201 with the full saved link.

## Get saved link

`GET /saved-links/{id}`

Returns the full saved link, including `folderId`, `position`, `name` and `version`. Non-members of
a private channel receive 404.

## Update saved link

`PATCH /saved-links/{id}` or `POST /saved-links/{id}`

The POST alias exists because some reverse proxies reject PATCH. The body accepts any subset of:

```json
{
  "teamId": "teamid",
  "channelId": "channelid",
  "folderId": "folderid",
  "url": "https://example.com/new",
  "name": "New name",
  "tags": ["updated"],
  "sourcePostId": "postid"
}
```

An empty object is rejected. Metadata edits require the creator or a system administrator with
current scope access. If `channelId` is supplied, the server resolves and validates its team.
Sending `"folderId": ""` moves the saved link out of its folder. If the scope changes and no folder
is sent, the old folder is dropped. An explicitly sent folder must belong to the new scope. Moving
a saved link to another folder or scope appends it to the new group.

Returns 200 with the updated full saved link.

## Reorder saved links

`POST /saved-links/reorder`

```json
{"savedLinkIds": ["id1", "id2", "id3"]}
```

Saves a new manual order for the listed links. All IDs must belong to the same team/channel and
folder group, and the request may contain at most 100 IDs. Any member who can view the group may
reorder it. The server assigns a new position to each listed link in the supplied order; links not
listed keep their positions. Send the entire visible group for a predictable result.

Returns 204. The server publishes one `reordered` change event for the group.

## Delete saved link

`DELETE /saved-links/{id}`

Requires the creator or a system administrator. Returns 204. The record and its index entries are
removed.

## List folders

`GET /folders?channel_id=...` or `GET /folders?team_id=...`

Exactly one scope is required: `channel_id` for a channel folder list, or `team_id` for
team-level folders. Results are sorted by name.

```json
{
  "items": [
    {
      "id": "26characterid",
      "teamId": "teamid",
      "channelId": "channelid",
      "name": "Work",
      "creatorId": "userid",
      "savedLinkCount": 3,
      "createdAt": 1700000000000,
      "updatedAt": 1700000000000
    }
  ],
  "total": 1
}
```

`savedLinkCount` is computed from the compact saved-link index for the folder's scope.

## Create folder

`POST /folders`

```json
{
  "teamId": "teamid",
  "channelId": "channelid",
  "name": "Work"
}
```

`channelId` creates a channel folder and derives its team; otherwise `teamId` creates a team-level
folder. Names are required, limited to 100 characters, and unique within the scope
(case-insensitively). A scope can have at most 100 folders. Returns 201 with the full folder.

## Get folder

`GET /folders/{id}`

Returns the full folder, including `version`. Non-members of a private channel receive 404.

## Update folder

`PATCH /folders/{id}` or `POST /folders/{id}`

```json
{"name": "Reference"}
```

Only the name can change. Requires the creator or a system administrator with current scope
access. Names follow the same validation and uniqueness rules as creation. Returns 200 with the
updated folder.

## Delete folder

`DELETE /folders/{id}`

Requires the creator or a system administrator. Returns 204. Saved links in the folder are kept and
become ungrouped; the folder record and its index entries are removed.

## WebSocket

The server publishes
`custom_com.github.3tty0n.mattermost-saved-links_changed` to the affected channel (or team for a
team-level record) after create/update/delete/reorder. The payload contains the entity
(`savedLink` or `folder`), action, record ID (`id`, plus `savedLinkId`/`folderId`), team ID and
channel ID; it never contains the URL or name.
