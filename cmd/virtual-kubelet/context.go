package main

import (
	"context"
	"os"
	"os/signal"
)

func BaseContext(ctx context.Context) (context.Context, func()) {
	sigC := make(chan os.Signal, 1)
	ctx, cancel := context.WithCancel(ctx)
	
	go func() {
		for {
			select {
			case <-ctx.Done():
				signal.Stop(sigC)
				return
			case <-sigC:
				cancel()
			}
		}
	}()
	
	signal.Notify(sigC, cancelSigs()...)
	return ctx, cancel
}
