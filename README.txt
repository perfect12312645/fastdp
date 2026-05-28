fastdp - 批量SSH运维工具

版本：v6.0.0

功能：
  - 批量执行命令(shell)
  - 批量文件推送(copy)
  - 批量文件拉取(fetch)
  - 批量远程脚本(script)
  - 批量主机检查(check)

使用方法：
  fastdp --help
  fastdp shell -a "uptime" all
  fastdp copy -s localfile -d /remote/path all
  fastdp fetch -r "/remote/logs/*" all
  fastdp check all

配置文件：
  /etc/fastdp/config.toml
  /etc/fastdp/host
  /etc/fastdp/fastdp-check.sh

详细文档：
  https://gitee.com/zhao-pengfei2/fastdp