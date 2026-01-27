package push

import "context"

type Provider interface {
	Type() string
	Push(ctx context.Context, msg Message) (Result, error)
}
