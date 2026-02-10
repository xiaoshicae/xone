package single

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/xiaoshicae/xone/xgorm"
	"github.com/xiaoshicae/xone/xserver"
	"github.com/xiaoshicae/xone/xutil"
	"gorm.io/gorm"

	. "github.com/bytedance/mockey"
	. "github.com/smartystreets/goconvey/convey"
)

// User 用户表模型
type User struct {
	ID        uint   `gorm:"primaryKey"`
	Name      string `gorm:"size:100"`
	Email     string `gorm:"size:100;uniqueIndex"`
	Age       int
	CreatedAt time.Time `gorm:"column:created_at"`
}

func (User) TableName() string {
	return "users"
}

func TestXGormSingleClient(t *testing.T) {
	t.Skip("真实环境测试，需要先启动本地 PostgreSQL，可注释掉该 Skip 进行测试")

	PatchConvey("TestXGormSingleClient", t, func() {
		// 初始化 XOne
		err := xserver.R()
		So(err, ShouldBeNil)

		ctx := context.Background()

		// 测试查询所有用户
		PatchConvey("查询所有用户", func() {
			var users []User
			err = xgorm.CWithCtx(ctx).Find(&users).Error
			So(err, ShouldBeNil)
			So(len(users), ShouldBeGreaterThan, 0)
			t.Log("users:", xutil.ToJsonString(users))
		})

		// 测试按ID查询
		PatchConvey("按ID查询用户", func() {
			var user User
			err = xgorm.CWithCtx(ctx).First(&user, 1).Error
			So(err, ShouldBeNil)
			So(user.ID, ShouldEqual, 1)
			t.Log("user:", xutil.ToJsonString(user))
		})

		// 测试条件查询
		PatchConvey("条件查询用户", func() {
			var users []User
			err = xgorm.CWithCtx(ctx).Where("age > ?", 25).Find(&users).Error
			So(err, ShouldBeNil)
			t.Log("users with age > 25:", xutil.ToJsonString(users))
		})

		// 测试创建用户
		PatchConvey("创建用户", func() {
			newUser := User{
				Name:  "测试用户",
				Email: "test_" + time.Now().Format("20060102150405") + "@example.com",
				Age:   20,
			}
			err = xgorm.CWithCtx(ctx).Create(&newUser).Error
			So(err, ShouldBeNil)
			So(newUser.ID, ShouldBeGreaterThan, 0)
			t.Log("created user:", xutil.ToJsonString(newUser))
		})

		// 测试更新用户
		PatchConvey("更新用户", func() {
			err = xgorm.CWithCtx(ctx).Model(&User{}).Where("id = ?", 1).Update("age", 26).Error
			So(err, ShouldBeNil)
		})

		// 测试记录不存在
		PatchConvey("查询不存在的记录", func() {
			var user User
			err = xgorm.CWithCtx(ctx).First(&user, 99999).Error
			So(err, ShouldNotBeNil)
			So(errors.Is(err, gorm.ErrRecordNotFound), ShouldBeTrue)
			t.Log("expected error:", err)
		})

		// 测试事务
		PatchConvey("事务测试", func() {
			err = xgorm.CWithCtx(ctx).Transaction(func(tx *gorm.DB) error {
				user := User{
					Name:  "事务测试用户2",
					Email: "tx_" + time.Now().Format("20060102150405") + "@example.com",
					Age:   30,
				}
				if err := tx.Create(&user).Error; err != nil {
					return err
				}
				t.Log("transaction created user:", xutil.ToJsonString(user))
				return errors.New("for test")
			})
			So(err, ShouldNotBeNil)
		})
	})
}
