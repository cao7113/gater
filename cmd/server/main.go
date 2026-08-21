package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/cao7113/gater/entry"
	"github.com/cao7113/gater/internal/manager"
	"github.com/cao7113/gater/internal/store"
)

const (
	defaultPort      = "8080"
	defaultAdminHost = "admin.lab"
	defaultStorePath = "~/.config/gater/store.json"
	shutdownTimeout  = 5 * time.Second
)

type options struct {
	port      string
	storePath string
	adminHost string
}

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	opts := parseOptions()
	st, err := store.NewStore(opts.storePath)
	if err != nil {
		log.Fatalf("初始化存储层失败: %v", err)
	}

	mgr := manager.New(ctx, st)
	server := entry.New(entry.Config{Port: opts.port, AdminHost: opts.adminHost}, mgr)

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-signals
		signal.Stop(signals)

		log.Println("[Gater Daemon] 接收到退出信号，优雅销毁所有子进程组...")
		cancel()
		for _, application := range mgr.GetAllApps() {
			application.Stop()
		}

		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer shutdownCancel()
		_ = server.Shutdown(shutdownCtx)
	}()

	log.Printf("Gater server ready | admin: http://%s:%s", opts.adminHost, opts.port)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("Gater 异常中断: %v", err)
	}
}

func parseOptions() options {
	defaults := options{
		port:      env("PORT", defaultPort),
		storePath: env("GATER_STORE", defaultStorePath),
		adminHost: env("ADMIN_HOST", defaultAdminHost),
	}

	port := flag.String("port", defaults.port, "HTTP 服务端口")
	storePath := flag.String("store", defaults.storePath, "应用注册表文件路径")
	adminHost := flag.String("admin-host", defaults.adminHost, "管理控制台域名")
	flag.Parse()

	return options{port: *port, storePath: expandHome(*storePath), adminHost: *adminHost}
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func expandHome(path string) string {
	if path == "~" {
		if home, err := os.UserHomeDir(); err == nil {
			return home
		}
	}
	if len(path) > 2 && path[:2] == "~/" {
		if home, err := os.UserHomeDir(); err == nil {
			return home + path[1:]
		}
	}
	return path
}
