package trans

import (
	"errors"

	"github.com/gin-gonic/gin/binding"
	"github.com/go-playground/locales/zh"
	ut "github.com/go-playground/universal-translator"
	"github.com/go-playground/validator/v10"
	zt "github.com/go-playground/validator/v10/translations/zh"
	"github.com/sirupsen/logrus"
)

// RegisterZHTranslations 注册中文翻译器（线程安全，仅首次调用生效）
// 返回 error 以便调用者知道注册是否成功
func RegisterZHTranslations() error {
	var regErr error
	transOnce.Do(func() {
		// 创建翻译器
		zhTrans := zh.New()
		uni := ut.New(zhTrans, zhTrans)

		t, ok := uni.GetTranslator("zh")
		if !ok {
			regErr = errors.New("zh translator not found")
			logrus.Warnf("zh translator not found, translator not take effect")
			return
		}

		v, ok := binding.Validator.Engine().(*validator.Validate)
		if !ok {
			regErr = errors.New("gin binding.Validator.Engine() type assign to *validator.Validate failed")
			logrus.Warnf("gin binding.Validator.Engine() type assign to *validator.Validate failed, translator not take effect")
			return
		}

		if err := zt.RegisterDefaultTranslations(v, t); err != nil {
			logrus.Warnf("register zh translator failed, translator not take effect, err=%v", err)
			regErr = err
			return
		}

		// 注册成功后设置全局 translator
		trans = t
	})
	return regErr
}
