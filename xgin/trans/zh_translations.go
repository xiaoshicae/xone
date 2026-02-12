package trans

import (
	"errors"

	"github.com/gin-gonic/gin/binding"
	"github.com/go-playground/locales/zh"
	ut "github.com/go-playground/universal-translator"
	"github.com/go-playground/validator/v10"
	zt "github.com/go-playground/validator/v10/translations/zh"
	"github.com/xiaoshicae/xone/v2/xutil"
)

// RegisterZHTranslations 注册中文翻译器（线程安全，成功后不再重试，失败后允许重试）
// 返回 error 以便调用者知道注册是否成功
func RegisterZHTranslations() error {
	transMu.Lock()
	defer transMu.Unlock()

	// 已注册成功，直接返回
	if trans != nil {
		return nil
	}

	// 创建翻译器
	zhTrans := zh.New()
	uni := ut.New(zhTrans, zhTrans)

	t, ok := uni.GetTranslator("zh")
	if !ok {
		xutil.WarnIfEnableDebug("zh translator not found, translator not take effect")
		return errors.New("zh translator not found")
	}

	v, ok := binding.Validator.Engine().(*validator.Validate)
	if !ok {
		xutil.WarnIfEnableDebug("gin binding.Validator.Engine() type assign to *validator.Validate failed, translator not take effect")
		return errors.New("gin binding.Validator.Engine() type assign to *validator.Validate failed")
	}

	if err := zt.RegisterDefaultTranslations(v, t); err != nil {
		xutil.WarnIfEnableDebug("register zh translator failed, translator not take effect, err=[%v]", err)
		return err
	}

	// 注册成功后设置全局 translator
	trans = t
	return nil
}
