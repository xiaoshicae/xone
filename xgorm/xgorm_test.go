package xgorm

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/xiaoshicae/xone/v2/xconfig"
	"github.com/xiaoshicae/xone/v2/xtrace"
	"github.com/xiaoshicae/xone/v2/xutil"

	"gorm.io/gorm"

	. "github.com/bytedance/mockey"
	// goconvey 使用别名导入，避免 convey.C 类型与 xgorm.C() 函数命名冲突
	c "github.com/smartystreets/goconvey/convey"
)

// ==================== config.go ====================

func TestConfigMergeDefault(t *testing.T) {
	PatchConvey("TestConfigMergeDefault", t, func() {
		PatchConvey("Nil-DefaultPostgres", func() {
			// 默认驱动为 postgres，MySQL 子块保持零值（不补 MySQL 专属默认值）
			config := configMergeDefault(nil)
			c.So(config, c.ShouldResemble, &Config{
				Driver:        "postgres",
				DialTimeout:   "500ms",
				MaxOpenConns:  50,
				MaxIdleConns:  xutil.ToPtr(50),
				MaxLifetime:   "5m",
				MaxIdleTime:   "5m",
				SlowThreshold: "3s",
				EnableMetric:  xutil.ToPtr(true),
			})
		})

		PatchConvey("MySQL-DefaultsApplied", func() {
			// 指定 mysql 驱动时才补 MySQL 子块默认值
			config := configMergeDefault(&Config{Driver: "mysql"})
			c.So(config.MySQL.ReadTimeout, c.ShouldEqual, "3s")
			c.So(config.MySQL.WriteTimeout, c.ShouldEqual, "5s")
		})

		PatchConvey("Postgres-NoMySQLDefaults", func() {
			// 显式 postgres 驱动时 MySQL 子块保持空
			config := configMergeDefault(&Config{Driver: "postgres"})
			c.So(config.MySQL.ReadTimeout, c.ShouldEqual, "")
			c.So(config.MySQL.WriteTimeout, c.ShouldEqual, "")
		})

		PatchConvey("ExistingValues", func() {
			config := configMergeDefault(&Config{
				Driver:      "mysql",
				DSN:         "test",
				DialTimeout: "1s",
				MySQL: MySQLOptions{
					ReadTimeout:  "2s",
					WriteTimeout: "3s",
				},
				MaxOpenConns:  10,
				MaxIdleConns:  xutil.ToPtr(5),
				MaxLifetime:   "10m",
				MaxIdleTime:   "8m",
				SlowThreshold: "5s",
				EnableLog:     true,
				Name:          "db1",
			})
			c.So(config.Driver, c.ShouldEqual, "mysql")
			c.So(config.MaxOpenConns, c.ShouldEqual, 10)
			c.So(config.maxIdleConns(), c.ShouldEqual, 5)
			c.So(config.MySQL.ReadTimeout, c.ShouldEqual, "2s")
			c.So(config.MySQL.WriteTimeout, c.ShouldEqual, "3s")
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

		PatchConvey("PingErr-连接池被关闭", func() {
			// 失败路径必须关掉连接池，否则每次建连失败泄漏一个常驻协程
			closed := 0
			Mock((*sql.DB).Close).To(func(_ *sql.DB) error {
				closed++
				return nil
			}).Build()
			Mock((*sql.DB).PingContext).Return(errors.New("ping err")).Build()

			_, err := newClient(&Config{})
			c.So(err, c.ShouldNotBeNil)
			c.So(err.Error(), c.ShouldContainSubstring, "db.PingContext failed")
			c.So(closed, c.ShouldEqual, 1)
		})

		PatchConvey("TracingErr-连接池被关闭", func() {
			closed := 0
			Mock((*sql.DB).Close).To(func(_ *sql.DB) error {
				closed++
				return nil
			}).Build()
			Mock((*sql.DB).PingContext).Return(nil).Build()
			Mock(xtrace.TraceEnabled).Return(true).Build()
			Mock((*gorm.DB).Use).Return(errors.New("use err")).Build()

			_, err := newClient(&Config{})
			c.So(err, c.ShouldNotBeNil)
			c.So(err.Error(), c.ShouldContainSubstring, "use tracing.NewPlugin failed")
			c.So(closed, c.ShouldEqual, 1)
		})

		PatchConvey("Success-连接池不被关闭", func() {
			closed := 0
			Mock((*sql.DB).Close).To(func(_ *sql.DB) error {
				closed++
				return nil
			}).Build()
			Mock((*sql.DB).PingContext).Return(nil).Build()
			Mock((*gorm.DB).Use).Return(nil).Build()

			_, err := newClient(&Config{})
			c.So(err, c.ShouldBeNil)
			c.So(closed, c.ShouldEqual, 0)
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
				DSN:         "root:pass@tcp(127.0.0.1:3306)/testdb",
				DialTimeout: "1s",
				MySQL: MySQLOptions{
					ReadTimeout:  "2s",
					WriteTimeout: "3s",
				},
			})
			c.So(err, c.ShouldBeNil)
			c.So(dsn, c.ShouldContainSubstring, "timeout=1s")
			c.So(dsn, c.ShouldContainSubstring, "readTimeout=2s")
			c.So(dsn, c.ShouldContainSubstring, "writeTimeout=3s")
		})

		PatchConvey("DSN-ExplicitWins", func() {
			// DSN 中已有 timeout=10s，configMergeDefault 的 DialTimeout 不应覆盖
			dsn, err := resolveMySQLDSN(&Config{
				DSN:         "root:pass@tcp(127.0.0.1:3306)/testdb?timeout=10s&readTimeout=20s",
				DialTimeout: "1s",
				MySQL: MySQLOptions{
					ReadTimeout:  "2s",
					WriteTimeout: "3s",
				},
			})
			c.So(err, c.ShouldBeNil)
			c.So(dsn, c.ShouldContainSubstring, "timeout=10s")
			c.So(dsn, c.ShouldContainSubstring, "readTimeout=20s")
			c.So(dsn, c.ShouldContainSubstring, "writeTimeout=3s")
			c.So(dsn, c.ShouldNotContainSubstring, "timeout=1s")
		})
	})
}

// ==================== resolvePostgresDSN ====================

func TestResolvePostgresDSN(t *testing.T) {
	PatchConvey("TestResolvePostgresDSN", t, func() {
		PatchConvey("EmptyInject-Passthrough", func() {
			// DialTimeout 为空且 Postgres 配置为空时 DSN 不变
			dsn, err := resolvePostgresDSN(&Config{DSN: "host=localhost"})
			c.So(err, c.ShouldBeNil)
			c.So(dsn, c.ShouldEqual, "host=localhost")
		})

		PatchConvey("KVFormat-AllFieldsInjected", func() {
			dsn, err := resolvePostgresDSN(&Config{
				DSN:         "host=localhost user=test dbname=testdb",
				DialTimeout: "2s",
				Postgres: PostgresOptions{
					StatementTimeout: "5s",
					LockTimeout:      "3s",
					IdleInTxTimeout:  "60s",
				},
			})
			c.So(err, c.ShouldBeNil)
			c.So(dsn, c.ShouldContainSubstring, "connect_timeout=2")
			c.So(dsn, c.ShouldContainSubstring, "statement_timeout=5000")
			c.So(dsn, c.ShouldContainSubstring, "lock_timeout=3000")
			c.So(dsn, c.ShouldContainSubstring, "idle_in_transaction_session_timeout=60000")
		})

		PatchConvey("KVFormat-ExplicitKeyPreserved", func() {
			// DSN 中已有 connect_timeout=5，配置的 DialTimeout 不应覆盖
			dsn, err := resolvePostgresDSN(&Config{
				DSN:         "host=localhost connect_timeout=5",
				DialTimeout: "2s",
				Postgres: PostgresOptions{
					StatementTimeout: "1s",
				},
			})
			c.So(err, c.ShouldBeNil)
			c.So(dsn, c.ShouldContainSubstring, "connect_timeout=5")
			c.So(dsn, c.ShouldNotContainSubstring, "connect_timeout=2")
			c.So(dsn, c.ShouldContainSubstring, "statement_timeout=1000")
		})

		PatchConvey("URLFormat-AllFieldsInjected", func() {
			dsn, err := resolvePostgresDSN(&Config{
				DSN:         "postgres://root:pass@localhost:5432/testdb?sslmode=disable",
				DialTimeout: "3s",
				Postgres: PostgresOptions{
					StatementTimeout: "8s",
				},
			})
			c.So(err, c.ShouldBeNil)
			c.So(dsn, c.ShouldContainSubstring, "sslmode=disable")
			c.So(dsn, c.ShouldContainSubstring, "connect_timeout=3")
			c.So(dsn, c.ShouldContainSubstring, "statement_timeout=8000")
		})

		PatchConvey("URLFormat-PostgresqlPrefix", func() {
			dsn, err := resolvePostgresDSN(&Config{
				DSN:         "postgresql://user:pw@host:5432/db",
				DialTimeout: "1s",
			})
			c.So(err, c.ShouldBeNil)
			c.So(dsn, c.ShouldContainSubstring, "connect_timeout=1")
		})

		PatchConvey("URLFormat-ExplicitKeyPreserved", func() {
			dsn, err := resolvePostgresDSN(&Config{
				DSN:         "postgres://u:p@h:5432/d?connect_timeout=9",
				DialTimeout: "2s",
			})
			c.So(err, c.ShouldBeNil)
			c.So(dsn, c.ShouldContainSubstring, "connect_timeout=9")
			c.So(dsn, c.ShouldNotContainSubstring, "connect_timeout=2")
		})

		PatchConvey("ParamsPassthrough", func() {
			dsn, err := resolvePostgresDSN(&Config{
				DSN:         "host=localhost",
				DialTimeout: "1s",
				Postgres: PostgresOptions{
					Params: map[string]string{
						"application_name":                 "my-svc",
						"client_connection_check_interval": "10000",
					},
				},
			})
			c.So(err, c.ShouldBeNil)
			c.So(dsn, c.ShouldContainSubstring, "application_name=my-svc")
			c.So(dsn, c.ShouldContainSubstring, "client_connection_check_interval=10000")
		})

		PatchConvey("ParamsOverrideFieldValue", func() {
			// Params 同 key 时优先级高于字段默认
			dsn, err := resolvePostgresDSN(&Config{
				DSN:         "host=localhost",
				DialTimeout: "2s",
				Postgres: PostgresOptions{
					StatementTimeout: "3s",
					Params: map[string]string{
						"statement_timeout": "9999",
					},
				},
			})
			c.So(err, c.ShouldBeNil)
			c.So(dsn, c.ShouldContainSubstring, "statement_timeout=9999")
			c.So(dsn, c.ShouldNotContainSubstring, "statement_timeout=3000")
		})

		PatchConvey("SubSecondDialTimeoutRoundsUp", func() {
			// 500ms 应向上取整为 1 秒
			dsn, err := resolvePostgresDSN(&Config{
				DSN:         "host=localhost",
				DialTimeout: "500ms",
			})
			c.So(err, c.ShouldBeNil)
			c.So(dsn, c.ShouldContainSubstring, "connect_timeout=1")
		})

		PatchConvey("URLInvalid", func() {
			// url.Parse 对于含控制字符的字符串会返回错误
			_, err := resolvePostgresDSN(&Config{
				DSN:         "postgres://bad\x00dsn",
				DialTimeout: "1s",
			})
			c.So(err, c.ShouldNotBeNil)
		})
	})
}

func TestInjectPostgresKV(t *testing.T) {
	PatchConvey("TestInjectPostgresKV", t, func() {
		PatchConvey("AppendWithLeadingSpace", func() {
			// 原 DSN 末尾无空格，追加时应先补空格
			got := injectPostgresKV("host=localhost", map[string]string{"connect_timeout": "2"})
			c.So(got, c.ShouldEqual, "host=localhost connect_timeout=2")
		})

		PatchConvey("SortedOutput", func() {
			got := injectPostgresKV("host=h", map[string]string{
				"z_last":  "1",
				"a_first": "2",
			})
			c.So(got, c.ShouldEqual, "host=h a_first=2 z_last=1")
		})

		PatchConvey("ValueQuoting", func() {
			got := injectPostgresKV("host=h", map[string]string{
				"application_name": "my svc",
			})
			c.So(got, c.ShouldContainSubstring, "application_name='my svc'")
		})
	})
}

func TestQuotePostgresKVValue(t *testing.T) {
	PatchConvey("TestQuotePostgresKVValue", t, func() {
		c.So(quotePostgresKVValue(""), c.ShouldEqual, "''")
		c.So(quotePostgresKVValue("plain"), c.ShouldEqual, "plain")
		c.So(quotePostgresKVValue("has space"), c.ShouldEqual, "'has space'")
		c.So(quotePostgresKVValue(`back\slash`), c.ShouldEqual, `'back\\slash'`)
		c.So(quotePostgresKVValue(`a'b`), c.ShouldEqual, `'a\'b'`)
	})
}

func TestDurationConversions(t *testing.T) {
	PatchConvey("TestDurationConversions", t, func() {
		PatchConvey("ToSeconds", func() {
			c.So(durationToSeconds(""), c.ShouldEqual, "")
			c.So(durationToSeconds("0s"), c.ShouldEqual, "")
			c.So(durationToSeconds("1s"), c.ShouldEqual, "1")
			c.So(durationToSeconds("500ms"), c.ShouldEqual, "1") // 向上取整
			c.So(durationToSeconds("2500ms"), c.ShouldEqual, "3")
			c.So(durationToSeconds("2s"), c.ShouldEqual, "2")
		})

		PatchConvey("ToMillis", func() {
			c.So(durationToMillis(""), c.ShouldEqual, "")
			c.So(durationToMillis("0s"), c.ShouldEqual, "")
			c.So(durationToMillis("1s"), c.ShouldEqual, "1000")
			c.So(durationToMillis("500ms"), c.ShouldEqual, "500")
			c.So(durationToMillis("1m"), c.ShouldEqual, "60000")
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

func TestSanitizeDSN_Leaks(t *testing.T) {
	PatchConvey("TestSanitizeDSN-不得泄漏凭证", t, func() {
		cases := []struct {
			name   string
			dsn    string
			secret string
		}{
			{"MySQL", "user:pass@tcp(127.0.0.1:3306)/db", "pass"},
			{"URL", "postgres://user:pass@host:5432/db", "pass"},
			{"KV", "host=127.0.0.1 user=app password=s3cret dbname=app", "s3cret"},
			// application_name 是 PG 常规字段，值里带 @ 很自然；
			// 按「有没有 @」判断格式会让整串 DSN 连同 password 原样进日志
			{"KV含@", "host=127.0.0.1 user=app password=s3cret application_name=svc@cluster", "s3cret"},
			// 密码含 @ 时，按第一个 @ 切分会把后半段留在日志里
			{"密码含@", "user:p@ssw0rd@tcp(127.0.0.1:3306)/db", "ssw0rd"},
			// 引号内的空格会让「找下一个空格」的实现提前截断
			{"引号包裹的密码", "host=127.0.0.1 password='my secret' dbname=app", "secret"},
			// 同名 key 出现两次是合法的，后者生效，只遮第一个等于没遮
			{"重复password", "host=h password=a user=u password=bxyz", "bxyz"},
			{"URL中的sslpassword", "postgres://u:p@h:5432/db?sslpassword=xyzsecret", "xyzsecret"},
		}
		for _, tc := range cases {
			PatchConvey(tc.name, func() {
				out := sanitizeDSN(tc.dsn)
				c.So(out, c.ShouldNotContainSubstring, tc.secret)
				c.So(out, c.ShouldContainSubstring, dsnMask)
			})
		}

		PatchConvey("无凭证的 DSN 原样保留", func() {
			c.So(sanitizeDSN("host=127.0.0.1 user=app dbname=app"), c.ShouldEqual, "host=127.0.0.1 user=app dbname=app")
			c.So(sanitizeDSN("user@tcp(127.0.0.1:3306)/db"), c.ShouldEqual, "user@tcp(127.0.0.1:3306)/db")
		})

		PatchConvey("URL 解析失败时整串遮掉", func() {
			c.So(sanitizeDSN("postgres://user:pa ss@ho st:5432/db"), c.ShouldEqual, dsnRedacted)
		})
	})
}

func TestRemoveClients(t *testing.T) {
	PatchConvey("TestRemoveClients", t, func() {
		// 初始化失败回滚时，已关闭的 client 必须从 clientMap 中摘除，
		// 否则回滚到 BeforeStop 执行之间 C() 会返回已关闭的 client
		clientMu.Lock()
		clientMap = make(map[string]*gorm.DB)
		clientMu.Unlock()

		set("a", &gorm.DB{})
		set("b", &gorm.DB{})
		setDefault(&gorm.DB{})

		removeClients([]*Config{{Name: "a"}, {Name: "b"}})

		clientMu.RLock()
		size := len(clientMap)
		clientMu.RUnlock()
		c.So(size, c.ShouldEqual, 0)
	})
}

func TestPingTimeout(t *testing.T) {
	PatchConvey("TestPingTimeout", t, func() {
		PatchConvey("MySQL 含读超时而非只用建连超时", func() {
			// Ping 的耗时是建连加一个往返，只给建连预算会让连接刚建成就判超时
			cfg := configMergeDefault(&Config{Driver: "mysql", DialTimeout: "500ms"})
			c.So(pingTimeout(cfg), c.ShouldEqual, 500*time.Millisecond+3*time.Second)
		})

		PatchConvey("Postgres 用建连超时", func() {
			cfg := configMergeDefault(&Config{Driver: "postgres", DialTimeout: "800ms"})
			c.So(pingTimeout(cfg), c.ShouldEqual, 800*time.Millisecond)
		})

		PatchConvey("无法推算时用兜底值", func() {
			c.So(pingTimeout(&Config{}), c.ShouldEqual, defaultPingTimeout)
		})

		PatchConvey("总预算覆盖整轮重试", func() {
			c.So(pingTotalBudget(&Config{Driver: "postgres", DialTimeout: "1s"}), c.ShouldEqual, 3*time.Second+2*time.Second)
		})
	})
}

func TestMaxIdleConnsPointer(t *testing.T) {
	PatchConvey("TestMaxIdleConnsPointer", t, func() {
		PatchConvey("未配置时取 MaxOpenConns", func() {
			cfg := configMergeDefault(&Config{MaxOpenConns: 20})
			c.So(cfg.maxIdleConns(), c.ShouldEqual, 20)
		})

		PatchConvey("显式配 0 表示不保留空闲连接", func() {
			// 用值类型时 0 与「未配置」无法区分，会被悄悄改成 MaxOpenConns
			cfg := configMergeDefault(&Config{MaxOpenConns: 20, MaxIdleConns: xutil.ToPtr(0)})
			c.So(cfg.maxIdleConns(), c.ShouldEqual, 0)
		})
	})
}

func TestPoolCollector(t *testing.T) {
	PatchConvey("TestPoolCollector", t, func() {
		PatchConvey("导出各连接池的实时状态", func() {
			col := newPoolCollector()
			col.statsFor = func() map[string]sql.DBStats {
				return map[string]sql.DBStats{
					"primary": {OpenConnections: 7, InUse: 3, Idle: 4, MaxOpenConnections: 50, WaitCount: 2, WaitDuration: 1500 * time.Millisecond},
				}
			}

			reg := prometheus.NewRegistry()
			c.So(reg.Register(col), c.ShouldBeNil)
			got, err := reg.Gather()
			c.So(err, c.ShouldBeNil)

			values := map[string]float64{}
			for _, f := range got {
				for _, m := range f.Metric {
					if m.Gauge != nil {
						values[f.GetName()] = m.Gauge.GetValue()
					}
					if m.Counter != nil {
						values[f.GetName()] = m.Counter.GetValue()
					}
				}
			}
			// 用字面量而非常量：指标名是使用者看板和告警依赖的对外契约，
			// 跟着常量一起改名的话测试就发现不了这种破坏性变更
			c.So(values["db_connections_open"], c.ShouldEqual, 7)
			c.So(values["db_connections_in_use"], c.ShouldEqual, 3)
			c.So(values["db_connections_idle"], c.ShouldEqual, 4)
			c.So(values["db_connections_max_open"], c.ShouldEqual, 50)
			c.So(values["db_connections_wait_total"], c.ShouldEqual, 2)
			// 耗时按秒记录，与 Prometheus 基准单位约定一致
			c.So(values["db_connections_wait_duration_seconds_total"], c.ShouldEqual, 1.5)
		})

		PatchConvey("Describe 覆盖全部指标", func() {
			ch := make(chan *prometheus.Desc, 16)
			newPoolCollector().Describe(ch)
			close(ch)
			n := 0
			for range ch {
				n++
			}
			c.So(n, c.ShouldEqual, len(poolMetrics))
		})
	})
}

func TestCollectPoolStats(t *testing.T) {
	PatchConvey("TestCollectPoolStats", t, func() {
		Mock((*gorm.DB).DB).Return(&sql.DB{}, nil).Build()
		Mock((*sql.DB).Stats).Return(sql.DBStats{OpenConnections: 1}).Build()

		PatchConvey("default 是具名 client 的别名，不重复导出", func() {
			// 不跳过会让同一个池子的指标出现两份、总量翻倍
			db := &gorm.DB{}
			clientMu.Lock()
			clientMap = map[string]*gorm.DB{defaultClientName: db, "primary": db}
			clientMu.Unlock()

			stats := collectPoolStats()
			c.So(len(stats), c.ShouldEqual, 1)
			_, ok := stats["primary"]
			c.So(ok, c.ShouldBeTrue)
		})

		PatchConvey("单 client 场景用 default 兜底", func() {
			clientMu.Lock()
			clientMap = map[string]*gorm.DB{defaultClientName: &gorm.DB{}}
			clientMu.Unlock()

			stats := collectPoolStats()
			c.So(len(stats), c.ShouldEqual, 1)
			_, ok := stats[defaultClientName]
			c.So(ok, c.ShouldBeTrue)
		})

		PatchConvey("无 client 时返回空", func() {
			clientMu.Lock()
			clientMap = map[string]*gorm.DB{}
			clientMu.Unlock()
			c.So(collectPoolStats(), c.ShouldBeEmpty)
		})
	})
}

func TestMetricEnabled(t *testing.T) {
	PatchConvey("TestMetricEnabled", t, func() {
		c.So((&Config{}).metricEnabled(), c.ShouldBeTrue)
		c.So((&Config{EnableMetric: xutil.ToPtr(false)}).metricEnabled(), c.ShouldBeFalse)
		c.So(configMergeDefault(nil).metricEnabled(), c.ShouldBeTrue)
	})
}

func TestNewClientErrorPaths(t *testing.T) {
	PatchConvey("TestNewClientErrorPaths", t, func() {
		PatchConvey("resolveDialector 失败", func() {
			_, err := newClient(&Config{Driver: "sqlite", DSN: "x.db"})
			c.So(err, c.ShouldNotBeNil)
			c.So(err.Error(), c.ShouldContainSubstring, "resolveDialector failed")
		})

		PatchConvey("gorm.Open 失败时也关连接池", func() {
			// gorm.Open 内部已经建好连接池才做自动 ping，失败时它不会关掉那个池子
			closed := 0
			Mock(resolveDialector).Return(nil, nil).Build()
			Mock(gorm.Open).Return(&gorm.DB{}, errors.New("open failed")).Build()
			Mock((*gorm.DB).DB).Return(&sql.DB{}, nil).Build()
			Mock((*sql.DB).Close).To(func(_ *sql.DB) error { closed++; return nil }).Build()

			_, err := newClient(&Config{})
			c.So(err, c.ShouldNotBeNil)
			c.So(err.Error(), c.ShouldContainSubstring, "gorm.Open failed")
			c.So(closed, c.ShouldEqual, 1)
		})

		PatchConvey("client.DB 失败", func() {
			Mock(resolveDialector).Return(nil, nil).Build()
			Mock(gorm.Open).Return(&gorm.DB{}, nil).Build()
			Mock((*gorm.DB).DB).Return(nil, errors.New("no db")).Build()

			_, err := newClient(&Config{})
			c.So(err, c.ShouldNotBeNil)
			c.So(err.Error(), c.ShouldContainSubstring, "client.DB failed")
		})

		PatchConvey("EnableLog 时装配 gorm logger", func() {
			var gotCfg *gorm.Config
			Mock(resolveDialector).Return(nil, nil).Build()
			Mock(gorm.Open).To(func(_ gorm.Dialector, cfgs ...gorm.Option) (*gorm.DB, error) {
				if len(cfgs) > 0 {
					gotCfg, _ = cfgs[0].(*gorm.Config)
				}
				return nil, errors.New("stop here")
			}).Build()

			_, err := newClient(&Config{EnableLog: true, SlowThreshold: "1s"})
			c.So(err, c.ShouldNotBeNil)
			c.So(gotCfg, c.ShouldNotBeNil)
			c.So(gotCfg.Logger, c.ShouldNotBeNil)
		})
	})
}

func TestCloseGormDB(t *testing.T) {
	PatchConvey("TestCloseGormDB", t, func() {
		PatchConvey("nil client 安全返回", func() {
			closeGormDB(nil) // 不应 panic
		})

		PatchConvey("取不到底层 db 时安全返回", func() {
			Mock((*gorm.DB).DB).Return(nil, errors.New("no db")).Build()
			closeGormDB(&gorm.DB{})
		})

		PatchConvey("Close 出错只记日志", func() {
			warned := 0
			Mock((*gorm.DB).DB).Return(&sql.DB{}, nil).Build()
			Mock((*sql.DB).Close).Return(errors.New("close failed")).Build()
			Mock(xutil.WarnIfEnableDebug).To(func(_ string, _ ...any) { warned++ }).Build()

			closeGormDB(&gorm.DB{})
			c.So(warned, c.ShouldEqual, 1)
		})
	})
}

func TestResolveDialectorDSNErrors(t *testing.T) {
	PatchConvey("TestResolveDialectorDSNErrors", t, func() {
		PatchConvey("MySQL DSN 解析失败", func() {
			_, err := resolveDialector(&Config{Driver: "mysql", DSN: "!!!not a dsn!!!"})
			c.So(err, c.ShouldNotBeNil)
			c.So(err.Error(), c.ShouldContainSubstring, "resolve mysql dsn failed")
		})

		PatchConvey("Postgres DSN 解析失败", func() {
			// URL 形式但 host 含空格，url.Parse 会报错
			_, err := resolveDialector(&Config{
				Driver:      "postgres",
				DSN:         "postgres://u:p@ho st:5432/db",
				DialTimeout: "1s",
			})
			c.So(err, c.ShouldNotBeNil)
			c.So(err.Error(), c.ShouldContainSubstring, "resolve postgres dsn failed")
		})
	})
}

func TestGetMultiConfigValidation(t *testing.T) {
	PatchConvey("TestGetMultiConfigValidation", t, func() {
		mockConfigs := func(cs []*Config) {
			Mock(xconfig.UnmarshalConfig).To(func(_ string, out any) error {
				*(out.(*[]*Config)) = cs
				return nil
			}).Build()
		}

		PatchConvey("Name 不能是保留名", func() {
			mockConfigs([]*Config{{DSN: "d", Name: defaultClientName}})
			_, err := getMultiConfig()
			c.So(err, c.ShouldNotBeNil)
			c.So(err.Error(), c.ShouldContainSubstring, "reserved name")
		})

		PatchConvey("Name 不能重复", func() {
			mockConfigs([]*Config{{DSN: "d", Name: "a"}, {DSN: "d", Name: "a"}})
			_, err := getMultiConfig()
			c.So(err, c.ShouldNotBeNil)
			c.So(err.Error(), c.ShouldContainSubstring, "duplicated")
		})
	})
}

func TestInitMultiRollback(t *testing.T) {
	PatchConvey("TestInitMultiRollback", t, func() {
		// 第二个 client 失败时，第一个必须被关闭并从 map 中摘除
		clientMu.Lock()
		clientMap = make(map[string]*gorm.DB)
		clientMu.Unlock()

		closed := 0
		Mock(xconfig.ContainKey).Return(true).Build()
		Mock(xutil.IsSlice).Return(true).Build()
		Mock(xutil.InfoIfEnableDebug).Return().Build()
		Mock(getMultiConfig).Return([]*Config{{Name: "a", DSN: "d"}, {Name: "b", DSN: "d"}}, nil).Build()
		Mock((*gorm.DB).DB).Return(&sql.DB{}, nil).Build()
		Mock((*sql.DB).Close).To(func(_ *sql.DB) error { closed++; return nil }).Build()

		calls := 0
		Mock(newClient).To(func(_ *Config) (*gorm.DB, error) {
			calls++
			if calls == 1 {
				return &gorm.DB{}, nil
			}
			return nil, errors.New("second failed")
		}).Build()

		err := initXGorm()
		c.So(err, c.ShouldNotBeNil)
		c.So(err.Error(), c.ShouldContainSubstring, "second failed")
		c.So(closed, c.ShouldEqual, 1)

		clientMu.RLock()
		size := len(clientMap)
		clientMu.RUnlock()
		c.So(size, c.ShouldEqual, 0)
	})
}

func TestSanitizeDSNEdgeCases(t *testing.T) {
	PatchConvey("TestSanitizeDSNEdgeCases", t, func() {
		PatchConvey("scheme 含非法字符不算 URL", func() {
			c.So(isURLDSN("pos tgres://u:p@h/db"), c.ShouldBeFalse)
			c.So(isURLDSN("://h/db"), c.ShouldBeFalse)
			c.So(isURLDSN("postgresql+ssl://u:p@h/db"), c.ShouldBeTrue)
		})

		PatchConvey("孤立 token 原样保留", func() {
			c.So(sanitizeKVDSN("host=h standalone password=x"), c.ShouldEqual, "host=h standalone password=***")
		})

		PatchConvey("末尾是空白时不越界", func() {
			c.So(sanitizeKVDSN("host=h password=x   "), c.ShouldEqual, "host=h password=***   ")
		})

		PatchConvey("引号值含转义反斜杠", func() {
			// 转义的引号不能被当成值的结束，否则后半段密码会漏出去
			out := sanitizeKVDSN(`host=h password='a\'b c' dbname=d`)
			c.So(out, c.ShouldEqual, "host=h password=*** dbname=d")
		})

		PatchConvey("引号未闭合时吃到末尾", func() {
			c.So(skipKVValue("'unterminated", 0), c.ShouldEqual, len("'unterminated"))
		})
	})
}

func TestCollectPoolStatsDBError(t *testing.T) {
	PatchConvey("TestCollectPoolStatsDBError", t, func() {
		// 取不到底层 db 的 client 跳过，不应让整个采集失败
		Mock((*gorm.DB).DB).Return(nil, errors.New("no db")).Build()

		clientMu.Lock()
		clientMap = map[string]*gorm.DB{"broken": {}}
		clientMu.Unlock()

		c.So(collectPoolStats(), c.ShouldBeEmpty)
	})
}
