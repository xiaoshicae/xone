package xserver

import (
	"errors"
	"testing"
	"time"

	"github.com/xiaoshicae/xone/v2/xhook"

	. "github.com/bytedance/mockey"
	. "github.com/smartystreets/goconvey/convey"
)

func TestRunServer(t *testing.T) {
	PatchConvey("TestRunServer", t, func() {
		Mock(xhook.BeforeStart).Return(nil).Build()
		Mock(runWithServer).Return(errors.New("for test")).Build()
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

func TestRunWithServerRun(t *testing.T) {
	PatchConvey("TestRunWithServerRun", t, func() {
		PatchConvey("Panic-NilServer", func() {
			err := runWithServer(nil)
			So(err.Error(), ShouldEqual, "XOne xserver run failed, err=[panic occurred, runtime error: invalid memory address or nil pointer dereference]")
		})

		PatchConvey("Panic-UserPanic", func() {
			err := runWithServer(PanicRunServer{})
			So(err.Error(), ShouldEqual, "XOne xserver run failed, err=[panic occurred, panic run]")
		})

		PatchConvey("RunError", func() {
			err := runWithServer(ErrRunServer{})
			So(err.Error(), ShouldEqual, "XOne xserver run failed, err=[err run]")
		})

		PatchConvey("ExitWithNil", func() {
			err := runWithServer(NormalServer{})
			So(err, ShouldBeNil)
		})
	})
}

func TestSafeInvokeServerStop(t *testing.T) {
	PatchConvey("TestSafeInvokeServerStop", t, func() {
		PatchConvey("Panic-NilServer", func() {
			err := safeInvokeServerStop(nil)
			So(err.Error(), ShouldEqual, "XOne xserver stop failed, err=[panic occurred, runtime error: invalid memory address or nil pointer dereference]")
		})

		PatchConvey("Panic-UserPanic", func() {
			err := safeInvokeServerStop(PanicStopServer{})
			So(err.Error(), ShouldEqual, "XOne xserver stop failed, err=[panic occurred, stop panic]")
		})

		PatchConvey("StopError", func() {
			err := safeInvokeServerStop(ErrStopServer{})
			So(err.Error(), ShouldEqual, "XOne xserver stop failed, err=[stop err]")
		})

		PatchConvey("ExitWithNil", func() {
			err := safeInvokeServerStop(NormalServer{})
			So(err, ShouldBeNil)
		})
	})
}

func TestRunWithServerStopError(t *testing.T) {
	t.Skip("测试主动打断进程，退出err，需要手动执行")
	PatchConvey("TestRunWithServerStopError", t, func() {
		err := runWithServer(DemoServerStopError{})
		So(err.Error(), ShouldEqual, "XOne xserver stop failed, err=[stop err]")
	})
}

func TestRunWithServerStopSuccess(t *testing.T) {
	t.Skip("测试主动打断进程，正常退出情况，需要手动执行")
	PatchConvey("TestRunWithServerStopSuccess", t, func() {
		err := runWithServer(DemoServerStopSuccess{})
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

func TestBlockingServer(t *testing.T) {
	t.Skip("测试BlockingServer，需要主动打断进程")
	PatchConvey("TestBlockingServer", t, func() {
		s := &blockingServer{}
		_ = Run(s)
	})
}

func TestBlockingServerRunAndStop(t *testing.T) {
	PatchConvey("TestBlockingServerRunAndStop", t, func() {
		s := &blockingServer{}

		// Start Run in goroutine since it blocks
		done := make(chan error)
		go func() {
			done <- s.Run()
		}()

		// Give it a moment to start
		time.Sleep(10 * time.Millisecond)

		// Call Stop
		err := s.Stop()
		So(err, ShouldBeNil)

		// Run should complete
		select {
		case err := <-done:
			So(err, ShouldBeNil)
		case <-time.After(time.Second):
			t.Fatal("Run did not complete after Stop")
		}

		// Calling Stop again should be safe (idempotent)
		err = s.Stop()
		So(err, ShouldBeNil)
	})
}

func TestBlockingServerStopBeforeRun(t *testing.T) {
	PatchConvey("TestBlockingServerStopBeforeRun", t, func() {
		s := &blockingServer{}

		err := s.Stop()
		So(err, ShouldBeNil)

		done := make(chan error, 1)
		go func() {
			done <- s.Run()
		}()

		select {
		case err := <-done:
			So(err, ShouldBeNil)
		case <-time.After(time.Second):
			t.Fatal("Run did not complete after Stop")
		}
	})
}
