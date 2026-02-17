package xpipeline

// Frame Pipeline 中传递的数据单元接口
type Frame interface {
	FrameType() string
}

// StartFrame 启动信号，携带任意上下文
type StartFrame struct {
	Context any
}

// FrameType 返回帧类型标识
func (f *StartFrame) FrameType() string { return "start" }

// EndFrame 结束信号
type EndFrame struct{}

// FrameType 返回帧类型标识
func (f *EndFrame) FrameType() string { return "end" }

// ErrorFrame 错误信号
type ErrorFrame struct {
	Err     error
	Message string
}

// FrameType 返回帧类型标识
func (f *ErrorFrame) FrameType() string { return "error" }

// MetadataFrame 处理器间元数据传递
type MetadataFrame struct {
	Key   string
	Value any
}

// FrameType 返回帧类型标识
func (f *MetadataFrame) FrameType() string { return "metadata" }
