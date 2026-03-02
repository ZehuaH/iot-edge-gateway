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

	gatewaypb "iot/proto"
)

type sample struct {
	ts       time.Time
	latency  time.Duration
	ok       bool
}

func main() {
	var (
		target      = flag.String("target", "127.0.0.1:50051", "gRPC server address")
		edgeID      = flag.Int("edge", 0, "edge index (0-based)")
		deviceIndex = flag.Int("device", 0, "device index within edge (0-based)")
		vibPerMsg   = flag.Int("vib-per-msg", 2000, "vibration samples per message")
		intervalMs  = flag.Int("interval-ms", 20, "send interval in ms")
		durationSec = flag.Int("duration-sec", 60, "total duration of test in seconds")
		outPrefix   = flag.String("out", "edge_metrics", "output file prefix")
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

	client := gatewaypb.NewEdgeGatewayServiceClient(conn)

	deviceID := fmt.Sprintf("edge-%02d-device-%03d", *edgeID, *deviceIndex)
	log.Printf("start edge metrics: device=%s target=%s duration=%ds",
		deviceID, *target, *durationSec)

	rand.Seed(time.Now().UnixNano())

	var samples []sample
	qpsPerSecond := make(map[int64]int) // key: unix second

	ticker := time.NewTicker(time.Duration(*intervalMs) * time.Millisecond)
	defer ticker.Stop()

	endAt := time.Now().Add(time.Duration(*durationSec) * time.Second)

	for now := range ticker.C {
		if now.After(endAt) {
			break
		}

		metrics := make(map[string]float64, *vibPerMsg+1)
		for i := 0; i < *vibPerMsg; i++ {
			key := fmt.Sprintf("vib_%d", i)
			metrics[key] = rand.Float64()
		}
		metrics["temp"] = 20.0 + rand.Float64()*10.0

		// 客户端本地计算平均值，写入请求
		var sum float64
		var count int
		for _, v := range metrics {
			sum += v
			count++
		}
		clientAvg := sum / float64(count)

		req := &gatewaypb.Telemetry{
			DeviceId:    deviceID,
			TimestampMs: time.Now().UnixMilli(),
			Metrics:     metrics,
			ClientAvg:   clientAvg,
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		start := time.Now()
		resp, err := client.PublishTelemetry(ctx, req)
		lat := time.Since(start)
		cancel()

		ok := (err == nil)
		if !ok {
			log.Printf("PublishTelemetry error: %v", err)
		} else if resp != nil && !resp.Verified {
			log.Printf("VERIFY FAILED: client_avg=%.6f server_avg=%.6f diff=%.6f",
				resp.ClientAvg, resp.ServerAvg, resp.Diff)
		}

		samples = append(samples, sample{
			ts:      start,
			latency: lat,
			ok:      ok,
		})

		sec := start.Unix()
		qpsPerSecond[sec]++
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

func writeLatencyCSV(path string, samples []sample) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	w := csv.NewWriter(f)
	defer w.Flush()

	// header
	if err := w.Write([]string{"timestamp_ms", "latency_ms", "ok"}); err != nil {
		return err
	}

	for _, s := range samples {
		tsMs := s.ts.UnixNano() / 1e6
		latMs := float64(s.latency) / 1e6
		okStr := "0"
		if s.ok {
			okStr = "1"
		}
		record := []string{
			fmt.Sprintf("%d", tsMs),
			fmt.Sprintf("%.3f", latMs),
			okStr,
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

	// header
	if err := w.Write([]string{"second", "count"}); err != nil {
		return err
	}

	// 简单按时间排序输出
	var secs []int64
	for sec := range qps {
		secs = append(secs, sec)
	}
	// 原地排序
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