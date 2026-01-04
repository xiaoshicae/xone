package xone

import (
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/xiaoshicae/xone/xconfig"
	"github.com/xiaoshicae/xone/xhook"

	"github.com/gin-gonic/gin"

	. "github.com/bytedance/mockey"
	. "github.com/smartystreets/goconvey/convey"
)

func TestRunServer(t *testing.T) {
	PatchConvey("TestRunServer", t, func() {
		Mock(xhook.BeforeStart).Return(nil).Build()
		Mock(runWithSever).Return(errors.New("for test")).Build()
		Mock(xhook.InvokeBeforeStopHook).Return(errors.New("for test 2")).Build()
		err := run(MockServer{})
		So(err.Error(), ShouldEqual, "for test\nfor test 2")
	})
}

type MockServer struct{}

func (m MockServer) Run() error {
	return nil
}

func (m MockServer) Stop() error {
	return nil
}

func TestRunWithSeverRun(t *testing.T) {
	PatchConvey("TestRunWithSeverRun-Panic", t, func() {
		err := runWithSever(nil)
		So(err.Error(), ShouldEqual, "XOne Run server failed, err=[panic occurred, runtime error: invalid memory address or nil pointer dereference]")
	})

	PatchConvey("TestRunWithSeverRun-Panic2", t, func() {
		err := runWithSever(PanicRunServer{})
		So(err.Error(), ShouldEqual, "XOne Run server failed, err=[panic occurred, panic run]")
	})

	PatchConvey("TestRunWithSeverRun-Err", t, func() {
		err := runWithSever(ErrRunServer{})
		So(err.Error(), ShouldEqual, "XOne Run server failed, err=[err run]")
	})

	PatchConvey("TestRunWithSeverRun-ExistWitNil", t, func() {
		err := runWithSever(NormalServer{})
		So(err, ShouldBeNil)
	})
}

func TestSafeInvokeServerStop(t *testing.T) {
	PatchConvey("TestRunWithSeverRun-Panic", t, func() {
		err := safeInvokeServerStop(nil)
		So(err.Error(), ShouldEqual, "panic occurred, runtime error: invalid memory address or nil pointer dereference")
	})

	PatchConvey("TestRunWithSeverRun-Panic2", t, func() {
		err := safeInvokeServerStop(PanicStopServer{})
		So(err.Error(), ShouldEqual, "panic occurred, stop panic")
	})

	PatchConvey("TestRunWithSeverRun-Err", t, func() {
		err := safeInvokeServerStop(ErrStopServer{})
		So(err.Error(), ShouldEqual, "stop err")
	})

	PatchConvey("TestRunWithSeverStop-ExistWitNil", t, func() {
		err := safeInvokeServerStop(NormalServer{})
		So(err, ShouldBeNil)
	})
}

func TestRunWithSeverStopError(t *testing.T) {
	t.Skip("测试主动打断进程，退出err，需要手动执行")
	PatchConvey("TestRunWithSeverStopError", t, func() {
		err := runWithSever(DemoServerStopError{})
		So(err.Error(), ShouldEqual, "XOne Stop server failed, err=[stop err]")
	})
}

func TestRunWithSeverStopSuccess(t *testing.T) {
	t.Skip("测试主动打断进程，正常退出情况，需要手动执行")
	PatchConvey("TestRunWithSeverStopSuccess", t, func() {
		err := runWithSever(DemoServerStopSuccess{})
		So(err, ShouldBeNil)
	})
}

type PanicRunServer struct{}

func (d PanicRunServer) Run() error {
	panic("panic run")
}

func (d PanicRunServer) Stop() error {
	return nil
}

type ErrRunServer struct{}

func (d ErrRunServer) Run() error {
	return errors.New("err run")
}

func (d ErrRunServer) Stop() error {
	return nil
}

type NormalServer struct{}

func (d NormalServer) Run() error {
	return nil
}

func (d NormalServer) Stop() error {
	return nil
}

type PanicStopServer struct{}

func (d PanicStopServer) Run() error {
	return nil
}

func (d PanicStopServer) Stop() error {
	panic("stop panic")
}

type ErrStopServer struct{}

func (d ErrStopServer) Run() error {
	return nil
}

func (d ErrStopServer) Stop() error {
	return errors.New("stop err")
}

type DemoServerStopError struct{}

func (d DemoServerStopError) Run() error {
	time.Sleep(10 * time.Second)
	return nil
}

func (d DemoServerStopError) Stop() error {
	return errors.New("stop err")
}

type DemoServerStopSuccess struct{}

func (d DemoServerStopSuccess) Run() error {
	time.Sleep(10 * time.Second)
	return nil
}

func (d DemoServerStopSuccess) Stop() error {
	return nil
}

func TestNewGinServer(t *testing.T) {
	PatchConvey("TestNewGinServer", t, func() {
		Mock(xconfig.GetGinConfig).Return(&xconfig.Gin{UseHttp2: true, Host: "123", Port: 456}).Build()
		Mock((*http.Server).ListenAndServe).Return(errors.New("for test")).Build()
		Mock((*http.Server).Shutdown).Return(errors.New("for test2")).Build()

		server := newGinServer(gin.New())
		So(server, ShouldNotBeNil)

		err := server.Run()
		So(err.Error(), ShouldEqual, "for test")

		err = server.Stop()
		So(err.Error(), ShouldEqual, "gin server stop failed, err=[for test2]")
	})
}

func TestBlockingServer(t *testing.T) {
	t.Skip("测试BlockingServer，需要主动打断进程")
	PatchConvey("TestBlockingServer", t, func() {
		s := &blockingServer{}
		_ = RunServer(s)
	})
}
