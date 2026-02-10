package xgorm

import (
	"database/sql"
	"errors"
	"testing"

	"github.com/xiaoshicae/xone/v2/xconfig"
	"github.com/xiaoshicae/xone/v2/xutil"

	"github.com/bytedance/mockey"
	c "github.com/smartystreets/goconvey/convey"
	"gorm.io/gorm"
)

func TestXGormConfig(t *testing.T) {
	mockey.PatchConvey("TestXGormConfig-configMergeDefault-Nil", t, func() {
		config := configMergeDefault(nil)
		c.So(config, c.ShouldResemble, &Config{
			Driver:                       "postgres",
			DSN:                          "",
			DialTimeout:                  "500ms",
			ReadTimeout:                  "3s",
			WriteTimeout:                 "5s",
			MaxOpenConns:                 50,
			MaxIdleConns:                 50,
			MaxLifetime:                  "5m",
			MaxIdleTime:                  "5m",
			EnableLog:                    false,
			SlowThreshold:                "3s",
			IgnoreRecordNotFoundErrorLog: false,
			Name:                         "",
		})
	})

	mockey.PatchConvey("TestXGormConfig-configMergeDefault-NotNil", t, func() {
		config := &Config{
			Driver:                       "postgres",
			DSN:                          "1",
			DialTimeout:                  "5",
			ReadTimeout:                  "6",
			WriteTimeout:                 "7",
			MaxOpenConns:                 8,
			MaxIdleConns:                 11,
			MaxLifetime:                  "12",
			MaxIdleTime:                  "13",
			EnableLog:                    true,
			SlowThreshold:                "14",
			IgnoreRecordNotFoundErrorLog: false,
			Name:                         "15",
		}
		config = configMergeDefault(config)
		c.So(config, c.ShouldResemble, &Config{
			Driver:                       "postgres",
			DSN:                          "1",
			DialTimeout:                  "5",
			ReadTimeout:                  "6",
			WriteTimeout:                 "7",
			MaxOpenConns:                 8,
			MaxIdleConns:                 11,
			MaxLifetime:                  "12",
			MaxIdleTime:                  "13",
			EnableLog:                    true,
			SlowThreshold:                "14",
			IgnoreRecordNotFoundErrorLog: false,
			Name:                         "15",
		})
	})
}

func TestXGormClient(t *testing.T) {
	mockey.PatchConvey("TestXGormClient-NotFound", t, func() {
		client := C()
		c.So(client, c.ShouldBeNil)
		client = C("x")
		c.So(client, c.ShouldBeNil)
	})

	mockey.PatchConvey("TestXGormClient-Found", t, func() {
		dbX := &gorm.DB{}
		set("x", dbX)

		client := C()
		c.So(client, c.ShouldBeNil)

		client = C("x")
		c.So(client == dbX, c.ShouldBeTrue)

		dbY := &gorm.DB{}
		setDefault(dbY)
		client = C()
		c.So(client == dbY, c.ShouldBeTrue)

		client = C("x", "y")
		c.So(client == dbX, c.ShouldBeTrue)
	})
}

func TestInitXGorm(t *testing.T) {
	mockey.PatchConvey("TestInitXGorm-ConfigKeyNotFound", t, func() {
		mockey.Mock(xconfig.ContainKey).Return(false).Build()
		err := initXGorm()
		c.So(err, c.ShouldBeNil)
	})

	mockey.PatchConvey("TestInitXGorm-MultiClient", t, func() {
		mockey.Mock(xconfig.ContainKey).Return(true).Build()
		mockey.Mock(xutil.IsSlice).Return(true).Build()
		mockey.Mock(getMultiConfig).Return([]*Config{{Name: "n1", DSN: "test"}}, nil).Build()
		mockey.Mock(newClient).Return(&gorm.DB{}, nil).Build()
		err := initXGorm()
		c.So(err, c.ShouldBeNil)
	})

	mockey.PatchConvey("TestInitXGorm-MultiClient-GetConfigErr", t, func() {
		mockey.Mock(xconfig.ContainKey).Return(true).Build()
		mockey.Mock(xutil.IsSlice).Return(true).Build()
		mockey.Mock(getMultiConfig).Return(nil, errors.New("for multi test")).Build()
		err := initXGorm()
		c.So(err, c.ShouldNotBeNil)
		c.So(err.Error(), c.ShouldContainSubstring, "getMultiConfig failed")
	})

	mockey.PatchConvey("TestInitXGorm-MultiClient-NewClientErr", t, func() {
		mockey.Mock(xconfig.ContainKey).Return(true).Build()
		mockey.Mock(xutil.IsSlice).Return(true).Build()
		mockey.Mock(getMultiConfig).Return([]*Config{{Name: "n1", DSN: "test"}}, nil).Build()
		mockey.Mock(newClient).Return(nil, errors.New("for new test")).Build()
		err := initXGorm()
		c.So(err, c.ShouldNotBeNil)
		c.So(err.Error(), c.ShouldContainSubstring, "newClient failed")
	})

	mockey.PatchConvey("TestInitXGorm-SingleClient", t, func() {
		mockey.Mock(xconfig.ContainKey).Return(true).Build()
		mockey.Mock(xutil.IsSlice).Return(false).Build()
		mockey.Mock(getConfig).Return(&Config{DSN: "test"}, nil).Build()
		mockey.Mock(newClient).Return(&gorm.DB{}, nil).Build()
		err := initXGorm()
		c.So(err, c.ShouldBeNil)
	})

	mockey.PatchConvey("TestInitXGorm-SingleClient-GetConfigErr", t, func() {
		mockey.Mock(xconfig.ContainKey).Return(true).Build()
		mockey.Mock(xutil.IsSlice).Return(false).Build()
		mockey.Mock(getConfig).Return(nil, errors.New("for single test")).Build()
		err := initXGorm()
		c.So(err, c.ShouldNotBeNil)
		c.So(err.Error(), c.ShouldContainSubstring, "getConfig failed")
	})

	mockey.PatchConvey("TestInitXGorm-SingleClient-NewClientErr", t, func() {
		mockey.Mock(xconfig.ContainKey).Return(true).Build()
		mockey.Mock(xutil.IsSlice).Return(false).Build()
		mockey.Mock(getConfig).Return(&Config{DSN: "test"}, nil).Build()
		mockey.Mock(newClient).Return(nil, errors.New("for new test")).Build()
		err := initXGorm()
		c.So(err, c.ShouldNotBeNil)
		c.So(err.Error(), c.ShouldContainSubstring, "newClient failed")
	})
}

func TestResolveDialector(t *testing.T) {
	mockey.PatchConvey("TestResolveDialector-NilConfig", t, func() {
		_, err := resolveDialector(nil)
		c.So(err.Error(), c.ShouldEqual, "config can't be empty")
	})

	mockey.PatchConvey("TestResolveDialector-EmptyDSN", t, func() {
		_, err := resolveDialector(&Config{})
		c.So(err.Error(), c.ShouldEqual, "dsn can't be empty")
	})

	mockey.PatchConvey("TestResolveDialector-UnsupportedDriver", t, func() {
		_, err := resolveDialector(&Config{
			Driver: "sqlite",
			DSN:    "test.db",
		})
		c.So(err, c.ShouldNotBeNil)
		c.So(err.Error(), c.ShouldContainSubstring, "unsupported driver")
	})

	mockey.PatchConvey("TestResolveDialector-MySQL-Success", t, func() {
		dialector, err := resolveDialector(&Config{
			Driver: "mysql",
			DSN:    "root:pass@tcp(127.0.0.1:3306)/testdb",
		})
		c.So(err, c.ShouldBeNil)
		c.So(dialector, c.ShouldNotBeNil)
	})

	mockey.PatchConvey("TestResolveDialector-Postgres-Success", t, func() {
		dialector, err := resolveDialector(&Config{
			Driver: "postgres",
			DSN:    "host=localhost user=test password=test dbname=testdb port=5432 sslmode=disable",
		})
		c.So(err, c.ShouldBeNil)
		c.So(dialector, c.ShouldNotBeNil)
	})
}

func TestResolveMySQLDSN(t *testing.T) {
	mockey.PatchConvey("TestResolveMySQLDSN-Invalid", t, func() {
		_, err := resolveMySQLDSN(&Config{DSN: "invalid"})
		c.So(err, c.ShouldNotBeNil)
	})

	mockey.PatchConvey("TestResolveMySQLDSN-Success", t, func() {
		dsn, err := resolveMySQLDSN(&Config{DSN: "/testdb"})
		c.So(err, c.ShouldBeNil)
		c.So(dsn, c.ShouldEqual, "tcp(127.0.0.1:3306)/testdb")

		dsn, err = resolveMySQLDSN(&Config{
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
}

func TestNewClient(t *testing.T) {
	mockey.PatchConvey("TestNewClient", t, func() {
		mockey.Mock(resolveDialector).Return(nil, nil).Build()
		mockey.Mock(gorm.Open).Return(&gorm.DB{}, nil).Build()
		mockey.Mock((*gorm.DB).DB).Return(&sql.DB{}, nil).Build()

		mockey.PatchConvey("TestNewClient-Err", func() {
			mockey.Mock((*sql.DB).PingContext).Return(errors.New("for new test")).Build()
			_, err := newClient(&Config{})
			c.So(err.Error(), c.ShouldContainSubstring, "db.PingContext failed")
		})

		mockey.PatchConvey("TestNewClient-Success", func() {
			mockey.Mock((*sql.DB).PingContext).Return(nil).Build()
			mockey.Mock((*gorm.DB).Use).Return(nil).Build()
			_, err := newClient(&Config{})
			c.So(err, c.ShouldBeNil)
		})
	})
}

func TestGetConfig(t *testing.T) {
	mockey.PatchConvey("TestGetConfig-Err", t, func() {
		mockey.Mock(xconfig.UnmarshalConfig).Return(errors.New("for test")).Build()
		_, err := getConfig()
		c.So(err.Error(), c.ShouldEqual, "for test")
	})

	mockey.PatchConvey("TestGetConfig-DSN-Empty1", t, func() {
		mockey.Mock(xconfig.UnmarshalConfig).Return(nil).Build()
		mockey.Mock(configMergeDefault).Return(&Config{}).Build()
		_, err := getConfig()
		c.So(err.Error(), c.ShouldEqual, "config XGorm.DSN can not be empty")
	})

	mockey.PatchConvey("TestGetConfig-DSN-Empty2", t, func() {
		mockey.Mock(xconfig.UnmarshalConfig).Return(nil).Build()
		mockey.Mock(configMergeDefault).Return(&Config{DSN: ""}).Build()
		_, err := getConfig()
		c.So(err.Error(), c.ShouldEqual, "config XGorm.DSN can not be empty")
	})

	mockey.PatchConvey("TestGetConfig-Success", t, func() {
		mockey.Mock(xconfig.UnmarshalConfig).Return(nil).Build()
		mockey.Mock(configMergeDefault).Return(&Config{DSN: "X"}).Build()
		_, err := getConfig()
		c.So(err, c.ShouldBeNil)
	})
}

func TestGetMultiConfig(t *testing.T) {
	mockey.PatchConvey("TestGetMultiConfig-Err", t, func() {
		mockey.Mock(xconfig.UnmarshalConfig).Return(errors.New("for test")).Build()
		_, err := getMultiConfig()
		c.So(err.Error(), c.ShouldEqual, "for test")
	})

	mockey.PatchConvey("TestGetMultiConfig-Param-Check", t, func() {
		mockey.Mock(xconfig.UnmarshalConfig).To(func(key string, conf interface{}) error {
			v := conf.(*[]*Config)
			*v = []*Config{{}}
			return nil
		}).Build()

		mockey.PatchConvey("TestGetMultiConfig-DSN-Empty1", func() {
			mockey.Mock(configMergeDefault).Return(&Config{}).Build()
			_, err := getMultiConfig()
			c.So(err.Error(), c.ShouldEqual, "multi config XGorm.DSN can not be empty")
		})

		mockey.PatchConvey("TestGetMultiConfig-DSN-Empty2", func() {
			mockey.Mock(configMergeDefault).Return(&Config{}).Build()
			_, err := getMultiConfig()
			c.So(err.Error(), c.ShouldEqual, "multi config XGorm.DSN can not be empty")
		})

		mockey.PatchConvey("TestGetMultiConfig-Name-Empty", func() {
			mockey.Mock(configMergeDefault).Return(&Config{DSN: "X"}).Build()
			_, err := getMultiConfig()
			c.So(err.Error(), c.ShouldEqual, "multi config XGorm.Name can not be empty")
		})

		mockey.PatchConvey("TestGetMultiConfig-Success", func() {
			mockey.Mock(configMergeDefault).Return(&Config{DSN: "X", Name: "n"}).Build()
			_, err := getMultiConfig()
			c.So(err, c.ShouldBeNil)
		})
	})
}

func TestGetDriver(t *testing.T) {
	mockey.PatchConvey("TestGetDriver-Nil", t, func() {
		config := &Config{}
		c.So(config.GetDriver(), c.ShouldEqual, DriverPostgres)
	})

	mockey.PatchConvey("TestGetDriver-MySQL", t, func() {
		config := &Config{Driver: "mysql"}
		c.So(config.GetDriver(), c.ShouldEqual, DriverMySQL)
	})

	mockey.PatchConvey("TestGetDriver-Postgres", t, func() {
		config := &Config{Driver: "postgres"}
		c.So(config.GetDriver(), c.ShouldEqual, DriverPostgres)
	})
}
