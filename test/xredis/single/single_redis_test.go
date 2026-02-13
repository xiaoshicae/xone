package single

import (
	"context"
	"testing"
	"time"

	"github.com/xiaoshicae/xone/v2/xredis"
	"github.com/xiaoshicae/xone/v2/xserver"
	"github.com/xiaoshicae/xone/v2/xutil"

	. "github.com/bytedance/mockey"
	. "github.com/smartystreets/goconvey/convey"
)

func TestXRedisSingleClient(t *testing.T) {
	t.Skip("真实环境测试，需要先启动本地 Redis，可注释掉该 Skip 进行测试")

	PatchConvey("TestXRedisSingleClient", t, func() {
		// 初始化 XOne
		err := xserver.R()
		So(err, ShouldBeNil)

		ctx := context.Background()
		client := xredis.C()
		So(client, ShouldNotBeNil)

		// 测试 Ping
		PatchConvey("Ping", func() {
			result, err := client.Ping(ctx).Result()
			So(err, ShouldBeNil)
			So(result, ShouldEqual, "PONG")
			t.Log("ping:", result)
		})

		// 测试 String 类型 Set/Get
		PatchConvey("String-Set/Get", func() {
			key := "xone:test:string:" + time.Now().Format("20060102150405")

			err := client.Set(ctx, key, "hello xone", 10*time.Second).Err()
			So(err, ShouldBeNil)

			val, err := client.Get(ctx, key).Result()
			So(err, ShouldBeNil)
			So(val, ShouldEqual, "hello xone")
			t.Log("get:", val)

			// 清理
			client.Del(ctx, key)
		})

		// 测试 Key 过期
		PatchConvey("Key-TTL", func() {
			key := "xone:test:ttl:" + time.Now().Format("20060102150405")

			client.Set(ctx, key, "ttl-test", 5*time.Second)

			ttl, err := client.TTL(ctx, key).Result()
			So(err, ShouldBeNil)
			So(ttl, ShouldBeGreaterThan, 0)
			So(ttl, ShouldBeLessThanOrEqualTo, 5*time.Second)
			t.Log("ttl:", ttl)

			// 清理
			client.Del(ctx, key)
		})

		// 测试 Key 不存在
		PatchConvey("Key-NotExists", func() {
			val, err := client.Get(ctx, "xone:test:nonexistent:key").Result()
			So(err, ShouldNotBeNil)
			So(val, ShouldEqual, "")
			t.Log("expected error:", err)
		})

		// 测试 Hash 类型
		PatchConvey("Hash-HSet/HGet", func() {
			key := "xone:test:hash:" + time.Now().Format("20060102150405")

			err := client.HSet(ctx, key, map[string]any{
				"name": "xone",
				"age":  "3",
			}).Err()
			So(err, ShouldBeNil)

			name, err := client.HGet(ctx, key, "name").Result()
			So(err, ShouldBeNil)
			So(name, ShouldEqual, "xone")

			all, err := client.HGetAll(ctx, key).Result()
			So(err, ShouldBeNil)
			So(len(all), ShouldEqual, 2)
			t.Log("hash:", xutil.ToJsonString(all))

			// 清理
			client.Del(ctx, key)
		})

		// 测试 List 类型
		PatchConvey("List-LPush/LRange", func() {
			key := "xone:test:list:" + time.Now().Format("20060102150405")

			client.LPush(ctx, key, "a", "b", "c")

			vals, err := client.LRange(ctx, key, 0, -1).Result()
			So(err, ShouldBeNil)
			So(len(vals), ShouldEqual, 3)
			t.Log("list:", vals)

			// 清理
			client.Del(ctx, key)
		})

		// 测试 Set 类型
		PatchConvey("Set-SAdd/SMembers", func() {
			key := "xone:test:set:" + time.Now().Format("20060102150405")

			client.SAdd(ctx, key, "x", "y", "z")

			members, err := client.SMembers(ctx, key).Result()
			So(err, ShouldBeNil)
			So(len(members), ShouldEqual, 3)
			t.Log("set members:", members)

			// 清理
			client.Del(ctx, key)
		})

		// 测试 Pipeline
		PatchConvey("Pipeline", func() {
			key1 := "xone:test:pipe1:" + time.Now().Format("20060102150405")
			key2 := "xone:test:pipe2:" + time.Now().Format("20060102150405")

			pipe := client.Pipeline()
			pipe.Set(ctx, key1, "val1", 10*time.Second)
			pipe.Set(ctx, key2, "val2", 10*time.Second)
			cmds, err := pipe.Exec(ctx)
			So(err, ShouldBeNil)
			So(len(cmds), ShouldEqual, 2)

			val1, _ := client.Get(ctx, key1).Result()
			val2, _ := client.Get(ctx, key2).Result()
			So(val1, ShouldEqual, "val1")
			So(val2, ShouldEqual, "val2")
			t.Log("pipeline results:", val1, val2)

			// 清理
			client.Del(ctx, key1, key2)
		})

		// 测试自增
		PatchConvey("Incr/Decr", func() {
			key := "xone:test:counter:" + time.Now().Format("20060102150405")

			val, err := client.Incr(ctx, key).Result()
			So(err, ShouldBeNil)
			So(val, ShouldEqual, 1)

			val, err = client.IncrBy(ctx, key, 10).Result()
			So(err, ShouldBeNil)
			So(val, ShouldEqual, 11)

			val, err = client.Decr(ctx, key).Result()
			So(err, ShouldBeNil)
			So(val, ShouldEqual, 10)
			t.Log("counter:", val)

			// 清理
			client.Del(ctx, key)
		})
	})
}
