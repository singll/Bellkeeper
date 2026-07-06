// goroutine.go 提供 goroutine panic 护栏辅助（1.0 §6 护栏：所有 go func() 加 recover + zap）。
//
// 用 SafeGo(fn) 替代 go func(){...}，确保 panic 不致进程崩溃，统一记录到 zap logger。
// 复用 middleware.GetLogger()，无新依赖。

package service

import (
	"github.com/singll/bellkeeper/internal/middleware"
	"go.uber.org/zap"
)

// SafeGo 启动一个带 panic recover 的 goroutine。
// fn 内 panic 会被捕获并记录到 zap，不传播到 goroutine 外（避免进程崩溃）。
func SafeGo(name string, fn func()) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				middleware.GetLogger().Error("goroutine panic recovered",
					zap.String("goroutine", name),
					zap.Any("panic", r),
				)
			}
		}()
		fn()
	}()
}
