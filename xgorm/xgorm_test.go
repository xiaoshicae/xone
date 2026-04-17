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
		PatchConvey("Nil-DefaultPostgres", func() {
			// 默认驱动为 postgres，MySQL 子块保持零值（不补 MySQL 专属默认值）
			config := configMergeDefault(nil)
			c.So(config, c.ShouldResemble, &Config{
				Driver:        "postgres",
				DialTimeout:   "500ms",
				MaxOpenConns:  50,
				MaxIdleConns:  50,
				MaxLifetime:   "5m",
				MaxIdleTime:   "5m",
				SlowThreshold: "3s",
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
