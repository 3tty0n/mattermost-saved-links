// Package testutil provides the small test doubles shared by the plugin's server tests. It is
// only imported from _test.go files.
package testutil

import (
	"github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/plugin/plugintest"
)

// API wraps plugintest.API and absorbs log/audit calls, which would otherwise need expectations
// in every test. Assertions on business API calls still use the embedded mock.
type API struct {
	*plugintest.API
	Audits []*model.AuditRecord
	Logs   []string
}

// NewAPI returns an API with no expectations yet.
func NewAPI() *API {
	return &API{API: &plugintest.API{}}
}

func (a *API) LogDebug(string, ...any) {}
func (a *API) LogInfo(string, ...any)  {}
func (a *API) LogWarn(string, ...any)  {}
func (a *API) LogError(string, ...any) {}

func (a *API) LogAuditRec(rec *model.AuditRecord) {
	a.Audits = append(a.Audits, rec)
}

// AuditSuccesses counts successful audit records, for quick assertions.
func (a *API) AuditSuccesses() int {
	count := 0
	for _, rec := range a.Audits {
		if rec != nil && rec.Status == model.AuditStatusSuccess {
			count++
		}
	}
	return count
}
