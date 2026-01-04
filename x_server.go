package xone

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/xiaoshicae/xone/xutil"
)

type Server interface {
	// Run 启动服务
	// 建议服务以阻塞方式运行，框架会以异步方式运行服务，且阻塞等待退出信号，如果服务Run()结束，那么XOne.Server也会运行结束
	Run() error

	// Stop 停止服务
	// 建议放一些资源清理逻辑
	Stop() error
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
