#!/bin/bash
#===============================================================================
# fastdp 主机巡检脚本 - 标准输出格式：key=value
#
# 【规则】
# 1. 内置字段：工具固定解析，中文名称、有序展示、格式化输出
# 2. 自定义字段：只需追加 echo "key=value"，工具自动识别展示
# 3. 多行检查项：用 # BEGIN key 和 # END key 包裹，支持 --only 选择性执行
#    单行检查项：直接写 echo "key=value"，工具自动从 echo 语句解析 key
#
# 【--only 用法】
#   fastdp check all --only hostname,os    # 只执行 hostname 和 os 的检查
#   fastdp check all -l                     # 列出所有可用的字段 key
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

# CPU 型号（型号中可能含冒号，如 "Hygon C86-4G (OPN: ...)"，取冒号后全部内容）
echo "cpu_model=$(grep -m1 'model name' /proc/cpuinfo | cut -d: -f2- | xargs)"

# 系统架构
echo "arch=$(uname -m)"

# 内核版本
echo "kernel=$(uname -r)"

# 内存大小
echo "mem=$(free -h | awk '/^Mem:/{print $2}')"

# BEGIN disk
# 磁盘信息（自动过滤U盘/光盘/loop设备，自动识别 NVMe/SSD/HDD）
disk_info=$(lsblk -d -n -o NAME,SIZE,ROTA | grep -Ev '^loop|^dm-|^sr[0-9]|^zram|^md' | awk '{
    type = "SSD"
    if ($1 ~ /^nvme/) type = "NVMe"
    else if ($3 == 1) type = "HDD"
    printf "%s:%s:%s,", $1, $2, type
}' | sed 's/,$//')
echo "disk=$disk_info"
# END disk

# BEGIN firewall
# 防火墙状态：active/enabled
if systemctl list-unit-files --type=service | grep -q firewalld; then
    fw_status=$(systemctl is-active firewalld)
    fw_enable=$(systemctl is-enabled firewalld)
else
    fw_status="inactive"
    fw_enable="disabled"
fi
echo "firewall=${fw_status}/${fw_enable}"
# END firewall

# BEGIN selinux
# SELinux 状态
if command -v getenforce &>/dev/null; then
    selinux_current=$(getenforce)
else
    selinux_current="Disabled"
fi
selinux_config="Disabled"
[ -f /etc/selinux/config ] && selinux_config=$(grep -E '^SELINUX=' /etc/selinux/config | awk -F= '{print $2}')
echo "selinux=${selinux_current}/${selinux_config}"
# END selinux

# 交换分区
echo "swap=$(free -h | awk '/^Swap:/{print $2}')"

# 时区
echo "timezone=$(timedatectl show -p Timezone --value 2>/dev/null || echo "unknown")"

# 系统时间
echo "sys_time=$(date +"%Y-%m-%d %H:%M:%S")"

# 硬件时间（需要 root 权限）
hw_out=$(hwclock -r 2>/dev/null)
echo "hw_time=${hw_out:-unknown}"

# BEGIN gpu
# NVIDIA GPU 设备 ID → 型号映射（常见卡型，未匹配的返回空）
nvidia_name() {
    case "$1" in
        # === Hopper ===
        2330|2331|2336|2337|2338|2339|233d) echo "H100" ;;
        2335|233b) echo "H200" ;;
        2322|2324) echo "H800" ;;
        230c|230e|2329|232c) echo "H20" ;;
        2342|2348) echo "GH200" ;;
        # === Ampere ===
        20b0|20b2|20b3|20b5|20f1) echo "A100" ;;
        20bd|20f3|20f5|20f6) echo "A800" ;;
        20b7|20f9) echo "A30" ;;
        2235) echo "A40" ;;
        2236) echo "A10" ;;
        25b6) echo "A16" ;;
        1db1|1db4|1db5|1db6|1df0|1df2|1df5|1df6) echo "V100" ;;
        1eb4|1eb8) echo "T4" ;;
        1b39) echo "P40" ;;
        # === Ada (数据中心) ===
        26b5) echo "L40" ;;
        26b8) echo "L40G" ;;
        26b9) echo "L40S" ;;
        26b7|26ba) echo "L20" ;;
        27b6) echo "L2" ;;
        27b8) echo "L4" ;;
        # === Blackwell / Ada (消费级) ===
        2b85|2b87) echo "RTX 5090" ;;
        2684|2685) echo "RTX 4090" ;;
        2702|2704|2705) echo "RTX 4080" ;;
        2782|2783|2786) echo "RTX 4070" ;;
        2803|2805|2882|28e0) echo "RTX 4060" ;;
        *) echo "" ;;
    esac
}

# GPU 信息（多厂商识别，lspci -nn 匹配；NVIDIA 常见卡型自动识别，昇腾识别 910B3，昆仑芯识别 P800，其余标记 unknown）
if ! command -v lspci &>/dev/null; then
    echo "gpu=0|lspci_not_installed"
else
    gpu=""
    # NVIDIA（10de）- 按显卡类别过滤，避免误计音频设备
    n=$(lspci -nn -d 10de: 2>/dev/null | grep -icE 'vga|3d controller|display controller' || true)
    if [ "$n" -gt 0 ]; then
        # 取首张卡的 device ID 查表获取型号
        dev_id=$(lspci -nn -d 10de: 2>/dev/null | grep -iE 'vga|3d controller|display controller' | head -1 | sed -n 's/.*\[10de:\([0-9a-f]*\)\].*/\1/p')
        m=$(nvidia_name "$dev_id")
        if [ -z "$m" ]; then
            # 未知卡型：取 lspci 原始描述；若数据库无记录则显示 "NVIDIA [10de:XXXX]"
            raw=$(lspci -nn -d 10de: 2>/dev/null | grep -iE 'vga|3d controller|display controller' | head -1 | cut -d: -f3- | xargs)
            case "$raw" in
                *"Device [10de:"*) m="${dev_id}" ;;
                *) m="$raw" ;;
            esac
        fi
        gpu="${gpu}NVIDIA:${n}:${m};"
    fi
    # AMD（1002）
    n=$(lspci -nn -d 1002: 2>/dev/null | grep -icE 'vga|3d controller|display controller' || true)
    if [ "$n" -gt 0 ]; then
        dev_id=$(lspci -nn -d 1002: 2>/dev/null | grep -iE 'vga|3d controller|display controller' | head -1 | sed -n 's/.*\[1002:\([0-9a-f]*\)\].*/\1/p')
        gpu="${gpu}AMD:${n}:${dev_id};"
    fi
    # Intel（8086）
    n=$(lspci -nn -d 8086: 2>/dev/null | grep -icE 'vga|3d controller|display controller' || true)
    if [ "$n" -gt 0 ]; then
        dev_id=$(lspci -nn -d 8086: 2>/dev/null | grep -iE 'vga|3d controller|display controller' | head -1 | sed -n 's/.*\[8086:\([0-9a-f]*\)\].*/\1/p')
        gpu="${gpu}Intel:${n}:${dev_id};"
    fi
    # 华为昇腾（19e5）- 仅统计加速器类别，避免误算华为网卡
    n=$(lspci -nn -d 19e5: 2>/dev/null | grep -ic 'processing accelerators' || true)
    if [ "$n" -gt 0 ]; then
        d=$(lspci -nn -d 19e5: 2>/dev/null | grep -i 'processing accelerators' | grep -c '\[19e5:d802\]' || true)
        [ "$d" -gt 0 ] && gpu="${gpu}昇腾:${d}:910B3;"
        o=$((n - d))
        if [ "$o" -gt 0 ]; then
            dev_id=$(lspci -nn -d 19e5: 2>/dev/null | grep -i 'processing accelerators' | grep -v '\[19e5:d802\]' | head -1 | sed -n 's/.*\[19e5:\([0-9a-f]*\)\].*/\1/p')
            gpu="${gpu}昇腾:${o}:${dev_id};"
        fi
    fi
    # 昆仑芯（1d22）
    n=$(lspci -nn -d 1d22: 2>/dev/null | wc -l | xargs)
    if [ "$n" -gt 0 ]; then
        d=$(lspci -nn -d 1d22: 2>/dev/null | grep -c '\[1d22:3688\]' || true)
        [ "$d" -gt 0 ] && gpu="${gpu}昆仑芯:${d}:P800;"
        o=$((n - d))
        if [ "$o" -gt 0 ]; then
            dev_id=$(lspci -nn -d 1d22: 2>/dev/null | grep -v '\[1d22:3688\]' | head -1 | sed -n 's/.*\[1d22:\([0-9a-f]*\)\].*/\1/p')
            gpu="${gpu}昆仑芯:${o}:${dev_id};"
        fi
    fi
    # 寒武纪（1b22）
    n=$(lspci -nn -d 1b22: 2>/dev/null | wc -l | xargs)
    if [ "$n" -gt 0 ]; then
        dev_id=$(lspci -nn -d 1b22: 2>/dev/null | head -1 | sed -n 's/.*\[1b22:\([0-9a-f]*\)\].*/\1/p')
        gpu="${gpu}寒武纪:${n}:${dev_id};"
    fi
    # 壁仞科技（1e9f）
    n=$(lspci -nn -d 1e9f: 2>/dev/null | wc -l | xargs)
    if [ "$n" -gt 0 ]; then
        dev_id=$(lspci -nn -d 1e9f: 2>/dev/null | head -1 | sed -n 's/.*\[1e9f:\([0-9a-f]*\)\].*/\1/p')
        gpu="${gpu}壁仞:${n}:${dev_id};"
    fi
    # 沐曦 MetaX（1eae）
    n=$(lspci -nn -d 1eae: 2>/dev/null | wc -l | xargs)
    if [ "$n" -gt 0 ]; then
        dev_id=$(lspci -nn -d 1eae: 2>/dev/null | head -1 | sed -n 's/.*\[1eae:\([0-9a-f]*\)\].*/\1/p')
        gpu="${gpu}沐曦:${n}:${dev_id};"
    fi
    # 摩尔线程（1e2b）
    n=$(lspci -nn -d 1e2b: 2>/dev/null | wc -l | xargs)
    if [ "$n" -gt 0 ]; then
        dev_id=$(lspci -nn -d 1e2b: 2>/dev/null | head -1 | sed -n 's/.*\[1e2b:\([0-9a-f]*\)\].*/\1/p')
        gpu="${gpu}摩尔线程:${n}:${dev_id};"
    fi
    # 燧原 Enflame（1e81）
    n=$(lspci -nn -d 1e81: 2>/dev/null | wc -l | xargs)
    if [ "$n" -gt 0 ]; then
        dev_id=$(lspci -nn -d 1e81: 2>/dev/null | head -1 | sed -n 's/.*\[1e81:\([0-9a-f]*\)\].*/\1/p')
        gpu="${gpu}燧原:${n}:${dev_id};"
    fi
    # 海光 DCU（1efd）
    n=$(lspci -nn -d 1efd: 2>/dev/null | wc -l | xargs)
    if [ "$n" -gt 0 ]; then
        dev_id=$(lspci -nn -d 1efd: 2>/dev/null | head -1 | sed -n 's/.*\[1efd:\([0-9a-f]*\)\].*/\1/p')
        gpu="${gpu}海光:${n}:${dev_id};"
    fi
    [ -z "$gpu" ] && gpu="none"
    echo "gpu=${gpu%;}"
fi
# END gpu

# BEGIN net
# 网卡信息（自动过滤 docker/k8s 虚拟网卡，含速率）
net=$(ip -4 addr show up | grep -w inet | grep -v 127.0.0.1 | \
grep -Ev 'docker|flannel|calico|vxlan|cni|kube-ipvs|veth|bridge' | \
awk '{print $NF" "$2}' | sort -u | while read -r iface addr; do
    spd=$(cat /sys/class/net/"$iface"/speed 2>/dev/null)
    if [ -n "$spd" ] && [ "$spd" -gt 0 ] 2>/dev/null; then
        if [ "$spd" -ge 1000 ]; then spd="$((spd/1000))Gbps"; else spd="${spd}Mbps"; fi
    else
        spd="Unknown"
    fi
    echo "${iface}:${addr}:${spd}"
done | tr '\n' ',' | sed 's/,$//')
echo "net=$net"
# END net

# BEGIN gateway
# 默认网关
gateway=$(ip route show default 2>/dev/null | awk '/default/ {print $3}' | head -1)
[ -z "$gateway" ] && gateway="none"
echo "gateway=$gateway"
# END gateway

#===============================================================================
# ===================== 【用户自定义字段 - 可自由添加】=====================
# 格式：
#   单行：echo "key=value"
#   多行：用 # BEGIN key 和 # END key 包裹，支持 --only 选择性执行
#===============================================================================

# 示例 1：当前登录用户（单行）
# echo "login_user=$(whoami)"

# 示例 2：系统运行时间（单行）
# echo "uptime=$(uptime -p 2>/dev/null || uptime)"

# 示例 3：多行自定义字段（用 BEGIN/END 包裹）
# BEGIN my_custom
# my_val=$(date +%s)
# echo "my_custom=${my_val}"
# END my_custom


# 👇 下面开始写你的自定义字段
#===============================================================================
