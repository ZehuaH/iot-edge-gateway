package gateway

import (
	"sync"

	optimizedpb "iot/optimized/proto"
)

// TelemetryPool 用于复用 Telemetry 对象，减少 GC 压力
var TelemetryPool = sync.Pool{
	New: func() interface{} {
		return &optimizedpb.Telemetry{
			Metrics: make([]*optimizedpb.Metric, 0, 128),
		}
	},
}

// MetricSlicePool 用于复用 Metric 切片
var MetricSlicePool = sync.Pool{
	New: func() interface{} {
		slice := make([]*optimizedpb.Metric, 0, 4096)
		return &slice
	},
}

// MetricPool 用于复用单个 Metric 对象
var MetricPool = sync.Pool{
	New: func() interface{} {
		return &optimizedpb.Metric{}
	},
}

// BatchTelemetryPool 用于复用批量遥测对象
var BatchTelemetryPool = sync.Pool{
	New: func() interface{} {
		return &optimizedpb.BatchTelemetry{
			Samples: make([]*optimizedpb.TelemetrySample, 0, 64),
		}
	},
}

// FloatSlicePool 用于复用 float64 切片（计算用）
var FloatSlicePool = sync.Pool{
	New: func() interface{} {
		slice := make([]float64, 0, 4096)
		return &slice
	},
}

// AcquireTelemetry 从池中获取一个 Telemetry 对象
func AcquireTelemetry() *optimizedpb.Telemetry {
	t := TelemetryPool.Get().(*optimizedpb.Telemetry)
	return t
}

// ReleaseTelemetry 将 Telemetry 对象归还到池中
func ReleaseTelemetry(t *optimizedpb.Telemetry) {
	if t == nil {
		return
	}
	t.DeviceId = ""
	t.TimestampMs = 0
	t.ClientAvg = 0
	t.Priority = 0
	t.Metrics = t.Metrics[:0]
	TelemetryPool.Put(t)
}

// AcquireMetric 从池中获取一个 Metric 对象
func AcquireMetric() *optimizedpb.Metric {
	m := MetricPool.Get().(*optimizedpb.Metric)
	return m
}

// ReleaseMetric 将 Metric 对象归还到池中
func ReleaseMetric(m *optimizedpb.Metric) {
	if m == nil {
		return
	}
	m.Id = 0
	m.Value = 0
	MetricPool.Put(m)
}

// AcquireFloatSlice 从池中获取一个 float64 切片
func AcquireFloatSlice() *[]float64 {
	return FloatSlicePool.Get().(*[]float64)
}

// ReleaseFloatSlice 将 float64 切片归还到池中
func ReleaseFloatSlice(s *[]float64) {
	if s == nil {
		return
	}
	*s = (*s)[:0]
	FloatSlicePool.Put(s)
}
