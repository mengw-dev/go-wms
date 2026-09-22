package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"gowms/internal/pkg/log"
)

// serve 在收到退出信号或任一监听失败时关闭全部 HTTP 服务。
func serve(ctx context.Context, shutdownTimeout time.Duration, servers ...*http.Server) error {
	if len(servers) == 0 {
		return errors.New("at least one HTTP server is required")
	}
	serveResults := make(chan error, len(servers))
	for _, srv := range servers {
		go func() {
			log.L().Info("http server starting", "addr", srv.Addr)
			err := srv.ListenAndServe()
			if errors.Is(err, http.ErrServerClosed) {
				err = nil
			}
			if err != nil {
				err = fmt.Errorf("serve %s: %w", srv.Addr, err)
			}
			serveResults <- err
		}()
	}

	var serveErr error
	remaining := len(servers)
	select {
	case <-ctx.Done():
	case serveErr = <-serveResults:
		remaining--
	}

	log.L().Info("shutting down HTTP servers")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	for _, srv := range servers {
		if err := srv.Shutdown(shutdownCtx); err != nil {
			serveErr = errors.Join(serveErr, fmt.Errorf("shutdown %s: %w", srv.Addr, err))
			// Shutdown 超时不会关闭活跃连接，需要显式强制关闭。
			if closeErr := srv.Close(); closeErr != nil {
				serveErr = errors.Join(serveErr, fmt.Errorf("close %s: %w", srv.Addr, closeErr))
			}
		}
	}
	for range remaining {
		serveErr = errors.Join(serveErr, <-serveResults)
	}
	return serveErr
}
