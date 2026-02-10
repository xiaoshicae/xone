package trans

import (
	"errors"
	"sort"
	"strings"

	ut "github.com/go-playground/universal-translator"
	"github.com/go-playground/validator/v10"
)

var trans ut.Translator

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
	if trans == nil {
		return err
	}

	var ves validator.ValidationErrors
	if !errors.As(err, &ves) {
		return err
	}

	result := make(map[string][]string)
	for _, e := range ves {
		result[e.Field()] = append(result[e.Field()], e.Translate(trans))
	}

	kvs := make([]VErrKV, 0)
	for k, v := range result {
		kvs = append(kvs, VErrKV{Field: k, Transl: strings.Join(v, ", ")})
	}

	sort.Slice(kvs, func(i, j int) bool {
		return kvs[i].Field < kvs[j].Field
	})

	errMessages := make([]string, 0)
	for _, kv := range kvs {
		errMessages = append(errMessages, kv.Transl)
	}

	return &ZHErr{Msg: strings.Join(errMessages, ", "), CauseErr: err}
}

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
