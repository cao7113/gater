package web

import (
	"embed"
	"io/fs"
	"net/http"
)

// 1. 自动根据 bun.lock 安装/校验依赖
//go:generate bun install --cwd ..

// 2. 调用 package.json 中定义的打包脚本
//go:generate bun run --cwd .. build.css
//go:generate bun run --cwd .. build.js
//go:generate bun run --cwd .. cp.pages

//go:embed dist/*
var staticFS embed.FS

func GetFileSystem() http.FileSystem {
	subFS, err := fs.Sub(staticFS, "dist")
	if err != nil {
		panic(err)
	}
	return http.FS(subFS)
}
