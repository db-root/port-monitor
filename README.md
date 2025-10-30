# Port Monitor

这是一个网络端口监控工具，可以监控系统中的TCP/UDP服务和网络接口。

## 项目结构

项目采用前后端分离架构：

```
port-monitor/
├── LICENSE
├── README.md
├── config/
│   └── config.yaml
├── data.json
├── frontend/
│   └── static/
│       ├── index.html
│       ├── css/
│       │   └── style.css
│       └── js/
│           └── script.js
├── go.mod
├── go.sum
├── main.go
└── backend/
    └── main.go
```

## 启动服务

```bash
cd backend
go run main.go
```

然后在浏览器中访问 http://localhost:10810

## 功能特性

- 实时监控TCP/UDP服务
- 显示网络接口信息
- 支持自定义服务名称
- 可配置URL路径
- 支持列显示配置
- 接口链接开关控制
- 一键复制功能
