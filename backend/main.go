package backend

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
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

// 使用相对路径而不是绝对路径
var dataFile = "data.json"

// 添加日志文件
var logFile *os.File

// 获取项目根目录路径
func getProjectRoot() string {
	// 获取当前工作目录
	wd, err := os.Getwd()
	if err != nil {
		log.Fatal(err)
	}
	return wd
}

// 获取配置文件路径
func getConfigPath() string {
	// 尝试项目根目录下的config文件夹
	projectRoot := getProjectRoot()
	configPath := filepath.Join(projectRoot, "config", "config.yaml")
	
	// 如果config目录不存在，则尝试当前目录下的config.yaml
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		configPath = filepath.Join(projectRoot, "config.yaml")
		// 如果还是不存在，则使用backend目录下的config.yaml
		if _, err := os.Stat(configPath); os.IsNotExist(err) {
			configPath = "config.yaml"
		}
	}
	
	return configPath
}

// 获取前端静态文件路径
func getFrontendPath() string {
	projectRoot := getProjectRoot()
	frontendPath := filepath.Join(projectRoot, "frontend", "static")
	
	// 如果frontend目录不存在，则尝试当前目录下的frontend
	if _, err := os.Stat(frontendPath); os.IsNotExist(err) {
		frontendPath = filepath.Join("frontend", "static")
	}
	
	return frontendPath
}

// 获取索引文件路径
func getIndexFilePath() string {
	projectRoot := getProjectRoot()
	indexFilePath := filepath.Join(projectRoot, "frontend", "static", "index.html")
	
	// 如果文件不存在，尝试当前目录下的路径
	if _, err := os.Stat(indexFilePath); os.IsNotExist(err) {
		indexFilePath = filepath.Join("frontend", "static", "index.html")
	}
	
	return indexFilePath
}

// StartServer 启动Web服务器
func StartServer() {
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

	// 获取配置文件路径
	configPath := getConfigPath()

	// 检查配置文件是否存在，如果不存在则创建
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		log.Println("config.yaml文件不存在，正在生成默认配置文件...")

		// 创建默认配置
		defaultConfig := `service-config:
  - addr: "0.0.0.0"  # 监听地址
    port: 10810           # 监听端口
    exclude: "lo,br-,veth,docker0" # 忽略网卡
    get_ip_url: "https://4.ipw.cn"  # 公网IP服务地址
`

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
	logFile, err := os.OpenFile("server.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		fmt.Printf("无法打开日志文件: %v\n", err)
		os.Exit(1)
	}
	defer logFile.Close()

	// 设置日志输出到文件和控制台
	log.SetOutput(io.MultiWriter(os.Stdout, logFile))

	// 加载已保存的服务名称
	loadServiceNames()

	// 设置API路由
	http.HandleFunc("/api/services", servicesHandler)
	http.HandleFunc("/api/interfaces", interfacesHandler)
	// 添加保存服务名称的路由
	http.HandleFunc("/api/save-service-name", saveServiceNameHandler)
	// 添加获取已保存服务名称的路由
	http.HandleFunc("/api/saved-service-names", savedServiceNamesHandler)
	// 添加保存列配置的路由
	http.HandleFunc("/api/save-column-config", saveColumnConfigHandler)
	// 添加保存URL路径的路由
	http.HandleFunc("/api/save-url-path", saveURLPathHandler)

	// 移除静态文件处理器，由前端路由处理
	// 前端构建后的文件将通过根路径处理器提供服务

	// 设置主页路由
	http.HandleFunc("/", indexHandler)

	log.Printf("服务器启动，监听地址: %s:%d\n", mainConfig.Addr, config.WebPort)
	log.Fatal(http.ListenAndServe(fmt.Sprintf("%s:%d", mainConfig.Addr, config.WebPort), nil))
}

// 首页处理器
func indexHandler(w http.ResponseWriter, r *http.Request) {
	// 获取项目根目录
	projectRoot := getProjectRoot()
	
	// 构建前端文件路径
	indexFilePath := filepath.Join(projectRoot, "frontend", "static", "index.html")
	
	// 如果文件不存在，尝试当前目录下的路径
	if _, err := os.Stat(indexFilePath); os.IsNotExist(err) {
		indexFilePath = filepath.Join("frontend", "static", "index.html")
	}
	
	// 如果请求的是根路径，则返回前端页面
	if r.URL.Path == "/" {
		http.ServeFile(w, r, indexFilePath)
		return
	}

	// 其他情况返回404
	http.NotFound(w, r)
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
	// 获取配置文件路径
	configPath := getConfigPath()
	
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