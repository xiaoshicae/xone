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

// RegisterZHTranslations 注册中文翻译器
// 返回 error 以便调用者知道注册是否成功
func RegisterZHTranslations() error {
	// 创建翻译器
	zhTrans := zh.New()
	uni := ut.New(zhTrans, zhTrans)

	var ok bool
	trans, ok = uni.GetTranslator("zh")
	if !ok {
		err := errors.New("zh translator not found")
		logrus.Warnf("zh translator not found, translator not take effect")
		return err
	}

	v, ok := binding.Validator.Engine().(*validator.Validate)
	if !ok {
		err := errors.New("gin binding.Validator.Engine() type assign to *validator.Validate failed")
		logrus.Warnf("gin binding.Validator.Engine() type assign to *validator.Validate failed, translator not take effect")
		return err
	}

	if err := zt.RegisterDefaultTranslations(v, trans); err != nil {
		logrus.Warnf("register zh translator failed, translator not take effect, err=%v", err)
		return err
	}

	return nil
}
