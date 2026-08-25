package exitcode

const (
	Success       = 0 // 全部成功
	PartialFail   = 1 // 部分失败（模块执行失败）
	ParamError    = 2 // 参数/配置错误
	ConnectFail   = 3 // 连接失败
	Timeout       = 4 // 执行超时
	AuthFail      = 5 // 认证失败
	InternalError = 6 // 程序内部错误
)
