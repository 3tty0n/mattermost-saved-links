# Deployment

## Requirements

- Mattermost 11.7.0 or newer.
- No external database or service. The plugin uses the Mattermost plugin KV store.
- No special outbound network access; the plugin never fetches bookmarked URLs.

## Install

1. Build or download the bundle:
   `dist/com.github.3tty0n.mattermost-bookmarks-<version>.tar.gz`.
2. **System Console → Plugins → Plugin Management → Upload Plugin**.
3. Enable **Bookmarks**.
4. Pin the plugin version in your deployment tooling. The app has no settings.

## Upgrade

Upload the new bundle and enable it. Bookmark records and indexes are stored under the plugin ID;
no database migration is required. Restarting the plugin or Mattermost does not clear data.

## Rollback

Disable the plugin. Data remains in the plugin KV store. Re-enabling the same or a newer bundle
continues to use it. Rolling back to a completely different plugin ID (for example the previous
combined Tasks & Bookmarks app) does not read this app's data.

## Backup

Bookmarks live in the Mattermost database's plugin KV table. Use the normal Mattermost database
backup procedure; there is no separate volume or file store.

## Resource use

The plugin has no workers or external calls. List requests read up to four index shards and the
full record is one KV read, making it suitable for a 1 vCPU / 512 MB app node.
