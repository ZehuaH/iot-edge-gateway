package gateway

import (
	"context"
	"log"
	"math"
	"sync/atomic"
	"time"

	gatewaypb "iot/proto"
)

// Server 实现 EdgeGatewayService gRPC 接口。
type Server struct {
	gatewaypb.UnimplementedEdgeGatewayServiceServer

	receivedTelemetryCount uint64
}

// NewServer 创建一个新的网关服务实例。
func NewServer() *Server {
	return &Server{}
}

// PublishTelemetry 处理设备上报的遥测数据。
func (s *Server) PublishTelemetry(ctx context.Context, req *gatewaypb.Telemetry) (*gatewaypb.TelemetryAck, error) {
	if req == nil || len(req.Metrics) == 0 {
		return &gatewaypb.TelemetryAck{
			Status:  "ERROR",
			Message: "empty request",
		}, nil
	}

	atomic.AddUint64(&s.receivedTelemetryCount, 1)

	// 1. 服务端重新计算平均值
	var sum float64
	var count int
	for _, v := range req.Metrics {
		sum += v
		count++
	}
	serverAvg := sum / float64(count)

	// 2. 模拟“等待 GPU 推理结果”
	//    例如：sleep 50ms，你可以做成常量或 flag 参数
	time.Sleep(50 * time.Millisecond)

	// 3. 对比 client_avg 和 server_avg
	clientAvg := req.ClientAvg
	diff := math.Abs(serverAvg - clientAvg)
	const epsilon = 1e-6
	verified := diff <= epsilon

	log.Printf("[Telemetry] device=%s metrics=%d client_avg=%.6f server_avg=%.6f diff=%.6f verified=%v",
		req.GetDeviceId(), len(req.GetMetrics()), clientAvg, serverAvg, diff, verified)

	return &gatewaypb.TelemetryAck{
		Status:    "OK",
		Message:   "received",
		ServerAvg: serverAvg,
		ClientAvg: clientAvg,
		Diff:      diff,
		Verified:  verified,
	}, nil
}

// HealthCheck 简单健康检查接口。
func (s *Server) HealthCheck(ctx context.Context, req *gatewaypb.HealthCheckRequest) (*gatewaypb.HealthCheckResponse, error) {
	// 这里可以扩展更多内部检查逻辑（如依赖服务、缓存、MQ 等）
	_ = req

	log.Printf("[HealthCheck] at=%s", time.Now().Format(time.RFC3339Nano))

	return &gatewaypb.HealthCheckResponse{
		Status: "SERVING",
	}, nil
}

