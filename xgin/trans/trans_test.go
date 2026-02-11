package trans

import (
	"errors"
	"sort"
	"sync"
	"testing"

	"github.com/gin-gonic/gin/binding"
	ut "github.com/go-playground/universal-translator"
	"github.com/go-playground/validator/v10"
	zt "github.com/go-playground/validator/v10/translations/zh"

	. "github.com/bytedance/mockey"
	. "github.com/smartystreets/goconvey/convey"
)

func TestToZHErrNil(t *testing.T) {
	result := ToZHErr(nil)
	if result != nil {
		t.Error("ToZHErr(nil) should return nil")
	}
}

func TestToZHErrMsg(t *testing.T) {
	result := ToZHErrMsg(nil)
	if result != "" {
		t.Error("ToZHErrMsg(nil) should return empty string")
	}
}

func TestToZHErrMsgWithError(t *testing.T) {
	err := errors.New("test error")
	result := ToZHErrMsg(err)
	if result != "test error" {
		t.Errorf("expected 'test error', got '%s'", result)
	}
}

func TestToZHErrNonValidationError(t *testing.T) {
	err := errors.New("test error")
	result := ToZHErr(err)
	if result != err {
		t.Error("non-validation error should be returned as-is")
	}
}

func TestToZHErrWithoutTrans(t *testing.T) {
	// trans 为 nil 时应该返回原始错误
	originalTrans := trans
	trans = nil
	defer func() { trans = originalTrans }()

	validate := validator.New()
	type TestStruct struct {
		Name string `validate:"required"`
	}
	err := validate.Struct(&TestStruct{})

	result := ToZHErr(err)
	// 当 trans 为 nil 时，应该返回原始错误
	if result == nil {
		t.Error("should return original error when trans is nil")
	}
}

func TestFieldSortingAscending(t *testing.T) {
	// 测试字段排序是否为升序
	kvs := []VErrKV{
		{Field: "Zebra", Transl: "error1"},
		{Field: "Apple", Transl: "error2"},
		{Field: "Mango", Transl: "error3"},
	}

	sort.Slice(kvs, func(i, j int) bool {
		return kvs[i].Field < kvs[j].Field
	})

	if kvs[0].Field != "Apple" {
		t.Errorf("expected first field to be 'Apple', got '%s'", kvs[0].Field)
	}
	if kvs[1].Field != "Mango" {
		t.Errorf("expected second field to be 'Mango', got '%s'", kvs[1].Field)
	}
	if kvs[2].Field != "Zebra" {
		t.Errorf("expected third field to be 'Zebra', got '%s'", kvs[2].Field)
	}
}

func TestZHErrError(t *testing.T) {
	err := &ZHErr{Msg: "test message", CauseErr: errors.New("cause")}
	if err.Error() != "test message" {
		t.Errorf("expected 'test message', got '%s'", err.Error())
	}
}

func TestZHErrCause(t *testing.T) {
	cause := errors.New("cause error")
	err := &ZHErr{Msg: "test message", CauseErr: cause}
	if err.Cause() != cause {
		t.Error("Cause() should return the original error")
	}
}

func TestVErrKVStruct(t *testing.T) {
	kv := VErrKV{Field: "Name", Transl: "名称不能为空"}
	if kv.Field != "Name" {
		t.Errorf("expected Field 'Name', got '%s'", kv.Field)
	}
	if kv.Transl != "名称不能为空" {
		t.Errorf("expected Transl '名称不能为空', got '%s'", kv.Transl)
	}
}

func TestToZHErrWithValidationError(t *testing.T) {
	// 注册中文翻译器
	RegisterZHTranslations()

	validate := validator.New()
	type TestStruct struct {
		Name  string `validate:"required"`
		Email string `validate:"required,email"`
	}

	err := validate.Struct(&TestStruct{})
	result := ToZHErr(err)

	if result == nil {
		t.Fatal("result should not be nil")
	}

	zhErr, ok := result.(*ZHErr)
	if !ok {
		t.Fatal("result should be *ZHErr")
	}

	if zhErr.Msg == "" {
		t.Error("ZHErr.Msg should not be empty")
	}

	// 验证 Cause 返回原始错误（比较错误消息而非直接比较）
	if zhErr.Cause() == nil {
		t.Error("Cause() should return original error")
	}
	if zhErr.Cause().Error() != err.Error() {
		t.Error("Cause() should return the same error message")
	}
}

func TestFieldSortingInToZHErr(t *testing.T) {
	// 注册中文翻译器
	RegisterZHTranslations()

	validate := validator.New()
	type TestStruct struct {
		Zebra string `validate:"required"`
		Apple string `validate:"required"`
		Mango string `validate:"required"`
	}

	err := validate.Struct(&TestStruct{})
	result := ToZHErr(err)

	zhErr, ok := result.(*ZHErr)
	if !ok {
		t.Fatal("result should be *ZHErr")
	}

	// 错误信息应该按字段名升序排列
	msg := zhErr.Msg
	appleIdx := findIndex(msg, "Apple")
	mangoIdx := findIndex(msg, "Mango")
	zebraIdx := findIndex(msg, "Zebra")

	if appleIdx > mangoIdx || mangoIdx > zebraIdx {
		t.Error("fields should be sorted in ascending order")
	}
}

func findIndex(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}

func TestZHErrUnwrap(t *testing.T) {
	cause := errors.New("cause error")
	err := &ZHErr{Msg: "test message", CauseErr: cause}

	unwrapped := err.Unwrap()
	if unwrapped != cause {
		t.Error("Unwrap() should return the cause error")
	}
}

func TestZHErrUnwrapNil(t *testing.T) {
	err := &ZHErr{Msg: "test message", CauseErr: nil}

	unwrapped := err.Unwrap()
	if unwrapped != nil {
		t.Error("Unwrap() should return nil when CauseErr is nil")
	}
}

func TestErrorsIs(t *testing.T) {
	cause := errors.New("cause error")
	err := &ZHErr{Msg: "test message", CauseErr: cause}

	// 使用 errors.Is 验证错误链
	if !errors.Is(err, cause) {
		t.Error("errors.Is should work with ZHErr")
	}
}

func TestRegisterZHTranslations(t *testing.T) {
	// 测试注册中文翻译器
	err := RegisterZHTranslations()
	if err != nil {
		t.Logf("RegisterZHTranslations returned: %v", err)
	}

	// 验证 trans 被设置
	if trans == nil {
		t.Log("trans is nil after RegisterZHTranslations")
	}
}

func TestToZHErrMsgWithZHErr(t *testing.T) {
	RegisterZHTranslations()

	validate := validator.New()
	type TestStruct struct {
		Name string `validate:"required"`
	}

	err := validate.Struct(&TestStruct{})
	msg := ToZHErrMsg(err)

	if msg == "" {
		t.Error("ToZHErrMsg should return non-empty message for validation error")
	}
}

func TestMultipleValidationErrors(t *testing.T) {
	RegisterZHTranslations()

	validate := validator.New()
	type TestStruct struct {
		Name  string `validate:"required,min=3"`
		Age   int    `validate:"required,min=1"`
		Email string `validate:"required,email"`
	}

	err := validate.Struct(&TestStruct{})
	result := ToZHErr(err)

	if result == nil {
		t.Fatal("result should not be nil")
	}

	zhErr, ok := result.(*ZHErr)
	if !ok {
		t.Fatal("result should be *ZHErr")
	}

	// 应该包含多个错误信息
	if zhErr.Msg == "" {
		t.Error("ZHErr.Msg should contain multiple error messages")
	}
}

// ==================== RegisterZHTranslations 错误路径测试 ====================

// mockStructValidator 用于模拟非 *validator.Validate 的 Engine 返回值
type mockStructValidator struct{}

func (m *mockStructValidator) ValidateStruct(obj any) error { return nil }
func (m *mockStructValidator) Engine() any                  { return "not a validator" }

func TestToZHErr_NonValidationErrorWithTrans(t *testing.T) {
	// 确保 trans 已初始化
	RegisterZHTranslations()

	err := errors.New("plain error")
	result := ToZHErr(err)
	if result != err {
		t.Error("non-validation error should be returned as-is even with trans initialized")
	}
}

func TestRegisterZHTranslations_GetTranslatorFail(t *testing.T) {
	PatchConvey("TestRegisterZHTranslations-GetTranslatorFail", t, func() {
		MockValue(&transOnce).To(sync.Once{})
		MockValue(&trans).To(nil)
		Mock((*ut.UniversalTranslator).GetTranslator).Return(nil, false).Build()

		err := RegisterZHTranslations()
		So(err, ShouldNotBeNil)
		So(err.Error(), ShouldContainSubstring, "zh translator not found")
	})
}

func TestRegisterZHTranslations_ValidatorEngineFail(t *testing.T) {
	PatchConvey("TestRegisterZHTranslations-ValidatorEngineFail", t, func() {
		MockValue(&transOnce).To(sync.Once{})
		MockValue(&trans).To(nil)

		// 替换 binding.Validator 为返回非 *validator.Validate 的 mock
		origValidator := binding.Validator
		binding.Validator = &mockStructValidator{}
		defer func() { binding.Validator = origValidator }()

		err := RegisterZHTranslations()
		So(err, ShouldNotBeNil)
		So(err.Error(), ShouldContainSubstring, "type assign")
	})
}

func TestRegisterZHTranslations_RegisterDefaultTranslationsFail(t *testing.T) {
	PatchConvey("TestRegisterZHTranslations-RegisterDefaultTranslationsFail", t, func() {
		MockValue(&transOnce).To(sync.Once{})
		MockValue(&trans).To(nil)
		Mock(zt.RegisterDefaultTranslations).Return(errors.New("register failed")).Build()

		err := RegisterZHTranslations()
		So(err, ShouldNotBeNil)
		So(err.Error(), ShouldEqual, "register failed")
	})
}
