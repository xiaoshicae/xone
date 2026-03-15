package xgorm

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/xiaoshicae/xone/v2/xconfig"
	"github.com/xiaoshicae/xone/v2/xlog"
	"github.com/xiaoshicae/xone/v2/xutil"

	"gorm.io/gorm"
	"gorm.io/gorm/logger"

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

		PatchConvey("MaxIdleConns-Negative-SetToDefault", func() {
			// MaxIdleConns 为 -1 时被设为默认值（等于 MaxOpenConns）
			config := configMergeDefault(&Config{
				MaxIdleConns: -1,
			})
			// MaxOpenConns 默认 50，MaxIdleConns <= 0 时等于 MaxOpenConns
			c.So(config.MaxIdleConns, c.ShouldEqual, 50)
		})

		PatchConvey("MaxOpenConns-Negative-SetToDefault", func() {
			// MaxOpenConns 为 -1 时被设为默认值 50
			config := configMergeDefault(&Config{
				MaxOpenConns: -1,
			})
			c.So(config.MaxOpenConns, c.ShouldEqual, 50)
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

		PatchConvey("MultiClient-PartialFail-ClientMapNotPolluted", func() {
			// 回滚场景中 clientMap 不被污染：第二个 client 创建失败时，第一个的连接被回滚，clientMap 不被写入
			Mock(xconfig.ContainKey).Return(true).Build()
			Mock(xutil.IsSlice).Return(true).Build()
			Mock(getMultiConfig).Return([]*Config{
				{Name: "n1", DSN: "test1"},
				{Name: "n2", DSN: "test2"},
			}, nil).Build()
			Mock(xutil.InfoIfEnableDebug).Return().Build()

			callCount := 0
			mockDB := &gorm.DB{}
			mockSqlDB := &sql.DB{}
			Mock(newClient).To(func(cfg *Config) (*gorm.DB, error) {
				callCount++
				if callCount == 1 {
					return mockDB, nil
				}
				return nil, errors.New("connect failed")
			}).Build()
			// 回滚时 client.DB() 被调用
			Mock((*gorm.DB).DB).Return(mockSqlDB, nil).Build()
			Mock((*sql.DB).Close).Return(nil).Build()

			// 清空 clientMap
			clientMu.Lock()
			clear(clientMap)
			clientMu.Unlock()

			err := initXGorm()
			c.So(err, c.ShouldNotBeNil)
			c.So(err.Error(), c.ShouldContainSubstring, "newClient failed")

			// 验证 clientMap 未被写入任何 client（延迟写入的效果）
			clientMu.RLock()
			c.So(len(clientMap), c.ShouldEqual, 0)
			clientMu.RUnlock()
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

// ==================== xgorm_logger.go ====================

func TestResolveLoglevel(t *testing.T) {
	PatchConvey("TestResolveLoglevel", t, func() {
		PatchConvey("Info", func() {
			c.So(resolveLoglevel("info"), c.ShouldEqual, logger.Info)
		})

		PatchConvey("Warn", func() {
			c.So(resolveLoglevel("warn"), c.ShouldEqual, logger.Warn)
		})

		PatchConvey("Warning", func() {
			c.So(resolveLoglevel("warning"), c.ShouldEqual, logger.Warn)
		})

		PatchConvey("Error", func() {
			c.So(resolveLoglevel("error"), c.ShouldEqual, logger.Error)
		})

		PatchConvey("Unknown-DefaultInfo", func() {
			c.So(resolveLoglevel("unknown"), c.ShouldEqual, logger.Info)
		})

		PatchConvey("UpperCase", func() {
			c.So(resolveLoglevel("ERROR"), c.ShouldEqual, logger.Error)
		})
	})
}

func TestNewGormLogger(t *testing.T) {
	PatchConvey("TestNewGormLogger", t, func() {
		Mock(xlog.XLogLevel).Return("warn").Build()

		gl := newGormLogger(&Config{
			SlowThreshold:                "5s",
			IgnoreRecordNotFoundErrorLog: true, //nolint:govet
		})
		c.So(gl, c.ShouldNotBeNil)
		c.So(gl.logLevel, c.ShouldEqual, logger.Warn)
		c.So(gl.slowThreshold, c.ShouldEqual, 5*time.Second)
		c.So(gl.ignoreRecordNotFoundError, c.ShouldBeTrue)
	})
}

func TestGormLoggerLogMode(t *testing.T) {
	PatchConvey("TestGormLoggerLogMode", t, func() {
		gl := &gormLogger{logLevel: logger.Info}
		newLogger := gl.LogMode(logger.Error)
		// 返回新实例，不影响原实例
		c.So(newLogger.(*gormLogger).logLevel, c.ShouldEqual, logger.Error)
		c.So(gl.logLevel, c.ShouldEqual, logger.Info)
	})
}

func TestGormLoggerInfoWarnError(t *testing.T) {
	PatchConvey("TestGormLoggerInfoWarnError", t, func() {
		gl := &gormLogger{logLevel: logger.Info}
		ctx := context.Background()

		PatchConvey("Info", func() {
			Mock(xlog.Info).Return().Build()
			gl.Info(ctx, "test %s", "info")
		})

		PatchConvey("Warn", func() {
			Mock(xlog.Warn).Return().Build()
			gl.Warn(ctx, "test %s", "warn")
		})

		PatchConvey("Error", func() {
			Mock(xlog.Error).Return().Build()
			gl.Error(ctx, "test %s", "error")
		})
	})
}

func TestGormLoggerTrace(t *testing.T) {
	PatchConvey("TestGormLoggerTrace", t, func() {
		ctx := context.Background()
		fc := func() (string, int64) {
			return "SELECT * FROM users", 10
		}

		PatchConvey("ErrorPath-WithError", func() {
			errorCalled := false
			Mock(xlog.Error).To(func(ctx context.Context, msg string, args ...any) {
				errorCalled = true
			}).Build()

			gl := &gormLogger{logLevel: logger.Error, slowThreshold: 3 * time.Second}
			gl.Trace(ctx, time.Now(), fc, errors.New("query error"))
			c.So(errorCalled, c.ShouldBeTrue)
		})

		PatchConvey("ErrorPath-RecordNotFound-Ignored", func() {
			errorCalled := false
			Mock(xlog.Error).To(func(ctx context.Context, msg string, args ...any) {
				errorCalled = true
			}).Build()

			gl := &gormLogger{logLevel: logger.Error, slowThreshold: 3 * time.Second, ignoreRecordNotFoundError: true}
			gl.Trace(ctx, time.Now(), fc, gorm.ErrRecordNotFound)
			c.So(errorCalled, c.ShouldBeFalse)
		})

		PatchConvey("SlowQueryPath", func() {
			warnCalled := false
			Mock(xlog.Warn).To(func(ctx context.Context, msg string, args ...any) {
				warnCalled = true
			}).Build()

			gl := &gormLogger{logLevel: logger.Warn, slowThreshold: 1 * time.Millisecond}
			// begin 设为过去，确保 cost > slowThreshold
			gl.Trace(ctx, time.Now().Add(-1*time.Second), fc, nil)
			c.So(warnCalled, c.ShouldBeTrue)
		})

		PatchConvey("InfoPath", func() {
			infoCalled := false
			Mock(xlog.Info).To(func(ctx context.Context, msg string, args ...any) {
				infoCalled = true
			}).Build()

			gl := &gormLogger{logLevel: logger.Info, slowThreshold: 1 * time.Hour}
			gl.Trace(ctx, time.Now(), fc, nil)
			c.So(infoCalled, c.ShouldBeTrue)
		})

		PatchConvey("SilentPath-NoLog", func() {
			logCalled := false
			Mock(xlog.Error).To(func(ctx context.Context, msg string, args ...any) {
				logCalled = true
			}).Build()
			Mock(xlog.Warn).To(func(ctx context.Context, msg string, args ...any) {
				logCalled = true
			}).Build()
			Mock(xlog.Info).To(func(ctx context.Context, msg string, args ...any) {
				logCalled = true
			}).Build()

			gl := &gormLogger{logLevel: logger.Silent, slowThreshold: 1 * time.Millisecond}
			gl.Trace(ctx, time.Now().Add(-1*time.Second), fc, errors.New("err"))
			c.So(logCalled, c.ShouldBeFalse)
		})
	})
}

func TestFormatRows(t *testing.T) {
	PatchConvey("TestFormatRows", t, func() {
		PatchConvey("Unknown", func() {
			c.So(formatRows(-1), c.ShouldEqual, "-")
		})

		PatchConvey("Normal", func() {
			c.So(formatRows(10), c.ShouldEqual, int64(10))
		})
	})
}
