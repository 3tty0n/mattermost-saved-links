package main

import (
	"net/http"
	"sync"

	"github.com/mattermost/mattermost/server/public/plugin"
	"github.com/mattermost/mattermost/server/public/pluginapi"

	"github.com/3tty0n/mattermost-bookmarks/server/access"
	"github.com/3tty0n/mattermost-bookmarks/server/api"
	"github.com/3tty0n/mattermost-bookmarks/server/bookmarks"
	"github.com/3tty0n/mattermost-bookmarks/server/events"
	"github.com/3tty0n/mattermost-bookmarks/server/store/kvstore"
)

// Plugin implements the interface expected by the Mattermost server.
type Plugin struct {
	plugin.MattermostPlugin

	client      *pluginapi.Client
	store       kvstore.Store
	bookmarks   *bookmarks.Service
	httpHandler *api.Handler

	configurationLock sync.RWMutex
	configuration     *configuration
}

// OnActivate wires the plugin. Failure here deactivates the plugin.
func (p *Plugin) OnActivate() error {
	p.client = pluginapi.NewClient(p.API, p.Driver)
	checker := access.NewChecker(p.client)
	p.store = kvstore.NewStore(p.client)
	p.bookmarks = bookmarks.NewService(p.client, p.store, checker, events.New(p.client))
	p.httpHandler = api.NewHandler(p.API, p.client, checker, p.bookmarks)
	return nil
}

// ServeHTTP is invoked for requests to /plugins/{plugin_id}.
func (p *Plugin) ServeHTTP(c *plugin.Context, w http.ResponseWriter, r *http.Request) {
	if p.httpHandler == nil {
		http.Error(w, "Plugin is not ready", http.StatusServiceUnavailable)
		return
	}
	p.httpHandler.ServeHTTP(c, w, r)
}
