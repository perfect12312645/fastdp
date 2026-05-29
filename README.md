# fastdp
轻量级 Ansible 风格运维工具，支持在指定主机组上执行批量运维操作，提供多模块管理（shell 命令执行、文件复制、文件拉取、远程脚本、主机连通性检测、环境巡检等）。

## 功能特点
- 支持 6 大模块操作（shell / copy / fetch / script / ping / check）
- 基于主机组的批量管理，支持组名和 IP 混合指定
- 并发连接控制，基于 Go 协程实现高效调度
- 支持 SSH 密码认证和密钥认证（自动查找私钥）
- 灵活的配置文件多级加载

## 性能对比

在相同测试场景下（对 4 台远程主机执行 `date` 命令）：

| 工具     | 实际耗时（real） | 性能对比       |
|----------|------------------|----------------|
| fastdp   | 0m0.141s         | ✔️ 快 5.6 倍   |
| Ansible  | 0m0.787s         |                |

![image-20260529175910671](./assets/ansible.png)

## 为什么快？

### 极致并发调度：
- 基于 Go 原生协程（Goroutine）实现细粒度并发控制
- 精准管理 SSH 连接生命周期
- 避免传统多进程模型的资源浪费（如 Ansible 的 fork 开销）

### 无冗余设计：
- 摒弃复杂的兼容性逻辑和冗余配置解析
- 专注核心运维场景，让每一次执行都「轻装上阵」
- 上手成本低，二进制文件大小只有 8MB 左右，无任何依赖，无需处理多版本 Python 适配

## 对运维的意义

对于批量命令执行、主机状态巡检等高频场景，fastdp 可将分钟级操作压缩至秒级，真正实现「瞬时响应」的批量运维体验 —— 这意味着：

- 巡检效率提升 10 倍以上，大规模集群操作不再漫长等待
- 故障排查更及时，秒级反馈加速问题定位

> **注**：测试环境为 4 台同网段 Linux 主机，网络延迟 <1ms；实际性能因网络环境、并发数配置略有差异，但核心优势稳定。

## 安装

### 下载发行包

#### tar.gz 包（Linux / macOS 通用）

| 平台 | 架构 | 下载地址 |
|------|------|---------|
| Linux | amd64 | [fastdp-v6-linux-amd64.tar.gz](https://gitee.com/zhao-pengfei2/fastdp/releases/download/v6/fastdp-v6-linux-amd64.tar.gz) |
| Linux | arm64 | [fastdp-v6-linux-arm64.tar.gz](https://gitee.com/zhao-pengfei2/fastdp/releases/download/v6/fastdp-v6-linux-arm64.tar.gz) |
| macOS | amd64 | [fastdp-v6-darwin-amd64.tar.gz](https://gitee.com/zhao-pengfei2/fastdp/releases/download/v6/fastdp-v6-darwin-amd64.tar.gz) |
| macOS | arm64 | [fastdp-v6-darwin-arm64.tar.gz](https://gitee.com/zhao-pengfei2/fastdp/releases/download/v6/fastdp-v6-darwin-arm64.tar.gz) |

tar.gz 包内容：

```
fastdp-v6-linux-amd64/
├── fastdp              # 主程序（可执行）
├── config.toml         # 配置文件模板
├── host                # 主机组配置模板
├── fastdp-check.sh     # 巡检脚本（check 模块使用）
└── README.txt          # 说明文件
```

安装步骤：

```bash
# 解压
tar -zxvf fastdp-v6-linux-amd64.tar.gz

# 进入目录
cd fastdp-v6-linux-amd64

# 复制二进制到系统路径
cp fastdp /usr/local/bin/

# 创建配置目录并复制模板
mkdir -p /etc/fastdp
cp config.toml host fastdp-check.sh /etc/fastdp/

# 编辑主机组配置
vim /etc/fastdp/host
```

#### RPM 包（CentOS / Rocky / openEuler 等）

> **注意**：包名中的 `ky10` 是构建环境所致，实际无任何系统依赖，可在 CentOS / Rocky / openEuler 等主流发行版上正常安装使用。

```bash
# 下载
wget https://gitee.com/zhao-pengfei2/fastdp/releases/download/v6/fastdp-6-1.ky10.x86_64.rpm

# 安装（需要 root 权限）
sudo rpm -ivh fastdp-6-1.ky10.x86_64.rpm

# 安装完成后即可使用
fastdp --help
```

#### DEB 包（Ubuntu / Debian 等）

```bash
# 下载
wget https://gitee.com/zhao-pengfei2/fastdp/releases/download/v6/fastdp-v6-linux-amd64.deb

# 安装（需要 root 权限）
sudo dpkg -i fastdp-v6-linux-amd64.deb

# 安装完成后即可使用
fastdp --help
```

RPM/DEB 安装后，所有普通用户都能使用 fastdp。配置文件位于 /etc/fastdp/ 目录下。

### 普通用户自定义配置

RPM/DEB 安装后，普通用户无法修改 /etc/fastdp/ 下的文件。如需自定义配置，可以复制到家目录：

```bash
# 创建用户配置目录
mkdir -p ~/.fastdp

# 复制模板
cp /etc/fastdp/config.toml ~/.fastdp/
cp /etc/fastdp/host ~/.fastdp/
cp /etc/fastdp/fastdp-check.sh ~/.fastdp/

# 编辑用户自有配置
vim ~/.fastdp/config.toml
# 将host_inventory的值改成~/.fastdp/host
vim ~/.fastdp/host
```

### 从源码编译

```bash
# 克隆仓库
git clone https://gitee.com/zhao-pengfei2/fastdp.git
cd fastdp

# 执行构建脚本（运行后按提示选择构建类型：1 tar.gz / 2 rpm / 3 deb）
chmod +x build.sh
./build.sh

# 或直接编译
go build -o fastdp ./cmd/main.go
cp fastdp /usr/local/bin/
```

## 配置文件加载优先级

```
1. ~/.fastdp/config.toml      （用户自定义，优先级最高）
2. /etc/fastdp/config.toml    （系统默认）
3. ./config.toml              （当前目录，兜底）
```

## 巡检脚本路径优先级（check 模块）

```
1. ~/.fastdp/fastdp-check.sh  （用户自定义，优先级最高）
2. /etc/fastdp/fastdp-check.sh（系统默认）
```

## 命令补全

### Zsh（macOS / Linux）

```bash
# 查看 $FPATH 是否包含补全目录
echo $FPATH

# macOS 安装 zsh-completions
brew install zsh-completions

# 安装 fastdp 补全（如目录不存在需先 mkdir -p）
sudo mkdir -p /usr/local/share/zsh/site-functions
sudo fastdp completion zsh > /usr/local/share/zsh/site-functions/_fastdp

# ~/.zshrc 文件中确保有下面这两行
autoload -Uz compinit
compinit

# 生效
source ~/.zshrc

# 验证
fastdp [tab][tab]
```

### Bash（Linux）

```bash
# linux 安装 bash-completions（rpm系示例）
yum -y install bash-completion

# 生成并安装补全脚本
fastdp completion bash > /usr/local/etc/bash_completion.d/fastdp
chmod +r /usr/local/etc/bash_completion.d/fastdp

# 重开终端或重新加载
source ~/.bash_profile

# 验证
fastdp [tab][tab]
```

## 快速开始

```bash
# 查看帮助
fastdp --help

# shell：在 web 组执行 uptime
fastdp shell -a "uptime" web

# shell：多主机组/IP 混合执行
fastdp shell -a "lsblk" master node 192.168.10.100

# copy：复制本地文件到 db 组（自动 MD5 校验 + 权限同步）
fastdp copy -s ./config.ini -d /etc/app/config.ini db

# fetch：批量拉取远程日志文件
fastdp fetch -r "/var/log/messages" all

# script：推送脚本到远程主机并执行
fastdp script -f init.sh db

# ping：检测 all 组主机 SSH 存活状态
fastdp ping all

# check：批量主机环境巡检
fastdp check all

# 开启调试模式
fastdp -v shell -a "df -h" web

# 控制执行的并发数量，默认为配置文件中的值
fastdp -c 10 shell -a "lsblk" web

# 指定主机清单文件 (优先于配置文件)
fastdp -i ./host shell -a "df -h" gpu
```

## 模块说明

### 1. shell 模块

在远程主机执行 shell 命令。

参数：
- `-a` / `--args`：要执行的 shell 命令（必需）

```bash
# 基础用法
fastdp shell -a "df -h" web

# 混合指定：组 + IP 同时执行
fastdp shell -a "free -h" master node 192.168.10.100

# 引号自由使用
fastdp shell -a 'ls -l /root' all
fastdp shell -a "echo hello world" all

# 高阶用法：批量输出 IP + 主机名
fastdp shell -a 'echo $(hostname -I|awk '\''{print $1}'\'') $(hostname)' all | grep -v 'output:'
```

![image-20260529175910671](./assets/shell.png)

### 2. copy 模块

复制本地文件到远程主机，支持 MD5 校验（文件相同则跳过）和权限同步。

参数：
- `-s` / `--source`：本地源文件路径（必需）
- `-d` / `--dest`：远程目标路径，需为绝对路径（必需）

```bash
# 推送配置文件到 web 组
fastdp copy -s app.conf -d /etc/ web

# 推送脚本到指定主机
fastdp copy -s run.sh -d /tmp/run.sh 192.168.1.101
```

![image-20260529175910671](./assets/copy.png)

### 3. fetch 模块

批量从远程主机拉取文件（基于 SFTP），支持通配符匹配。

参数：
- `-r` / `--remote`：远程文件路径，支持 `*` `?` `[]` 通配符（必需）
- `-d` / `--dest`：本地保存目录（优先使用命令行参数，其次配置文件 `default_fetch_path`，兜底 `./fastdp-fetch`）
- `--no-ip-dir`：不创建 IP 目录，文件名改为 `IP_原文件名`

> 使用通配符时必须加引号

```bash
# 批量拉取所有主机 /tmp/sec* 文件
# 重要：远程路径含通配符时，必须用引号包裹
fastdp fetch --remote "/tmp/sec*" all

# 拉取指定组/IP 的日志文件
fastdp fetch -r "/var/log/messages" master
fastdp fetch -r "/root/*.txt" 192.168.1.101

# 指定本地保存目录
fastdp fetch -r "/tmp/sec?" --dest ./my-download all

# 不创建 IP 目录，文件名为 IP_文件名
fastdp fetch -r "/tmp/*.log" --no-ip-dir all
```

![image-20260529175910671](./assets/fetch.png)

### 4. script 模块

推送本地脚本到远程主机并执行（自动清理临时文件）。

参数：
- `-f` / `--file`：本地脚本路径（必需，文本文件，最大 512KB）

```bash
# 上传脚本并在所有主机执行
fastdp script -f run.sh all

# 执行到指定组和 IP
fastdp script -f check.sh master node 192.168.10.100
```

![image-20260529175910671](./assets/script.png)

### 5. ping 模块

测试远程主机 SSH 连通性。

```bash
# 全部主机
fastdp ping all
```

### 6. check 模块

批量主机环境巡检。执行巡检脚本（fastdp-check.sh）并格式化输出结果。

参数：
- `-g`：竖向格式化输出（类似 mysql \G）
- `-f`：导出格式，支持 csv / md / html

固定输出字段（18+ 标准字段，支持自定义字段）：

| 字段 | 说明 |
|------|------|
| hostname | 主机名 |
| virt | 虚拟化 |
| os | 系统版本 |
| kernel | 内核 |
| cpu_cores | CPU 核心 |
| cpu_model | CPU 型号 |
| arch | 架构 |
| mem | 内存 |
| net | 网卡 |
| gateway | 网关 |
| disk | 磁盘 |
| firewall | 防火墙 |
| selinux | SELinux |
| swap | Swap |
| timezone | 时区 |
| sys_time | 系统时间 |
| hw_time | 硬件时间 |
| gpu | GPU |

```bash
# 对所有主机执行环境检查（默认表格输出）
fastdp check all

# 竖向格式化输出
fastdp check all -g

# 导出巡检报告
fastdp check all -f csv  > report.csv
fastdp check all -f md   > report.md
fastdp check all -f html > report.html
```

**自定义字段**：编辑巡检脚本（`~/.fastdp/fastdp-check.sh` 或 `/etc/fastdp/fastdp-check.sh`），在末尾追加 `key=value` 格式即可自动识别展示：

```bash
# 示例：添加自定义字段
echo "my_custom_field=hello"
echo "app_version=$(cat /opt/app/VERSION)"
```

![image-20260529175910671](./assets/check.png)

竖向展示

![image-20260529175910671](./assets/check-g.png)

```zsh
fastdp check all -f csv  > report.csv
open report.csv
```



生成execl表格如图所示，html和md格式同理

![image-20260529175910671](./assets/check-c.png)

## 配置文件

### 路径与加载优先级

```
1. ~/.fastdp/config.toml      （用户自定义，优先级最高）
2. /etc/fastdp/config.toml    （系统默认）
3. ./config.toml              （当前目录，兜底）
```

### 配置项说明

```toml
# 主机清单路径
host_inventory = "/etc/fastdp/host"

# 默认并发数
concurrency = 15

# 默认 SSH 端口
default_ssh_port = 22

# 默认 SSH 用户名
default_ssh_user = "root"

# 默认 SSH 连接超时（秒），不设置或者值为0代表永不超时
default_ssh_timeout = 5

# 日志级别：debug/info
log_level = "info"

# 全局默认密码（所有机器统一密码的情况）
default_ssh_password = ""

# 默认文件拉取存放位置
default_fetch_path = "./fastdp-fetch"
```

## 主机组配置

主机组清单文件用于定义主机分组及主机连接参数，路径在配置文件中通过 `host_inventory` 指定。

### 格式说明

```ini
[组名]
主机地址 [参数=值 ...]
```

### 支持的参数

| 参数     | 说明                       | 默认值      |
|----------|----------------------------|-------------|
| user     | SSH 登录用户               | root        |
| port     | SSH 端口                   | 22          |
| password | SSH 登录密码（空则密钥认证）| 空（密钥）  |

### 示例

```ini
# Web 服务器组
[web]
192.168.1.100 user=admin port=2222
192.168.1.101 password=secure@123

# 数据库服务器组
[db]
192.168.2.50 user=dbadmin
192.168.2.51 port=2200

# 混合组
[test]
10.0.0.5 user=test
```

## 全局参数

| 参数          | 缩写 | 说明                               | 默认值 |
| ------------- | ---- | ---------------------------------- | ------ |
| --concurrency | -c   | 并发连接数                         | 15     |
| --debug       | -v   | 开启调试模式                       | false  |
| --inventory   | -i   | 指定主机清单文件（优先于配置文件） | ""     |
| --version     | -V   | 显示版本信息                       | false  |
| --help        | -h   | 查看帮助信息                       | -      |

## 注意事项

1. **主机组配置**：
   - 主机地址支持 IP 或域名
   - 若未指定 password，默认使用 SSH 密钥认证（优先读取 ~/.ssh/id_rsa、id_ed25519、id_ecdsa、id_dsa）
2. **文件复制**：
   - 远程目标路径（-d）必须为绝对路径
   - 支持保留源文件权限（自动同步源文件权限到远程文件）
   - 自动 MD5 校验，文件相同则跳过传输
3. **文件拉取**：
   - 远程路径含 `*` `?` 等通配符时，必须用引号包裹
   - 默认按 IP 创建子目录，`--no-ip-dir` 可改为 IP_文件名 模式
4. **远程脚本**：
   - 仅支持纯文本脚本（最大 512KB），禁止上传二进制文件
   - 非 .sh 后缀的文件会发出警告但不阻止执行
5. **错误排查**：
   - 开启调试模式（-v）可查看详细的 SSH 连接日志和命令执行过程
   - 若主机连接失败，检查 SSH 端口、认证方式及网络连通性
6. **check 模块**：
   - 巡检脚本路径：`~/.fastdp/fastdp-check.sh` > `/etc/fastdp/fastdp-check.sh`
   - 可自行编辑脚本内容，输出 `key=value` 格式即可自动识别

## 帮助与反馈

- 查看命令帮助：`fastdp --help` 或 `fastdp [模块名] --help`
- 提交 issue：https://gitee.com/zhao-pengfei2/fastdp/issues
