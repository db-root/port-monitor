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

从项目根目录运行:

```bash
go run main.go
```

然后在浏览器中访问 http://localhost:10810

> 注意: 服务默认监听在所有接口的10810端口，可以在 [config/config.yaml](file:///opt/code/golang/port-monitor/config/config.yaml) 中修改配置

## 功能特性

- 实时监控TCP/UDP服务
- 显示网络接口信息
- 支持自定义服务名称
- 可配置URL路径
- 支持列显示配置
- 接口链接开关控制
- 一键复制功能

## 配置文件说明

配置文件位于 [config/config.yaml](file:///opt/code/golang/port-monitor/config/config.yaml)：

- `addr`: 服务监听地址，默认为 0.0.0.0 (监听所有接口)
- `port`: 服务监听端口，默认为 10810
- `exclude`: 要排除显示的网络接口前缀，用逗号分隔
- `get_ip_url`: 获取公网IP的API地址

## 数据存储

用户自定义的配置保存在 [data.json](file:///opt/code/golang/port-monitor/data.json) 文件中，包括：
- 自定义服务名称
- 接口配置
- 列显示设置
- URL路径映射
