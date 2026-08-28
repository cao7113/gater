package types

import (
	"context"
	"strings"

	"github.com/cao7113/gater/internal/config"
)

type TypeContext struct {
	Config     config.AppConfig
	Domain     string
	Port       int
	WorkingDir string
	Args       []string
	Env        map[string]string
}

type Handler interface {
	Prepare(*TypeContext) error
	BeforeStart(context.Context, *TypeContext) error
	AfterStart(context.Context, *TypeContext) error
}

type HandlerFactory func() Handler

var handlerFactories = map[string]HandlerFactory{}

func Register(appType string, factory HandlerFactory) {
	appType = strings.ToLower(strings.TrimSpace(appType))
	if appType == "" || factory == nil {
		return
	}
	handlerFactories[appType] = factory
}

func HandlerFor(appType string) Handler {
	if factory, ok := handlerFactories[strings.ToLower(strings.TrimSpace(appType))]; ok {
		return factory()
	}
	return handlerFactories["default"]()
}
