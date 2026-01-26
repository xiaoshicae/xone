package xone

import "sync"

// blockingServer 不再需要 signal 包，信号由 runWithSever 统一处理

// blockingServer 会以阻塞方式启动Server，且等待退出信号，用于consumer服务等
type blockingServer struct {
	quit     chan struct{}
	initOnce sync.Once
	stopOnce sync.Once
}

func (b *blockingServer) Run() error {
	b.initQuit()
	<-b.quit // 阻塞直到 Stop() 被调用
	return nil
}

func (b *blockingServer) Stop() error {
	b.initQuit()
	b.stopOnce.Do(func() {
		close(b.quit)
	})
	return nil
}

func (b *blockingServer) initQuit() {
	b.initOnce.Do(func() {
		b.quit = make(chan struct{})
	})
}
