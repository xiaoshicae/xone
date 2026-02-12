package middleware

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"runtime"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
)

// GinXRecoverMiddleware panic recover 中间件
// 使用logrus全局log，如果该log被其它框架初始化过，则可以直接复用设置好的格式，否则默认打印到控制台
func GinXRecoverMiddleware(recoveryFunc gin.RecoveryFunc) gin.HandlerFunc {
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

				panicInfo := logrus.Fields{
					"panic_brokenPipe": brokenPipe,
					"panic_err":        fmt.Sprintf("%v", err),
					"panic_stack":      string(stack(3)),
				}
				logrus.WithContext(c.Request.Context()).WithFields(panicInfo).Errorf("panic recover, err=[%v]", err)

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
