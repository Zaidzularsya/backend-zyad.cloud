package app

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"
)

func (a *App) serve(ctx context.Context) error {
	defer a.cleanup()
	serverErr := make(chan error, 1)

	go func() {
		serverErr <- a.server.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return a.server.Shutdown(shutdownCtx)
	case err := <-serverErr:
		if err == nil || errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return fmt.Errorf("listen and serve: %w", err)
	}
}
