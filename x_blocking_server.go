package xone

import "sync"

// blockingServer 不再需要 signal 包，信号由 runWithSever 统一处理

// blockingServer 会以阻塞方式启动Server，且等待退出信号，用于consumer服务等
type blockingServer struct {
	quit     chan struct{}
	stopOnce sync.Once
}

func (b *blockingServer) Run() error {
	b.quit = make(chan struct{})
	<-b.quit // 阻塞直到 Stop() 被调用
	return nil
}

func (b *blockingServer) Stop() error {
	b.stopOnce.Do(func() {
		if b.quit != nil {
			close(b.quit)
		}
	})
	return nil
}
