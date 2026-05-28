#!/bin/bash
set -e

# ====================== 【可配置变量】 ======================
VERSION="v6"                  # 版本号，随便改
REPO_URL="https://gitee.com/zhao-pengfei2/fastdp.git"
BUILD_DIR="fastdp-build"
OUTPUT_DIR="releases"
# =============================================================

# 1. 检查依赖：git + go
echo -e "\n==== 检查依赖 ===="
if ! command -v git &> /dev/null; then
    echo "错误：未找到 git 命令，请先安装 git"
    echo "安装命令（CentOS/RHEL/Rocky/麒麟）："
    echo "yum -y install git"
    exit 1
fi

if ! command -v go &> /dev/null; then
    echo "错误：未找到 go 命令，请先安装 Go 环境"
    echo -e "\n==================== Go 一键安装命令 ===================="
    echo "wget https://golang.google.cn/dl/go1.26.3.linux-amd64.tar.gz"
    echo "tar -xf go1.26.3.linux-amd64.tar.gz -C /usr/local/"
    echo ""
    echo "配置环境变量："
    echo "echo 'export PATH=\$PATH:/usr/local/go/bin' >> /etc/profile"
    echo "echo 'export GOROOT=/usr/local/go' >> /etc/profile"
    echo "echo 'export GO111MODULE=auto' >> /etc/profile"
    echo ""
    echo "生效配置："
    echo "source /etc/profile"
    echo ""
    echo "验证安装："
    echo "go version"
    echo -e "==========================================================\n"
    exit 1
fi
# 检查是否安装 rpmbuild
if ! command -v rpmbuild &> /dev/null; then
    echo "⚠️ 未安装 rpmbuild，跳过 RPM 构建"
    echo "yum install rpm-build -y"
    exit 1
fi
echo "✅ git 版本：$(git --version | head -n1)"
echo "✅ go 版本：$(go version | head -n1)"

# 2. 清理并创建目录
echo -e "\n==== 准备目录 ===="
rm -rf $BUILD_DIR $OUTPUT_DIR
mkdir -p $BUILD_DIR $OUTPUT_DIR

# 3. 拉取代码
echo -e "\n==== 拉取代码 ===="
git clone $REPO_URL $BUILD_DIR
cd $BUILD_DIR

# 4. 定义构建目标（Linux + macOS，amd64 + arm64）
PLATFORMS=(
    "linux amd64"
    "linux arm64"
    "darwin amd64"
    "darwin arm64"
)
go env -w GOPROXY=https://goproxy.cn

# 5. 开始交叉编译
echo -e "\n==== 开始编译 ===="
for platform in "${PLATFORMS[@]}"; do
    GOOS=$(echo $platform | awk '{print $1}')
    GOARCH=$(echo $platform | awk '{print $2}')
    BIN_NAME="fastdp"

    echo -e "\n→ 编译 $GOOS/$GOARCH"
    CGO_ENABLED=0 GOOS=$GOOS GOARCH=$GOARCH go build -o $BIN_NAME -ldflags "-w -s" cmd/main.go

    # 创建打包目录
    PKG_NAME="fastdp-$VERSION-$GOOS-$GOARCH"
    PKG_DIR=$PKG_NAME
    mkdir -p $PKG_DIR

    # 复制文件
    cp -a fastdp $PKG_DIR/
    cp -a config.toml host fastdp-check.sh README.txt $PKG_DIR/ 2>/dev/null || true

    # 加执行权限
    chmod 755 $PKG_DIR/fastdp

    # 打包 tar.gz
    tar -zcvf "../$OUTPUT_DIR/$PKG_NAME.tar.gz" $PKG_DIR

    # 清理
    rm -rf $PKG_DIR
done

# ====================== RPM 构建 ======================
echo -e "\n==== 构建 RPM 包 ===="
# 询问是否构建 RPM
read -p "是否构建 RPM 包？(y/n): " build_rpm
if [[ "$build_rpm" != "y" && "$build_rpm" != "Y" ]]; then
    echo "⚠️ 跳过 RPM 构建"
    goto rpm_end
fi

# 检查 rpmbuild
if ! command -v rpmbuild &> /dev/null; then
    echo "⚠️ 未安装 rpmbuild，正在自动安装..."
    yum install rpm-build -y
fi

# 自动获取当前系统架构
CURRENT_ARCH=$(arch)
echo "✅ 当前机器架构：$CURRENT_ARCH"
RPM_ARCHS=("$CURRENT_ARCH")
VERSION_NO_V=${VERSION//v/}

for ARCH in "${RPM_ARCHS[@]}"; do
    echo -e "\n→ 构建 RPM：$ARCH"

    GOOS=linux
    GOARCH=$ARCH

    if [ "$ARCH" = "x86_64" ]; then
        GOARCH="amd64"
    elif [ "$ARCH" = "aarch64" ]; then
        GOARCH="arm64"
    fi

    echo "→ GO 编译架构：$GOOS/$GOARCH"
    CGO_ENABLED=0 GOOS=$GOOS GOARCH=$GOARCH go build -o fastdp -ldflags "-w -s" cmd/main.go

    RPM_BUILD_DIR=rpmbuild
    mkdir -p $RPM_BUILD_DIR/{BUILD,RPMS,SOURCES,SPECS,SRPMS}

    mkdir -p $RPM_BUILD_DIR/SOURCES/fastdp-$VERSION_NO_V/usr/local/bin
    mkdir -p $RPM_BUILD_DIR/SOURCES/fastdp-$VERSION_NO_V/etc/fastdp

    cp -a fastdp $RPM_BUILD_DIR/SOURCES/fastdp-$VERSION_NO_V/usr/local/bin/
    cp -a config.toml host fastdp-check.sh README.txt $RPM_BUILD_DIR/SOURCES/fastdp-$VERSION_NO_V/etc/fastdp/ 2>/dev/null || true

    cat > $RPM_BUILD_DIR/SPECS/fastdp.spec << EOF
Name:           fastdp
Version:        $VERSION_NO_V
Release:        1
Summary:        Batch SSH operation tool
License:        MIT
URL:            $REPO_URL
BuildArch:      $ARCH

%description
fastdp - 批量SSH运维工具（无依赖）

%install
mkdir -p %{buildroot}/usr/local/bin
mkdir -p %{buildroot}/etc/fastdp
cp -a %{_sourcedir}/fastdp-%{version}/usr/local/bin/fastdp %{buildroot}/usr/local/bin/
cp -a %{_sourcedir}/fastdp-%{version}/etc/fastdp/* %{buildroot}/etc/fastdp/

%files
/usr/local/bin/fastdp
/etc/fastdp/*

%changelog
EOF

    rpmbuild --define "_topdir $(pwd)/$RPM_BUILD_DIR" -bb $RPM_BUILD_DIR/SPECS/fastdp.spec
    find $RPM_BUILD_DIR/RPMS -name "*.rpm" -exec cp -a {} ../$OUTPUT_DIR/ \;
    rm -rf $RPM_BUILD_DIR
done

# 结束
cd ..
echo -e "\n==== 构建完成 ===="
echo -e "产物目录：\033[32m$OUTPUT_DIR/\033[0m"
ls -lh $OUTPUT_DIR/