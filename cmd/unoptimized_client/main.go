package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"math/rand"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	gatewaypb "iot/proto" // 对应你当前生成的 Telemetry/PublishTelemetry
)

func main() {
	target := flag.String("target", "127.0.0.1:50051", "gRPC server address")
	deviceID := flag.String("device", "device-1", "device id")
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

	const vibrationSamplesPerMsg = 4000        // 每条消息很多振动点
	const sendInterval = 20 * time.Millisecond // 每 20ms 发一条

	log.Printf("start unoptimized stream: device=%s target=%s", *deviceID, *target)

	for {
		metrics := make(map[string]float64, vibrationSamplesPerMsg+1)

		for i := 0; i < vibrationSamplesPerMsg; i++ {
			key := fmt.Sprintf("vib_%d", i)
			metrics[key] = rand.Float64()
		}

		metrics["temp"] = 20.0 + rand.Float64()*10.0

		req := &gatewaypb.Telemetry{
			DeviceId:    *deviceID,
			TimestampMs: time.Now().UnixMilli(),
			Metrics:     metrics,
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		_, err := client.PublishTelemetry(ctx, req)
		cancel()

		if err != nil {
			log.Printf("PublishTelemetry error: %v", err)
		}

		time.Sleep(sendInterval)
	}
}