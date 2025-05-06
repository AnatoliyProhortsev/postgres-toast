#!/usr/bin/env python3
import argparse
import glob
import json
import os
from datetime import datetime
import matplotlib.pyplot as plt

def load_snapshots(export_dir):
    # Собираем список файлов в export/
    files = glob.glob(os.path.join(export_dir, "stats_*.json"))
    snapshots = []
    for filename in sorted(files):
        with open(filename, 'r') as f:
            try:
                data = json.load(f)
                # Предполагается, что data имеет поля "timestamp" и "stats"
                # Преобразуем строку времени в объект datetime, если необходимо
                ts_str = data.get("timestamp")
                if isinstance(ts_str, str):
                    ts = datetime.fromisoformat(ts_str)
                else:
                    ts = datetime.now()
                snapshots.append({
                    "timestamp": ts,
                    "stats": data.get("stats", [])
                })
            except Exception as e:
                print(f"Ошибка обработки файла {filename}: {e}")
    return snapshots

def build_timeseries(snapshots, metric):
    # Структура: { datname: ([timestamps], [значения]) }
    timeseries = {}
    for snap in snapshots:
        t = snap["timestamp"]
        for record in snap["stats"]:
            dbname = record.get("datname", "unknown")
            value = record.get(metric)
            if value is None:
                continue
            if dbname not in timeseries:
                timeseries[dbname] = ([], [])
            times, values = timeseries[dbname]
            times.append(t)
            values.append(value)
    return timeseries

def plot_metric(timeseries, metric):
    plt.figure(figsize=(10, 6))
    for dbname, (times, values) in timeseries.items():
        plt.plot(times, values, marker='o', label=dbname)
    plt.xlabel("Время")
    plt.ylabel(metric)
    plt.title(f"Динамика метрики {metric}")
    plt.legend()
    plt.grid(True)
    plt.tight_layout()
    plt.show()

def main():
    parser = argparse.ArgumentParser(description="Построение графика по выбранной метрике из JSON статистики")
    parser.add_argument("--metric", required=True, help="Метрика для построения графика (например, xact_commit, numbackends, blks_hit и др.)")
    parser.add_argument("--export-dir", default="export", help="Путь к каталогу с JSON статистикой (по умолчанию: export)")
    args = parser.parse_args()

    snapshots = load_snapshots(args.export_dir)
    if not snapshots:
        print("Нет файлов со статистикой в каталоге", args.export_dir)
        return

    timeseries = build_timeseries(snapshots, args.metric)
    if not timeseries:
        print(f"Метрика {args.metric} не найдена в загруженных данных.")
        return

    plot_metric(timeseries, args.metric)

if __name__ == "__main__":
    main()
