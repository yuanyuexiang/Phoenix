// Package httpx 提供各服务共用的 HTTP 服务器封装:
// 统一优雅退出(SIGINT/SIGTERM 后停止接收新连接,给未完成请求收尾时间)
// 与基础超时配置。所有服务入口都应经由 Serve 启动,而不是裸用 ListenAndServe。
package httpx

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os/signal"
	"syscall"
	"time"
)

const (
	readHeaderTimeout = 10 * time.Second // 防慢连接(Slowloris)
	shutdownGrace     = 30 * time.Second // 优雅关闭窗口;OCR/LLM 长请求超过该时长会被截断
)

// Serve 启动 HTTP 服务并阻塞,直到出错或收到退出信号。
// 正常优雅退出时返回 nil。
func Serve(addr string, handler http.Handler, serviceName string) error {
	srv := &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: readHeaderTimeout,
		IdleTimeout:       2 * time.Minute,
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)
	go func() {
		slog.Info(serviceName+" 已启动", "addr", addr)
		errCh <- srv.ListenAndServe()
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		slog.Info("收到退出信号,正在优雅关闭", "service", serviceName, "grace", shutdownGrace.String())
		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownGrace)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil && !errors.Is(err, context.DeadlineExceeded) {
			return err
		}
		slog.Info(serviceName + " 已退出")
		return nil
	}
}
