package multi

import (
	"context"
	"testing"
	"time"

	"github.com/xiaoshicae/xone/v2/xredis"
	"github.com/xiaoshicae/xone/v2/xserver"

	. "github.com/bytedance/mockey"
	. "github.com/smartystreets/goconvey/convey"
)

func TestXRedisMultiClient(t *testing.T) {
	t.Skip("真实环境测试，需要先启动本地 Redis，可注释掉该 Skip 进行测试")

	PatchConvey("TestXRedisMultiClient", t, func() {
		// 初始化 XOne
		err := xserver.R()
		So(err, ShouldBeNil)

		ctx := context.Background()

		// 获取两个不同的 client
		cacheClient := xredis.C("cache")
		sessionClient := xredis.C("session")
		So(cacheClient, ShouldNotBeNil)
		So(sessionClient, ShouldNotBeNil)

		// 默认 client 等于第一个（cache）
		PatchConvey("默认client等于cache", func() {
			defaultClient := xredis.C()
			So(defaultClient, ShouldNotBeNil)

			key := "xone:test:default:" + time.Now().Format("20060102150405")
			defaultClient.Set(ctx, key, "from-default", 10*time.Second)

			// 通过 cache client 也能读到
			val, err := cacheClient.Get(ctx, key).Result()
			So(err, ShouldBeNil)
			So(val, ShouldEqual, "from-default")

			// 清理
			defaultClient.Del(ctx, key)
		})

		// 测试 cache 和 session 独立写入
		PatchConvey("多实例独立读写", func() {
			cacheKey := "xone:test:multi:cache:" + time.Now().Format("20060102150405")
			sessionKey := "xone:test:multi:session:" + time.Now().Format("20060102150405")

			// 分别写入不同 DB
			cacheClient.Set(ctx, cacheKey, "cache-data", 10*time.Second)
			sessionClient.Set(ctx, sessionKey, "session-data", 10*time.Second)

			// 各自读取
			cacheVal, err := cacheClient.Get(ctx, cacheKey).Result()
			So(err, ShouldBeNil)
			So(cacheVal, ShouldEqual, "cache-data")

			sessionVal, err := sessionClient.Get(ctx, sessionKey).Result()
			So(err, ShouldBeNil)
			So(sessionVal, ShouldEqual, "session-data")

			t.Log("cache:", cacheVal, "session:", sessionVal)

			// 清理
			cacheClient.Del(ctx, cacheKey)
			sessionClient.Del(ctx, sessionKey)
		})

		// 测试 DB 隔离：cache(DB0) 和 session(DB1) 互相看不到
		PatchConvey("DB隔离", func() {
			key := "xone:test:isolation:" + time.Now().Format("20060102150405")

			// 写入 cache (DB0)
			cacheClient.Set(ctx, key, "only-in-cache", 10*time.Second)

			// session (DB1) 读不到
			_, err := sessionClient.Get(ctx, key).Result()
			So(err, ShouldNotBeNil)
			t.Log("DB隔离验证通过: session 读不到 cache 的数据")

			// 清理
			cacheClient.Del(ctx, key)
		})

		// 测试 Pipeline 跨 client
		PatchConvey("多实例Pipeline", func() {
			cacheKey := "xone:test:pipe:cache:" + time.Now().Format("20060102150405")
			sessionKey := "xone:test:pipe:session:" + time.Now().Format("20060102150405")

			// cache pipeline
			cachePipe := cacheClient.Pipeline()
			cachePipe.Set(ctx, cacheKey, "pipe-cache", 10*time.Second)
			_, err := cachePipe.Exec(ctx)
			So(err, ShouldBeNil)

			// session pipeline
			sessionPipe := sessionClient.Pipeline()
			sessionPipe.Set(ctx, sessionKey, "pipe-session", 10*time.Second)
			_, err = sessionPipe.Exec(ctx)
			So(err, ShouldBeNil)

			// 验证
			val1, _ := cacheClient.Get(ctx, cacheKey).Result()
			val2, _ := sessionClient.Get(ctx, sessionKey).Result()
			So(val1, ShouldEqual, "pipe-cache")
			So(val2, ShouldEqual, "pipe-session")
			t.Log("pipeline cache:", val1, "session:", val2)

			// 清理
			cacheClient.Del(ctx, cacheKey)
			sessionClient.Del(ctx, sessionKey)
		})
	})
}
