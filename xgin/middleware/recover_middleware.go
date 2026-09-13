package middleware

import (
	"errors"
	"fmt"
	"github.com/xiaoshicae/xone/v2/xlog"
	"net"
	"net/http"
	"os"
	"runtime"
	"strings"

	"github.com/gin-gonic/gin"
)

// Recover panic recover 中间件
// panic 日志通过 xlog 输出，与业务日志共用同一套格式与输出目标
func Recover(recoveryFunc gin.RecoveryFunc) gin.HandlerFunc {
	if recoveryFunc == nil {
		recoveryFunc = defaultHandleRecovery
	}
	return customRecoveryWithWriter(recoveryFunc)
}

const maxStackSize = 16384 // 栈信息最大 16KB

// customRecoveryWithWriter returns a middleware for a given writer that recovers from any panics and calls the provided handle func to handle it.
func customRecoveryWithWriter(handle gin.RecoveryFunc) gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if err := recover(); err != nil {
				// Check for a broken connection, as it is not really a
				// condition that warrants a panic stack trace.
				var brokenPipe bool
				if ne, ok := err.(*net.OpError); ok {
					var se *os.SyscallError
					if errors.As(ne, &se) {
						seStr := strings.ToLower(se.Error())
						if strings.Contains(seStr, "broken pipe") ||
							strings.Contains(seStr, "connection reset by peer") {
							brokenPipe = true
						}
					}
				}

				panicInfo := map[string]any{
					"panic_brokenPipe": brokenPipe,
					"panic_err":        fmt.Sprintf("%v", err),
					"panic_stack":      string(stack(3)),
				}
				// 走 xlog 而非全局 logrus，确保 panic 日志与业务日志使用同一套输出配置
				xlog.Error(c.Request.Context(), "panic recover, err=[%v]", err, xlog.KVMap(panicInfo))

				if brokenPipe {
					// brokenPipe 仅在 err 为 *net.OpError 时为 true，*net.OpError 实现了 error 接口
					_ = c.Error(err.(*net.OpError)) //nolint: errcheck
					c.Abort()
					return
				}

				if c.Writer.Written() {
					c.Abort()
					return
				}

				handle(c, err)
			}
		}()
		c.Next()
	}
}

func defaultHandleRecovery(c *gin.Context, _ any) {
	c.AbortWithStatus(http.StatusInternalServerError)
}

// stack 返回当前 goroutine 的格式化栈信息
// 使用 runtime.Stack 替代逐帧读取源文件，避免 panic 恢复期间的文件 I/O
func stack(_ int) []byte {
	buf := make([]byte, maxStackSize)
	n := runtime.Stack(buf, false)
	return buf[:n]
}
