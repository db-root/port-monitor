package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"html/template"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"strings"

	// 添加yaml支持
	"gopkg.in/yaml.v2"
)

type Service struct {
	Name        string `json:"name"`
	Protocol    string `json:"protocol"`
	LocalAddr   string `json:"local_addr"`
	LocalPort   string `json:"local_port"`
	ForeignAddr string `json:"foreign_addr"`
	State       string `json:"state"`
	PID         string `json:"pid"`
}

type InterfaceInfo struct {
	Name string `json:"name"`
	IP   string `json:"ip"`
}

type Config struct {
	WebPort           int
	ExcludeInterfaces []string
}

type InterfaceConfig struct {
	Name      string `json:"name"`
	ShowLinks bool   `json:"show_links"`
}

// 添加配置结构体
type YAMLConfig struct {
	ServiceConfig []struct {
		Addr     string `yaml:"addr"`
		Port     int    `yaml:"port"`
		Exclude  string `yaml:"exclude"`
		GetIpUrl string `yaml:"get_ip_url"` // 添加GetIpUrl字段
	} `yaml:"service-config"`
}

// 添加列配置结构体
type ColumnConfig struct {
	Table   string `json:"table"`
	Column  string `json:"column"`
	Visible bool   `json:"visible"`
}

// 添加用于保存服务名称的数据结构
type ServiceNameMapping struct {
	Service_id string `json:"service_id"`
	Name       string `json:"name"`
}

// 添加URL路径映射结构体
type URLPathMapping struct {
	Service_id string `json:"service_id"`
	Path       string `json:"path"`
}

var config Config

// 添加全局变量存储服务名称映射
var serviceNames = make(map[string]string)

// 添加全局变量存储URL路径映射
var urlPaths = make(map[string]string)

// 添加全局变量存储接口配置
var interfaceConfigs = make(map[string]bool)

// 添加全局变量存储列配置
var columnConfigs = make(map[string]map[string]bool)
var dataFile = "/opt/port-monitor/data.json"

// 添加日志文件
var logFile *os.File

func main() {
	// 解析命令行参数
	webPort := flag.Int("webport", 10810, "Web界面监听端口")
	exclude := flag.String("exclude", "lo,br-,veth,docker0", "排除的网卡前缀，逗号分隔")
	flag.Parse()

	// 读取YAML配置文件
	yamlConfig := &YAMLConfig{
		ServiceConfig: []struct {
			Addr     string `yaml:"addr"`
			Port     int    `yaml:"port"`
			Exclude  string `yaml:"exclude"`
			GetIpUrl string `yaml:"get_ip_url"`
		}{
			{
				Addr:     "0.0.0.0", // 设置默认监听地址
				Port:     *webPort,
				Exclude:  *exclude,
				GetIpUrl: "https://4.ipw.cn", // 设置默认公网IP服务地址
			},
		},
	}

	// 检查配置文件是否存在，如果不存在则创建
	configPath := "/opt/port-monitor/config.yaml"
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		log.Println("config.yaml文件不存在，正在生成默认配置文件...")

		// 创建默认配置
		defaultConfig := `service-config:
  - addr: "0.0.0.0"  # 监听地址
    port: 10810           # 监听端口
    exclude: "lo,br-,veth,docker0" # 忽略网卡
    get_ip_url: "https://4.ipw.cn"  # 公网IP服务地址
`

		// 确保目录存在
		if err := os.MkdirAll("/opt/port-monitor", 0755); err != nil {
			log.Printf("创建目录失败: %v\n", err)
		}

		// 写入文件
		err := os.WriteFile(configPath, []byte(defaultConfig), 0644)
		if err != nil {
			log.Printf("创建默认配置文件失败: %v\n", err)
		} else {
			log.Println("成功生成默认配置文件 config.yaml")
		}
	}

	if _, err := os.Stat(configPath); err == nil {
		yamlFile, err := os.ReadFile(configPath)
		if err == nil {
			err = yaml.Unmarshal(yamlFile, yamlConfig)
			if err != nil {
				log.Printf("解析config.yaml失败: %v\n", err)
			} else {
				log.Println("成功加载config.yaml配置文件")
			}
		}
	} else {
		log.Println("config.yaml文件不存在，使用命令行参数或默认值")
	}

	// 使用第一个服务配置作为主配置
	mainConfig := yamlConfig.ServiceConfig[0]
	config.WebPort = mainConfig.Port
	config.ExcludeInterfaces = strings.Split(mainConfig.Exclude, ",")

	// 初始化日志文件
	logDir := "/opt/port-monitor"
	if _, err := os.Stat(logDir); os.IsNotExist(err) {
		if err := os.MkdirAll(logDir, 0755); err != nil {
			fmt.Printf("创建日志目录失败: %v\n", err)
			os.Exit(1)
		}
	}

	var err error
	logFile, err = os.OpenFile("/opt/port-monitor/server.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		fmt.Printf("无法打开日志文件: %v\n", err)
		os.Exit(1)
	}
	defer logFile.Close()

	// 设置日志输出到文件和控制台
	log.SetOutput(io.MultiWriter(os.Stdout, logFile))

	// 加载已保存的服务名称
	loadServiceNames()

	http.HandleFunc("/", indexHandler)
	http.HandleFunc("/services", servicesHandler)
	http.HandleFunc("/interfaces", interfacesHandler)
	// 添加保存服务名称的路由
	http.HandleFunc("/save-service-name", saveServiceNameHandler)
	// 添加获取已保存服务名称的路由
	http.HandleFunc("/saved-service-names", savedServiceNamesHandler)
	// 添加保存列配置的路由
	http.HandleFunc("/save-column-config", saveColumnConfigHandler)
	// 添加保存URL路径的路由
	http.HandleFunc("/save-url-path", saveURLPathHandler)

	log.Printf("服务器启动，监听地址: %s:%d\n", mainConfig.Addr, config.WebPort)
	log.Fatal(http.ListenAndServe(fmt.Sprintf("%s:%d", mainConfig.Addr, config.WebPort), nil))
}

// 加载已保存的服务名称
func loadServiceNames() {
	file, err := os.Open(dataFile)
	if err != nil {
		if !os.IsNotExist(err) {
			log.Printf("读取数据文件失败: %v\n", err)
		} else {
			log.Println("数据文件不存在，将创建新文件")
		}
		return
	}
	defer file.Close()

	decoder := json.NewDecoder(file)
	var data map[string]interface{}
	if err := decoder.Decode(&data); err != nil {
		log.Printf("解析数据文件失败: %v\n", err)
		return
	}

	// 加载服务名称映射
	if mappings, ok := data["service_names"].([]interface{}); ok {
		for _, item := range mappings {
			if mapping, ok := item.(map[string]interface{}); ok {
				if serviceID, ok := mapping["service_id"].(string); ok {
					if name, ok := mapping["name"].(string); ok {
						serviceNames[serviceID] = name
					}
				}
			}
		}
		log.Printf("加载了 %d 个服务名称映射\n", len(serviceNames))
	}

	// 加载接口配置
	if configs, ok := data["interface_configs"].([]interface{}); ok {
		for _, item := range configs {
			if config, ok := item.(map[string]interface{}); ok {
				if name, ok := config["name"].(string); ok {
					if showLinks, ok := config["show_links"].(bool); ok {
						interfaceConfigs[name] = showLinks
					}
				}
			}
		}
		log.Printf("加载了 %d 个接口配置\n", len(interfaceConfigs))
	}

	// 加载列配置
	if colConfigs, ok := data["column_configs"].([]interface{}); ok {
		for _, item := range colConfigs {
			if config, ok := item.(map[string]interface{}); ok {
				table := ""
				column := ""
				visible := true

				if t, ok := config["table"].(string); ok {
					table = t
				}
				if c, ok := config["column"].(string); ok {
					column = c
				}
				if v, ok := config["visible"].(bool); ok {
					visible = v
				}

				if table != "" && column != "" {
					if columnConfigs[table] == nil {
						columnConfigs[table] = make(map[string]bool)
					}
					columnConfigs[table][column] = visible
				}
			}
		}
		log.Printf("加载了 %d 个表格的列配置\n", len(colConfigs))
	}

	// 加载URL路径映射
	if mappings, ok := data["url_paths"].([]interface{}); ok {
		for _, item := range mappings {
			if mapping, ok := item.(map[string]interface{}); ok {
				if serviceID, ok := mapping["service_id"].(string); ok {
					if path, ok := mapping["path"].(string); ok {
						urlPaths[serviceID] = path
					}
				}
			}
		}
		log.Printf("加载了 %d 个URL路径映射\n", len(urlPaths))
	}

	log.Printf("总共加载了 %d 个服务名称和 %d 个接口配置\n", len(serviceNames), len(interfaceConfigs))
}

// 保存服务名称到文件
func saveServiceNames() error {
	// 确保目录存在
	dir := "/opt/port-monitor"
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
	}

	file, err := os.Create(dataFile)
	if err != nil {
		return err
	}
	defer file.Close()

	// 准备服务名称数据
	var nameMappings []ServiceNameMapping
	for serviceID, name := range serviceNames {
		nameMappings = append(nameMappings, ServiceNameMapping{
			Service_id: serviceID,
			Name:       name,
		})
	}

	// 准备接口配置数据
	var interfaceConfigsData []InterfaceConfig
	for name, showLinks := range interfaceConfigs {
		interfaceConfigsData = append(interfaceConfigsData, InterfaceConfig{
			Name:      name,
			ShowLinks: showLinks,
		})
	}

	// 准备列配置数据
	var columnConfigsData []ColumnConfig
	for table, columns := range columnConfigs {
		for column, visible := range columns {
			columnConfigsData = append(columnConfigsData, ColumnConfig{
				Table:   table,
				Column:  column,
				Visible: visible,
			})
		}
	}

	// 准备URL路径数据
	var urlPathMappings []URLPathMapping
	for serviceID, path := range urlPaths {
		urlPathMappings = append(urlPathMappings, URLPathMapping{
			Service_id: serviceID,
			Path:       path,
		})
	}

	// 组合所有数据
	data := map[string]interface{}{
		"service_names":     nameMappings,
		"interface_configs": interfaceConfigsData,
		"column_configs":    columnConfigsData, // 添加列配置数据
		"url_paths":         urlPathMappings,   // 添加URL路径数据
	}

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")

	err = encoder.Encode(data)
	if err != nil {
		log.Printf("保存数据到文件失败: %v\n", err)
		return err
	}

	log.Printf("成功保存数据到文件，服务名称: %d个, 接口配置: %d个\n", len(nameMappings), len(interfaceConfigsData))
	return nil
}

func indexHandler(w http.ResponseWriter, r *http.Request) {
	tmpl := `
<!DOCTYPE html>
<html>
<head>
    <title>端口监控服务</title>
    <meta charset="utf-8">
    <style>
        body { font-family: Arial, sans-serif; margin: 20px; }
        h1 { color: #333; }
        .container { max-width: 1200px; margin: 0 auto; }
        .card { border: 1px solid #ddd; border-radius: 5px; padding: 15px; margin-bottom: 20px; }
        .card h2 { margin-top: 0; color: #555; }
        table { width: 100%; border-collapse: collapse; }
        th, td { padding: 10px; text-align: left; border-bottom: 1px solid #ddd; }
        th { background-color: #f2f2f2; }
        tr:hover { background-color: #f5f5f5; }
        a { color: #007bff; text-decoration: none; }
        a:hover { text-decoration: underline; }
        .refresh-btn { background-color: #007bff; color: white; padding: 10px 15px; border: none; border-radius: 5px; cursor: pointer; }
        .refresh-btn:hover { background-color: #0056b3; }
        /* Tab styles */
        .tab { overflow: hidden; border: 1px solid #ccc; background-color: #f1f1f1; }
        .tab button { background-color: #ddd; /* 未选中时比背景颜色更深 */ float: left; border: none; outline: none; cursor: pointer; padding: 14px 16px; transition: 0.3s; }
        .tab button:hover { background-color: #ccc; }
        .tab button.active { background-color: #007bff; /* 选中时与刷新服务按钮颜色一致 */ color: white; }
        .tabcontent { display: none; padding: 6px 12px; border: 1px solid #ccc; border-top: none; }
        .edit-icon { cursor: pointer; margin-left: 5px; color: #007bff; }
        .edit-input { width: 80px; }
        .save-icon, .cancel-icon { cursor: pointer; margin: 0 2px; }
        .save-icon { color: green; }
        .cancel-icon { color: red; }
        /* Switch styles */
        .switch {
            position: relative;
            display: inline-block;
            width: 60px;
            height: 26px; /* 原来是34px，减少四分之一变为26px */
        }
        .switch input {
            opacity: 0;
            width: 0;
            height: 0;
        }
        .slider {
            position: absolute;
            cursor: pointer;
            top: 0;
            left: 0;
            right: 0;
            bottom: 0;
            background-color: #ccc;
            -webkit-transition: .4s;
            transition: .4s;
            border-radius: 26px; /* 设置为圆角 */
        }
        .slider:before {
            position: absolute;
            content: "";
            height: 20px; /* 调整滑块内圆点的高度 */
            width: 20px;
            left: 3px;
            bottom: 3px;
            background-color: white;
            -webkit-transition: .4s;
            transition: .4s;
            border-radius: 50%; /* 保持圆形 */
        }
        /* 灰色齿轮图标样式 */
        .gear-icon {
            color: #888888;
            cursor: pointer;
            font-size: 16px;
            vertical-align: middle;
        }
        input:checked + .slider {
            background-color: #2196F3;
        }
        input:focus + .slider {
            box-shadow: 0 0 1px #2196F3;
        }
        input:checked + .slider:before {
            -webkit-transform: translateX(34px); /* 调整移动距离 */
            -ms-transform: translateX(34px);
            transform: translateX(34px);
        }
    </style>
</head>
<body>
    <div class="container">
        <h1>端口监控服务</h1>
        
        <div class="card">
            <h2>网络接口</h2>
            <div style="text-align: right; margin-bottom: 10px;">
                <button class="refresh-btn" onclick="loadInterfaces()">刷新接口</button>
            </div>
            <div id="interfaces-list"></div>
        </div>
        
        <div class="card">
            <h2>运行中的服务</h2>
            <div style="text-align: right; margin-bottom: 10px;">
                <button class="refresh-btn" onclick="loadServices()">刷新服务</button>
            </div>
            <div class="tab">
                <button class="tablinks active" onclick="openTab(event, 'tcpv4')">TCPv4 服务</button>
                <button class="tablinks" onclick="openTab(event, 'tcpv6')">TCPv6 服务</button>
                <button class="tablinks" onclick="openTab(event, 'udpv4')">UDPv4 服务</button>
                <button class="tablinks" onclick="openTab(event, 'udpv6')">UDPv6 服务</button>
            </div>
            <!-- 列配置弹窗 -->
            <div id="column-config-modal" style="display: none; position: fixed; z-index: 1000; left: 0; top: 0; width: 100%; height: 100%; background-color: rgba(0,0,0,0.4);">
                <div style="background-color: #fefefe; margin: 15% auto; padding: 20px; border: 1px solid #888; width: 300px; border-radius: 5px;">
                    <span onclick="closeColumnConfig()" style="color: #aaa; float: right; font-size: 28px; font-weight: bold; cursor: pointer;">&times;</span>
                    <h3>选择要显示的列</h3>
                    <div id="column-config-options"></div>
                    <button onclick="saveColumnConfig()" style="margin-top: 15px; background-color: #007bff; color: white; padding: 8px 15px; border: none; border-radius: 5px; cursor: pointer;">保存</button>
                </div>
            </div>
            <div id="tcpv4" class="tabcontent" style="display: block;">
                <div id="tcpv4-services-list"></div>
            </div>
            <div id="tcpv6" class="tabcontent">
                <div id="tcpv6-services-list"></div>
            </div>
            <div id="udpv4" class="tabcontent">
                <div id="udpv4-services-list"></div>
            </div>
            <div id="udpv6" class="tabcontent">
                <div id="udpv6-services-list"></div>
            </div>
        </div>
    </div>

    <script>
        // 存储服务名称的映射
        let serviceNames = {};
        // 存储接口配置
        let interfaceConfigs = {};
        // 存储当前正在配置的表格
        let currentConfigTable = '';
        // 存储列配置
        let columnConfigs = {};
        // 存储URL路径映射
        let urlPaths = {};

        window.onload = function() {
            loadInterfaces();
            loadServices();
        };

        // 从服务器加载已保存的服务名称
        function loadServiceNamesFromServer() {
            return fetch('/saved-service-names')
                .then(response => response.json())
                .then(data => {
                    serviceNames = data;
                    return Promise.resolve();
                })
                .catch(error => {
                    console.error('加载服务名称失败:', error);
                    return Promise.resolve();
                });
        }
        // 计算列数（包括齿轮图标列）
        let colCount = 0;
        for (let col in columnConfigs[tableType]) {
            if (columnConfigs[tableType][col]) {
                colCount++;
            }
        }
        colCount++;
        function openTab(evt, tabName) {
            var i, tabcontent, tablinks;
            tabcontent = document.getElementsByClassName("tabcontent");
            for (i = 0; i < tabcontent.length; i++) {
                tabcontent[i].style.display = "none";
            }
            tablinks = document.getElementsByClassName("tablinks");
            for (i = 0; i < tablinks.length; i++) {
                tablinks[i].className = tablinks[i].className.replace(" active", "");
            }
            document.getElementById(tabName).style.display = "block";
            evt.currentTarget.className += " active";
        }

        function loadInterfaces() {
            // 先加载保存的接口配置，再加载接口列表
            fetch('/saved-service-names')
                .then(response => response.json())
                .then(data => {
                    // 从返回数据中提取接口配置
                    if (data.interface_configs) {
                        interfaceConfigs = {};
                        data.interface_configs.forEach(config => {
                            interfaceConfigs[config.name] = config.show_links;
                        });
                    }
                    return fetch('/interfaces').then(response => response.json());
                })
                .then(data => {
                    let html = '<table><tr><th>接口名称</th><th>IP地址</th><th>获取超链接</th></tr>';
                    // 检查数据是否存在且不为空
                    if (data && Array.isArray(data) && data.length > 0) {
                        data.forEach(iface => {
                            // 检查每个接口对象的属性是否存在
                            const name = iface.name || 'N/A';
                            const ip = iface.ip || 'N/A';
                            // 检查接口链接是否启用，默认为true
                            const showLinks = interfaceConfigs[name] !== undefined ? interfaceConfigs[name] : true;
                            html += '<tr><td>' + name + '</td><td>' + ip + '</td><td><label class="switch"><input type="checkbox" onchange="toggleInterfaceLink(\'' + name + '\', this.checked)" ' + (showLinks ? 'checked' : '') + '><span class="slider"></span></label></td></tr>';
                        });
                    } else {
                        html += '<tr><td colspan="3">未找到网络接口</td></tr>';
                    }
                    html += '</table>';
                    document.getElementById('interfaces-list').innerHTML = html;
                })
                .catch(error => {
                    console.error('Error loading interfaces:', error);
                    document.getElementById('interfaces-list').innerHTML = '<p>加载接口信息失败</p>';
                });
        }

        function loadServices() {
            // 先加载保存的服务名称和接口配置，再加载服务列表
            fetch('/saved-service-names')
                .then(response => response.json())
                .then(data => {
                    serviceNames = {};
                    interfaceConfigs = {};
                    columnConfigs = {};
                    
                    // 加载服务名称
                    if (data.service_names) {
                        data.service_names.forEach(mapping => {
                            serviceNames[mapping.service_id] = mapping.name;
                        });
                    }
                    
                    // 加载接口配置
                    if (data.interface_configs) {
                        data.interface_configs.forEach(config => {
                            interfaceConfigs[config.name] = config.show_links;
                        });
                    }
                    
                    // 加载列配置
                    if (data.column_configs) {
                        data.column_configs.forEach(config => {
                            if (!columnConfigs[config.table]) {
                                columnConfigs[config.table] = {};
                            }
                            columnConfigs[config.table][config.column] = config.visible;
                        });
                    }
                    
                    // 加载URL路径
                    if (data.url_paths) {
                        data.url_paths.forEach(mapping => {
                            urlPaths[mapping.service_id] = mapping.path;
                        });
                    }
                    
                    return Promise.all([
                        fetch('/services').then(response => response.json()),
                        fetch('/interfaces').then(response => response.json())
                    ]);
                })
                .then(([services, interfaces]) => {
                // 分离TCPv4、TCPv6、UDPv4和UDPv6服务
                const tcpv4Services = services.filter(service => service.protocol === 'tcp' && 
                    (service.local_addr === '0.0.0.0' || service.local_addr === '*' || 
                     (service.local_addr.indexOf(':') === -1 && service.local_addr !== '::')));
                const tcpv6Services = services.filter(service => service.protocol === 'tcp' && 
                    (service.local_addr === '::' || service.local_addr.indexOf(':') !== -1));
                const udpv4Services = services.filter(service => service.protocol === 'udp' && 
                    (service.local_addr === '0.0.0.0' || service.local_addr === '*' || 
                     (service.local_addr.indexOf(':') === -1 && service.local_addr !== '::')));
                const udpv6Services = services.filter(service => service.protocol === 'udp' && 
                    (service.local_addr === '::' || service.local_addr.indexOf(':') !== -1));
                
                // 排序服务（按端口号）
                tcpv4Services.sort((a, b) => parseInt(a.local_port) - parseInt(b.local_port));
                tcpv6Services.sort((a, b) => parseInt(a.local_port) - parseInt(b.local_port));
                udpv4Services.sort((a, b) => parseInt(a.local_port) - parseInt(b.local_port));
                udpv6Services.sort((a, b) => parseInt(a.local_port) - parseInt(b.local_port));
                
                // 显示TCPv4服务
                displayServices(tcpv4Services, interfaces, 'tcpv4-services-list');
                // 显示TCPv6服务
                displayServices(tcpv6Services, interfaces, 'tcpv6-services-list');
                // 显示UDPv4服务
                displayServices(udpv4Services, interfaces, 'udpv4-services-list');
                // 显示UDPv6服务
                displayServices(udpv6Services, interfaces, 'udpv6-services-list');
            })
            .catch(error => {
                console.error('Error loading services:', error);
                document.getElementById('tcpv4-services-list').innerHTML = '<p>加载服务信息失败</p>';
                document.getElementById('tcpv6-services-list').innerHTML = '<p>加载服务信息失败</p>';
                document.getElementById('udpv4-services-list').innerHTML = '<p>加载服务信息失败</p>';
                document.getElementById('udpv6-services-list').innerHTML = '<p>加载服务信息失败</p>';
            });
        }
        
        function displayServices(services, interfaces, elementId) {
            // 确定表格类型
            const tableType = elementId.replace('-services-list', '');
            
            // 默认列配置
            const defaultColumns = {
                'process_name': true,
                'service_name': true,
                'protocol': true,
                'listen_addr': true,
                'state': true,
                'url_path': true,
                'access_links': true
            };
            
            // 合并默认配置和用户配置
            if (!columnConfigs[tableType]) {
                columnConfigs[tableType] = {...defaultColumns};
            } else {
                // 确保所有列都在配置中
                for (let col in defaultColumns) {
                    if (columnConfigs[tableType][col] === undefined) {
                        columnConfigs[tableType][col] = defaultColumns[col];
                    }
                }
            }
            
            // 构建表头
            let html = '<table><tr>';
            
            if (columnConfigs[tableType]['process_name']) {
                html += '<th>进程名称</th>';
            }
            
            if (columnConfigs[tableType]['service_name']) {
                html += '<th>服务名称</th>';
            }
            
            if (columnConfigs[tableType]['protocol']) {
                html += '<th>协议</th>';
            }
            
            if (columnConfigs[tableType]['listen_addr']) {
                html += '<th onclick="sortTable(this, 0, \'' + elementId + '\')" style="cursor: pointer;">监听地址 <span style="font-size: 12px;">↕</span></th>';
            }
            
            if (columnConfigs[tableType]['state']) {
                html += '<th onclick="sortTable(this, 1, \'' + elementId + '\')" style="cursor: pointer;">状态 <span style="font-size: 12px;">↕</span></th>';
            }
            
            // 添加URL路径列标题
            if (columnConfigs[tableType]['url_path']) {
                html += '<th>URL路径</th>';
            }
            
            if (columnConfigs[tableType]['access_links']) {
                html += '<th>访问链接</th>';
            }
            
            // 在表头最右侧添加齿轮图标
            html += '<th style="text-align: right;"><span class="gear-icon" onclick="showColumnConfig(event)">⚙</span></th>';
            
            html += '</tr>';
            
            // 检查服务数据
            if (services && Array.isArray(services) && services.length > 0) {
                services.forEach(service => {
                    // 检查服务对象的属性
                    const name = service.name || 'N/A';
                    const protocol = service.protocol || 'N/A';
                    const localAddr = service.local_addr || 'N/A';
                    const localPort = service.local_port || 'N/A';
                    const state = service.state || 'N/A';
                    const pid = service.pid || '';
                    
                    // 生成唯一标识符用于编辑
                    const serviceId = localAddr + ':' + localPort + ':' + protocol;
                    
                    html += '<tr>';
                    
                    // 进程名称列
                    if (columnConfigs[tableType]['process_name']) {
                        // 转义引号以确保在HTML属性中正确显示
                        const escapedPid = pid.replace(/"/g, '&quot;');
                        html += '<td title="' + escapedPid + '" style="position: relative; cursor: pointer;" onmouseover="hoverEffect(this)" onmouseout="normalEffect(this)" onclick="copyToClipboard(this, \'' + escapedPid.replace(/'/g, "\\'") + '\')">' + name + '</td>';
                    }
                    
                    // 服务名称列
                    if (columnConfigs[tableType]['service_name']) {
                        // 获取服务名称，先从用户定义获取，再从端口映射获取
                        let serviceName = serviceNames[serviceId] || getServiceNameByPort(localPort);
                        
                        html += '<td id="service-name-' + serviceId + '">' + serviceName;
                        html += ' <span class="edit-icon" onclick="editServiceName(\'' + serviceId + '\', \'' + serviceName + '\')"><svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" fill="currentColor" class="bi bi-pencil" viewBox="0 0 16 16"><path d="M12.146.146a.5.5 0 0 1 .708 0l3 3a.5.5 0 0 1 0 .708l-10 10a.5.5 0 0 1-.168.11l-5 2a.5.5 0 0 1-.65-.65l2-5a.5.5 0 0 1 .11-.168l10-10zM11.207 2.5 13.5 4.793 14.793 3.5 12.5 1.207 11.207 2.5zm1.586 3L10.5 3.207 4 9.707V10h.5a.5.5 0 0 1 .5.5v.5h.5a.5.5 0 0 1 .5.5v.5h.293l6.5-6.5zm-9.761 5.175-.106.106-1.528 3.821 3.821-1.528.106-.106A.5.5 0 0 1 5 12.5V12h-.5a.5.5 0 0 1-.5-.5V11h-.5a.5.5 0 0 1-.468-.325z"/></svg></span></td>';
                    }
                    
                    // 协议列
                    if (columnConfigs[tableType]['protocol']) {
                        html += '<td>' + protocol + '</td>';
                    }
                    
                    // 监听地址列
                    if (columnConfigs[tableType]['listen_addr']) {
                        const fullAddress = localAddr + ':' + localPort;
                        let displayAddress = fullAddress;
                        // 对于udpv4和udpv6表格，只显示前12个字符
                        if ((tableType === 'udpv4' || tableType === 'udpv6') && fullAddress.length > 12) {
                            displayAddress = fullAddress.substring(0, 12) + '...';
                        }
                        
                        // 添加鼠标悬停显示完整内容和点击复制功能
                        const escapedFullAddress = fullAddress.replace(/"/g, '&quot;').replace(/'/g, '&#39;');
                        html += '<td title="' + escapedFullAddress + '" style="cursor: pointer;" onmouseover="hoverEffect(this)" onmouseout="normalEffect(this)" onclick="copyToClipboard(event, \'' + escapedFullAddress.replace(/'/g, "\\'") + '\')">' + displayAddress + '</td>';
                    }
                    
                    // 状态列
                    if (columnConfigs[tableType]['state']) {
                        html += '<td>' + state + '</td>';
                    }
                    
                    // URL路径列
                    if (columnConfigs[tableType]['url_path']) {
                        // 获取URL路径
                        let urlPath = urlPaths[serviceId] || '/';
                        
                        html += '<td id="url-path-' + serviceId + '">' + urlPath;
                        html += ' <span class="edit-icon" onclick="editURLPath(\'' + serviceId + '\', \'' + urlPath + '\')"><svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" fill="currentColor" class="bi bi-pencil" viewBox="0 0 16 16"><path d="M12.146.146a.5.5 0 0 1 .708 0l3 3a.5.5 0 0 1 0 .708l-10 10a.5.5 0 0 1-.168.11l-5 2a.5.5 0 0 1-.65-.65l2-5a.5.5 0 0 1 .11-.168l10-10zM11.207 2.5 13.5 4.793 14.793 3.5 12.5 1.207 11.207 2.5zm1.586 3L10.5 3.207 4 9.707V10h.5a.5.5 0 0 1 .5.5v.5h.5a.5.5 0 0 1 .5.5v.5h.293l6.5-6.5zm-9.761 5.175-.106.106-1.528 3.821 3.821-1.528.106-.106A.5.5 0 0 1 5 12.5V12h-.5a.5.5 0 0 1-.5-.5V11h-.5a.5.5 0 0 1-.468-.325z"/></svg></span></td>';
                    }
                    
                    // 访问链接列
                    if (columnConfigs[tableType]['access_links']) {
                        html += '<td>';
                        
                        // 检查接口数据
                        if (interfaces && Array.isArray(interfaces) && interfaces.length > 0) {
                            // 为每个接口生成链接
                            let linkCount = 0;
                            interfaces.forEach(iface => {
                                // 只有当接口启用时才显示链接
                                const showLinks = interfaceConfigs[iface.name] !== undefined ? interfaceConfigs[iface.name] : true;
                                if (showLinks && iface.ip) {  // 确保IP存在且接口启用
                                    if (linkCount > 0) {
                                        html += ' | '; // 添加分隔线
                                    }
                                    const targetAddr = (localAddr === "0.0.0.0" || localAddr === "*" || localAddr === "::") ? iface.ip : localAddr;
                                    // 获取URL路径
                                    const urlPath = urlPaths[serviceId] || '/';
                                    const fullAddress = targetAddr + ':' + localPort + urlPath;
                                    // 修改为显示网卡名称而不是IP地址
                                    html += '<a href="http://' + targetAddr + ':' + localPort + urlPath + '" target="_blank">' + iface.name + '</a> ' +
                                           '<button onclick="copyToClipboard(event, \'' + fullAddress.replace(/'/g, "\\'") + '\')" onmouseover="hoverEffect(this)" onmouseout="normalEffect(this)" style="margin-left: 5px; padding: 2px 5px; font-size: 12px;">复制</button>';
                                    linkCount++;
                                }
                            });
                            
                            // 如果没有启用的接口，显示提示
                            if (linkCount === 0) {
                                html += '无可用链接';
                            }
                        } else {
                            html += '无接口信息';
                        }
                        
                        html += '</td>';
                    }
                    
                    html += '</tr>';
                });
            } else {
                // 计算列数
                let colCount = 0;
                for (let col in columnConfigs[tableType]) {
                    if (columnConfigs[tableType][col]) {
                        colCount++;
                    }
                }
                
                html += '<tr><td colspan="' + colCount + '">未找到运行中的服务</td></tr>';
            }
            html += '</table>';
            document.getElementById(elementId).innerHTML = html;
        }
        
        function getServiceNameByPort(port) {
            const portMap = {
                "22": "sshd",
                "80": "http",
                "443": "http",
                "3306": "mysql",
                "5432": "postgresql",
                "6379": "redis",
                "8080": "http"
            };
            
            return portMap[port] || "N/A";
        }
        
        function editServiceName(serviceId, currentName) {
            const cell = document.getElementById('service-name-' + serviceId);
            const escapedCurrentName = currentName.replace(/"/g, '&quot;').replace(/'/g, '&#39;');
            cell.innerHTML = '<input type="text" class="edit-input" value="' + escapedCurrentName + '" id="edit-input-' + serviceId + '" onkeydown="handleEditKeyDown(event, \'' + serviceId + '\')"> ' +
                            '<span class="save-icon" onclick="saveServiceName(\'' + serviceId + '\')"><svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" fill="currentColor" class="bi bi-pencil" viewBox="0 0 16 16"><path d="M12.146.146a.5.5 0 0 1 .708 0l3 3a.5.5 0 0 1 0 .708l-10 10a.5.5 0 0 1-.168.11l-5 2a.5.5 0 0 1-.65-.65l2-5a.5.5 0 0 1 .11-.168l10-10zM11.207 2.5 13.5 4.793 14.793 3.5 12.5 1.207 11.207 2.5zm1.586 3L10.5 3.207 4 9.707V10h.5a.5.5 0 0 1 .5.5v.5h.5a.5.5 0 0 1 .5.5v.5h.293l6.5-6.5zm-9.761 5.175-.106.106-1.528 3.821 3.821-1.528.106-.106A.5.5 0 0 1 5 12.5V12h-.5a.5.5 0 0 1-.5-.5V11h-.5a.5.5 0 0 1-.468-.325z"/></svg></span> ' +
                            '<span class="cancel-icon" onclick="cancelEditServiceName(\'' + serviceId + '\', \'' + escapedCurrentName + '\')"><svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" fill="currentColor" class="bi bi-pencil" viewBox="0 0 16 16"><path d="M12.146.146a.5.5 0 0 1 .708 0l3 3a.5.5 0 0 1 0 .708l-10 10a.5.5 0 0 1-.168.11l-5 2a.5.5 0 0 1-.65-.65l2-5a.5.5 0 0 1 .11-.168l10-10zM11.207 2.5 13.5 4.793 14.793 3.5 12.5 1.207 11.207 2.5zm1.586 3L10.5 3.207 4 9.707V10h.5a.5.5 0 0 1 .5.5v.5h.5a.5.5 0 0 1 .5.5v.5h.293l6.5-6.5zm-9.761 5.175-.106.106-1.528 3.821 3.821-1.528.106-.106A.5.5 0 0 1 5 12.5V12h-.5a.5.5 0 0 1-.5-.5V11h-.5a.5.5 0 0 1-.468-.325z"/></svg></span>';
            document.getElementById('edit-input-' + serviceId).focus();
        }
        
        // 新增：处理编辑时的键盘事件
        function handleEditKeyDown(event, serviceId) {
            if (event.key === 'Enter') {
                saveServiceName(serviceId);
            }
        }
        
        function saveServiceName(serviceId) {
            const input = document.getElementById('edit-input-' + serviceId);
            const newName = input.value.trim();
            serviceNames[serviceId] = newName;
            
            // 发送请求到后端保存
            fetch('/save-service-name', {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json'
                },
                body: JSON.stringify({
                    service_id: serviceId,
                    name: newName
                })
            }).then(response => {
                if (!response.ok) {
                    throw new Error('保存失败');
                }
            }).catch(error => {
                console.error('保存服务名称失败:', error);
            });
            
            const cell = document.getElementById('service-name-' + serviceId);
            cell.innerHTML = newName + ' <span class="edit-icon" onclick="editServiceName(\'' + serviceId + '\', \'' + newName + '\')"><svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" fill="currentColor" class="bi bi-pencil" viewBox="0 0 16 16"><path d="M12.146.146a.5.5 0 0 1 .708 0l3 3a.5.5 0 0 1 0 .708l-10 10a.5.5 0 0 1-.168.11l-5 2a.5.5 0 0 1-.65-.65l2-5a.5.5 0 0 1 .11-.168l10-10zM11.207 2.5 13.5 4.793 14.793 3.5 12.5 1.207 11.207 2.5zm1.586 3L10.5 3.207 4 9.707V10h.5a.5.5 0 0 1 .5.5v.5h.5a.5.5 0 0 1 .5.5v.5h.293l6.5-6.5zm-9.761 5.175-.106.106-1.528 3.821 3.821-1.528.106-.106A.5.5 0 0 1 5 12.5V12h-.5a.5.5 0 0 1-.5-.5V11h-.5a.5.5 0 0 1-.468-.325z"/></svg></span>';
        }
        
        function cancelEditServiceName(serviceId, originalName) {
            const cell = document.getElementById('service-name-' + serviceId);
            cell.innerHTML = originalName + ' <span class="edit-icon" onclick="editServiceName(\'' + serviceId + '\', \'' + originalName + '\')"><svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" fill="currentColor" class="bi bi-pencil" viewBox="0 0 16 16"><path d="M12.146.146a.5.5 0 0 1 .708 0l3 3a.5.5 0 0 1 0 .708l-10 10a.5.5 0 0 1-.168.11l-5 2a.5.5 0 0 1-.65-.65l2-5a.5.5 0 0 1 .11-.168l10-10zM11.207 2.5 13.5 4.793 14.793 3.5 12.5 1.207 11.207 2.5zm1.586 3L10.5 3.207 4 9.707V10h.5a.5.5 0 0 1 .5.5v.5h.5a.5.5 0 0 1 .5.5v.5h.293l6.5-6.5zm-9.761 5.175-.106.106-1.528 3.821 3.821-1.528.106-.106A.5.5 0 0 1 5 12.5V12h-.5a.5.5 0 0 1-.5-.5V11h-.5a.5.5 0 0 1-.468-.325z"/></svg></span>';
        }
        
        function sortTable(header, columnIndex, elementId) {
            const table = header.parentElement.parentElement;
            const rows = Array.from(table.rows).slice(1); // 排除表头
            
            // 获取当前排序方向
            let sortOrder = header.getAttribute('data-order') || 'asc';
            sortOrder = sortOrder === 'asc' ? 'desc' : 'asc';
            header.setAttribute('data-order', sortOrder);
            
            // 只更新可排序列的表头（监听地址和状态列）
            const headers = table.querySelectorAll('th');
            headers.forEach((th, index) => {
                // 只处理第4列（监听地址）和第5列（状态）
                if (index === 3 || index === 4) {
                    const arrowElement = th.querySelector('span');
                    if (th === header) {
                        arrowElement.textContent = sortOrder === 'asc' ? '▲' : '▼';
                    } else {
                        arrowElement.textContent = '↕';
                        th.setAttribute('data-order', '');
                    }
                }
            });
            
            // 排序
            rows.sort((a, b) => {
                let aValue = a.cells[columnIndex + 3].textContent.trim(); // +3 because we have 3 non-sortable columns before
                let bValue = b.cells[columnIndex + 3].textContent.trim(); // +3 because we have 3 non-sortable columns before
                
                // 特殊处理监听地址列（按端口号排序）
                if (columnIndex === 0) {
                    const aPort = parseInt(aValue.split(':')[1]) || 0;
                    const bPort = parseInt(bValue.split(':')[1]) || 0;
                    return sortOrder === 'asc' ? aPort - bPort : bPort - aPort;
                }
                // 特殊处理状态列（按首字母排序）
                else if (columnIndex === 1) {
                    return sortOrder === 'asc' ? 
                        aValue.localeCompare(bValue) : 
                        bValue.localeCompare(aValue);
                }
            });
            
            // 重新插入行
            const tbody = table.querySelector('tbody') || table;
            rows.forEach(row => tbody.appendChild(row));
        }
        
        function copyToClipboard(event, text) {
            // 阻止事件冒泡，防止触发其他点击事件
            if (event && event.stopPropagation) {
                event.stopPropagation();
            }
            
            // 如果文本为空，则不执行复制操作
            if (!text) {
                showCopyAlert('没有内容可复制', false);
                return;
            }
            
            // 尝试使用现代 clipboard API
            if (navigator.clipboard && window.isSecureContext) {
                navigator.clipboard.writeText(text).then(() => {
                    showCopyAlert('复制成功', true);
                }).catch(err => {
                    console.error('复制失败: ', err);
                    showCopyAlert('复制失败', false);
                });
            } else {
                // 降级使用 document.execCommand('copy')
                const textArea = document.createElement("textarea");
                textArea.value = text;
                
                // 避免滚动到底部
                textArea.style.top = "0";
                textArea.style.left = "0";
                textArea.style.position = "fixed";
                textArea.style.opacity = "0";
                
                document.body.appendChild(textArea);
                textArea.focus();
                textArea.select();
                
                try {
                    const successful = document.execCommand('copy');
                    showCopyAlert(successful ? '复制成功' : '复制失败', successful);
                } catch (err) {
                    console.error('复制失败: ', err);
                    showCopyAlert('复制失败', false);
                }
                
                document.body.removeChild(textArea);
            }
        }
        
        function showCopyAlert(message, isSuccess) {
            // 检查是否已存在提示框，如果存在则先移除
            const existingAlert = document.getElementById('copy-alert');
            if (existingAlert) {
                existingAlert.remove();
            }
            
            // 创建提示元素
            const alert = document.createElement('div');
            alert.id = 'copy-alert';
            alert.textContent = message;
            alert.style.position = 'fixed';
            alert.style.top = '20px';
            alert.style.left = '50%';
            alert.style.transform = 'translateX(-50%)';
            alert.style.color = 'white';
            alert.style.padding = '10px 20px';
            alert.style.borderRadius = '4px';
            alert.style.zIndex = '1000';
            alert.style.boxShadow = '0 2px 5px rgba(0,0,0,0.2)';
            alert.style.transition = 'opacity 0.5s';
            alert.style.opacity = '1';
            alert.style.minWidth = '150px';
            alert.style.textAlign = 'center';
            
            if (isSuccess) {
                // 修改为与刷新按钮相同的颜色
                alert.style.backgroundColor = '#007bff';
                alert.style.border = '2px solid #007bff';
            } else {
                alert.style.backgroundColor = 'white';
                alert.style.color = '#f44336';
                alert.style.border = '2px solid #f44336';
            }
            
            document.body.appendChild(alert);

            // 2秒后自动移除提示并添加淡出效果
            setTimeout(() => {
                alert.style.opacity = '0';
                setTimeout(() => {
                    if (alert.parentNode) {
                        document.body.removeChild(alert);
                    }
                }, 500);
            }, 2000);
        }
        
        // 添加接口链接开关控制函数
        function toggleInterfaceLink(interfaceName, isChecked) {
            // 更新本地配置
            interfaceConfigs[interfaceName] = isChecked;
            
            // 发送请求到后端保存
            fetch('/save-service-name', {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json'
                },
                body: JSON.stringify({
                    interface_name: interfaceName,
                    show_links: isChecked,
                    type: 'interface_config'
                })
            }).then(response => {
                if (!response.ok) {
                    throw new Error('保存失败');
                }
                // 重新加载服务以更新链接显示
                loadServices();
            }).catch(error => {
                console.error('保存接口配置失败:', error);
            });
        }
        
        // 显示列配置弹窗
        function showColumnConfig(event) {
            event.stopPropagation();
            
            // 获取所有表格的列配置（使用第一个非空的，或默认配置）
            let unifiedConfig = {
                'process_name': true,
                'service_name': true,
                'protocol': true,
                'listen_addr': true,
                'state': true,
                'url_path': true,
                'access_links': true
            };
            
            // 查找现有的配置
            for (let tableType in columnConfigs) {
                if (columnConfigs[tableType]) {
                    unifiedConfig = {...columnConfigs[tableType]};
                    break;
                }
            }
            
            // 构建配置选项
            let optionsHtml = '';
            const columnLabels = {
                'process_name': '进程名称',
                'service_name': '服务名称',
                'protocol': '协议',
                'listen_addr': '监听地址',
                'state': '状态',
                'url_path': 'URL路径',
                'access_links': '访问链接'
            };
            
            for (let column in columnLabels) {
                const checked = unifiedConfig[column] ? 'checked' : '';
                optionsHtml += '<div><label><input type="checkbox" id="col-' + column + '" ' + checked + '> ' + columnLabels[column] + '</label></div>';
            }
            
            document.getElementById('column-config-options').innerHTML = optionsHtml;
            document.getElementById('column-config-modal').style.display = 'block';
        }
        
        // 关闭列配置弹窗
        function closeColumnConfig() {
            document.getElementById('column-config-modal').style.display = 'none';
        }
        
        // 保存列配置
        function saveColumnConfig() {
            // 获取配置
            const columnNames = ['process_name', 'service_name', 'protocol', 'listen_addr', 'state', 'url_path', 'access_links'];
            const config = {};
            columnNames.forEach(col => {
                const checkbox = document.getElementById('col-' + col);
                if (checkbox) {
                    config[col] = checkbox.checked;
                }
            });
            
            // 更新所有表格的内存配置
            const tableTypes = ['tcpv4', 'tcpv6', 'udpv4', 'udpv6'];
            tableTypes.forEach(tableType => {
                if (!columnConfigs[tableType]) {
                    columnConfigs[tableType] = {};
                }
                columnNames.forEach(col => {
                    columnConfigs[tableType][col] = config[col];
                });
            });
            
            // 发送到服务器保存（使用任意一个表格类型，因为配置是统一的）
            fetch('/save-column-config', {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json'
                },
                body: JSON.stringify({
                    table: 'tcpv4', // 使用一个默认值
                    column_configs: config
                })
            }).then(response => {
                if (!response.ok) {
                    throw new Error('保存失败');
                }
                // 重新加载服务以更新显示
                loadServices();
                closeColumnConfig();
            }).catch(error => {
                console.error('保存列配置失败:', error);
            });
        }
        
        // 点击弹窗外部关闭弹窗
        window.onclick = function(event) {
            const modal = document.getElementById('column-config-modal');
            if (event.target == modal) {
                closeColumnConfig();
            }
        }
        
        // 添加悬停效果函数
        function hoverEffect(element) {
            element.style.fontWeight = 'bold';
            element.style.color = '#007bff'; // 与刷新按钮相同的颜色
        }
        
        // 添加恢复正常样式函数
        function normalEffect(element) {
            element.style.fontWeight = 'normal';
            element.style.color = '';
        }
        
        // 添加编辑URL路径的函数
        function editURLPath(serviceId, currentPath) {
            const cell = document.getElementById('url-path-' + serviceId);
            const escapedCurrentPath = currentPath.replace(/"/g, '&quot;').replace(/'/g, '&#39;');
            cell.innerHTML = '<input type="text" class="edit-input" value="' + escapedCurrentPath + '" id="edit-url-input-' + serviceId + '" onkeydown="handleURLPathKeyDown(event, \'' + serviceId + '\')"> ' +
                            '<span class="save-icon" onclick="saveURLPath(\'' + serviceId + '\')"><svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" fill="currentColor" class="bi bi-pencil" viewBox="0 0 16 16"><path d="M12.146.146a.5.5 0 0 1 .708 0l3 3a.5.5 0 0 1 0 .708l-10 10a.5.5 0 0 1-.168.11l-5 2a.5.5 0 0 1-.65-.65l2-5a.5.5 0 0 1 .11-.168l10-10zM11.207 2.5 13.5 4.793 14.793 3.5 12.5 1.207 11.207 2.5zm1.586 3L10.5 3.207 4 9.707V10h.5a.5.5 0 0 1 .5.5v.5h.5a.5.5 0 0 1 .5.5v.5h.293l6.5-6.5zm-9.761 5.175-.106.106-1.528 3.821 3.821-1.528.106-.106A.5.5 0 0 1 5 12.5V12h-.5a.5.5 0 0 1-.5-.5V11h-.5a.5.5 0 0 1-.468-.325z"/></svg></span> ' +
                            '<span class="cancel-icon" onclick="cancelEditURLPath(\'' + serviceId + '\', \'' + escapedCurrentPath + '\')"><svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" fill="currentColor" class="bi bi-pencil" viewBox="0 0 16 16"><path d="M12.146.146a.5.5 0 0 1 .708 0l3 3a.5.5 0 0 1 0 .708l-10 10a.5.5 0 0 1-.168.11l-5 2a.5.5 0 0 1-.65-.65l2-5a.5.5 0 0 1 .11-.168l10-10zM11.207 2.5 13.5 4.793 14.793 3.5 12.5 1.207 11.207 2.5zm1.586 3L10.5 3.207 4 9.707V10h.5a.5.5 0 0 1 .5.5v.5h.5a.5.5 0 0 1 .5.5v.5h.293l6.5-6.5zm-9.761 5.175-.106.106-1.528 3.821 3.821-1.528.106-.106A.5.5 0 0 1 5 12.5V12h-.5a.5.5 0 0 1-.5-.5V11h-.5a.5.5 0 0 1-.468-.325z"/></svg></span>';
            document.getElementById('edit-url-input-' + serviceId).focus();
        }
        
        // 添加处理URL路径编辑时的键盘事件
        function handleURLPathKeyDown(event, serviceId) {
            if (event.key === 'Enter') {
                saveURLPath(serviceId);
            }
        }
        
        // 添加保存URL路径的函数
        function saveURLPath(serviceId) {
            const input = document.getElementById('edit-url-input-' + serviceId);
            let newPath = input.value.trim();
            
            // 确保路径以/开头
            if (newPath === "") {
                newPath = "/";
            } else if (!newPath.startsWith("/")) {
                newPath = "/" + newPath;
            }
            
            urlPaths[serviceId] = newPath;
            
            // 发送请求到后端保存
            fetch('/save-url-path', {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json'
                },
                body: JSON.stringify({
                    service_id: serviceId,
                    path: newPath
                })
            }).then(response => {
                if (!response.ok) {
                    throw new Error('保存失败');
                }
                // 保存成功后刷新服务列表
                loadServices();
            }).catch(error => {
                console.error('保存URL路径失败:', error);
            });
            
            const cell = document.getElementById('url-path-' + serviceId);
            cell.innerHTML = newPath + ' <span class="edit-icon" onclick="editURLPath(\'' + serviceId + '\', \'' + newPath + '\')"><svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" fill="currentColor" class="bi bi-pencil" viewBox="0 0 16 16"><path d="M12.146.146a.5.5 0 0 1 .708 0l3 3a.5.5 0 0 1 0 .708l-10 10a.5.5 0 0 1-.168.11l-5 2a.5.5 0 0 1-.65-.65l2-5a.5.5 0 0 1 .11-.168l10-10zM11.207 2.5 13.5 4.793 14.793 3.5 12.5 1.207 11.207 2.5zm1.586 3L10.5 3.207 4 9.707V10h.5a.5.5 0 0 1 .5.5v.5h.5a.5.5 0 0 1 .5.5v.5h.293l6.5-6.5zm-9.761 5.175-.106.106-1.528 3.821 3.821-1.528.106-.106A.5.5 0 0 1 5 12.5V12h-.5a.5.5 0 0 1-.5-.5V11h-.5a.5.5 0 0 1-.468-.325z"/></svg></span>';
        }
        
        // 添加取消编辑URL路径的函数
        function cancelEditURLPath(serviceId, originalPath) {
            const cell = document.getElementById('url-path-' + serviceId);
            cell.innerHTML = originalPath + ' <span class="edit-icon" onclick="editURLPath(\'' + serviceId + '\', \'' + originalPath + '\')"><svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" fill="currentColor" class="bi bi-pencil" viewBox="0 0 16 16"><path d="M12.146.146a.5.5 0 0 1 .708 0l3 3a.5.5 0 0 1 0 .708l-10 10a.5.5 0 0 1-.168.11l-5 2a.5.5 0 0 1-.65-.65l2-5a.5.5 0 0 1 .11-.168l10-10zM11.207 2.5 13.5 4.793 14.793 3.5 12.5 1.207 11.207 2.5zm1.586 3L10.5 3.207 4 9.707V10h.5a.5.5 0 0 1 .5.5v.5h.5a.5.5 0 0 1 .5.5v.5h.293l6.5-6.5zm-9.761 5.175-.106.106-1.528 3.821 3.821-1.528.106-.106A.5.5 0 0 1 5 12.5V12h-.5a.5.5 0 0 1-.5-.5V11h-.5a.5.5 0 0 1-.468-.325z"/></svg></span>';
        }
    </script>
</body>
</html>
`
	t, _ := template.New("index").Parse(tmpl)
	t.Execute(w, nil)
}

func servicesHandler(w http.ResponseWriter, r *http.Request) {
	log.Println("接收到获取服务列表的请求")
	services, err := getServices()
	if err != nil {
		log.Printf("获取服务信息失败: %v\n", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(services)
	log.Printf("成功返回 %d 个服务\n", len(services))
}

func interfacesHandler(w http.ResponseWriter, r *http.Request) {
	log.Println("接收到获取网络接口列表的请求")
	interfaces, err := getNetworkInterfaces()
	if err != nil {
		log.Printf("获取接口信息失败: %v\n", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(interfaces)
	log.Printf("成功返回 %d 个网络接口\n", len(interfaces))
}

// 保存服务名称处理器
func saveServiceNameHandler(w http.ResponseWriter, r *http.Request) {
	log.Println("接收到保存配置的请求")
	if r.Method != http.MethodPost {
		log.Printf("请求方法错误: %s\n", r.Method)
		http.Error(w, "只支持POST方法", http.StatusMethodNotAllowed)
		return
	}

	var data map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&data); err != nil {
		log.Printf("解析JSON数据失败: %v\n", err)
		http.Error(w, "无效的JSON数据", http.StatusBadRequest)
		return
	}

	// 判断是保存服务名称还是接口配置
	if configType, ok := data["type"].(string); ok && configType == "interface_config" {
		log.Println("处理接口配置保存请求")
		// 保存接口配置
		if name, ok := data["interface_name"].(string); ok {
			if showLinks, ok := data["show_links"].(bool); ok {
				interfaceConfigs[name] = showLinks
				log.Printf("更新接口配置: %s = %v\n", name, showLinks)

				// 保存到文件
				if err := saveServiceNames(); err != nil {
					log.Printf("保存接口配置失败: %v\n", err)
					http.Error(w, "保存失败", http.StatusInternalServerError)
					return
				}

				w.WriteHeader(http.StatusOK)
				w.Write([]byte("保存成功"))
				log.Println("接口配置保存成功")
				return
			}
		}
		log.Println("无效的接口配置数据")
		http.Error(w, "无效的接口配置数据", http.StatusBadRequest)
		return
	} else {
		log.Println("处理服务名称保存请求")
		// 保存服务名称（使用已解析的数据）
		serviceID, ok1 := data["service_id"].(string)
		name, ok2 := data["name"].(string)

		if !ok1 || !ok2 {
			log.Println("无效的服务名称数据")
			http.Error(w, "无效的服务名称数据", http.StatusBadRequest)
			return
		}

		// 更新内存中的映射
		serviceNames[serviceID] = name
		log.Printf("更新服务名称映射: %s = %s\n", serviceID, name)

		// 保存到文件
		if err := saveServiceNames(); err != nil {
			log.Printf("保存服务名称失败: %v\n", err)
			http.Error(w, "保存失败", http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusOK)
		w.Write([]byte("保存成功"))
		log.Println("服务名称保存成功")
	}
}

// 添加获取已保存服务名称的处理器
func savedServiceNamesHandler(w http.ResponseWriter, r *http.Request) {
	log.Println("接收到获取已保存配置的请求")
	w.Header().Set("Content-Type", "application/json")

	// 准备服务名称数据
	var nameMappings []ServiceNameMapping
	for serviceID, name := range serviceNames {
		nameMappings = append(nameMappings, ServiceNameMapping{
			Service_id: serviceID,
			Name:       name,
		})
	}

	// 准备接口配置数据
	var interfaceConfigsData []InterfaceConfig
	for name, showLinks := range interfaceConfigs {
		interfaceConfigsData = append(interfaceConfigsData, InterfaceConfig{
			Name:      name,
			ShowLinks: showLinks,
		})
	}

	// 准备列配置数据
	var columnConfigsData []ColumnConfig
	for table, columns := range columnConfigs {
		for column, visible := range columns {
			columnConfigsData = append(columnConfigsData, ColumnConfig{
				Table:   table,
				Column:  column,
				Visible: visible,
			})
		}
	}

	// 准备URL路径数据
	var urlPathMappings []URLPathMapping
	for serviceID, path := range urlPaths {
		urlPathMappings = append(urlPathMappings, URLPathMapping{
			Service_id: serviceID,
			Path:       path,
		})
	}

	// 组合所有数据
	data := map[string]interface{}{
		"service_names":     nameMappings,
		"interface_configs": interfaceConfigsData,
		"column_configs":    columnConfigsData,
		"url_paths":         urlPathMappings, // 添加URL路径数据
	}

	json.NewEncoder(w).Encode(data)
	log.Printf("返回已保存配置: 服务名称 %d 个, 接口配置 %d 个\n", len(nameMappings), len(interfaceConfigsData))
}

// 添加保存列配置的处理器
func saveColumnConfigHandler(w http.ResponseWriter, r *http.Request) {
	log.Println("接收到保存列配置的请求")
	if r.Method != http.MethodPost {
		log.Printf("请求方法错误: %s\n", r.Method)
		http.Error(w, "只支持POST方法", http.StatusMethodNotAllowed)
		return
	}

	var requestData struct {
		Table         string          `json:"table"`
		ColumnConfigs map[string]bool `json:"column_configs"`
	}

	if err := json.NewDecoder(r.Body).Decode(&requestData); err != nil {
		log.Printf("解析JSON数据失败: %v\n", err)
		http.Error(w, "无效的JSON数据", http.StatusBadRequest)
		return
	}

	// 更新内存中的配置 - 为所有表格类型统一配置

	// 修改为统一更新所有表格类型的配置
	tableTypes := []string{"tcpv4", "tcpv6", "udpv4", "udpv6"}
	for _, tableType := range tableTypes {
		if columnConfigs[tableType] == nil {
			columnConfigs[tableType] = make(map[string]bool)
		}
		for column, visible := range requestData.ColumnConfigs {
			columnConfigs[tableType][column] = visible
			log.Printf("更新列配置: %s.%s = %v\n", tableType, column, visible)
		}
	}

	// 保存到文件
	if err := saveServiceNames(); err != nil {
		log.Printf("保存列配置失败: %v\n", err)
		http.Error(w, "保存失败", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("保存成功"))
	log.Println("列配置保存成功")
}

// 添加保存URL路径的处理器
func saveURLPathHandler(w http.ResponseWriter, r *http.Request) {
	log.Println("接收到保存URL路径的请求")
	if r.Method != http.MethodPost {
		log.Printf("请求方法错误: %s\n", r.Method)
		http.Error(w, "只支持POST方法", http.StatusMethodNotAllowed)
		return
	}

	var data struct {
		ServiceID string `json:"service_id"`
		Path      string `json:"path"`
	}

	if err := json.NewDecoder(r.Body).Decode(&data); err != nil {
		log.Printf("解析JSON数据失败: %v\n", err)
		http.Error(w, "无效的JSON数据", http.StatusBadRequest)
		return
	}

	// 确保路径以/开头
	path := data.Path
	if path == "" {
		path = "/"
	} else if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}

	// 更新内存中的映射
	urlPaths[data.ServiceID] = path
	log.Printf("更新URL路径映射: %s = %s\n", data.ServiceID, path)

	// 保存到文件
	if err := saveServiceNames(); err != nil {
		log.Printf("保存URL路径失败: %v\n", err)
		http.Error(w, "保存失败", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("保存成功"))
	log.Println("URL路径保存成功")
}

func getServices() ([]Service, error) {
	// 使用ss命令获取网络连接信息
	cmd := exec.Command("ss", "-tulnp")
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	lines := strings.Split(string(output), "\n")
	var services []Service

	// 解析输出
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) < 6 || (fields[0] != "tcp" && fields[0] != "udp") {
			continue
		}

		// 解析本地地址和端口
		localAddrPort := strings.Split(fields[4], ":")
		if len(localAddrPort) < 2 {
			continue
		}

		localAddr := strings.Join(localAddrPort[:len(localAddrPort)-1], ":")
		// 处理IPv6地址
		if strings.HasPrefix(localAddr, "[") && strings.HasSuffix(localAddr, "]") {
			localAddr = localAddr[1 : len(localAddr)-1]
		}
		localPort := localAddrPort[len(localAddrPort)-1]

		// 解析进程信息
		processInfo := ""
		if len(fields) > 6 {
			processInfo = strings.Join(fields[6:], " ")
		}

		// 解析进程名，只取第一个引号中的内容
		processName := "N/A"
		if processInfo != "" {
			re := regexp.MustCompile(`users:\(\("([^"]+)".*?\)`)
			matches := re.FindStringSubmatch(processInfo)
			if len(matches) > 1 {
				processName = matches[1]
			}
		}

		// 跳过某些端口（如SSH等系统端口可以在这里过滤）
		service := Service{
			Name:        processName, // 进程名称保持从process获取
			Protocol:    fields[0],
			LocalAddr:   localAddr,
			LocalPort:   localPort,
			State:       getServiceState(fields[1]),
			ForeignAddr: "",
			PID:         processInfo, // 保存完整进程信息用于悬停显示
		}

		services = append(services, service)
	}

	return services, nil
}

func getServiceName(port string) string {
	portMap := map[string]string{
		"22":   "SSH",
		"80":   "HTTP",
		"443":  "HTTPS",
		"3306": "MySQL",
		"5432": "PostgreSQL",
		"6379": "Redis",
		"8080": "HTTP Alt",
	}

	if name, exists := portMap[port]; exists {
		return name
	}
	return "Unknown Service"
}

func getServiceState(state string) string {
	stateMap := map[string]string{
		"LISTEN":     "Listening",
		"ESTAB":      "Established",
		"TIME-WAIT":  "Time Wait",
		"CLOSE-WAIT": "Close Wait",
	}

	if name, exists := stateMap[state]; exists {
		return name
	}
	return state
}

func getNetworkInterfaces() ([]InterfaceInfo, error) {
	var interfaces []InterfaceInfo

	// 获取网络接口信息
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}

	for _, iface := range ifaces {
		// 检查是否需要排除该接口
		if shouldExcludeInterface(iface.Name) || iface.Name == "docker0" {
			continue
		}

		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}

		// 收集该接口的所有有效IPv4地址
		var validIPs []string
		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}

			// 跳过IPv6和回环地址
			if ip.To4() == nil || ip.IsLoopback() {
				continue
			}

			// 跳过docker等虚拟接口的地址
			if ip.IsLinkLocalUnicast() {
				continue
			}

			validIPs = append(validIPs, ip.String())
		}

		// 如果有有效IP，则添加到结果中
		if len(validIPs) > 0 {
			// 添加所有有效IP地址
			for _, ip := range validIPs {
				// 确保接口名称和IP都不为空
				if iface.Name != "" && ip != "" {
					interfaces = append(interfaces, InterfaceInfo{
						Name: iface.Name,
						IP:   ip,
					})
				}
			}
		}
	}

	// 获取公网IP
	publicIP, err := getPublicIP()
	if err == nil && publicIP != "" {
		interfaces = append(interfaces, InterfaceInfo{
			Name: "公网",
			IP:   publicIP,
		})
	}

	log.Printf("获取到 %d 个网络接口\n", len(interfaces))
	return interfaces, nil
}

// 获取公网IP地址
func getPublicIP() (string, error) {
	// 读取YAML配置获取公网IP服务地址
	yamlConfig := &YAMLConfig{
		ServiceConfig: []struct {
			Addr     string `yaml:"addr"`
			Port     int    `yaml:"port"`
			Exclude  string `yaml:"exclude"`
			GetIpUrl string `yaml:"get_ip_url"`
		}{
			{
				Addr:     "0.0.0.0", // 设置默认监听地址
				Port:     10810,
				Exclude:  "lo,br-,veth",
				GetIpUrl: "https://4.ipw.cn", // 默认公网IP服务地址
			},
		},
	}

	// 修改配置文件路径
	configPath := "/opt/port-monitor/config.yaml"
	if _, err := os.Stat(configPath); err == nil {
		yamlFile, err := os.ReadFile(configPath)
		if err == nil {
			err = yaml.Unmarshal(yamlFile, yamlConfig)
			if err != nil {
				log.Printf("解析config.yaml失败: %v\n", err)
			}
		}
	}

	// 检查配置的URL是否为空
	if len(yamlConfig.ServiceConfig) == 0 || yamlConfig.ServiceConfig[0].GetIpUrl == "" {
		return "", fmt.Errorf("get_ip_url配置为空")
	}

	// 发送HTTP请求获取公网IP
	resp, err := http.Get(yamlConfig.ServiceConfig[0].GetIpUrl)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	// 读取响应内容
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	// 返回公网IP地址
	return strings.TrimSpace(string(body)), nil
}

func shouldExcludeInterface(name string) bool {
	for _, prefix := range config.ExcludeInterfaces {
		if matched, _ := regexp.MatchString("^"+prefix+".*", name); matched {
			return true
		}
	}
	return false
}
