# HTTP API

All routes are below:

```
<SiteURL>/plugins/com.github.3tty0n.mattermost-bookmarks/api/v1
```

Authentication is the normal Mattermost session cookie. The server adds `Mattermost-User-ID`
after authenticating; the plugin rejects requests without it. Non-GET requests also require the
`X-CSRF-Token` header (the webapp reads `MMCSRF`).

Errors are JSON `{"error":"..."}`. Validation errors return 400, missing or inaccessible records
404, permission failures 403, compare-and-swap conflicts 409, and unexpected failures 500. All
responses set `Cache-Control: no-store` because titles and URLs are private data.

## List bookmarks

`GET /bookmarks?scope=team|channel|all&team_id=...&channel_id=...&q=...&tag=...&page=0&per_page=30`

- `scope=team` requires `team_id`.
- `scope=channel` requires `channel_id`.
- `scope=all` returns every bookmark in teams the user can access.
- `q` searches title, URL and tags; `tag` matches a tag case-insensitively.
- `per_page` defaults to 30 and is capped at 100.

Response:

```json
{
  "items": [
    {
      "id": "26characterid",
      "teamId": "teamid",
      "channelId": "channelid",
      "url": "https://example.com/docs",
      "title": "Docs",
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

## Create bookmark

`POST /bookmarks`

```json
{
  "teamId": "teamid",
  "channelId": "channelid",
  "url": "https://example.com/docs",
  "title": "Docs",
  "description": "Optional Markdown text",
  "tags": ["work", "reference"],
  "sourcePostId": "optionalpostid"
}
```

`teamId` or `channelId` selects the scope; when `channelId` is present the server derives the
team and rejects a mismatched `teamId`. Only `http`/`https` URLs up to 2048 bytes are accepted;
URLs with credentials are rejected. Titles are limited to 200 characters, descriptions to 8000
characters, tags to 20 entries of 50 characters each. A source post must be in an accessible
public/private channel.

Returns 201 with the full bookmark.

## Get bookmark

`GET /bookmarks/{id}`

Returns the full bookmark, including `description` and `version`. Non-members of a private
channel receive 404.

## Update bookmark

`PATCH /bookmarks/{id}` or `POST /bookmarks/{id}`

The POST alias exists because some reverse proxies reject PATCH. The body accepts any subset of:

```json
{
  "teamId": "teamid",
  "channelId": "channelid",
  "url": "https://example.com/new",
  "title": "New title",
  "description": "New description",
  "tags": ["updated"],
  "sourcePostId": "postid"
}
```

An empty object is rejected. Metadata edits require the creator or a system administrator with
current scope access. If `channelId` is supplied, the server resolves and validates its team.

Returns 200 with the updated full bookmark.

## Delete bookmark

`DELETE /bookmarks/{id}`

Requires the creator or a system administrator. Returns 204. The record and its index entries are
removed.

## WebSocket

The server publishes `custom_com.github.3tty0n.mattermost-bookmarks_changed` to the bookmark's
channel (or team for a team-level bookmark) after create/update/delete. The payload contains the
entity name, action, bookmark ID, team ID and channel ID; it never contains the URL or title.
