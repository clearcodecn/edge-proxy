package alert

import (
	"context"
	"log"
	"time"

	"edge-proxy/internal/store"
)

// Channel is the minimal contract every alert sink implements.
type Channel interface {
	Name() string
	Send(ctx context.Context, text string) error
}

// Notifier fans out alerts to all configured channels with SQLite-backed dedup.
// If all channels fail for a given key, the dedup table is NOT updated, so the
// next cron tick may retry delivery once the failing channel recovers.
type Notifier struct {
	dedup    *store.AlertDedupRepo
	window   time.Duration
	channels []Channel
	timeout  time.Duration
}

func NewNotifier(dedup *store.AlertDedupRepo, window time.Duration, channels ...Channel) *Notifier {
	return &Notifier{
		dedup:    dedup,
		window:   window,
		channels: channels,
		timeout:  10 * time.Second,
	}
}

func (n *Notifier) Alert(key, message string) {
	cooled, err := n.dedup.IsCooled(key, n.window)
	if err != nil {
		log.Printf("[alert] dedup check failed (fail-open): key=%s err=%v", key, err)
		cooled = false
	}
	if cooled {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), n.timeout)
	defer cancel()

	anySuccess := false
	for _, c := range n.channels {
		if err := c.Send(ctx, message); err != nil {
			log.Printf("[alert] %s send failed: key=%s err=%v", c.Name(), key, err)
			continue
		}
		anySuccess = true
	}

	if anySuccess {
		if err := n.dedup.MarkFired(key); err != nil {
			log.Printf("[alert] mark fired failed: key=%s err=%v", key, err)
		}
	}
}
