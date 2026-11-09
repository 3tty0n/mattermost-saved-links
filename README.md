# Saved Links for Mattermost

A standalone Mattermost plugin for collecting and finding URL links in teams and channels,
organized into folders. It uses Mattermost's existing teams, channels, posts and permissions, and
stores everything in the plugin's own KV store. There is no external service, no separate database
and no URL fetching.

- Team-level and channel-level saved links.
- **Folders** live directly in the saved-link list. Click a folder to open it, then use the back
  button to return to the root. Renaming a folder or deleting it never deletes its links; deleting
  a folder makes its links ungrouped.
- **Save link** in any public or private channel post menu; a post with several links shows a URL
  picker.
- A saved link needs only a URL. The name is optional: when it is empty the plugin derives a
  readable name from the URL, so there is no page fetch and no third-party request.
- **Drag to sort**: drag the handle on any row, or focus it and use the arrow keys. New and moved
  links are appended to the end of their folder.
- Tags and search by name, URL or tag.
- A dedicated right-hand sidebar and link icon in the channel header.
- Private-channel saved links and folders are only visible to members; inaccessible records return
  404.
- Server-side permission checks on every route.
- No outbound HTTP, so there is no SSRF surface.

The target server is the oldest supported ESR, **Mattermost 11.7.0**.

## Installation

1. Download the release bundle (`com.github.3tty0n.mattermost-saved-links-*.tar.gz`).
2. In Mattermost, go to **System Console → Plugins → Plugin Management**.
3. Upload the bundle and enable **Saved Links**.

There is no external service or separate database to install. The plugin KV store lives in the
Postgres/MySQL database Mattermost already uses.

## Usage

### Saved Links panel

Click the **link icon** in the channel header to open the Saved Links sidebar (it is deliberately
not the bookmark icon used by Mattermost's Saved items).

- Choose **Team**, **Channel** or **All**. Team shows every saved link in the current team; Channel
  shows the current channel; All is a flat overview of every saved link you can access.
- In Channel and Team views, the root list contains folder rows followed by ungrouped saved links.
  Click a folder row to see its links. The folder header has a back button plus rename and delete
  actions, and its count updates after links are added or removed.
- Create a folder with the folder-plus button in the toolbar. Folders are sorted by name.
- Search searches the current scope. At the root, matching links inside folders are shown with a
  folder label and folder rows are hidden; inside a folder, search is limited to that folder.
- The **+** button creates a link in the current folder (or ungrouped at the root). A URL is
  required; the name can be left blank and is derived from the URL.
- Use the drag handle or arrow keys on a focused handle to sort links. Sorting is available when
  the list shows one folder (or ungrouped links in one channel), the search box is empty, and all
  items fit on one page. New and moved links go to the end.
- Edit or delete a link from its row. Deleting asks for confirmation.

### Saved links from posts

Every post in a public or private channel has **Save link** in its **⋯** menu. The editor opens
prefilled with the first link and the post's first line as the name. A post with several links
shows a URL picker. The saved link keeps the source post ID and links back to the original post.
The server re-checks the post's channel and the user's read permission before saving.

DMs and group messages are not saved-link scopes, because a saved link is shared through a team or
channel index.

### Permissions

| Operation | Who |
| --- | --- |
| View saved links and folders | Members who can read the team or channel |
| Create a saved link or folder in a team/channel | Members who can read that scope; posts require channel read access |
| Edit saved-link URL, name, tags, scope, folder | Creator or system administrator, with current scope access |
| Delete a saved link | Creator or system administrator |
| Rename or delete a folder | Creator or system administrator |
| Drag-sort saved links | Any member who can view the list |

A saved link can only be placed in a folder of its own scope. Moving a link to another channel or
team drops its folder. Private channel records are filtered out of list responses and return 404
to non-members, so their existence is not disclosed. Hiding UI is never the check; the server
enforces each rule.

## Development

Requirements: Go 1.26+, Node.js 22+, npm, Docker for integration tests.

```bash
make apply                  # generate manifests from plugin.json
make test                   # server + webapp tests and the bundle
SKIP_DOCKER_TESTS=1 go test ./server/...
cd webapp && npx jest --runInBand
```

The plugin bundle is written to
`dist/com.github.3tty0n.mattermost-saved-links-<version>.tar.gz`.

## Storage

Saved links are stored under `saved_link:<id>` with four compact index shards per team/channel
(`saved_link_team:<id>:<shard>` and `saved_link_channel:<id>:<shard>`). Folders use `folder:<id>`,
`folder_team:<id>:<shard>` and `folder_channel:<id>:<shard>`. Each record carries its manual
`position` for drag sorting. Lists read the index and apply search and folder filters in memory, so
no query depends on a full KV scan. See [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) and
[docs/API.md](docs/API.md).
