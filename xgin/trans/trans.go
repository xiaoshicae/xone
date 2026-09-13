package trans

import (
	"errors"
	"maps"
	"slices"
	"strings"
	"sync"

	ut "github.com/go-playground/universal-translator"
	"github.com/go-playground/validator/v10"
)

var (
	// transMu 保护 trans 的读写
	//
	// 只锁写侧是不够的：ToZHErr 在请求期读 trans，RegisterZHTranslations 是公开 API，
	// 调用时机由使用者决定，两者并发时 -race 能直接测出数据竞争
	transMu sync.RWMutex
	trans   ut.Translator
)

// getTrans 读取当前翻译器（线程安全）
func getTrans() ut.Translator {
	transMu.RLock()
	defer transMu.RUnlock()
	return trans
}

// ToZHErrMsg 翻译错误信息字符串
func ToZHErrMsg(err error) string {
	if err = ToZHErr(err); err != nil {
		return err.Error()
	}
	return ""
}

// ToZHErr 翻译成功中文错误
func ToZHErr(err error) error {
	if err == nil {
		return nil
	}

	// 没有初始化，说明没有启用
	t := getTrans()
	if t == nil {
		return err
	}

	var ves validator.ValidationErrors
	if !errors.As(err, &ves) {
		return err
	}

	result := make(map[string][]string)
	for _, e := range ves {
		result[e.Field()] = append(result[e.Field()], e.Translate(t))
	}

	// 按字段名排序输出，保证同一组校验错误每次得到相同的消息
	msgs := make([]string, 0, len(result))
	for _, field := range slices.Sorted(maps.Keys(result)) {
		msgs = append(msgs, strings.Join(result[field], ", "))
	}

	return &ZHErr{Msg: strings.Join(msgs, ", "), CauseErr: err}
}

// VErrKV 字段名与其翻译后的错误消息
//
// Deprecated: 保留仅为兼容，内部已不再使用
type VErrKV struct {
	Field  string
	Transl string
}

type ZHErr struct {
	Msg      string
	CauseErr error
}

func (e *ZHErr) Error() string {
	return e.Msg
}

func (e *ZHErr) Cause() error {
	return e.CauseErr
}

// Unwrap 实现 Go 标准库 errors.Unwrap 接口
func (e *ZHErr) Unwrap() error {
	return e.CauseErr
}
