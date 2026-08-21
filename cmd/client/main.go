package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/cao7113/gater/entry/api"
)

const defaultServerURL = "http://localhost:8080"

type client struct {
	baseURL    *url.URL
	httpClient *http.Client
}

func main() {
	flag.Usage = usage
	flag.Parse()

	args := flag.Args()
	if len(args) == 0 {
		usage()
		return
	}

	baseURL, err := parseURL(serverURL())
	if err != nil {
		fatal(err)
	}
	c := &client{baseURL: baseURL, httpClient: http.DefaultClient}

	if err := run(c, args[0], args[1:]); err != nil {
		fatal(err)
	}
}

func init() {
	flag.String("addr", env("GATER_ADDR", defaultServerURL), "Gater server URL")
}

func run(c *client, command string, args []string) error {
	switch command {
	case "list", "ls", "l":
		if len(args) != 0 {
			return errors.New("list 不接受参数")
		}
		return c.list()
	case "show", "view":
		return c.show(oneAppArg(command, args))
	case "log":
		return c.logs(oneAppArg(command, args))
	case "start":
		return c.action("start", oneAppArg(command, args))
	case "stop":
		return c.action("stop", oneAppArg(command, args))
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
	fmt.Printf("%-24s %-10s %-8s %s\n", "NAME", "STATE", "PORT", "PATH")
	for _, app := range apps {
		fmt.Printf("%-24s %-10s %-8d %s\n", app.Name, app.State, app.Port, app.Path)
	}
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

func (c *client) action(action, name string) error {
	return c.post("/api/apps/"+url.PathEscape(name)+"/"+action, nil)
}

func (c *client) get(path string, result any) error {
	return c.request(http.MethodGet, path, nil, result)
}

func (c *client) post(path string, result any) error {
	return c.request(http.MethodPost, path, nil, result)
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

func serverURL() string {
	for _, arg := range os.Args[1:] {
		if strings.HasPrefix(arg, "-addr=") {
			return strings.TrimPrefix(arg, "-addr=")
		}
	}
	return flag.Lookup("addr").Value.String()
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
	fmt.Fprintln(os.Stderr, "  show <app>        查看应用配置与状态")
	fmt.Fprintln(os.Stderr, "  logs <app>        查看应用日志")
	fmt.Fprintln(os.Stderr, "  start <app>       启动应用")
	fmt.Fprintln(os.Stderr, "  stop <app>        停止应用")
	fmt.Fprintln(os.Stderr, "\n默认地址: "+defaultServerURL)
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "错误:", err)
	os.Exit(1)
}
