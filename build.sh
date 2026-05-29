#!/bin/bash
set -e

# ====================== 【可配置变量】 ======================
VERSION="v6"
REPO_URL="https://gitee.com/zhao-pengfei2/fastdp.git"
BUILD_DIR="fastdp-build"
OUTPUT_DIR="releases"
# =============================================================

# 颜色输出
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

# 版本号去v
VERSION_NO_V=${VERSION//v/}

# 依赖检查
check_deps() {
    echo -e "\n==== 检查依赖 ===="
    if ! command -v git &> /dev/null; then
        echo -e "${RED}错误：未找到 git${NC}"
        echo "yum -y install git 或 apt install git -y"
        exit 1
    fi

    if ! command -v go &> /dev/null; then
        echo -e "${RED}错误：未找到 go${NC}"
        cat <<EOF
wget https://golang.google.cn/dl/go1.26.3.linux-amd64.tar.gz
tar -xf go1.26.3.linux-amd64.tar.gz -C /usr/local/
echo 'export PATH=\$PATH:/usr/local/go/bin' >> /etc/profile
source /etc/profile
go version
EOF
        exit 1
    fi

    echo -e "${GREEN}✅ git: $(git --version | head -n1)${NC}"
    echo -e "${GREEN}✅ go: $(go version | head -n1)${NC}"
}

# 准备目录
prepare() {
    echo -e "\n==== 准备目录 ===="
    rm -rf $BUILD_DIR $OUTPUT_DIR
    mkdir -p $BUILD_DIR $OUTPUT_DIR
    git clone $REPO_URL $BUILD_DIR
    cd $BUILD_DIR
    go env -w GOPROXY=https://goproxy.cn
}

# 构建 tar.gz
build_tar() {
    prepare
    echo -e "\n==== 构建 tar.gz ===="
    PLATFORMS=(
        "linux amd64"
        "linux arm64"
        "darwin amd64"
        "darwin arm64"
    )

    for p in "${PLATFORMS[@]}"; do
        GOOS=$(echo $p | awk '{print $1}')
        GOARCH=$(echo $p | awk '{print $2}')
        echo -e "\n→ 编译 $GOOS/$GOARCH"
        CGO_ENABLED=0 GOOS=$GOOS GOARCH=$GOARCH go build -o fastdp -ldflags "-w -s" cmd/main.go

        PKG_DIR="fastdp-$VERSION-$GOOS-$GOARCH"
        mkdir -p $PKG_DIR
        cp -a fastdp config.toml host fastdp-check.sh README.txt $PKG_DIR/ 2>/dev/null || true
        chmod 755 $PKG_DIR/fastdp
        tar -zcvf "../$OUTPUT_DIR/$PKG_DIR.tar.gz" $PKG_DIR
        rm -rf $PKG_DIR
    done
}

# 构建 rpm
build_rpm() {
    prepare
    echo -e "\n==== 构建 RPM ===="
    if ! command -v rpmbuild &> /dev/null; then
        echo "安装 rpmbuild..."
        yum install rpm-build -y
    fi

    ARCH=$(arch)
    GOARCH=$ARCH
    [ "$ARCH" = "x86_64" ] && GOARCH="amd64"
    [ "$ARCH" = "aarch64" ] && GOARCH="arm64"

    CGO_ENABLED=0 GOOS=linux GOARCH=$GOARCH go build -o fastdp -ldflags "-w -s" cmd/main.go

    RPM_DIR=rpmbuild
    mkdir -p $RPM_DIR/{BUILD,RPMS,SOURCES,SPECS}
    mkdir -p $RPM_DIR/SOURCES/fastdp-$VERSION_NO_V/{/usr/local/bin,/etc/fastdp}

    cp fastdp $RPM_DIR/SOURCES/fastdp-$VERSION_NO_V/usr/local/bin/
    cp config.toml host fastdp-check.sh README.txt $RPM_DIR/SOURCES/fastdp-$VERSION_NO_V/etc/fastdp/ 2>/dev/null || true

    cat > $RPM_DIR/SPECS/fastdp.spec <<EOF
Name: fastdp
Version: $VERSION_NO_V
Release: 1
Summary: Batch SSH tool
License: MIT
BuildArch: $ARCH

%install
cp -r %{_sourcedir}/fastdp-%{version}/* %{buildroot}/

%files
/usr/local/bin/fastdp
/etc/fastdp/*
EOF

    rpmbuild --define "_topdir $(pwd)/$RPM_DIR" -bb $RPM_DIR/SPECS/fastdp.spec
    find $RPM_DIR/RPMS -name "*.rpm" -exec cp {} ../$OUTPUT_DIR/ \;
    rm -rf $RPM_DIR
}

# 构建 deb
# 构建 deb
build_deb() {
    prepare
    echo -e "\n==== 构建 DEB ===="
    if ! command -v dpkg-deb &> /dev/null; then
        echo "安装 dpkg-deb..."
        apt install dpkg-dev -y
    fi

    # 核心修复：Debian 架构名称映射
    ARCH=$(arch)
    if [ "$ARCH" = "x86_64" ]; then
        DEB_ARCH="amd64"
    elif [ "$ARCH" = "aarch64" ]; then
        DEB_ARCH="arm64"
    else
        DEB_ARCH="$ARCH"
    fi

    DEB_DIR="fastdp-$VERSION_NO_V"
    mkdir -p $DEB_DIR/{DEBIAN,usr/local/bin,etc/fastdp}

    # 编译对应架构
    GOARCH=$DEB_ARCH
    CGO_ENABLED=0 GOOS=linux GOARCH=$GOARCH go build -o $DEB_DIR/usr/local/bin/fastdp -ldflags "-w -s" cmd/main.go
    chmod 755 $DEB_DIR/usr/local/bin/fastdp

    cp config.toml host fastdp-check.sh README.txt $DEB_DIR/etc/fastdp/ 2>/dev/null || true

    # 关键：DEB 架构必须是 amd64 / arm64
    cat > $DEB_DIR/DEBIAN/control <<EOF
Package: fastdp
Version: $VERSION_NO_V
Section: utils
Priority: optional
Architecture: $DEB_ARCH
Maintainer: you
Homepage: $REPO_URL
Description: Batch SSH operation tool
EOF

    dpkg-deb --build $DEB_DIR ../$OUTPUT_DIR/fastdp-$VERSION-linux-$DEB_ARCH.deb
    rm -rf $DEB_DIR
}

# ====================== 主菜单 ======================
clear
echo -e "${YELLOW}==== fastdp 构建工具 ====${NC}"
echo "1) 仅构建 tar.gz"
echo "2) 仅构建 rpm"
echo "3) 仅构建 deb"
read -p "请选择 [1-3]: " choice

check_deps


case $choice in
    1) build_tar ;;
    2) build_rpm ;;
    3) build_deb ;;
    *) echo "无效选择"; exit 1 ;;
esac

cd ..
echo -e "\n${GREEN}==== 构建完成！====${NC}"
ls -lh $OUTPUT_DIR/