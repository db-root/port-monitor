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
    return fetch('/api/saved-service-names')
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
    fetch('/api/saved-service-names')
        .then(response => response.json())
        .then(data => {
            // 从返回数据中提取接口配置
            if (data.interface_configs) {
                interfaceConfigs = {};
                data.interface_configs.forEach(config => {
                    interfaceConfigs[config.name] = config.show_links;
                });
            }
            return fetch('/api/interfaces').then(response => response.json());
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
    fetch('/api/saved-service-names')
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
                fetch('/api/services').then(response => response.json()),
                fetch('/api/interfaces').then(response => response.json())
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
        
        html += '<tr><td colspan="' + (colCount + 1) + '">未找到运行中的服务</td></tr>';
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
    fetch('/api/save-service-name', {
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
    fetch('/api/save-service-name', {
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
    fetch('/api/save-column-config', {
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
    fetch('/api/save-url-path', {
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