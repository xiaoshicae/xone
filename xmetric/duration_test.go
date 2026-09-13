package xmetric

import (
	"testing"
	"time"

	. "github.com/bytedance/mockey"
	dto "github.com/prometheus/client_model/go"
	. "github.com/smartystreets/goconvey/convey"
)

func TestDurationMetricName(t *testing.T) {
	PatchConvey("TestDurationMetricName", t, func() {
		So(durationMetricName("db_query"), ShouldEqual, "db_query_seconds")
		So(durationMetricName("db_query_seconds"), ShouldEqual, "db_query_seconds")
		So(durationMetricName(""), ShouldEqual, "_seconds")
	})
}

func TestObserveDuration(t *testing.T) {
	PatchConvey("TestObserveDuration", t, func() {
		resetState()

		PatchConvey("按秒记录，并落进默认桶而不是溢出到 +Inf", func() {
			// 典型接口耗时：直接传毫秒数值会全部溢出，传 Duration 则正常分布
			for _, ms := range []int{80, 120, 250, 900} {
				ObserveDuration("api_latency", time.Duration(ms)*time.Millisecond)
			}

			h := gatherHistogram(t, "api_latency_seconds")
			So(h.GetSampleCount(), ShouldEqual, 4)
			// 总和 = 1.35s
			So(h.GetSampleSum(), ShouldAlmostEqual, 1.35, 0.0001)

			// 最大有限桶（le=10）必须装下全部样本，否则分位数不可用
			buckets := h.GetBucket()
			last := buckets[len(buckets)-1]
			So(last.GetUpperBound(), ShouldEqual, 10)
			So(last.GetCumulativeCount(), ShouldEqual, 4)
		})

		PatchConvey("指标名已带 _seconds 时不重复追加", func() {
			ObserveDuration("cache_get_seconds", time.Millisecond)
			So(gatheredNames(), ShouldContain, "cache_get_seconds")
			So(gatheredNames(), ShouldNotContain, "cache_get_seconds_seconds")
		})

		PatchConvey("标签照常生效", func() {
			ObserveDuration("rpc_call", 10*time.Millisecond, T("method", "GetUser"))
			h := gatherHistogram(t, "rpc_call_seconds")
			So(h.GetSampleCount(), ShouldEqual, 1)
		})
	})
}

func TestTimer(t *testing.T) {
	PatchConvey("TestTimer", t, func() {
		resetState()

		PatchConvey("返回的函数执行时记录耗时", func() {
			stop := Timer("handle_order")
			time.Sleep(2 * time.Millisecond)
			stop()

			h := gatherHistogram(t, "handle_order_seconds")
			So(h.GetSampleCount(), ShouldEqual, 1)
			So(h.GetSampleSum(), ShouldBeGreaterThan, 0.001)
		})

		PatchConvey("标签在计时开始时固定", func() {
			stop := Timer("tagged_op", T("kind", "a"))
			stop()

			names, _ := parseTags([]Tag{T("kind", "a")})
			counter := getOrCreateHistogram("tagged_op_seconds", names)
			So(counter, ShouldNotBeNil)
			So(gatheredNames(), ShouldContain, "tagged_op_seconds")
		})

		PatchConvey("未调用返回值则不记录", func() {
			_ = Timer("never_stopped")
			So(gatheredNames(), ShouldNotContain, "never_stopped_seconds")
		})
	})
}

// gatherHistogram 取出指定名称的 histogram 指标
func gatherHistogram(t *testing.T, name string) *dto.Histogram {
	t.Helper()
	fams, err := Registry().Gather()
	So(err, ShouldBeNil)
	for _, f := range fams {
		if f.GetName() == name {
			return f.GetMetric()[0].GetHistogram()
		}
	}
	t.Fatalf("metric %s not found, got %v", name, gatheredNames())
	return nil
}

func TestTrackInFlight(t *testing.T) {
	PatchConvey("TestTrackInFlight", t, func() {
		resetState()

		gaugeValue := func(name string) float64 {
			fams, _ := Registry().Gather()
			for _, f := range fams {
				if f.GetName() == name {
					return f.GetMetric()[0].GetGauge().GetValue()
				}
			}
			return -1
		}

		PatchConvey("进入时 +1，返回的函数执行时 -1", func() {
			done := TrackInFlight("active_requests")
			So(gaugeValue("active_requests"), ShouldEqual, 1)

			done()
			So(gaugeValue("active_requests"), ShouldEqual, 0)
		})

		PatchConvey("并发进行中数量正确", func() {
			var stops []func()
			for range 5 {
				stops = append(stops, TrackInFlight("active_requests"))
			}
			So(gaugeValue("active_requests"), ShouldEqual, 5)

			for _, stop := range stops {
				stop()
			}
			So(gaugeValue("active_requests"), ShouldEqual, 0)
		})

		PatchConvey("重复调用返回值不会把计数减穿", func() {
			done := TrackInFlight("active_requests")
			done()
			done()
			done()
			So(gaugeValue("active_requests"), ShouldEqual, 0)
		})

		PatchConvey("标签照常生效", func() {
			done := TrackInFlight("active_requests", T("api", "/order"))
			defer done()

			fams, _ := Registry().Gather()
			var found bool
			for _, f := range fams {
				if f.GetName() != "active_requests" {
					continue
				}
				for _, m := range f.GetMetric() {
					for _, l := range m.GetLabel() {
						if l.GetName() == "api" && l.GetValue() == "/order" {
							found = true
						}
					}
				}
			}
			So(found, ShouldBeTrue)
		})

		PatchConvey("defer 写法下 panic 也能正确减回", func() {
			func() {
				defer func() { recover() }()
				defer TrackInFlight("active_requests")()
				panic("boom")
			}()
			So(gaugeValue("active_requests"), ShouldEqual, 0)
		})
	})
}
