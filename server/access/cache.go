package access

// PermissionCache memoizes team and channel checks for one list request. A team index can contain
// many bookmarks from the same few channels, so caching avoids repeating the same server
// permission lookup for every ref while still evaluating changes on the next request.
type PermissionCache struct {
	checker  *Checker
	teams    map[string]bool
	channels map[string]bool
}

// Cache starts a request-scoped permission cache.
func (c *Checker) Cache() *PermissionCache {
	return &PermissionCache{
		checker:  c,
		teams:    make(map[string]bool),
		channels: make(map[string]bool),
	}
}

// Seed records an already-checked result.
func (p *PermissionCache) Seed(teamID, channelID string, allowed bool) {
	if channelID != "" {
		p.channels[channelID] = allowed
		return
	}
	if teamID != "" {
		p.teams[teamID] = allowed
	}
}

// CanViewTeam is the cached form of Checker.CanViewTeam.
func (p *PermissionCache) CanViewTeam(userID, teamID string) bool {
	if allowed, ok := p.teams[teamID]; ok {
		return allowed
	}
	allowed := p.checker.CanViewTeam(userID, teamID)
	p.teams[teamID] = allowed
	return allowed
}

// CanViewChannel is the cached form of Checker.CanViewChannel.
func (p *PermissionCache) CanViewChannel(userID, channelID string) bool {
	if allowed, ok := p.channels[channelID]; ok {
		return allowed
	}
	allowed := p.checker.CanViewChannel(userID, channelID)
	p.channels[channelID] = allowed
	return allowed
}

// CanView is the cached form of Checker.CanView.
func (p *PermissionCache) CanView(userID, teamID, channelID string) bool {
	if channelID != "" {
		return p.CanViewChannel(userID, channelID)
	}
	if teamID == "" {
		return false
	}
	return p.CanViewTeam(userID, teamID)
}
