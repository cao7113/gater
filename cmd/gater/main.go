package main

import (
	"context"
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
	shutdownTimeout  = 5 * time.Second
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	st, err := store.NewStore()
	if err != nil {
		log.Fatalf("初始化存储层失败: %v", err)
	}

	mgr := manager.New(ctx, st)
	configuredPort := env("PORT", defaultPort)
	configuredAdminHost := env("ADMIN_HOST", defaultAdminHost)
	server := entry.New(entry.Config{Port: configuredPort, AdminHost: configuredAdminHost}, mgr)

	// safe exit
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

	log.Printf("🐊 Gater Application Engine 已就绪 | 管理控制台: http://%s:%s", configuredAdminHost, configuredPort)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("Gater 异常中断: %v", err)
	}
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
