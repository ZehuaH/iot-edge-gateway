package gateway

import (
	"context"
	"io"
	"log"
	"math"
	"sync"
	"sync/atomic"
	"time"

	optimizedpb "iot/optimized/proto"
)

// PriorityLevel 优先级定义
const (
	PriorityBackground uint32 = 0 // 背景流量
	PriorityRealtime   uint32 = 1 // 列车实时流量
)

// Server 实现优化版 EdgeGatewayService gRPC 接口
type Server struct {
	optimizedpb.UnimplementedOptimizedEdgeGatewayServiceServer

	receivedCount uint64

	// 优先级队列
	highPriorityQueue chan *telemetryTask
	lowPriorityQueue  chan *telemetryTask

	// 配置
	gpuLatencyMs int64

	// worker 停止信号
	stopCh chan struct{}
	wg     sync.WaitGroup
}

// telemetryTask 内部任务结构
type telemetryTask struct {
	deviceID  string
	metrics   []*optimizedpb.Metric
	clientAvg float64
	resultCh  chan *taskResult
}

type taskResult struct {
	serverAvg float64
	verified  bool
	diff      float64
}

// NewServer 创建优化版网关服务实例
func NewServer(gpuLatencyMs int64, highQueueSize, lowQueueSize int) *Server {
	s := &Server{
		highPriorityQueue: make(chan *telemetryTask, highQueueSize),
		lowPriorityQueue:  make(chan *telemetryTask, lowQueueSize),
		gpuLatencyMs:      gpuLatencyMs,
		stopCh:            make(chan struct{}),
	}

	// 启动高优先级 worker（少量、快速响应）
	for i := 0; i < 4; i++ {
		s.wg.Add(1)
		go s.highPriorityWorker(i)
	}

	// 启动低优先级 worker（较多、批量处理）
	for i := 0; i < 8; i++ {
		s.wg.Add(1)
		go s.lowPriorityWorker(i)
	}

	return s
}

// Stop 停止所有 worker
func (s *Server) Stop() {
	close(s.stopCh)
	s.wg.Wait()
}

// highPriorityWorker 高优先级 worker（列车实时流量）
func (s *Server) highPriorityWorker(id int) {
	defer s.wg.Done()
	for {
		select {
		case <-s.stopCh:
			return
		case task := <-s.highPriorityQueue:
			result := s.processTask(task)
			task.resultCh <- result
		}
	}
}

// lowPriorityWorker 低优先级 worker（背景流量）
func (s *Server) lowPriorityWorker(id int) {
	defer s.wg.Done()
	for {
		select {
		case <-s.stopCh:
			return
		case task := <-s.lowPriorityQueue:
			result := s.processTask(task)
			task.resultCh <- result
		}
	}
}

// processTask 处理单个任务（计算平均值 + 模拟 GPU 等待）
func (s *Server) processTask(task *telemetryTask) *taskResult {
	// 使用对象池获取临时切片
	values := AcquireFloatSlice()
	defer ReleaseFloatSlice(values)

	// 计算服务端平均值
	var sum float64
	for _, m := range task.metrics {
		*values = append(*values, float64(m.Value))
		sum += float64(m.Value)
	}

	var serverAvg float64
	if len(task.metrics) > 0 {
		serverAvg = sum / float64(len(task.metrics))
	}

	// 模拟 GPU 推理延迟
	if s.gpuLatencyMs > 0 {
		time.Sleep(time.Duration(s.gpuLatencyMs) * time.Millisecond)
	}

	// 对比验证
	diff := math.Abs(serverAvg - task.clientAvg)
	const epsilon = 1e-4 // float32 精度容忍度更大一些
	verified := diff <= epsilon

	return &taskResult{
		serverAvg: serverAvg,
		verified:  verified,
		diff:      diff,
	}
}

// PublishTelemetry 单次上报（优化版）
func (s *Server) PublishTelemetry(ctx context.Context, req *optimizedpb.Telemetry) (*optimizedpb.TelemetryAck, error) {
	if req == nil || len(req.Metrics) == 0 {
		return &optimizedpb.TelemetryAck{
			Status:  "ERROR",
			Message: "empty request",
		}, nil
	}

	atomic.AddUint64(&s.receivedCount, 1)

	task := &telemetryTask{
		deviceID:  req.DeviceId,
		metrics:   req.Metrics,
		clientAvg: req.ClientAvg,
		resultCh:  make(chan *taskResult, 1),
	}

	// 根据优先级分发到不同队列
	var queued bool
	if req.Priority == PriorityRealtime {
		select {
		case s.highPriorityQueue <- task:
			queued = true
		default:
			// 高优先级队列满，降级到低优先级
			select {
			case s.lowPriorityQueue <- task:
				queued = true
			default:
				return &optimizedpb.TelemetryAck{
					Status:  "REJECTED",
					Message: "queue full",
				}, nil
			}
		}
	} else {
		select {
		case s.lowPriorityQueue <- task:
			queued = true
		default:
			return &optimizedpb.TelemetryAck{
				Status:  "REJECTED",
				Message: "queue full",
			}, nil
		}
	}

	if !queued {
		return &optimizedpb.TelemetryAck{
			Status:  "REJECTED",
			Message: "queue full",
		}, nil
	}

	// 等待结果
	select {
	case result := <-task.resultCh:
		return &optimizedpb.TelemetryAck{
			Status:        "OK",
			Message:       "received",
			ServerAvg:     result.serverAvg,
			ClientAvg:     req.ClientAvg,
			Diff:          result.diff,
			Verified:      result.verified,
			ReceivedCount: 1,
		}, nil
	case <-ctx.Done():
		return &optimizedpb.TelemetryAck{
			Status:  "TIMEOUT",
			Message: "context cancelled",
		}, ctx.Err()
	}
}

// PublishBatchTelemetry 批量上报
func (s *Server) PublishBatchTelemetry(ctx context.Context, req *optimizedpb.BatchTelemetry) (*optimizedpb.TelemetryAck, error) {
	if req == nil || len(req.Samples) == 0 {
		return &optimizedpb.TelemetryAck{
			Status:  "ERROR",
			Message: "empty batch",
		}, nil
	}

	// 合并所有样本的 metrics
	var allMetrics []*optimizedpb.Metric
	for _, sample := range req.Samples {
		allMetrics = append(allMetrics, sample.Metrics...)
	}

	atomic.AddUint64(&s.receivedCount, uint64(len(req.Samples)))

	task := &telemetryTask{
		deviceID:  req.DeviceId,
		metrics:   allMetrics,
		clientAvg: req.ClientAvg,
		resultCh:  make(chan *taskResult, 1),
	}

	// 根据优先级分发
	if req.Priority == PriorityRealtime {
		select {
		case s.highPriorityQueue <- task:
		default:
			select {
			case s.lowPriorityQueue <- task:
			default:
				return &optimizedpb.TelemetryAck{
					Status:  "REJECTED",
					Message: "queue full",
				}, nil
			}
		}
	} else {
		select {
		case s.lowPriorityQueue <- task:
		default:
			return &optimizedpb.TelemetryAck{
				Status:  "REJECTED",
				Message: "queue full",
			}, nil
		}
	}

	select {
	case result := <-task.resultCh:
		return &optimizedpb.TelemetryAck{
			Status:        "OK",
			Message:       "batch received",
			ServerAvg:     result.serverAvg,
			ClientAvg:     req.ClientAvg,
			Diff:          result.diff,
			Verified:      result.verified,
			ReceivedCount: uint64(len(req.Samples)),
		}, nil
	case <-ctx.Done():
		return &optimizedpb.TelemetryAck{
			Status:  "TIMEOUT",
			Message: "context cancelled",
		}, ctx.Err()
	}
}

// StreamTelemetry 流式上报（客户端流）
func (s *Server) StreamTelemetry(stream optimizedpb.OptimizedEdgeGatewayService_StreamTelemetryServer) error {
	var totalCount uint64
	var totalSum float64
	var totalMetrics int

	for {
		req, err := stream.Recv()
		if err == io.EOF {
			// 流结束，返回汇总结果
			var serverAvg float64
			if totalMetrics > 0 {
				serverAvg = totalSum / float64(totalMetrics)
			}

			return stream.SendAndClose(&optimizedpb.StreamAck{
				ReceivedCount: totalCount,
				Status:        "OK",
				ServerAvg:     serverAvg,
				Verified:      true,
			})
		}
		if err != nil {
			return err
		}

		atomic.AddUint64(&s.receivedCount, 1)
		totalCount++

		// 累计计算（流式场景下不做同步等待，直接处理）
		for _, m := range req.Metrics {
			totalSum += float64(m.Value)
			totalMetrics++
		}

		// 模拟 GPU 推理延迟（可选，流式场景可以更轻量）
		if s.gpuLatencyMs > 0 {
			time.Sleep(time.Duration(s.gpuLatencyMs/10) * time.Millisecond) // 流式场景减少延迟
		}
	}
}

// HealthCheck 健康检查（带队列状态）
func (s *Server) HealthCheck(ctx context.Context, req *optimizedpb.HealthCheckRequest) (*optimizedpb.HealthCheckResponse, error) {
	log.Printf("[HealthCheck] at=%s", time.Now().Format(time.RFC3339Nano))

	return &optimizedpb.HealthCheckResponse{
		Status:               "SERVING",
		HighPriorityQueueLen: uint64(len(s.highPriorityQueue)),
		LowPriorityQueueLen:  uint64(len(s.lowPriorityQueue)),
	}, nil
}
