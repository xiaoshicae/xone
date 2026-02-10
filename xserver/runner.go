package xserver

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/xiaoshicae/xone/xhook"
	_ "github.com/xiaoshicae/xone/xtrace" // 默认加载trace
	"github.com/xiaoshicae/xone/xutil"
)

// Run 启动Server，会以阻塞方式启动，且等待退出信号
func Run(server Server) error {
	return run(server)
}

// RunBlocking 启动Server，会以阻塞方式启动，且等待退出信号，用于consumer或job服务等
func RunBlocking() error {
	return run(&blockingServer{})
}

// R 调用before start hook，建议用于调试
func R() error {
	return run(nil)
}

func run(server Server) error {
	if err := xhook.InvokeBeforeStartHook(); err != nil {
		return err
	}

	if server != nil {
		var serverRunErr error
		if err := runWithSever(server); err != nil { // 服务会以阻塞方式启动
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

func runWithSever(s Server) error {
	serverRunErrChan := make(chan error, 1)
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, quitSignals...)

	go func() {
		safeInvokeServerRun(s, serverRunErrChan)
	}()

	select {
	case err := <-serverRunErrChan: // 接收到服务运行失败消息，或者正常退出指令时
		if err != nil {
			return fmt.Errorf("XOne Run server failed, err=[%v]", err)
		}
		xutil.WarnIfEnableDebug("XOne Run server unexpected stopped")
		return nil
	case <-quit: // 接收到退出信号后，执行Server.Stop()
		xutil.InfoIfEnableDebug("********** XOne Stop server begin **********")
		if err := safeInvokeServerStop(s); err != nil {
			return fmt.Errorf("XOne Stop server failed, err=[%v]", err)
		}
		xutil.InfoIfEnableDebug("********** XOne Stop server success **********")
		return nil
	}
}

func safeInvokeServerRun(s Server, serverRunErrChan chan<- error) {
	defer func() {
		if r := recover(); r != nil {
			serverRunErrChan <- fmt.Errorf("panic occurred, %v", r)
		}
	}()

	err := s.Run() // 服务一般会阻塞在此处
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		serverRunErrChan <- err
	} else {
		serverRunErrChan <- nil
	}
}

func safeInvokeServerStop(s Server) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic occurred, %v", r)
		}
	}()

	if err = s.Stop(); err != nil {
		return err
	}
	return nil
}

var quitSignals = []os.Signal{
	syscall.SIGHUP,
	syscall.SIGINT,
	syscall.SIGTERM,
}
