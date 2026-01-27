package storeiface

import "context"

// DeliveryStore persists routing + offline + idempotency state for IM delivery.
type DeliveryStore interface {
	// Route (uid -> cometAddr)
	SetRoute(ctx context.Context, uid int64, cometAddr string, ttlSeconds int64) error
	GetRoute(ctx context.Context, uid int64) (string, error)

	// Offline queue (ordered by seq). packetJSON is stored as-is.
	EnqueueOffline(ctx context.Context, uid int64, convID string, seq int64, packetJSON string, maxKeep int64) error
	PullOffline(ctx context.Context, uid int64, convID string, afterSeq int64, limit int64) (packets []string, lastSeq int64, err error)
	Ack(ctx context.Context, uid int64, convID string, seq int64) error

	// Idempotency
	GetIdem(ctx context.Context, fromUID int64, clientMsgID string) (msgID int64, ok bool, err error)
	SetIdem(ctx context.Context, fromUID int64, clientMsgID string, msgID int64, ttlSeconds int64) error
}
