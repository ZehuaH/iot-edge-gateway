package main

import (
	"context"
	"encoding/csv"
	"flag"
	"fmt"
	"log"
	"math/rand"
	"os"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	optimizedpb "iot/optimized/proto"
)

// Metric ID 定义
const (
	MetricIDTemp      uint32 = 1
	MetricIDVibration uint32 = 100
)

type sample struct {
	ts       time.Time
	latency  time.Duration
	ok       bool
	verified bool
}

func main() {
	var (
		target      = flag.String("target", "127.0.0.1:50052", "gRPC server address")
		edgeID      = flag.Int("edge", 0, "edge index (0-based)")
		deviceIndex = flag.Int("device", 0, "device index within edge (0-based)")
		vibPerMsg   = flag.Int("vib-per-msg", 2000, "vibration samples per message")
		intervalMs  = flag.Int("interval-ms", 20, "send interval in ms")
		durationSec = flag.Int("duration-sec", 60, "total duration of test in seconds")
		outPrefix   = flag.String("out", "optimized_edge_metrics", "output file prefix")
		useBatch    = flag.Bool("batch", false, "use batch upload")
		batchSize   = flag.Int("batch-size", 5, "samples per batch")
		priority    = flag.Uint("priority", 0, "priority: 0=background, 1=realtime")
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

	deviceID := fmt.Sprintf("edge-%02d-device-%03d", *edgeID, *deviceIndex)
	log.Printf("start optimized edge metrics: device=%s target=%s duration=%ds batch=%v priority=%d",
		deviceID, *target, *durationSec, *useBatch, *priority)

	rand.Seed(time.Now().UnixNano())

	var samples []sample
	qpsPerSecond := make(map[int64]int)

	ticker := time.NewTicker(time.Duration(*intervalMs) * time.Millisecond)
	defer ticker.Stop()

	endAt := time.Now().Add(time.Duration(*durationSec) * time.Second)

	if *useBatch {
		runBatchMetrics(ticker, endAt, client, deviceID, *vibPerMsg, *batchSize, uint32(*priority), &samples, qpsPerSecond)
	} else {
		runUnaryMetrics(ticker, endAt, client, deviceID, *vibPerMsg, uint32(*priority), &samples, qpsPerSecond)
	}

	log.Printf("done, total samples=%d", len(samples))

	if err := writeLatencyCSV(*outPrefix+"_latency.csv", samples); err != nil {
		log.Fatalf("write latency csv failed: %v", err)
	}
	if err := writeQPSCSV(*outPrefix+"_qps.csv", qpsPerSecond); err != nil {
		log.Fatalf("write qps csv failed: %v", err)
	}

	log.Printf("csv written: %s_latency.csv, %s_qps.csv", *outPrefix, *outPrefix)
}

func buildMetrics(vibPerMsg int) ([]*optimizedpb.Metric, float64) {
	metrics := make([]*optimizedpb.Metric, 0, vibPerMsg+1)
	var sum float64

	temp := float32(20.0 + rand.Float64()*10.0)
	metrics = append(metrics, &optimizedpb.Metric{Id: MetricIDTemp, Value: temp})
	sum += float64(temp)

	for i := 0; i < vibPerMsg; i++ {
		v := float32(rand.Float64())
		metrics = append(metrics, &optimizedpb.Metric{Id: MetricIDVibration + uint32(i), Value: v})
		sum += float64(v)
	}

	clientAvg := sum / float64(len(metrics))
	return metrics, clientAvg
}

func runUnaryMetrics(ticker *time.Ticker, endAt time.Time, client optimizedpb.OptimizedEdgeGatewayServiceClient, deviceID string, vibPerMsg int, priority uint32, samples *[]sample, qpsPerSecond map[int64]int) {
	for now := range ticker.C {
		if now.After(endAt) {
			break
		}

		metrics, clientAvg := buildMetrics(vibPerMsg)

		req := &optimizedpb.Telemetry{
			DeviceId:    deviceID,
			TimestampMs: time.Now().UnixMilli(),
			Metrics:     metrics,
			ClientAvg:   clientAvg,
			Priority:    priority,
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		start := time.Now()
		resp, err := client.PublishTelemetry(ctx, req)
		lat := time.Since(start)
		cancel()

		ok := (err == nil)
		verified := false
		if ok && resp != nil {
			verified = resp.Verified
			if !verified {
				log.Printf("VERIFY FAILED: client_avg=%.6f server_avg=%.6f diff=%.6f",
					resp.ClientAvg, resp.ServerAvg, resp.Diff)
			}
		} else if !ok {
			log.Printf("PublishTelemetry error: %v", err)
		}

		*samples = append(*samples, sample{
			ts:       start,
			latency:  lat,
			ok:       ok,
			verified: verified,
		})

		sec := start.Unix()
		qpsPerSecond[sec]++
	}
}

func runBatchMetrics(ticker *time.Ticker, endAt time.Time, client optimizedpb.OptimizedEdgeGatewayServiceClient, deviceID string, vibPerMsg, batchSize int, priority uint32, samples *[]sample, qpsPerSecond map[int64]int) {
	batchSamples := make([]*optimizedpb.TelemetrySample, 0, batchSize)
	var totalSum float64
	var totalCount int

	for now := range ticker.C {
		if now.After(endAt) {
			break
		}

		metrics, _ := buildMetrics(vibPerMsg)
		for _, m := range metrics {
			totalSum += float64(m.Value)
			totalCount++
		}

		batchSamples = append(batchSamples, &optimizedpb.TelemetrySample{
			TimestampMs: time.Now().UnixMilli(),
			Metrics:     metrics,
		})

		if len(batchSamples) >= batchSize {
			clientAvg := totalSum / float64(totalCount)

			req := &optimizedpb.BatchTelemetry{
				DeviceId:  deviceID,
				Samples:   batchSamples,
				ClientAvg: clientAvg,
				Priority:  priority,
			}

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			start := time.Now()
			resp, err := client.PublishBatchTelemetry(ctx, req)
			lat := time.Since(start)
			cancel()

			ok := (err == nil)
			verified := false
			if ok && resp != nil {
				verified = resp.Verified
				if !verified {
					log.Printf("BATCH VERIFY FAILED: client_avg=%.6f server_avg=%.6f diff=%.6f",
						resp.ClientAvg, resp.ServerAvg, resp.Diff)
				}
			} else if !ok {
				log.Printf("PublishBatchTelemetry error: %v", err)
			}

			*samples = append(*samples, sample{
				ts:       start,
				latency:  lat,
				ok:       ok,
				verified: verified,
			})

			sec := start.Unix()
			qpsPerSecond[sec]++

			batchSamples = batchSamples[:0]
			totalSum = 0
			totalCount = 0
		}
	}
}

func writeLatencyCSV(path string, samples []sample) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	w := csv.NewWriter(f)
	defer w.Flush()

	if err := w.Write([]string{"timestamp_ms", "latency_ms", "ok", "verified"}); err != nil {
		return err
	}

	for _, s := range samples {
		tsMs := s.ts.UnixNano() / 1e6
		latMs := float64(s.latency) / 1e6
		okStr := "0"
		if s.ok {
			okStr = "1"
		}
		verifiedStr := "0"
		if s.verified {
			verifiedStr = "1"
		}
		record := []string{
			fmt.Sprintf("%d", tsMs),
			fmt.Sprintf("%.3f", latMs),
			okStr,
			verifiedStr,
		}
		if err := w.Write(record); err != nil {
			return err
		}
	}

	return nil
}

func writeQPSCSV(path string, qps map[int64]int) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	w := csv.NewWriter(f)
	defer w.Flush()

	if err := w.Write([]string{"second", "count"}); err != nil {
		return err
	}

	var secs []int64
	for sec := range qps {
		secs = append(secs, sec)
	}
	for i := 0; i < len(secs); i++ {
		for j := i + 1; j < len(secs); j++ {
			if secs[j] < secs[i] {
				secs[i], secs[j] = secs[j], secs[i]
			}
		}
	}

	for _, sec := range secs {
		record := []string{
			fmt.Sprintf("%d", sec),
			fmt.Sprintf("%d", qps[sec]),
		}
		if err := w.Write(record); err != nil {
			return err
		}
	}

	return nil
}
