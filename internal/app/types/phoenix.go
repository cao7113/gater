package types

import (
	"context"

	"github.com/cao7113/gater/internal/config"
)

func init() {
	Register(config.AppTypePhoenix, func() Handler { return phoenixHandler{} })
}

type phoenixHandler struct{}

func (phoenixHandler) Prepare(appTypeContext *TypeContext) error {
	// appTypeContext.Env["PHX_HOST"] = appTypeContext.Domain
	return nil
}

func (phoenixHandler) BeforeStart(context.Context, *TypeContext) error { return nil }

func (phoenixHandler) AfterStart(context.Context, *TypeContext) error { return nil }
