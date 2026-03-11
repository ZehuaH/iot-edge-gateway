"""
对比基础版和优化版的延迟和 QPS 曲线

用法：
  python optimized/plot_compare.py \
    --baseline-latency edge0_dev0_latency.csv \
    --baseline-qps edge0_dev0_qps.csv \
    --optimized-latency optimized_edge_metrics_latency.csv \
    --optimized-qps optimized_edge_metrics_qps.csv
"""

import argparse
import csv
import matplotlib.pyplot as plt


def load_latency(path):
    ts = []
    lat = []
    with open(path, newline='') as f:
        reader = csv.DictReader(f)
        for row in reader:
            ts_ms = int(row["timestamp_ms"])
            latency_ms = float(row["latency_ms"])
            ts.append(ts_ms / 1000.0)
            lat.append(latency_ms)
    return ts, lat


def load_qps(path):
    secs = []
    cnts = []
    with open(path, newline='') as f:
        reader = csv.DictReader(f)
        for row in reader:
            sec = int(row["second"])
            cnt = int(row["count"])
            secs.append(sec)
            cnts.append(cnt)
    return secs, cnts


def normalize_time(ts):
    if not ts:
        return ts
    t0 = ts[0]
    return [t - t0 for t in ts]


def main():
    parser = argparse.ArgumentParser(description="Compare baseline vs optimized metrics")
    parser.add_argument("--baseline-latency", default="edge0_dev0_latency.csv")
    parser.add_argument("--baseline-qps", default="edge0_dev0_qps.csv")
    parser.add_argument("--optimized-latency", default="optimized_edge_metrics_latency.csv")
    parser.add_argument("--optimized-qps", default="optimized_edge_metrics_qps.csv")
    args = parser.parse_args()

    # 加载数据
    b_ts, b_lat = load_latency(args.baseline_latency)
    b_secs, b_cnts = load_qps(args.baseline_qps)

    o_ts, o_lat = load_latency(args.optimized_latency)
    o_secs, o_cnts = load_qps(args.optimized_qps)

    # 归一化时间
    b_ts_rel = normalize_time(b_ts)
    o_ts_rel = normalize_time(o_ts)
    b_secs_rel = normalize_time(b_secs)
    o_secs_rel = normalize_time(o_secs)

    plt.figure(figsize=(12, 8))

    # 子图1：延迟对比
    plt.subplot(2, 1, 1)
    plt.plot(b_ts_rel, b_lat, linewidth=0.7, label="Baseline", alpha=0.7)
    plt.plot(o_ts_rel, o_lat, linewidth=0.7, label="Optimized", alpha=0.7)
    plt.xlabel("Time (s, relative)")
    plt.ylabel("Latency (ms)")
    plt.title("Latency Comparison: Baseline vs Optimized")
    plt.legend()

    # 子图2：QPS 对比
    plt.subplot(2, 1, 2)
    plt.plot(b_secs_rel, b_cnts, marker="o", label="Baseline", alpha=0.7)
    plt.plot(o_secs_rel, o_cnts, marker="s", label="Optimized", alpha=0.7)
    plt.xlabel("Time (s, relative)")
    plt.ylabel("Requests per second")
    plt.title("QPS Comparison: Baseline vs Optimized")
    plt.legend()

    plt.tight_layout()
    plt.show()

    # 打印统计
    if b_lat:
        print(f"\n=== Baseline Stats ===")
        print(f"  Avg latency: {sum(b_lat)/len(b_lat):.2f} ms")
        print(f"  Max latency: {max(b_lat):.2f} ms")
        print(f"  Min latency: {min(b_lat):.2f} ms")

    if o_lat:
        print(f"\n=== Optimized Stats ===")
        print(f"  Avg latency: {sum(o_lat)/len(o_lat):.2f} ms")
        print(f"  Max latency: {max(o_lat):.2f} ms")
        print(f"  Min latency: {min(o_lat):.2f} ms")


if __name__ == "__main__":
    main()
