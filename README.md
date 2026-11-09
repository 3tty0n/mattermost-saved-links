# Bookmarks for Mattermost

A standalone Mattermost plugin for collecting and finding URL bookmarks in teams and channels.
It uses Mattermost's existing teams, channels, posts and permissions, and stores everything in the
plugin's own KV store. There is no external service, no separate database and no URL fetching.

- Team-level and channel-level bookmarks.
- **Save URL as bookmark** in any public or private channel post menu; a post with several links
  shows a URL picker.
- Tags, title, description and search by title, URL or tag.
- A dedicated right-hand sidebar and link icon in the channel header.
- Private-channel bookmarks are only visible to members; inaccessible bookmarks return 404.
- Server-side permission checks on every route.
- No outbound HTTP, so there is no SSRF surface.

The target server is the oldest supported ESR, **Mattermost 11.7.0**.

## Installation

1. Download the release bundle (`com.github.3tty0n.mattermost-bookmarks-*.tar.gz`).
2. In Mattermost, go to **System Console → Plugins → Plugin Management**.
3. Upload the bundle and enable **Bookmarks**.

There is no external service or separate database to install. The plugin KV store lives in the
Postgres/MySQL database Mattermost already uses.

## Usage

### Bookmarks panel

Click the **link icon** in the channel header to open the Bookmarks sidebar (it is deliberately
not the bookmark icon used by Mattermost's Saved items).

- Choose **Team**, **Channel** or **All**. Team shows every bookmark in the current team; Channel
  shows the current channel; All shows every bookmark you can access.
- Search title, URL and tags; click a tag to filter by it.
- The **+** button creates a bookmark manually.
- Edit or delete a bookmark from its row. Deleting asks for confirmation.

### Bookmarks from posts

Every post in a public or private channel has **Save URL as bookmark** in its **⋯** menu. The
editor opens prefilled with the first link, the post's first line as the title and the post body
as the description. A post with several links shows a URL picker. The bookmark keeps the source
post ID and links back to the original post. The server re-checks the post's channel and the
user's read permission before saving.

DMs and group messages are not bookmark scopes, because a bookmark is shared through a team or
channel index.

### Permissions

| Operation | Who |
| --- | --- |
| View bookmarks | Members who can read the team or channel |
| Create in a team/channel | Members who can read that scope; posts require channel read access |
| Edit URL, title, description, tags, scope | Creator or system administrator, with current scope access |
| Delete | Creator or system administrator |

Private channel bookmarks are filtered out of list responses and return 404 to non-members, so
their existence is not disclosed. Hiding UI is never the check; the server enforces each rule.

## Development

Requirements: Go 1.26+, Node.js 22+, npm, Docker for integration tests.

```bash
make apply                  # generate manifests from plugin.json
make test                   # server + webapp tests and the bundle
SKIP_DOCKER_TESTS=1 go test ./server/...
cd webapp && npx jest --runInBand
```

The plugin bundle is written to
`dist/com.github.3tty0n.mattermost-bookmarks-<version>.tar.gz`.

## Storage

Bookmarks are stored under `bookmark:<id>` with four compact index shards per team/channel
(`bookmark_team:<id>:<shard>` and `bookmark_channel:<id>:<shard>`). Lists read the index and apply
search filters in memory, so no query depends on a full KV scan. See
[docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) and [docs/API.md](docs/API.md).
