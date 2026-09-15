package xserver

import (
	"errors"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/xiaoshicae/xone/v3/xerror"
	"github.com/xiaoshicae/xone/v3/xhook"
	_ "github.com/xiaoshicae/xone/v3/xtrace" // 默认加载trace
	"github.com/xiaoshicae/xone/v3/xutil"
)

// defaultWaitRunExitTimeout Stop 后等待 Run goroutine 退出的默认超时
const defaultWaitRunExitTimeout = 30 * time.Second

var (
	waitRunExitTimeout = defaultWaitRunExitTimeout
	waitRunExitMu      sync.RWMutex
)

// SetWaitRunExitTimeout 设置 Stop 后等待 Run goroutine 退出的超时时间（线程安全）
// timeout <= 0 时不生效，保持原值
func SetWaitRunExitTimeout(timeout time.Duration) {
	if timeout > 0 {
		waitRunExitMu.Lock()
		waitRunExitTimeout = timeout
		waitRunExitMu.Unlock()
	}
}

func getWaitRunExitTimeout() time.Duration {
	waitRunExitMu.RLock()
	defer waitRunExitMu.RUnlock()
	return waitRunExitTimeout
}

// Run 启动Server，会以阻塞方式启动，且等待退出信号
func Run(server Server) error {
	return run(server)
}

// RunBlocking 启动Server，会以阻塞方式启动，且等待退出信号，用于consumer或job服务等
func RunBlocking() error {
	return run(&blockingServer{})
}

// Init 只执行 BeforeStart Hook，完成各模块初始化但不启动任何服务
//
// 用于测试与调试：初始化完成后各模块保持可用状态，调用方可以直接使用
// xgorm.C()、xhttp.C() 等客户端。
//
// 注意：Init 不执行 BeforeStop Hook，日志写入器不会 flush，
// 进程退出前若要确保日志落盘，请改用 Run。
func Init() error {
	return run(nil)
}

func run(server Server) error {
	if err := xhook.InvokeBeforeStartHook(); err != nil {
		// 启动失败同样要执行 BeforeStop：hook 按 xconfig → xlog → xtrace → ... 正序执行，
		// 失败点之前的模块都已初始化完成。不回滚意味着日志写入器不 flush、
		// trace provider 不 shutdown、连接池不关闭——而"启动为什么失败"这条日志恰恰最需要落盘
		stopErr := xhook.InvokeBeforeStopHook()
		if stopErr != nil {
			xutil.ErrorIfEnableDebug("XOne rollback after BeforeStart failure got error, err=[%v]", stopErr)
		}
		return errors.Join(err, stopErr)
	}

	if server != nil {
		serverRunErr := runWithServer(server)             // 服务会以阻塞方式启动
		beforeStopHookErr := xhook.InvokeBeforeStopHook() // 无论服务是否报错，都执行 stop hook
		return errors.Join(serverRunErr, beforeStopHookErr)
	}

	// 如果不是Server，则只会执行InvokeBeforeStartHook，一般用于调试
	return nil
}

func runWithServer(s Server) error {
	serverRunErrChan := make(chan error, 1)
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, quitSignals...)
	defer signal.Stop(quit)

	// Run 自行返回与收到退出信号可能同时发生，用 Once 保证 Stop 只执行一次
	var (
		stopOnce sync.Once
		stopErr  error
	)
	stop := func() error {
		stopOnce.Do(func() { stopErr = safeInvokeServerStop(s) })
		return stopErr
	}

	go safeInvokeServerRun(s, serverRunErrChan)

	select {
	case runErr := <-serverRunErrChan: // 服务运行失败，或 Run 自行返回
		// 这条路径上同样要调用 Stop：Server 接口约定清理逻辑放在 Stop 中，
		// 只在收到退出信号时才调用，会让端口占用等场景下的资源永远不被释放
		sErr := stop()
		if runErr == nil {
			xutil.InfoIfEnableDebug("XOne Run server stopped")
		}
		return errors.Join(runErr, sErr)

	case <-quit: // 接收到退出信号后，执行Server.Stop()
		xutil.InfoIfEnableDebug("********** XOne Stop server begin **********")
		sErr := stop()

		// 等待 Run goroutine 退出，避免 goroutine 泄漏
		waitTimeout := getWaitRunExitTimeout()
		select {
		case runErr := <-serverRunErrChan:
			// Run 在 Stop 之后返回的错误不能丢：优雅退出失败时这往往是唯一线索
			if runErr != nil {
				xutil.ErrorIfEnableDebug("XOne Run returned error after Stop, err=[%v]", runErr)
				sErr = errors.Join(sErr, runErr)
			}
		case <-time.After(waitTimeout):
			xutil.WarnIfEnableDebug("XOne Run goroutine did not exit within %v after Stop", waitTimeout)
		}

		if sErr != nil {
			return sErr
		}
		xutil.InfoIfEnableDebug("********** XOne Stop server success **********")
		return nil
	}
}

func safeInvokeServerRun(s Server, serverRunErrChan chan<- error) {
	defer func() {
		if r := recover(); r != nil {
			serverRunErrChan <- xerror.Newf("xserver", "run", "panic occurred, %v", r)
		}
	}()

	err := s.Run() // 服务一般会阻塞在此处
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		serverRunErrChan <- xerror.New("xserver", "run", err)
	} else {
		serverRunErrChan <- nil
	}
}

func safeInvokeServerStop(s Server) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = xerror.Newf("xserver", "stop", "panic occurred, %v", r)
		}
	}()

	if err = s.Stop(); err != nil {
		return xerror.New("xserver", "stop", err)
	}
	return nil
}

var quitSignals = []os.Signal{
	syscall.SIGHUP,
	syscall.SIGINT,
	syscall.SIGTERM,
}
