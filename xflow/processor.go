package xflow

import "context"

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
//
// 类型参数 T 是贯穿整个流程的共享数据，建议用指针（如 *OrderData），
// 入参、出参与各处理器间的中间数据都放在其中，各处理器可直接读写。
// 这样处理器的方法签名里只出现业务自己的类型，不必重复框架的泛型类型。
type Processor[T any] interface {
	// Name 返回处理器名称，用于日志和错误标识
	Name() string
	// Dependency 返回依赖类型
	Dependency() Dependency
	// Process 执行处理逻辑
	Process(ctx context.Context, data T) error
	// Rollback 回滚逻辑，强依赖失败时逆序调用已成功的处理器
	//
	// 注意：回滚使用的 context 已剥离原 context 的取消与超时（见 Flow.Execute），
	// 因此请求超时后补偿逻辑依然能够执行。
	Rollback(ctx context.Context, data T) error
}
