package getui

import (
	"context"

	provider "github.com/lzyats/core-push-go/pkg/provider/getui"
	"github.com/lzyats/core-push-go/pkg/push"
)

type Vendor struct {
	p *provider.Provider
}

func New(cfg push.PushSettings) *Vendor {
	return &Vendor{p: provider.New(cfg)}
}

func (v *Vendor) PushNotify(ctx context.Context, uid int64, title, body string, data map[string]string) error {
	msg := push.Message{
		Title:   title,
		Body:    body,
		Data:    data,
		Targets: []string{itoa(uid)},
	}
	_, err := v.p.Push(ctx, msg)
	return err
}

func itoa(n int64) string {
	if n == 0 {
		return "0"
	}
	var buf [32]byte
	i := len(buf)
	neg := n < 0
	if neg {
		n = -n
	}
	for n > 0 {
		i--
		buf[i] = byte('0' + (n % 10))
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
