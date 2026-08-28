package types

import (
	"context"
)

func init() {
	Register("default", func() Handler { return defaultHandler{} })
}

type defaultHandler struct{}

func (defaultHandler) Prepare(*TypeContext) error { return nil }

func (defaultHandler) BeforeStart(context.Context, *TypeContext) error { return nil }

func (defaultHandler) AfterStart(context.Context, *TypeContext) error { return nil }
