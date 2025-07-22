# fastdp
轻量级 Ansible 风格运维工具，支持在指定主机组上执行批量运维操作，提供多模块管理（如 shell 命令执行、文件复制、主机连通性检测等）。
## 功能特点
- 支持多模块操作（shell/copy/ping 等）
- 基于主机组的批量管理
- 并发连接控制，提高执行效率
- 支持 SSH 密码认证和密钥认证
- 灵活的配置文件和主机组管理

## 性能标杆：比 Ansible 快 18 倍的批量执行效率

在相同测试场景下（对 4 台远程主机执行 `date` 命令），fastdp 的执行速度实现了对 Ansible 的跨越式超越：

| 工具     | 实际耗时（real） | 性能对比       |
|----------|------------------|----------------|
| fastdp   | 0m0.135s         | ✔️ 快 18 倍    |
| Ansible  | 0m2.523s         |                |

## 为什么快？

### 极致并发调度：
- 基于 Go 原生协程（Goroutine）实现细粒度并发控制
- 精准管理 SSH 连接生命周期
- 避免传统多进程模型的资源浪费（如 Ansible 的 fork 开销）

### 无冗余设计：
- 摒弃复杂的兼容性逻辑和冗余配置解析
- 专注核心运维场景，让每一次执行都「轻装上阵」

## 对运维的意义

对于批量命令执行、主机状态巡检等高频场景，fastdp 可将分钟级操作压缩至秒级，真正实现「瞬时响应」的批量运维体验 —— 这意味着：

- 巡检效率提升 10 倍以上，大规模集群操作不再漫长等待
- 故障排查更及时，秒级反馈加速问题定位

> **注**：测试环境为 4 台同网段 Linux 主机，网络延迟 <1ms；实际性能因网络环境、并发数配置略有差异，但核心优势稳定。


## 安装
### 从源码编译
```bash
# 克隆仓库
git clone https://gitee.com/zhao-pengfei2/fastdp.git
cd fastdp

# 编译二进制
go build -o fastdp ./cmd/main.go

# 移动到 PATH 目录（可选）
sudo mv fastdp /usr/local/bin/

# 创建配置文件
mkdir /etc/fastdp&&cp -a config.toml host /etc/fastdp
echo 'Host_inventory = "/etc/fastdp/host"' /etc/fastdp/config.toml
```

### 基本用法
```bash
# 查看帮助
fastdp --help

# 在指定主机组执行 shell 命令（显式指定 shell 模块）
fastdp shell -a "ls -l /tmp" web

# 缺省 shell 模块（直接使用 -a 执行命令）
fastdp -a "uptime" all

# 使用 copy 模块复制文件到远程主机
fastdp copy -s local.txt -d /remote/path/ web

# 测试主机连通性
fastdp ping db

# 开启调试模式（显示详细日志）
fastdp -v shell -a "df -h" web

# 调整并发连接数（默认 10）
fastdp -c 20 shell -a "free -m" all
```
## 配置文件
### 路径与加载优先级
默认从以下路径按优先级加载配置文件（找到第一个存在的文件即使用）：
1. 系统级：/etc/fastdp/config.toml
2. 用户级：~/.fastdp/config.toml
3. 项目级：当前工作目录 ./config.toml
### 配置文件格式
```BASH
# 配置文件为 toml 格式，主要用于指定主机组清单路径，示例：
# config.toml
Host_inventory = "hosts"  # 主机组清单文件路径（默认当前目录的 "hosts" 文件）
```
   
## 主机组配置
主机组清单文件用于定义主机分组及主机连接参数，默认加载路径为配置文件中 Host_inventory 指定的路径（默认 ./hosts）。
### 格式说明
```BASH
# 主机组定义：[组名]
[组名]
# 主机地址 [参数=值 ...]
主机地址1 参数1=值1 参数2=值2 ...
主机地址2 参数1=值1 ...
```

### 支持的参数
```BASH
参数	说明	默认值
user	SSH 登录用户	root
port	SSH 端口	22
password	SSH 登录密码（不指定则使用密钥认证）	空（密钥）
```
### 示例配置
```bash
# hosts 文件示例

# Web 服务器组
[web]
192.168.1.100 user=admin port=2222  # 自定义用户和端口
192.168.1.101 password=secure@123   # 使用密码认证

# 数据库服务器组
[db]
192.168.2.50 user=dbadmin           # 自定义用户，使用默认端口和密钥认证
192.168.2.51 port=2200              # 自定义端口

# 混合组（可直接指定 IP 而非组名）
[test]
10.0.0.5 user=test
```
### 模块说明
### 1. shell 模块
   在远程主机执行 shell 命令，支持缺省使用（无需显式指定模块名）。
   参数： 
   - -a/--args：要执行的 shell 命令（必需）
   示例：
```bash
# 在 web 组执行命令
fastdp shell -a "echo 'hello world'" web

# 缺省模块，直接执行命令
fastdp -a "df -h" all
```
### 2. copy 模块
   复制本地文件到远程主机。
   参数：
   - -s/--source：本地源文件路径（必需）
   - -d/--dest：远程目标路径（必需，需为绝对路径）
   示例：
```bash
# 复制本地文件到远程 web 组
fastdp copy -s ./config.ini -d /etc/app/config.ini web
   ```
### 3. ping 模块
   测试远程主机连通性（SSH 连接检测）。
   示例：
```bash
# 检测 db 组主机连通性
fastdp ping db
```

### 全局参数
```bash
参数	缩写	说明	默认值
--args	-a	shell 模块命令参数	空
--concurrency	-c	并发连接数	10
--debug	-v	开启调试模式（显示详细日志）	false
--help	-h	查看帮助信息	-
```

### 注意事项
1. 主机组配置：
   -  主机地址支持 IP 或域名
   -  若未指定 password，默认使用 SSH 密钥认证（优先读取 ~/.ssh/id_rsa）
2. 文件复制：
   -  远程目标路径（-d）必须为绝对路径
   -  支持保留源文件权限（自动同步源文件权限到远程文件）
3. 错误排查：
   -  开启调试模式（-v）可查看详细的 SSH 连接日志和命令执行过程
   -  若主机连接失败，检查 SSH 端口、认证方式及网络连通性
### 帮助与反馈
   -  查看命令帮助：fastdp --help 或 fastdp [模块名] --help
   -  提交 issue：https://gitee.com/zhao-pengfei2/fastdp/issues