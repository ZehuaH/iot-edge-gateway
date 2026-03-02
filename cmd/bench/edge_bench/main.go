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

	gatewaypb "iot/proto"
)

func main() {
	var (
		target         = flag.String("target", "127.0.0.1:50051", "gRPC server address")
		edges          = flag.Int("edges", 20, "number of edge segments (each ~10km)")
		devicesPerEdge = flag.Int("devices-per-edge", 10, "devices per edge segment")
		vibPerMsg      = flag.Int("vib-per-msg", 2000, "vibration samples per message")
		intervalMs     = flag.Int("interval-ms", 20, "send interval per device in ms")
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

	log.Printf("start edge bench: target=%s edges=%d devicesPerEdge=%d vibPerMsg=%d interval=%dms",
		*target, *edges, *devicesPerEdge, *vibPerMsg, *intervalMs)

	var wg sync.WaitGroup
	rand.Seed(time.Now().UnixNano())

	for e := 0; e < *edges; e++ {
		edgeID := e
		for d := 0; d < *devicesPerEdge; d++ {
			deviceIndex := d
			wg.Add(1)
			go func() {
				defer wg.Done()
				deviceID := fmt.Sprintf("edge-%02d-device-%03d", edgeID, deviceIndex)
				ticker := time.NewTicker(time.Duration(*intervalMs) * time.Millisecond)
				defer ticker.Stop()

				for range ticker.C {
					metrics := make(map[string]float64, *vibPerMsg+1)

					for i := 0; i < *vibPerMsg; i++ {
						key := fmt.Sprintf("vib_%d", i)
						metrics[key] = rand.Float64()
					}
					metrics["temp"] = 20.0 + rand.Float64()*10.0

					req := &gatewaypb.Telemetry{
						DeviceId:    deviceID,
						TimestampMs: time.Now().UnixMilli(),
						Metrics:     metrics,
					}

					ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
					_, err := client.PublishTelemetry(ctx, req)
					cancel()
					if err != nil {
						log.Printf("PublishTelemetry error (device=%s): %v", deviceID, err)
					}
				}
			}()
		}
	}

	wg.Wait()
}