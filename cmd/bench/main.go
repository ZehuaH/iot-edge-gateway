package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	gatewaypb "iot/proto"
)

func main() {
	var (
		target      = flag.String("target", "127.0.0.1:50051", "gRPC 服务器地址 host:port")
		concurrency = flag.Int("concurrency", 10, "并发协程数")
		total       = flag.Int("total", 1000, "总请求数")
		deviceID    = flag.String("device", "device-1", "默认设备 ID 前缀")
		timeoutMs   = flag.Int("timeout-ms", 2000, "单次请求超时时间（毫秒）")
	)
	flag.Parse()

	if *concurrency <= 0 || *total <= 0 {
		log.Fatalf("concurrency 和 total 必须为正数")
	}

	conn, err := grpc.Dial(
		*target,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
	)
	if err != nil {
		log.Fatalf("无法连接到 gRPC 服务器 %s: %v", *target, err)
	}
	defer conn.Close()

	client := gatewaypb.NewEdgeGatewayServiceClient(conn)

	log.Printf("开始压测: target=%s concurrency=%d total=%d", *target, *concurrency, *total)

	var (
		success uint64
		failed  uint64

		totalLatencyNs int64
		maxLatencyNs   int64
	)

	start := time.Now()

	var wg sync.WaitGroup
	requestsPerWorker := *total / *concurrency
	extra := *total % *concurrency

	rand.Seed(time.Now().UnixNano())

	for i := 0; i < *concurrency; i++ {
		n := requestsPerWorker
		if i < extra {
			n++
		}
		if n == 0 {
			continue
		}

		wg.Add(1)
		go func(workerID, numReq int) {
			defer wg.Done()

			for j := 0; j < numReq; j++ {
				ctx, cancel := context.WithTimeout(context.Background(), time.Duration(*timeoutMs)*time.Millisecond)

				req := &gatewaypb.Telemetry{
					DeviceId:    fmt.Sprintf("%s-%d", *deviceID, workerID),
					TimestampMs: time.Now().UnixMilli(),
					Metrics: map[string]float64{
						"temp": 20 + rand.Float64()*10,
						"hum":  40 + rand.Float64()*20,
					},
				}

				sentAt := time.Now()
				_, err := client.PublishTelemetry(ctx, req)
				latency := time.Since(sentAt)
				cancel()

				if err != nil {
					atomic.AddUint64(&failed, 1)
					continue
				}

				atomic.AddUint64(&success, 1)
				atomic.AddInt64(&totalLatencyNs, latency.Nanoseconds())

				for {
					oldMax := atomic.LoadInt64(&maxLatencyNs)
					if latency.Nanoseconds() <= oldMax {
						break
					}
					if atomic.CompareAndSwapInt64(&maxLatencyNs, oldMax, latency.Nanoseconds()) {
						break
					}
				}
			}
		}(i, n)
	}

	wg.Wait()
	elapsed := time.Since(start)

	totalReq := atomic.LoadUint64(&success) + atomic.LoadUint64(&failed)
	qps := float64(totalReq) / elapsed.Seconds()

	avgLatencyMs := float64(0)
	if success > 0 {
		avgLatencyMs = float64(atomic.LoadInt64(&totalLatencyNs)/int64(success)) / 1e6
	}

	log.Printf("====== gRPC 压测结果 ======")
	log.Printf("目标地址: %s", *target)
	log.Printf("并发数: %d, 总请求数: %d", *concurrency, totalReq)
	log.Printf("成功: %d, 失败: %d", success, failed)
	log.Printf("总耗时: %s (QPS=%.2f)", elapsed, qps)
	log.Printf("平均延迟: %.2f ms, 最大延迟: %.2f ms",
		avgLatencyMs, float64(atomic.LoadInt64(&maxLatencyNs))/1e6)
}

