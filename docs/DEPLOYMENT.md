# Deployment

## Requirements

- Mattermost 11.7.0 or newer.
- No external database or service. The plugin uses the Mattermost plugin KV store.
- No special outbound network access; the plugin never fetches saved URLs.

## Install

1. Build or download the bundle:
   `dist/com.github.3tty0n.mattermost-saved-links-<version>.tar.gz`.
2. **System Console → Plugins → Plugin Management → Upload Plugin**.
3. Enable **Saved Links**.
4. Pin the plugin version in your deployment tooling. The app has no settings.

## Upgrade

Upload the new bundle and enable it. Saved-link and folder records and indexes are stored under
the new plugin ID; no database schema change is required for a fresh install. Restarting the
plugin or Mattermost does not clear data. Records written before 0.2 used `title`; current builds
read them as `name` and ignore the old description.

### Migrating from Bookmarks

The Saved Links rename changes both the plugin ID and the KV key prefixes:

| Before | After |
| --- | --- |
| `com.github.3tty0n.mattermost-bookmarks` | `com.github.3tty0n.mattermost-saved-links` |
| `bookmark:<id>` | `saved_link:<id>` |
| `bookmark_team:<id>:<shard>` | `saved_link_team:<id>:<shard>` |
| `bookmark_channel:<id>:<shard>` | `saved_link_channel:<id>:<shard>` |
| `folder:*` | `folder:*` (unchanged) |

The new plugin cannot read the old plugin's namespaced KV rows automatically. Before enabling this
build, either:

- export and re-create the links through the API/UI, or
- copy the rows in the Mattermost plugin KV table (`PluginKeyValueStore`) to the new plugin ID and
  rename the key prefixes as shown above. Back up the database first and verify the row count and
  `PKey`/`PValue` encoding for your Mattermost version. Folder rows keep their existing keys but
  need the new plugin ID.

Disable the old plugin after migration so both builds do not write to different namespaces.

## Rollback

Disable the plugin. Data remains in the plugin KV store under the new plugin ID. Re-enabling the
same or a newer bundle continues to use it. Rolling back to the old Bookmarks plugin ID does not
read this app's data; migrate it back using the reverse of the table above.

## Backup

Saved links and folders live in the Mattermost database's plugin KV table. Use the normal
Mattermost database backup procedure; there is no separate volume or file store.

## Resource use

The plugin has no workers or external calls. A list request reads up to four saved-link index
shards. When the folder rows are visible it also reads up to four folder shards and, if folders
exist, re-reads the scope's saved-link shards to compute counts. Each full record is one KV read.
This makes it suitable for a 1 vCPU / 512 MB app node.
