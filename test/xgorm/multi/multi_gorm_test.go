package multi

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/xiaoshicae/xone/v2/xgorm"
	"github.com/xiaoshicae/xone/v2/xserver"
	"github.com/xiaoshicae/xone/v2/xutil"

	. "github.com/bytedance/mockey"
	. "github.com/smartystreets/goconvey/convey"
	"gorm.io/gorm"
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

func TestXGormMultiClient(t *testing.T) {
	t.Skip("真实环境测试，需要先启动本地 PostgreSQL，可注释掉该 Skip 进行测试")

	PatchConvey("TestXGormMultiClient", t, func() {
		// 初始化 XOne
		err := xserver.R()
		So(err, ShouldBeNil)

		ctx := context.Background()

		// 测试 primary 客户端
		PatchConvey("Primary 客户端查询", func() {
			var users []User
			err = xgorm.CWithCtx(ctx, "primary").Find(&users).Error
			So(err, ShouldBeNil)
			So(len(users), ShouldBeGreaterThan, 0)
			t.Log("primary - users:", xutil.ToJsonString(users))
		})

		// 测试 secondary 客户端
		PatchConvey("Secondary 客户端查询", func() {
			var users []User
			err = xgorm.CWithCtx(ctx, "secondary").Find(&users).Error
			So(err, ShouldBeNil)
			So(len(users), ShouldBeGreaterThan, 0)
			t.Log("secondary - users:", xutil.ToJsonString(users))
		})

		// 测试默认客户端（第一个配置的 primary）
		PatchConvey("默认客户端查询", func() {
			var user User
			err = xgorm.CWithCtx(ctx).First(&user, 1).Error
			So(err, ShouldBeNil)
			So(user.ID, ShouldEqual, 1)
			t.Log("default client - user:", xutil.ToJsonString(user))
		})

		// 测试两个客户端独立操作
		PatchConvey("多客户端独立创建", func() {
			timestamp := time.Now().Format("20060102150405")

			// primary 创建
			user1 := User{
				Name:  "Primary用户",
				Email: "primary_" + timestamp + "@example.com",
				Age:   25,
			}
			err = xgorm.CWithCtx(ctx, "primary").Create(&user1).Error
			So(err, ShouldBeNil)
			So(user1.ID, ShouldBeGreaterThan, 0)
			t.Log("primary created:", xutil.ToJsonString(user1))

			// secondary 查询刚创建的用户（同一数据库，应该能查到）
			var user2 User
			err = xgorm.CWithCtx(ctx, "secondary").First(&user2, user1.ID).Error
			So(err, ShouldBeNil)
			So(user2.Name, ShouldEqual, "Primary用户")
			t.Log("secondary found:", xutil.ToJsonString(user2))
		})

		// 测试 IgnoreRecordNotFoundErrorLog 配置差异
		PatchConvey("RecordNotFound 错误处理", func() {
			// primary: IgnoreRecordNotFoundErrorLog=false，会打印错误日志
			var user1 User
			err = xgorm.CWithCtx(ctx, "primary").First(&user1, 99999).Error
			So(err, ShouldNotBeNil)
			So(errors.Is(err, gorm.ErrRecordNotFound), ShouldBeTrue)
			t.Log("primary - expected error (should log):", err)

			// secondary: IgnoreRecordNotFoundErrorLog=true，不会打印错误日志
			var user2 User
			err = xgorm.CWithCtx(ctx, "secondary").First(&user2, 99999).Error
			So(err, ShouldNotBeNil)
			So(errors.Is(err, gorm.ErrRecordNotFound), ShouldBeTrue)
			t.Log("secondary - expected error (should NOT log):", err)
		})

		// 测试事务（在 primary 上）
		PatchConvey("Primary 客户端事务", func() {
			err = xgorm.CWithCtx(ctx, "primary").Transaction(func(tx *gorm.DB) error {
				timestamp := time.Now().Format("20060102150405")
				user := User{
					Name:  "事务测试用户",
					Email: "tx_multi_" + timestamp + "@example.com",
					Age:   30,
				}
				if err := tx.Create(&user).Error; err != nil {
					return err
				}
				t.Log("transaction created:", xutil.ToJsonString(user))
				return nil
			})
			So(err, ShouldBeNil)
		})

		// 测试条件查询
		PatchConvey("条件查询对比", func() {
			// primary 查询年龄 > 25
			var users1 []User
			err = xgorm.CWithCtx(ctx, "primary").Where("age > ?", 25).Find(&users1).Error
			So(err, ShouldBeNil)
			t.Log("primary - age > 25:", len(users1))

			// secondary 查询年龄 <= 25
			var users2 []User
			err = xgorm.CWithCtx(ctx, "secondary").Where("age <= ?", 25).Find(&users2).Error
			So(err, ShouldBeNil)
			t.Log("secondary - age <= 25:", len(users2))
		})
	})
}
