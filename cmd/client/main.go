package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/cao7113/gater/entry/api"
	"github.com/cao7113/gater/internal/version"
	"github.com/spf13/pflag"
)

const defaultServerURL = "http://localhost:8080"

type client struct {
	baseURL    *url.URL
	httpClient *http.Client
}

func main() {
	flags := pflag.NewFlagSet(os.Args[0], pflag.ExitOnError)
	flags.Usage = func() {
		usage()
		flags.PrintDefaults()
	}
	showVersion := flags.BoolP("version", "v", false, "显示版本")
	addr := flags.StringP("addr", "a", env("GATER_ADDR", defaultServerURL), "Gater server URL")
	flags.Parse(os.Args[1:])
	if *showVersion {
		fmt.Println(version.String())
		return
	}

	args := flags.Args()
	if len(args) == 0 {
		usage()
		return
	}

	baseURL, err := parseURL(*addr)
	if err != nil {
		fatal(err)
	}

	fmt.Println("# Connect to server:", baseURL)
	c := &client{baseURL: baseURL, httpClient: http.DefaultClient}

	if err := run(c, args[0], args[1:]); err != nil {
		fatal(err)
	}
}

func run(c *client, command string, args []string) error {
	switch command {
	case "list", "ls", "l":
		if len(args) != 0 {
			return errors.New("list 不接受参数")
		}
		return c.list()
	case "config":
		if len(args) != 0 {
			return errors.New("config 不接受参数")
		}
		return c.config()
	case "next-port":
		if len(args) != 0 {
			return errors.New("next-port 不接受参数")
		}
		return c.nextPort()
	case "show", "view":
		return c.show(oneAppArg(command, args))
	case "runtime", "env":
		return c.runtime(args)
	case "log":
		return c.logs(oneAppArg(command, args))
	case "start":
		return c.action("start", oneAppArg(command, args))
	case "stop":
		return c.action("stop", oneAppArg(command, args))
	case "add":
		return c.addYAML(oneAppArg(command, args))
	case "remove", "rm":
		return c.remove(oneAppArg(command, args))
	default:
		return fmt.Errorf("未知命令 %q", command)
	}
}

func (c *client) list() error {
	var apps []api.AppInfo
	if err := c.get("/api/apps", &apps); err != nil {
		return err
	}
	if len(apps) == 0 {
		fmt.Println("没有已注册的应用")
		return nil
	}
	fmt.Printf("%-24s %-10s %-8s %s\n", "NAME", "STATE", "PORT", "CWD")
	for _, app := range apps {
		fmt.Printf("%-24s %-10s %-8d %s\n", app.Name, app.State, app.Port, app.Cwd)
	}
	return nil
}

func (c *client) config() error {
	content, err := c.getText("/api/store/config")
	if err != nil {
		return err
	}
	_, err = fmt.Print(content)
	return err
}

func (c *client) nextPort() error {
	var response struct {
		Port int `json:"port"`
	}
	if err := c.post("/api/next-port", &response); err != nil {
		return err
	}
	fmt.Println(response.Port)
	return nil
}

func (c *client) show(name string) error {
	var app api.AppInfo
	if err := c.get("/api/apps/"+url.PathEscape(name), &app); err != nil {
		return err
	}
	return printJSON(app)
}

func (c *client) logs(name string) error {
	var response struct {
		Logs string `json:"logs"`
	}
	if err := c.get("/api/apps/"+url.PathEscape(name)+"/logs", &response); err != nil {
		return err
	}
	_, err := fmt.Print(response.Logs)
	return err
}

func (c *client) runtime(args []string) error {
	flags := pflag.NewFlagSet("runtime", pflag.ContinueOnError)
	showSensitive := flags.Bool("show-sensitive", false, "显示未脱敏的环境变量")
	if err := flags.Parse(args); err != nil {
		return err
	}
	name := oneAppArg("runtime", flags.Args())
	path := "/api/apps/" + url.PathEscape(name) + "/runtime"
	if *showSensitive {
		path += "?show_sensitive=true"
	}
	var runtime map[string]any
	if err := c.get(path, &runtime); err != nil {
		return err
	}
	return printJSON(runtime)
}

func (c *client) action(action, name string) error {
	return c.post("/api/apps/"+url.PathEscape(name)+"/"+action, nil)
}

func (c *client) addYAML(path string) error {
	if path == "" {
		return errors.New("path is required")
	}
	if !filepath.IsAbs(path) {
		if _, err := os.Stat(path); err == nil {
			absPath, err := filepath.Abs(path)
			if err != nil {
				return fmt.Errorf("resolve absolute path failed: %w", err)
			}
			path = absPath
		}
	}
	if !filepath.IsAbs(path) {
		return errors.New("path must be an absolute path to app.yaml")
	}

	body, err := json.Marshal(map[string]string{"path": path})
	if err != nil {
		return err
	}
	if err := c.postJSON("/api/apps/from-yaml", bytes.NewReader(body)); err != nil {
		return err
	}
	fmt.Printf("已添加应用 %q\n", path)
	return nil
}

func (c *client) remove(name string) error {
	if err := c.request(http.MethodDelete, "/api/apps/"+url.PathEscape(name), nil, nil); err != nil {
		return err
	}
	fmt.Printf("已删除应用 %q\n", name)
	return nil
}

func (c *client) get(path string, result any) error {
	return c.request(http.MethodGet, path, nil, result)
}

func (c *client) post(path string, result any) error {
	return c.request(http.MethodPost, path, nil, result)
}

func (c *client) postJSON(path string, body io.Reader) error {
	req, err := http.NewRequest(http.MethodPost, strings.TrimRight(c.baseURL.String(), "/")+path, body)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("请求 Gater 失败: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode < http.StatusOK || res.StatusCode >= http.StatusMultipleChoices {
		data, _ := io.ReadAll(res.Body)
		return fmt.Errorf("Gater 返回 %s: %s", res.Status, strings.TrimSpace(string(data)))
	}
	return nil
}

func (c *client) getText(path string) (string, error) {
	req, err := http.NewRequest(http.MethodGet, strings.TrimRight(c.baseURL.String(), "/")+path, nil)
	if err != nil {
		return "", err
	}
	res, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("请求 Gater 失败: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode < http.StatusOK || res.StatusCode >= http.StatusMultipleChoices {
		data, _ := io.ReadAll(res.Body)
		return "", fmt.Errorf("Gater 返回 %s: %s", res.Status, strings.TrimSpace(string(data)))
	}
	data, err := io.ReadAll(res.Body)
	return string(data), err
}

func (c *client) request(method, path string, body io.Reader, result any) error {
	req, err := http.NewRequest(method, strings.TrimRight(c.baseURL.String(), "/")+path, body)
	if err != nil {
		return err
	}
	res, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("请求 Gater 失败: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode < http.StatusOK || res.StatusCode >= http.StatusMultipleChoices {
		data, _ := io.ReadAll(res.Body)
		return fmt.Errorf("Gater 返回 %s: %s", res.Status, strings.TrimSpace(string(data)))
	}
	if result == nil || res.StatusCode == http.StatusNoContent {
		return nil
	}
	if err := json.NewDecoder(res.Body).Decode(result); err != nil {
		return fmt.Errorf("解析 Gater 响应失败: %w", err)
	}
	return nil
}

func parseURL(raw string) (*url.URL, error) {
	if !strings.Contains(raw, "://") {
		raw = "http://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return nil, fmt.Errorf("无效的 server 地址: %q", raw)
	}
	return u, nil
}

func oneAppArg(command string, args []string) string {
	if len(args) != 1 {
		fatal(fmt.Errorf("用法: gater %s <app>", command))
	}
	return args[0]
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func printJSON(value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	_, err = fmt.Println(string(data))
	return err
}

func usage() {
	fmt.Fprintln(os.Stderr, "用法: gater [-addr URL] <command> [app]")
	fmt.Fprintln(os.Stderr, "\n命令:")
	fmt.Fprintln(os.Stderr, "  list              列出所有应用")
	fmt.Fprintln(os.Stderr, "  config            显示 store 配置")
	fmt.Fprintln(os.Stderr, "  next-port         获取一个可用的本地应用端口")
	fmt.Fprintln(os.Stderr, "  show <app>        查看应用配置与状态")
	fmt.Fprintln(os.Stderr, "  runtime <app>     查看运行配置（默认脱敏；--show-sensitive 显示敏感值）")
	fmt.Fprintln(os.Stderr, "  logs <app>        查看应用日志")
	fmt.Fprintln(os.Stderr, "  start <app>       启动应用")
	fmt.Fprintln(os.Stderr, "  stop <app>        停止应用")
	fmt.Fprintln(os.Stderr, "  add <path-with-app.yaml>        通过路径添加应用")
	fmt.Fprintln(os.Stderr, "  remove <name>     删除应用 (别名: rm)")
	fmt.Fprintln(os.Stderr, "\n默认地址: "+defaultServerURL)
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "错误:", err)
	os.Exit(1)
}
