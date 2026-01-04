package xone

import (
	"errors"

	"github.com/xiaoshicae/xone/xhook"
	_ "github.com/xiaoshicae/xone/xtrace" // 默认加载trace

	"github.com/gin-gonic/gin"
)

// R 调用before start hook，建议用于调试
func R() error {
	return run(nil)
}

// RunServer 启动Server，会以阻塞方式启动，且等待退出信号
func RunServer(server Server) error {
	return run(server)
}

// RunGin 启动gin，会以阻塞方式启动，且等待退出信号
func RunGin(engine *gin.Engine) error {
	return run(engine)
}

// RunBlockingServer 启动Server，会以阻塞方式启动，且等待退出信号，用于consumer或job服务等
func RunBlockingServer() error {
	return run(&blockingServer{})
}

func run(server interface{}) error {
	if err := xhook.InvokeBeforeStartHook(); err != nil {
		return err
	}

	if s, ok := server.(*gin.Engine); ok {
		server = newGinServer(s)
	}

	if s, ok := server.(Server); ok {
		var serverRunErr error
		if err := runWithSever(s); err != nil { // 服务会以阻塞方式启动
			serverRunErr = err
		}

		var beforeStopHookErr error
		if err := xhook.InvokeBeforeStopHook(); err != nil {
			beforeStopHookErr = err
		}

		if serverRunErr != nil || beforeStopHookErr != nil { // 任何错误发生，则合并成一个返回
			return errors.Join(serverRunErr, beforeStopHookErr)
		}

		return nil
	}

	// 如果不是Server，则只会执行InvokeBeforeStartHook，一般用于调试
	return nil
}
