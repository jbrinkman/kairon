package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
)

// signalNotice is written once when SIGINT or SIGTERM cancels a signalContext.
const signalNotice = "kairon: signal received, cancelling in-flight calls (send it again to exit immediately)"

// signalContext returns a context that is cancelled by the first SIGINT or
// SIGTERM, or when parent is cancelled. A nil parent is treated as
// context.Background().
//
// When a signal cancels the context, one line is written to notice, and the
// signal handler is released so that a second SIGINT or SIGTERM terminates the
// process with default handling. That matters for work that does not observe
// the context, such as a `command` check or a sandboxed container call.
//
// The returned cancel function releases the handler, cancels the context and
// is safe to call more than once. It writes no notice. Callers should defer it.
func signalContext(parent context.Context, notice io.Writer) (context.Context, context.CancelFunc) {
	if parent == nil {
		parent = context.Background()
	}
	ctx, stop := signal.NotifyContext(parent, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-ctx.Done()
		// signal.NotifyContext cancels with a cause other than
		// context.Canceled only when a signal arrived. stop() (the caller's
		// release) cancels with context.Canceled, and a cancelled parent
		// leaves parent.Err() set.
		if parent.Err() == nil && context.Cause(ctx) != context.Canceled && notice != nil {
			fmt.Fprintln(notice, signalNotice)
		}
		stop()
	}()
	return ctx, stop
}
