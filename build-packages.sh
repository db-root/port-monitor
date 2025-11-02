#!/bin/bash

# Port Monitor 打包脚本
# 该脚本使用Docker容器构建RPM和DEB包

set -e

PROJECT_NAME="port-monitor"
VERSION="${1:-1.0.0}"  # 从命令行参数获取版本号，默认为1.0.0
GITHUB_URL="https://github.com/db-root/port-monitor"  # 更新为新的GitHub URL
PROJECT_DIR=$(pwd)
BUILD_DIR="${PROJECT_DIR}/build"
OUTPUT_DIR="${PROJECT_DIR}/dist"

# 检查Docker是否可用
if ! command -v docker &> /dev/null; then
    echo "错误: 未找到Docker，请先安装Docker"
    exit 1
fi

# 清理并创建构建目录
echo "清理构建目录..."
rm -rf "${BUILD_DIR}" "${OUTPUT_DIR}"
mkdir -p "${BUILD_DIR}" "${OUTPUT_DIR}"

echo "构建 ${PROJECT_NAME} 版本 ${VERSION}"

# 构建Go二进制文件
echo "构建Go二进制文件..."
CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o "${BUILD_DIR}/${PROJECT_NAME}" .

# 复制文件到构建目录
echo "复制项目文件..."
mkdir -p "${BUILD_DIR}/frontend/static"
cp -r frontend/static/* "${BUILD_DIR}/frontend/static/"
cp config.yaml "${BUILD_DIR}/"

# 创建RPM包
build_rpm() {
    echo "构建RPM包..."

    # 使用docker运行rpmbuild
    echo "正在运行Docker容器构建RPM..."
    if docker run --rm \
        -v "${PROJECT_DIR}:/source" \
        -v "${OUTPUT_DIR}:/output" \
        centos:8 \
        bash -c "
            # 配置清华源
            sed -i 's/mirrorlist/#mirrorlist/g' /etc/yum.repos.d/CentOS-*
            sed -i 's|#baseurl=http://mirror.centos.org|baseurl=https://mirrors.tuna.tsinghua.edu.cn/centos-vault|g' /etc/yum.repos.d/CentOS-*
            sed -i 's|centos/\$releasever|centos-vault/8.0.1905|g' /etc/yum.repos.d/CentOS-*
            
            yum makecache
            yum install -y rpm-build
            yum clean all
            
            # 创建构建目录结构
            echo '创建构建目录结构...'
            mkdir -p /root/rpmbuild/{BUILD,RPMS,SOURCES,SPECS,SRPMS}
            
            # 创建SPEC文件
            echo '创建SPEC文件...'
            cat > /root/rpmbuild/SPECS/${PROJECT_NAME}.spec << SPECEND
Name:           port-monitor
Version:        ${VERSION}
Release:        1%{?dist}
Summary:        网络端口监控工具

License:        MIT
URL:            ${GITHUB_URL}
BuildArch:      x86_64

%description
Port Monitor 是一个轻量级的网络端口监控工具，用于实时监控系统中 TCP/UDP 服务和网络接口状态。

%prep
# 无需准备步骤

%build
# 无需构建步骤

%install
mkdir -p %{buildroot}/opt/%{name}
mkdir -p %{buildroot}/opt/%{name}/frontend/static
mkdir -p %{buildroot}/usr/bin

# 复制预编译的二进制文件
install -m 755 /source/build/port-monitor %{buildroot}/usr/bin/port-monitor

# 复制前端文件
cp -r /source/build/frontend/static/* %{buildroot}/opt/%{name}/frontend/static/

# 复制配置文件
cp /source/build/config.yaml %{buildroot}/opt/%{name}/

# 创建日志和数据文件
touch %{buildroot}/opt/%{name}/server.log
touch %{buildroot}/opt/%{name}/data.json

# 创建systemd服务文件
mkdir -p %{buildroot}/usr/lib/systemd/system
cat > %{buildroot}/usr/lib/systemd/system/port-monitor.service << 'SERVICEEOF'
[Unit]
Description=Port Monitor Service
After=network.target

[Service]
Type=simple
User=root
WorkingDirectory=/opt/port-monitor
ExecStart=/usr/bin/port-monitor
Restart=always
RestartSec=3

[Install]
WantedBy=multi-user.target
SERVICEEOF

%files
/usr/bin/port-monitor
/opt/port-monitor/frontend/static/*
/opt/port-monitor/config.yaml
/opt/port-monitor/server.log
/opt/port-monitor/data.json
/usr/lib/systemd/system/port-monitor.service

%changelog
* Sun Nov 02 2025 Packager <packager@example.com> - ${VERSION}-1
- Initial build
SPECEND

            # 构建RPM包
            echo '开始构建RPM包...'
            rpmbuild --define '_topdir /root/rpmbuild' -bb /root/rpmbuild/SPECS/port-monitor.spec
            echo '复制RPM包到输出目录...'
            find /root/rpmbuild/RPMS -name '*.rpm' -exec cp {} /output/ \;
            echo 'RPM包构建完成'
        "; then
        echo "RPM包已生成到 ${OUTPUT_DIR}"
    else
        echo "警告: RPM构建失败"
    fi
}

# 创建DEB包
build_deb() {
    echo "构建DEB包..."

    DEB_BUILD_DIR="${BUILD_DIR}/deb/${PROJECT_NAME}-${VERSION}"
    mkdir -p "${DEB_BUILD_DIR}/DEBIAN"
    mkdir -p "${DEB_BUILD_DIR}/opt/${PROJECT_NAME}"
    mkdir -p "${DEB_BUILD_DIR}/usr/bin"
    mkdir -p "${DEB_BUILD_DIR}/etc/systemd/system"

    # 复制文件
    cp "${BUILD_DIR}/${PROJECT_NAME}" "${DEB_BUILD_DIR}/usr/bin/"
    cp -r "${BUILD_DIR}/frontend" "${DEB_BUILD_DIR}/opt/${PROJECT_NAME}/"
    cp "${BUILD_DIR}/config.yaml" "${DEB_BUILD_DIR}/opt/${PROJECT_NAME}/"

    # 创建空的日志和数据文件
    touch "${DEB_BUILD_DIR}/opt/${PROJECT_NAME}/server.log"
    touch "${DEB_BUILD_DIR}/opt/${PROJECT_NAME}/data.json"

    # 创建systemd服务文件
    cat > "${DEB_BUILD_DIR}/etc/systemd/system/${PROJECT_NAME}.service" << 'EOF'
[Unit]
Description=Port Monitor Service
After=network.target

[Service]
Type=simple
User=root
WorkingDirectory=/opt/port-monitor
ExecStart=/usr/bin/port-monitor
Restart=always
RestartSec=3

[Install]
WantedBy=multi-user.target
EOF

    # 创建DEBIAN控制文件
    cat > "${DEB_BUILD_DIR}/DEBIAN/control" << EOF
Package: ${PROJECT_NAME}
Version: ${VERSION}
Section: utils
Priority: optional
Architecture: amd64
Depends: systemd
Maintainer: Packager <packager@example.com>
Description: 网络端口监控工具
 Port Monitor 是一个轻量级的网络端口监控工具，用于实时监控系统中 TCP/UDP 服务和网络接口状态。
EOF

    # 创建postinst脚本
    cat > "${DEB_BUILD_DIR}/DEBIAN/postinst" << 'EOF'
#!/bin/bash
chmod 755 /usr/bin/port-monitor
chmod 644 /opt/port-monitor/config.yaml
chmod 666 /opt/port-monitor/server.log
chmod 666 /opt/port-monitor/data.json
systemctl daemon-reload
EOF

    chmod 755 "${DEB_BUILD_DIR}/DEBIAN/postinst"

    # 使用docker运行dpkg-deb
    echo "正在运行Docker容器构建DEB..."
    if docker run --rm \
        -v "${DEB_BUILD_DIR}:/source" \
        -v "${OUTPUT_DIR}:/output" \
        ubuntu:20.04 \
        bash -c "
            # 使用清华源
            sed -i 's/archive.ubuntu.com/mirrors.tuna.tsinghua.edu.cn/g' /etc/apt/sources.list
            sed -i 's/security.ubuntu.com/mirrors.tuna.tsinghua.edu.cn/g' /etc/apt/sources.list
            apt-get update
            apt-get install -y dpkg-dev
            cd /source
            dpkg-deb --build --root-owner-group . /output/${PROJECT_NAME}_${VERSION}_amd64.deb
        "; then
        echo "DEB包已生成到 ${OUTPUT_DIR}"
    else
        echo "警告: DEB构建失败"
    fi
}

# 构建包
build_rpm
build_deb

echo "打包完成！RPM和DEB包已生成到 ${OUTPUT_DIR} 目录"
if ls "${OUTPUT_DIR}"/*.rpm "${OUTPUT_DIR}"/*.deb 1> /dev/null 2>&1; then
    ls -la "${OUTPUT_DIR}"
else
    echo "未找到生成的包文件"
fi