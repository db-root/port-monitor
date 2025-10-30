package main

import (
	"log"
	"net/http"
	"os"
	"path/filepath"

	"port-monitor/backend"
)

func main() {
	// 设置静态文件服务
	wd, err := os.Getwd()
	if err != nil {
		log.Fatal("无法获取当前工作目录:", err)
	}

	// 设置静态文件服务 - 使用项目根目录下的 frontend/static 目录
	staticPath := filepath.Join(wd, "frontend", "static")
	if _, err := os.Stat(staticPath); os.IsNotExist(err) {
		log.Fatal("静态资源目录不存在:", staticPath)
	}

	fs := http.FileServer(http.Dir(staticPath))
	http.Handle("/static/", http.StripPrefix("/static/", fs))

	// 注册后端路由和启动服务
	backend.StartServer()
}
