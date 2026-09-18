package worker

import (
	"context"
	"log/slog"
	"time"
)

type MonitorChecker interface {
	Check(context.Context) error
	Pending() <-chan struct{}
}

// RunMonitor polls the watch list on a slow ticker; per-movie throttling
// lives in the service via next_check_at. A new monitor wakes it early.
func RunMonitor(ctx context.Context, monitor MonitorChecker, logger *slog.Logger) {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for {
		if err := monitor.Check(ctx); err != nil && ctx.Err() == nil {
			logger.ErrorContext(ctx, "check monitored movies", "error", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		case <-monitor.Pending():
		}
	}
}
