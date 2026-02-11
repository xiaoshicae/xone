package xgorm

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/xiaoshicae/xone/v2/xconfig"
	"github.com/xiaoshicae/xone/v2/xutil"

	"gorm.io/gorm"

	. "github.com/bytedance/mockey"
	// goconvey 使用别名导入，避免 convey.C 类型与 xgorm.C() 函数命名冲突
	c "github.com/smartystreets/goconvey/convey"
)

// ==================== config.go ====================

func TestConfigMergeDefault(t *testing.T) {
	PatchConvey("TestConfigMergeDefault", t, func() {
		PatchConvey("Nil", func() {
			config := configMergeDefault(nil)
			c.So(config, c.ShouldResemble, &Config{
				Driver:        "postgres",
				DialTimeout:   "500ms",
				ReadTimeout:   "3s",
				WriteTimeout:  "5s",
				MaxOpenConns:  50,
				MaxIdleConns:  50,
				MaxLifetime:   "5m",
				MaxIdleTime:   "5m",
				SlowThreshold: "3s",
			})
		})

		PatchConvey("ExistingValues", func() {
			config := configMergeDefault(&Config{
				Driver:        "mysql",
				DSN:           "test",
				DialTimeout:   "1s",
				ReadTimeout:   "2s",
				WriteTimeout:  "3s",
				MaxOpenConns:  10,
				MaxIdleConns:  5,
				MaxLifetime:   "10m",
				MaxIdleTime:   "8m",
				SlowThreshold: "5s",
				EnableLog:     true,
				Name:          "db1",
			})
			c.So(config.Driver, c.ShouldEqual, "mysql")
			c.So(config.MaxOpenConns, c.ShouldEqual, 10)
			c.So(config.MaxIdleConns, c.ShouldEqual, 5)
		})
	})
}

func TestGetDriver(t *testing.T) {
	PatchConvey("TestGetDriver", t, func() {
		PatchConvey("Empty", func() {
			c.So((&Config{}).GetDriver(), c.ShouldEqual, DriverPostgres)
		})

		PatchConvey("MySQL", func() {
			c.So((&Config{Driver: "mysql"}).GetDriver(), c.ShouldEqual, DriverMySQL)
		})

		PatchConvey("Postgres", func() {
			c.So((&Config{Driver: "postgres"}).GetDriver(), c.ShouldEqual, DriverPostgres)
		})
	})
}

// ==================== client.go ====================

func TestC(t *testing.T) {
	PatchConvey("TestC", t, func() {
		PatchConvey("NotFound", func() {
			c.So(C(), c.ShouldBeNil)
			c.So(C("x"), c.ShouldBeNil)
		})

		PatchConvey("Found", func() {
			dbX := &gorm.DB{}
			set("x", dbX)

			c.So(C(), c.ShouldBeNil) // 未设置 default
			c.So(C("x") == dbX, c.ShouldBeTrue)

			dbY := &gorm.DB{}
			setDefault(dbY)
			c.So(C() == dbY, c.ShouldBeTrue)

			// 多参数取第一个
			c.So(C("x", "y") == dbX, c.ShouldBeTrue)
		})
	})
}

func TestCWithCtx(t *testing.T) {
	PatchConvey("TestCWithCtx", t, func() {
		PatchConvey("NilClient", func() {
			Mock(C).Return(nil).Build()
			client := CWithCtx(context.Background())
			c.So(client, c.ShouldBeNil)
		})

		PatchConvey("WithClient", func() {
			mockDB := &gorm.DB{}
			Mock(C).Return(mockDB).Build()
			Mock((*gorm.DB).WithContext).Return(mockDB).Build()
			client := CWithCtx(context.Background())
			c.So(client, c.ShouldNotBeNil)
		})
	})
}

// ==================== xgorm_init.go ====================

func TestInitXGorm(t *testing.T) {
	PatchConvey("TestInitXGorm", t, func() {
		PatchConvey("ConfigKeyNotFound", func() {
			Mock(xconfig.ContainKey).Return(false).Build()
			Mock(xutil.WarnIfEnableDebug).Return().Build()
			err := initXGorm()
			c.So(err, c.ShouldBeNil)
		})

		PatchConvey("SingleClient-Success", func() {
			Mock(xconfig.ContainKey).Return(true).Build()
			Mock(xutil.IsSlice).Return(false).Build()
			Mock(getConfig).Return(&Config{DSN: "test"}, nil).Build()
			Mock(newClient).Return(&gorm.DB{}, nil).Build()
			Mock(xutil.InfoIfEnableDebug).Return().Build()
			err := initXGorm()
			c.So(err, c.ShouldBeNil)
		})

		PatchConvey("SingleClient-GetConfigErr", func() {
			Mock(xconfig.ContainKey).Return(true).Build()
			Mock(xutil.IsSlice).Return(false).Build()
			Mock(getConfig).Return(nil, errors.New("cfg err")).Build()
			err := initXGorm()
			c.So(err, c.ShouldNotBeNil)
			c.So(err.Error(), c.ShouldContainSubstring, "getConfig failed")
		})

		PatchConvey("SingleClient-NewClientErr", func() {
			Mock(xconfig.ContainKey).Return(true).Build()
			Mock(xutil.IsSlice).Return(false).Build()
			Mock(getConfig).Return(&Config{DSN: "test"}, nil).Build()
			Mock(newClient).Return(nil, errors.New("new err")).Build()
			Mock(xutil.InfoIfEnableDebug).Return().Build()
			err := initXGorm()
			c.So(err, c.ShouldNotBeNil)
			c.So(err.Error(), c.ShouldContainSubstring, "newClient failed")
		})

		PatchConvey("MultiClient-Success", func() {
			Mock(xconfig.ContainKey).Return(true).Build()
			Mock(xutil.IsSlice).Return(true).Build()
			Mock(getMultiConfig).Return([]*Config{{Name: "n1", DSN: "test"}}, nil).Build()
			Mock(newClient).Return(&gorm.DB{}, nil).Build()
			Mock(xutil.InfoIfEnableDebug).Return().Build()
			err := initXGorm()
			c.So(err, c.ShouldBeNil)
		})

		PatchConvey("MultiClient-GetConfigErr", func() {
			Mock(xconfig.ContainKey).Return(true).Build()
			Mock(xutil.IsSlice).Return(true).Build()
			Mock(getMultiConfig).Return(nil, errors.New("multi err")).Build()
			err := initXGorm()
			c.So(err, c.ShouldNotBeNil)
			c.So(err.Error(), c.ShouldContainSubstring, "getMultiConfig failed")
		})

		PatchConvey("MultiClient-NewClientErr", func() {
			Mock(xconfig.ContainKey).Return(true).Build()
			Mock(xutil.IsSlice).Return(true).Build()
			Mock(getMultiConfig).Return([]*Config{{Name: "n1", DSN: "test"}}, nil).Build()
			Mock(newClient).Return(nil, errors.New("new err")).Build()
			Mock(xutil.InfoIfEnableDebug).Return().Build()
			err := initXGorm()
			c.So(err, c.ShouldNotBeNil)
			c.So(err.Error(), c.ShouldContainSubstring, "newClient failed")
		})
	})
}

func TestCloseXGorm(t *testing.T) {
	PatchConvey("TestCloseXGorm", t, func() {
		PatchConvey("EmptyMap", func() {
			clientMap = make(map[string]*gorm.DB)
			err := closeXGorm()
			c.So(err, c.ShouldBeNil)
			c.So(clientMap, c.ShouldBeEmpty)
		})

		PatchConvey("Success", func() {
			mockDB := &sql.DB{}
			mockGormDB := &gorm.DB{}
			Mock((*gorm.DB).DB).Return(mockDB, nil).Build()
			Mock((*sql.DB).Close).Return(nil).Build()

			clientMap = map[string]*gorm.DB{
				defaultClientName: mockGormDB,
				"named":           mockGormDB, // 同一个 client，测试去重
			}
			err := closeXGorm()
			c.So(err, c.ShouldBeNil)
			c.So(clientMap, c.ShouldBeEmpty)
		})

		PatchConvey("GetDBError", func() {
			mockGormDB := &gorm.DB{}
			Mock((*gorm.DB).DB).Return(nil, errors.New("db err")).Build()

			clientMap = map[string]*gorm.DB{defaultClientName: mockGormDB}
			err := closeXGorm()
			c.So(err, c.ShouldNotBeNil)
			c.So(err.Error(), c.ShouldContainSubstring, "get underlying db failed")
			c.So(clientMap, c.ShouldBeEmpty)
		})

		PatchConvey("CloseError", func() {
			mockDB := &sql.DB{}
			mockGormDB := &gorm.DB{}
			Mock((*gorm.DB).DB).Return(mockDB, nil).Build()
			Mock((*sql.DB).Close).Return(errors.New("close err")).Build()

			clientMap = map[string]*gorm.DB{defaultClientName: mockGormDB}
			err := closeXGorm()
			c.So(err, c.ShouldNotBeNil)
			c.So(err.Error(), c.ShouldContainSubstring, "close db failed")
			c.So(clientMap, c.ShouldBeEmpty)
		})
	})
}

func TestNewClient(t *testing.T) {
	PatchConvey("TestNewClient", t, func() {
		Mock(resolveDialector).Return(nil, nil).Build()
		Mock(gorm.Open).Return(&gorm.DB{}, nil).Build()
		Mock((*gorm.DB).DB).Return(&sql.DB{}, nil).Build()

		PatchConvey("PingErr", func() {
			Mock((*sql.DB).PingContext).Return(errors.New("ping err")).Build()
			_, err := newClient(&Config{})
			c.So(err, c.ShouldNotBeNil)
			c.So(err.Error(), c.ShouldContainSubstring, "db.PingContext failed")
		})

		PatchConvey("Success", func() {
			Mock((*sql.DB).PingContext).Return(nil).Build()
			Mock((*gorm.DB).Use).Return(nil).Build()
			_, err := newClient(&Config{})
			c.So(err, c.ShouldBeNil)
		})
	})
}

func TestResolveDialector(t *testing.T) {
	PatchConvey("TestResolveDialector", t, func() {
		PatchConvey("NilConfig", func() {
			_, err := resolveDialector(nil)
			c.So(err, c.ShouldNotBeNil)
			c.So(err.Error(), c.ShouldContainSubstring, "config can't be empty")
		})

		PatchConvey("EmptyDSN", func() {
			_, err := resolveDialector(&Config{})
			c.So(err, c.ShouldNotBeNil)
			c.So(err.Error(), c.ShouldContainSubstring, "dsn can't be empty")
		})

		PatchConvey("UnsupportedDriver", func() {
			_, err := resolveDialector(&Config{Driver: "sqlite", DSN: "test.db"})
			c.So(err, c.ShouldNotBeNil)
			c.So(err.Error(), c.ShouldContainSubstring, "unsupported driver")
		})

		PatchConvey("MySQL-Success", func() {
			Mock(xutil.InfoIfEnableDebug).Return().Build()
			d, err := resolveDialector(&Config{Driver: "mysql", DSN: "root:pass@tcp(127.0.0.1:3306)/testdb"})
			c.So(err, c.ShouldBeNil)
			c.So(d, c.ShouldNotBeNil)
		})

		PatchConvey("Postgres-Success", func() {
			Mock(xutil.InfoIfEnableDebug).Return().Build()
			d, err := resolveDialector(&Config{Driver: "postgres", DSN: "host=localhost user=test dbname=testdb"})
			c.So(err, c.ShouldBeNil)
			c.So(d, c.ShouldNotBeNil)
		})
	})
}

func TestResolveMySQLDSN(t *testing.T) {
	PatchConvey("TestResolveMySQLDSN", t, func() {
		PatchConvey("InvalidDSN", func() {
			_, err := resolveMySQLDSN(&Config{DSN: "invalid"})
			c.So(err, c.ShouldNotBeNil)
		})

		PatchConvey("Success", func() {
			dsn, err := resolveMySQLDSN(&Config{
				DSN:          "root:pass@tcp(127.0.0.1:3306)/testdb",
				DialTimeout:  "1s",
				ReadTimeout:  "2s",
				WriteTimeout: "3s",
			})
			c.So(err, c.ShouldBeNil)
			c.So(dsn, c.ShouldContainSubstring, "timeout=1s")
			c.So(dsn, c.ShouldContainSubstring, "readTimeout=2s")
			c.So(dsn, c.ShouldContainSubstring, "writeTimeout=3s")
		})
	})
}

func TestGetConfig(t *testing.T) {
	PatchConvey("TestGetConfig", t, func() {
		PatchConvey("UnmarshalErr", func() {
			Mock(xconfig.UnmarshalConfig).Return(errors.New("unmarshal err")).Build()
			_, err := getConfig()
			c.So(err, c.ShouldNotBeNil)
		})

		PatchConvey("DSNEmpty", func() {
			Mock(xconfig.UnmarshalConfig).Return(nil).Build()
			Mock(configMergeDefault).Return(&Config{}).Build()
			_, err := getConfig()
			c.So(err, c.ShouldNotBeNil)
			c.So(err.Error(), c.ShouldContainSubstring, "DSN can not be empty")
		})

		PatchConvey("Success", func() {
			Mock(xconfig.UnmarshalConfig).Return(nil).Build()
			Mock(configMergeDefault).Return(&Config{DSN: "test"}).Build()
			cfg, err := getConfig()
			c.So(err, c.ShouldBeNil)
			c.So(cfg.DSN, c.ShouldEqual, "test")
		})
	})
}

func TestGetMultiConfig(t *testing.T) {
	PatchConvey("TestGetMultiConfig", t, func() {
		PatchConvey("UnmarshalErr", func() {
			Mock(xconfig.UnmarshalConfig).Return(errors.New("unmarshal err")).Build()
			_, err := getMultiConfig()
			c.So(err, c.ShouldNotBeNil)
		})

		PatchConvey("ParamCheck", func() {
			Mock(xconfig.UnmarshalConfig).To(func(key string, conf any) error {
				v := conf.(*[]*Config)
				*v = []*Config{{}}
				return nil
			}).Build()

			PatchConvey("DSNEmpty", func() {
				Mock(configMergeDefault).Return(&Config{}).Build()
				_, err := getMultiConfig()
				c.So(err, c.ShouldNotBeNil)
				c.So(err.Error(), c.ShouldContainSubstring, "DSN can not be empty")
			})

			PatchConvey("NameEmpty", func() {
				Mock(configMergeDefault).Return(&Config{DSN: "test"}).Build()
				_, err := getMultiConfig()
				c.So(err, c.ShouldNotBeNil)
				c.So(err.Error(), c.ShouldContainSubstring, "Name can not be empty")
			})

			PatchConvey("Success", func() {
				Mock(configMergeDefault).Return(&Config{DSN: "test", Name: "n1"}).Build()
				configs, err := getMultiConfig()
				c.So(err, c.ShouldBeNil)
				c.So(configs, c.ShouldHaveLength, 1)
			})
		})
	})
}
