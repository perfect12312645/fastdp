#!/bin/bash
#===============================================================================
# fastdp 主机巡检脚本 - 标准输出格式：key=value
#
# 【规则】
# 1. 内置字段：工具固定解析，中文名称、有序展示、格式化输出
# 2. 自定义字段：只需追加 echo "key=value"，工具自动识别展示
#
#===============================================================================

#===============================================================================
# ===================== 【内置标准字段 - 请勿修改/删除】=====================
# 说明：以下为工具固定解析字段，修改可能导致展示异常
#===============================================================================

# 主机名
echo "hostname=$(hostname)"

# 操作系统版本
echo "os=$(cat /etc/os-release | grep -E '^PRETTY_NAME' | awk -F= '{print $2}' | sed 's/"//g' | head -1)"

# 虚拟化类型：vmware / kvm / physical
echo "virt=$(systemd-detect-virt 2>/dev/null || echo "physical")"

# CPU 核心数
echo "cpu_cores=$(grep -c processor /proc/cpuinfo)"

# CPU 型号
echo "cpu_model=$(grep -m1 'model name' /proc/cpuinfo | cut -d: -f2 | xargs)"

# 系统架构
echo "arch=$(uname -m)"

# 内核版本
echo "kernel=$(uname -r)"

# 内存大小
echo "mem=$(free -h | awk '/^Mem:/{print $2}')"

# 磁盘信息（自动过滤U盘/光盘/loop设备，自动识别 NVMe/SSD/HDD）
disk_info=$(lsblk -d -n -o NAME,SIZE,ROTA | grep -Ev '^loop|^dm-|^sr[0-9]|^zram|^md' | awk '{
    type = "SSD"
    if ($1 ~ /^nvme/) type = "NVMe"
    else if ($3 == 1) type = "HDD"
    printf "%s:%s:%s,", $1, $2, type
}' | sed 's/,$//')
echo "disk=$disk_info"

# GPU 信息（NVIDIA）
if ! command -v lspci &>/dev/null; then
    gpu_count="0"
    gpu_model="lspci_not_installed"
else
    gpu_count=$(lspci | grep -i nvidia | wc -l | xargs)
    gpu_model=$(lspci | grep -i nvidia | head -1 | cut -d: -f3- | xargs)
    [ "$gpu_count" = "0" ] && gpu_model="none"
fi
echo "gpu=${gpu_count}|${gpu_model}"

# 防火墙状态：active/enabled
if systemctl list-unit-files --type=service | grep -q firewalld; then
    fw_status=$(systemctl is-active firewalld)
    fw_enable=$(systemctl is-enabled firewalld)
else
    fw_status="inactive"
    fw_enable="disabled"
fi
echo "firewall=${fw_status}/${fw_enable}"

# SELinux 状态
if command -v getenforce &>/dev/null; then
    selinux_current=$(getenforce)
else
    selinux_current="Disabled"
fi
selinux_config="Disabled"
[ -f /etc/selinux/config ] && selinux_config=$(grep -E '^SELINUX=' /etc/selinux/config | awk -F= '{print $2}')
echo "selinux=${selinux_current}/${selinux_config}"

# 交换分区
echo "swap=$(free -h | awk '/^Swap:/{print $2}')"

# 时区
echo "timezone=$(timedatectl show -p Timezone --value 2>/dev/null || echo "unknown")"

# 系统时间
echo "sys_time=$(date +"%Y-%m-%d %H:%M:%S")"

# 硬件时间
echo "hw_time=$(hwclock -r 2>/dev/null | head -1 || echo "unknown")"

# 网卡信息（自动过滤 docker/k8s 虚拟网卡）
net=$(ip -4 addr show up | grep -w inet | grep -v 127.0.0.1 | \
grep -Ev 'docker|flannel|calico|vxlan|cni|kube-ipvs|veth|bridge' | \
awk '{print $NF":"$2}' | sort | uniq | tr '\n' ',' | sed 's/,$//')
echo "net=$net"

# 默认网关
gateway=$(ip route show default 2>/dev/null | awk '/default/ {print $3}' | head -1)
[ -z "$gateway" ] && gateway="none"
echo "gateway=$gateway"

#===============================================================================
# ===================== 【用户自定义字段 - 可自由添加】=====================
# 格式：echo "key=value"
# 示例如下：
#===============================================================================

# 示例 1：当前使用的 Shell（环境变量）
# echo "shell=${SHELL}"

# 示例 2：当前登录用户
# echo "login_user=$(whoami)"

# 示例 3：系统语言/字符集
# echo "lang=${LANG}"


# 👇 下面开始写你的自定义字段
#===============================================================================
