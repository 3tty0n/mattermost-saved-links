// Package events publishes bookmark change notifications. Payloads carry IDs only: every
// recipient of a WebSocket event can read its payload, so bookmark URLs never leave the server
// this way.
package events

import (
	"github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/pluginapi"

	bookmarkmodel "github.com/3tty0n/mattermost-bookmarks/server/model"
)

// Event is the name the webapp registers as custom_<plugin-id>_changed.
const Event = "changed"

// Publisher sends change events to the affected Mattermost scope.
type Publisher struct {
	client *pluginapi.Client
}

// New returns a Publisher backed by the plugin API.
func New(client *pluginapi.Client) *Publisher {
	return &Publisher{client: client}
}

// BookmarkChanged notifies the bookmark's channel or team.
func (p *Publisher) BookmarkChanged(action string, bookmark *bookmarkmodel.Bookmark) {
	broadcast := &model.WebsocketBroadcast{TeamId: bookmark.TeamID}
	if bookmark.ChannelID != "" {
		broadcast = &model.WebsocketBroadcast{ChannelId: bookmark.ChannelID}
	}
	p.client.Frontend.PublishWebSocketEvent(Event, map[string]any{
		"entity":     "bookmark",
		"action":     action,
		"bookmarkId": bookmark.ID,
		"teamId":     bookmark.TeamID,
		"channelId":  bookmark.ChannelID,
	}, broadcast)
}
