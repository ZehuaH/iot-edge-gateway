import csv
from datetime import datetime
import matplotlib.pyplot as plt

def load_latency(path):
    ts = []
    lat = []
    with open(path, newline='') as f:
        reader = csv.DictReader(f)
        for row in reader:
            ts_ms = int(row["timestamp_ms"])
            latency_ms = float(row["latency_ms"])
            ts.append(ts_ms / 1000.0)  # 转成秒
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

def main():
    latency_csv = "edge0_latency.csv"
    qps_csv = "edge0_qps.csv"

    ts, lat = load_latency(latency_csv)
    secs, cnts = load_qps(qps_csv)

    # 将时间轴归一化（相对起始时间），画图更清晰
    if ts:
        t0 = ts[0]
        ts_rel = [t - t0 for t in ts]
    else:
        ts_rel = ts

    if secs:
        s0 = secs[0]
        secs_rel = [s - s0 for s in secs]
    else:
        secs_rel = secs

    plt.figure(figsize=(10, 6))

    # 子图1：延迟随时间变化
    plt.subplot(2, 1, 1)
    plt.plot(ts_rel, lat, linewidth=0.7)
    plt.xlabel("Time (s, relative)")
    plt.ylabel("Latency (ms)")
    plt.title("Edge Device Latency Over Time")

    # 子图2：每秒 QPS
    plt.subplot(2, 1, 2)
    plt.plot(secs_rel, cnts, marker="o")
    plt.xlabel("Time (s, relative)")
    plt.ylabel("Requests per second")
    plt.title("Edge Device QPS Over Time")

    plt.tight_layout()
    plt.show()

if __name__ == "__main__":
    main()