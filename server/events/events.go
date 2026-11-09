// Package events publishes saved link and folder change notifications. Payloads carry IDs only:
// every recipient of a WebSocket event can read its payload, so saved link URLs and names never
// leave the server this way.
package events

import (
	"github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/pluginapi"

	savedlinkmodel "github.com/3tty0n/mattermost-saved-links/server/model"
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

// SavedLinkChanged notifies the saved link's channel or team.
func (p *Publisher) SavedLinkChanged(action string, savedLink *savedlinkmodel.SavedLink) {
	p.publish(action, "savedLink", savedLink.ID, savedLink.TeamID, savedLink.ChannelID, map[string]any{
		"savedLinkId": savedLink.ID,
	})
}

// FolderChanged notifies the folder's channel or team.
func (p *Publisher) FolderChanged(action string, folder *savedlinkmodel.Folder) {
	p.publish(action, "folder", folder.ID, folder.TeamID, folder.ChannelID, map[string]any{
		"folderId": folder.ID,
	})
}

func (p *Publisher) publish(action, entity, id, teamID, channelID string, extra map[string]any) {
	broadcast := &model.WebsocketBroadcast{TeamId: teamID}
	if channelID != "" {
		broadcast = &model.WebsocketBroadcast{ChannelId: channelID}
	}
	payload := map[string]any{
		"entity":    entity,
		"action":    action,
		"id":        id,
		"teamId":    teamID,
		"channelId": channelID,
	}
	for key, value := range extra {
		payload[key] = value
	}
	p.client.Frontend.PublishWebSocketEvent(Event, payload, broadcast)
}
