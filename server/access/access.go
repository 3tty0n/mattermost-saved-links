// Package access centralizes the Mattermost permission checks used by saved links and folders, so
// every entry point enforces the same team/channel rules.
package access

import (
	"errors"
	"fmt"

	"github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/pluginapi"

	savedlinkmodel "github.com/3tty0n/mattermost-saved-links/server/model"
)

// Scope is a resolved team/channel pair. ChannelID is empty for team-level records.
type Scope struct {
	TeamID    string
	ChannelID string
}

// Checker wraps a plugin API client.
type Checker struct {
	client *pluginapi.Client
}

// NewChecker returns a Checker for the given client.
func NewChecker(client *pluginapi.Client) *Checker {
	return &Checker{client: client}
}

// CanViewTeam reports whether the user may see team-level records.
func (c *Checker) CanViewTeam(userID, teamID string) bool {
	return c.client.User.HasPermissionToTeam(userID, teamID, model.PermissionViewTeam)
}

// CanViewChannel reports whether the user may see records in a channel.
func (c *Checker) CanViewChannel(userID, channelID string) bool {
	return c.client.User.HasPermissionToChannel(userID, channelID, model.PermissionReadChannel)
}

// CanView reports access to a stored team/channel pair without fetching metadata.
func (c *Checker) CanView(userID, teamID, channelID string) bool {
	if channelID != "" {
		return c.CanViewChannel(userID, channelID)
	}
	if teamID == "" {
		return false
	}
	return c.CanViewTeam(userID, teamID)
}

// IsAdmin reports whether the user has the system administration permission.
func (c *Checker) IsAdmin(userID string) bool {
	return c.client.User.HasPermissionTo(userID, model.PermissionManageSystem)
}

// Resolve validates a requested scope, normalizes the team from the channel, and checks that the
// user may create or view saved links in it. Only open and private channels are valid scopes.
func (c *Checker) Resolve(userID, teamID, channelID string) (*Scope, error) {
	if teamID != "" && !model.IsValidId(teamID) {
		return nil, fmt.Errorf("%w: invalid team", savedlinkmodel.ErrInvalid)
	}
	if channelID != "" && !model.IsValidId(channelID) {
		return nil, fmt.Errorf("%w: invalid channel", savedlinkmodel.ErrInvalid)
	}

	if channelID == "" {
		if teamID == "" {
			return nil, fmt.Errorf("%w: a team or channel is required", savedlinkmodel.ErrInvalid)
		}
		if _, err := c.getTeam(teamID); err != nil {
			return nil, err
		}
		if !c.CanViewTeam(userID, teamID) {
			return nil, savedlinkmodel.ErrForbidden
		}
		return &Scope{TeamID: teamID}, nil
	}

	channel, err := c.getChannel(channelID)
	if err != nil {
		return nil, err
	}
	if channel.Type != model.ChannelTypeOpen && channel.Type != model.ChannelTypePrivate {
		return nil, fmt.Errorf("%w: saved links can only belong to public or private channels", savedlinkmodel.ErrInvalid)
	}
	if teamID != "" && channel.TeamId != teamID {
		return nil, fmt.Errorf("%w: channel does not belong to the team", savedlinkmodel.ErrInvalid)
	}
	if !c.CanViewChannel(userID, channelID) {
		return nil, savedlinkmodel.ErrForbidden
	}
	return &Scope{TeamID: channel.TeamId, ChannelID: channel.Id}, nil
}

// ResolvePost loads a post and the scope it belongs to, checking read access.
func (c *Checker) ResolvePost(userID, postID string) (*Scope, error) {
	if !model.IsValidId(postID) {
		return nil, fmt.Errorf("%w: invalid post", savedlinkmodel.ErrInvalid)
	}
	post, err := c.client.Post.GetPost(postID)
	if err != nil {
		if errors.Is(err, pluginapi.ErrNotFound) {
			return nil, savedlinkmodel.ErrNotFound
		}
		return nil, fmt.Errorf("failed to get source post: %w", err)
	}
	if post.DeleteAt > 0 {
		return nil, savedlinkmodel.ErrNotFound
	}
	channel, err := c.getChannel(post.ChannelId)
	if err != nil {
		return nil, err
	}
	if channel.Type != model.ChannelTypeOpen && channel.Type != model.ChannelTypePrivate {
		return nil, fmt.Errorf("%w: posts in direct messages can not be saved as saved links", savedlinkmodel.ErrInvalid)
	}
	if !c.CanViewChannel(userID, channel.Id) {
		return nil, savedlinkmodel.ErrForbidden
	}
	return &Scope{TeamID: channel.TeamId, ChannelID: channel.Id}, nil
}

func (c *Checker) getTeam(teamID string) (*model.Team, error) {
	team, err := c.client.Team.Get(teamID)
	if err != nil {
		if errors.Is(err, pluginapi.ErrNotFound) {
			return nil, savedlinkmodel.ErrNotFound
		}
		return nil, fmt.Errorf("failed to get team: %w", err)
	}
	return team, nil
}

func (c *Checker) getChannel(channelID string) (*model.Channel, error) {
	channel, err := c.client.Channel.Get(channelID)
	if err != nil {
		if errors.Is(err, pluginapi.ErrNotFound) {
			return nil, savedlinkmodel.ErrNotFound
		}
		return nil, fmt.Errorf("failed to get channel: %w", err)
	}
	return channel, nil
}
