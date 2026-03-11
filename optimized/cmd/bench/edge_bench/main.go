package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"math/rand"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	optimizedpb "iot/optimized/proto"
)

// Metric ID 定义
const (
	MetricIDTemp      uint32 = 1
	MetricIDHumidity  uint32 = 2
	MetricIDVibration uint32 = 100 // 100+ 为振动采样点
)

func main() {
	var (
		target         = flag.String("target", "127.0.0.1:50052", "gRPC server address")
		edges          = flag.Int("edges", 20, "number of edge segments")
		devicesPerEdge = flag.Int("devices-per-edge", 1, "devices per edge")
		vibPerMsg      = flag.Int("vib-per-msg", 2000, "vibration samples per message")
		intervalMs     = flag.Int("interval-ms", 20, "send interval per device in ms")
		useBatch       = flag.Bool("batch", false, "use batch upload (collect N samples then send)")
		batchSize      = flag.Int("batch-size", 5, "samples per batch (if -batch enabled)")
		useStream      = flag.Bool("stream", false, "use streaming RPC")
		priority       = flag.Uint("priority", 0, "priority: 0=background, 1=realtime")
	)
	flag.Parse()

	conn, err := grpc.Dial(
		*target,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
	)
	if err != nil {
		log.Fatalf("dial failed: %v", err)
	}
	defer conn.Close()

	client := optimizedpb.NewOptimizedEdgeGatewayServiceClient(conn)

	log.Printf("start optimized edge bench: target=%s edges=%d devicesPerEdge=%d vibPerMsg=%d interval=%dms batch=%v stream=%v priority=%d",
		*target, *edges, *devicesPerEdge, *vibPerMsg, *intervalMs, *useBatch, *useStream, *priority)

	var wg sync.WaitGroup
	rand.Seed(time.Now().UnixNano())

	for e := 0; e < *edges; e++ {
		edgeID := e
		for d := 0; d < *devicesPerEdge; d++ {
			deviceIndex := d
			wg.Add(1)

			if *useStream {
				go runStreamDevice(&wg, client, edgeID, deviceIndex, *vibPerMsg, *intervalMs, uint32(*priority))
			} else if *useBatch {
				go runBatchDevice(&wg, client, edgeID, deviceIndex, *vibPerMsg, *intervalMs, *batchSize, uint32(*priority))
			} else {
				go runUnaryDevice(&wg, client, edgeID, deviceIndex, *vibPerMsg, *intervalMs, uint32(*priority))
			}
		}
	}

	wg.Wait()
}

// buildMetrics 构建紧凑格式的 metrics
func buildMetrics(vibPerMsg int) ([]*optimizedpb.Metric, float64) {
	metrics := make([]*optimizedpb.Metric, 0, vibPerMsg+1)

	var sum float64

	// 温度
	temp := float32(20.0 + rand.Float64()*10.0)
	metrics = append(metrics, &optimizedpb.Metric{Id: MetricIDTemp, Value: temp})
	sum += float64(temp)

	// 振动采样点
	for i := 0; i < vibPerMsg; i++ {
		v := float32(rand.Float64())
		metrics = append(metrics, &optimizedpb.Metric{Id: MetricIDVibration + uint32(i), Value: v})
		sum += float64(v)
	}

	clientAvg := sum / float64(len(metrics))
	return metrics, clientAvg
}

// runUnaryDevice 单次上报模式
func runUnaryDevice(wg *sync.WaitGroup, client optimizedpb.OptimizedEdgeGatewayServiceClient, edgeID, deviceIndex, vibPerMsg, intervalMs int, priority uint32) {
	defer wg.Done()
	deviceID := fmt.Sprintf("edge-%02d-device-%03d", edgeID, deviceIndex)
	ticker := time.NewTicker(time.Duration(intervalMs) * time.Millisecond)
	defer ticker.Stop()

	for range ticker.C {
		metrics, clientAvg := buildMetrics(vibPerMsg)

		req := &optimizedpb.Telemetry{
			DeviceId:    deviceID,
			TimestampMs: time.Now().UnixMilli(),
			Metrics:     metrics,
			ClientAvg:   clientAvg,
			Priority:    priority,
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		resp, err := client.PublishTelemetry(ctx, req)
		cancel()

		if err != nil {
			log.Printf("PublishTelemetry error (device=%s): %v", deviceID, err)
		} else if resp != nil && !resp.Verified {
			log.Printf("VERIFY FAILED (device=%s): client_avg=%.6f server_avg=%.6f diff=%.6f",
				deviceID, resp.ClientAvg, resp.ServerAvg, resp.Diff)
		}
	}
}

// runBatchDevice 批量上报模式
func runBatchDevice(wg *sync.WaitGroup, client optimizedpb.OptimizedEdgeGatewayServiceClient, edgeID, deviceIndex, vibPerMsg, intervalMs, batchSize int, priority uint32) {
	defer wg.Done()
	deviceID := fmt.Sprintf("edge-%02d-device-%03d", edgeID, deviceIndex)
	ticker := time.NewTicker(time.Duration(intervalMs) * time.Millisecond)
	defer ticker.Stop()

	samples := make([]*optimizedpb.TelemetrySample, 0, batchSize)
	var totalSum float64
	var totalCount int

	for range ticker.C {
		metrics, _ := buildMetrics(vibPerMsg)

		// 计算这批的平均
		for _, m := range metrics {
			totalSum += float64(m.Value)
			totalCount++
		}

		samples = append(samples, &optimizedpb.TelemetrySample{
			TimestampMs: time.Now().UnixMilli(),
			Metrics:     metrics,
		})

		// 达到批量大小，发送
		if len(samples) >= batchSize {
			clientAvg := totalSum / float64(totalCount)

			req := &optimizedpb.BatchTelemetry{
				DeviceId:  deviceID,
				Samples:   samples,
				ClientAvg: clientAvg,
				Priority:  priority,
			}

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			resp, err := client.PublishBatchTelemetry(ctx, req)
			cancel()

			if err != nil {
				log.Printf("PublishBatchTelemetry error (device=%s): %v", deviceID, err)
			} else if resp != nil && !resp.Verified {
				log.Printf("BATCH VERIFY FAILED (device=%s): client_avg=%.6f server_avg=%.6f diff=%.6f",
					deviceID, resp.ClientAvg, resp.ServerAvg, resp.Diff)
			}

			// 重置
			samples = samples[:0]
			totalSum = 0
			totalCount = 0
		}
	}
}

// runStreamDevice 流式上报模式
func runStreamDevice(wg *sync.WaitGroup, client optimizedpb.OptimizedEdgeGatewayServiceClient, edgeID, deviceIndex, vibPerMsg, intervalMs int, priority uint32) {
	defer wg.Done()
	deviceID := fmt.Sprintf("edge-%02d-device-%03d", edgeID, deviceIndex)

	ctx := context.Background()
	stream, err := client.StreamTelemetry(ctx)
	if err != nil {
		log.Printf("StreamTelemetry open error (device=%s): %v", deviceID, err)
		return
	}

	ticker := time.NewTicker(time.Duration(intervalMs) * time.Millisecond)
	defer ticker.Stop()

	for range ticker.C {
		metrics, clientAvg := buildMetrics(vibPerMsg)

		req := &optimizedpb.Telemetry{
			DeviceId:    deviceID,
			TimestampMs: time.Now().UnixMilli(),
			Metrics:     metrics,
			ClientAvg:   clientAvg,
			Priority:    priority,
		}

		if err := stream.Send(req); err != nil {
			log.Printf("StreamTelemetry send error (device=%s): %v", deviceID, err)
			return
		}
	}
}
