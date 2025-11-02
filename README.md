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

## 构建安装包

项目支持构建RPM和DEB安装包，可以使用以下命令：

```bash
./build-packages.sh
```

该脚本会使用Docker容器构建RPM和DEB包，并输出到 `dist` 目录。

安装后文件布局：
- 二进制文件: `/usr/bin/port-monitor`
- 前端文件: `/opt/port-monitor/frontend/static`
- 配置文件: `/opt/port-monitor/config.yaml`
- 数据文件: `/opt/port-monitor/data.json`
- 日志文件: `/opt/port-monitor/server.log`
- systemd服务: `/etc/systemd/system/port-monitor.service`

### 安装RPM包:

```bash
sudo rpm -ivh dist/port-monitor-*.rpm
```

### 安装DEB包:

```bash
sudo dpkg -i dist/port-monitor-*.deb
```

### 启动服务:

```bash
sudo systemctl start port-monitor
sudo systemctl enable port-monitor  # 开机自启
```