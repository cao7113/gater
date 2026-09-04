package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/cao7113/gater/entry"
	"github.com/cao7113/gater/internal/config"
	"github.com/cao7113/gater/internal/manager"
	"github.com/cao7113/gater/internal/store"
	"github.com/cao7113/gater/internal/version"
	"github.com/spf13/pflag"
)

const (
	defaultPort      = "8080"
	defaultStorePath = "~/.config/gater/store.yaml"
	shutdownTimeout  = 5 * time.Second
)

type options struct {
	port        string
	storePath   string
	adminHosts  string
	targetHost  string
	suffixes    []config.AppSuffix
	showVersion bool
}

func main() {
	opts := parseOptions()

	if opts.showVersion {
		fmt.Println(version.String())
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	st, err := store.NewStore(opts.storePath)
	if err != nil {
		log.Fatalf("初始化存储层失败: %v", err)
	}
	config.TargetHost = opts.targetHost
	config.SetAdminHosts(opts.adminHosts)

	var suffixes []config.AppSuffix
	if len(opts.suffixes) > 0 {
		suffixes = opts.suffixes
	}
	mgr := manager.New(ctx, st, suffixes)
	server := entry.New(entry.Config{AdminPort: opts.port, StorePath: opts.storePath, AppSuffixes: opts.suffixes}, mgr)

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

	log.Printf("Gater server ready | admin: http://%s:%s", config.AdminHosts[0], opts.port)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("Gater 异常中断: %v", err)
	}
}

func parseOptions() options {
	defaultSuffixesStr := env("GATER_SUFFIXES", "")

	defaults := options{
		port:       env("PORT", defaultPort),
		storePath:  env("GATER_STORE", defaultStorePath),
		adminHosts: env("ADMIN_HOSTS", strings.Join(config.DefaultAdminHosts, ",")),
		targetHost: env("GATER_TARGET_HOST", config.DefaultTargetHost),
	}

	flags := pflag.NewFlagSet(os.Args[0], pflag.ExitOnError)
	flags.Usage = func() {
		fmt.Fprintf(flags.Output(), "用法: %s [选项]\n\n选项:\n", os.Args[0])
		flags.PrintDefaults()
	}
	showVersion := flags.BoolP("version", "v", false, "显示版本")
	port := flags.StringP("port", "p", defaults.port, "HTTP 服务端口")
	storePath := flags.StringP("store", "s", defaults.storePath, "应用注册表文件路径")
	adminHosts := flags.StringP("admin-host", "a", defaults.adminHosts, "管理控制台域名列表，逗号分隔")
	targetHost := flags.String("target-host", defaults.targetHost, "目标应用服务器地址")
	suffixesStr := flags.String("suffixes", defaultSuffixesStr,
		`代理加载时识别应用的 hostname 后缀列表，逗号分隔（例: .s,.lab,.l 或 https:.s,http:.lab）。为空时使用内置默认列表`)

	flags.Parse(os.Args[1:])

	var suffixes []config.AppSuffix
	if *suffixesStr != "" {
		for _, s := range strings.Split(*suffixesStr, ",") {
			if s = strings.TrimSpace(s); s != "" {
				suffixes = append(suffixes, config.ParseAppSuffix(s))
			}
		}
	}
	return options{
		port: *port, storePath: expandHome(*storePath),
		adminHosts: *adminHosts, targetHost: *targetHost, showVersion: *showVersion,
		suffixes: suffixes,
	}
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
