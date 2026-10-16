package jobs

// Traced background-job helpers (issue #317).
//
// Async work in this repo lives in services/* plus goroutines
// (backup Schedule, audit maintenance, cache Warm). Wrap each job entry
// point with RunTraced so it gets its own W3C trace span: pass the parent
// ctx when triggered by a request (trace continues) or context.Background()
// for scheduled runs (new trace starts). Uses middleware.StartJobSpan so
// trace IDs stay consistent with HTTP propagation.

import (
	"context"
	"fmt"
	"log"

	"github.com/yourusername/kor-assetforge/middleware"
)

// RunTraced executes fn inside a job span. The span context is passed to fn
// and must be forwarded to all service/DB/Redis calls inside the job.
func RunTraced(ctx context.Context, jobName string, fn func(ctx context.Context) error) error {
	if ctx == nil {
		ctx = context.Background()
	}
	tracedCtx, _ := middleware.StartJobSpan(ctx, jobName)
	traceID := middleware.TraceIDFromContext(tracedCtx)
	log.Printf("[jobs] start job=%s trace_id=%s", jobName, traceID)
	err := fn(tracedCtx)
	if err != nil {
		log.Printf("[jobs] job failed job=%s trace_id=%s err=%v", jobName, traceID, err)
		return fmt.Errorf("job %s: %w", jobName, err)
	}
	log.Printf("[jobs] done job=%s trace_id=%s", jobName, traceID)
	return nil
}

// ContextForSchedule returns a fresh background context carrying a new job
// trace. Use at the top of time.Ticker / time.After loops.
func ContextForSchedule(jobName string) (context.Context, func()) {
	ctx, end := middleware.StartJobSpan(context.Background(), jobName)
	return ctx, end
}
