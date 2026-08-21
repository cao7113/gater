package api

import (
	"github.com/cao7113/gater/internal/app"
	"github.com/cao7113/gater/internal/store"
)

type appManager interface {
	GetAllApps() map[string]*app.App
	GetApp(name string) (*app.App, bool)
	AddOrUpdateApp(spec store.AppSpec) error
	RemoveApp(name string) error
}

type AppInfo struct {
	Name             string            `json:"name"`
	Path             string            `json:"path"`
	Cmd              string            `json:"cmd"`
	Args             []string          `json:"args"`
	Env              map[string]string `json:"env"`
	Port             int               `json:"port"`
	State            string            `json:"state"`
	IdleTimeoutSec   int               `json:"idle_timeout_sec"`
	RemainingSeconds int               `json:"remaining_seconds"`
}
