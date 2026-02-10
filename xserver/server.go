package xserver

// Server 服务接口
type Server interface {
	// Run 启动服务
	// 建议服务以阻塞方式运行，框架会以异步方式运行服务，且阻塞等待退出信号，如果服务Run()结束，那么XOne.Server也会运行结束
	Run() error

	// Stop 停止服务
	// 建议放一些资源清理逻辑
	Stop() error
}
