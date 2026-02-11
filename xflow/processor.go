package xflow

// Dependency 依赖类型，标记 Processor 的依赖强弱
type Dependency int

const (
	// Strong 强依赖，失败时中断流程并触发回滚
	Strong Dependency = iota
	// Weak 弱依赖，失败时跳过并继续执行
	Weak
)

// String 返回依赖类型的字符串表示
func (d Dependency) String() string {
	switch d {
	case Strong:
		return "Strong"
	case Weak:
		return "Weak"
	default:
		return "Unknown"
	}
}

// Processor 流程处理器接口
type Processor[T any] interface {
	// Name 返回处理器名称，用于日志和错误标识
	Name() string
	// Dependency 返回依赖类型
	Dependency() Dependency
	// Process 执行处理逻辑
	Process(fc *FlowContext[T]) error
	// Rollback 回滚逻辑，强依赖失败时逆序调用已成功的处理器
	Rollback(fc *FlowContext[T]) error
}
