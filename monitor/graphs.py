import os
import json
from datetime import datetime
import matplotlib.pyplot as plt


def load_metrics(folder_path):
    data = []

    for filename in sorted(os.listdir(folder_path)):
        if not filename.endswith(".json"):
            continue

        file_path = os.path.join(folder_path, filename)
        try:
            with open(file_path, "r", encoding="utf-8") as f:
                content = json.load(f)
                timestamp = datetime.fromisoformat(content["timestamp"])

                # PostgreSQL blks_hit / blks_read (только для "postgres")
                pg_stat = next((db for db in content["db_stats"]
                                if db["datname"]["String"] == "postgres" and db["datname"]["Valid"]), None)

                blks_hit = pg_stat.get("blks_hit", 0) if pg_stat else 0
                blks_read = pg_stat.get("blks_read", 1) if pg_stat else 1  # избегаем деления на 0
                blks_ratio = blks_hit / blks_read

                # Сумма всех TOAST размеров
                toast_sum = sum(t.get("toast_size_bytes", 0) for t in content.get("toast_stats", []))

                # avg_... метрики
                data_point = {
                    "timestamp": timestamp,
                    "blks_ratio": blks_ratio,
                    "toast_total": toast_sum,
                    "avg_select_time_ms": content.get("avg_select_time_ms"),
                    "avg_insert_time_ms": content.get("avg_insert_time_ms"),
                    "avg_update_time_ms": content.get("avg_update_time_ms"),
                    "avg_delete_time_ms": content.get("avg_delete_time_ms"),
                    "avg_select_size_bytes": content.get("avg_select_size_bytes"),
                }

                data.append(data_point)

        except Exception as e:
            print(f"Ошибка при чтении {filename}: {e}")

    return data


def plot_two_line_time_series(data1, data2, key, title, ylabel, label1, label2):
    times1 = [d["timestamp"] for d in data1]
    values1 = [d[key] for d in data1]

    times2 = [d["timestamp"] for d in data2]
    values2 = [d[key] for d in data2]

    plt.figure(figsize=(10, 5))
    plt.plot(times1, values1, marker='.', label=label1)
    plt.plot(times2, values2, marker='.', label=label2)
    plt.title(title)
    plt.xlabel("Время")
    plt.ylabel(ylabel)
    plt.legend()
    plt.grid(True)
    plt.xticks(rotation=45)
    plt.tight_layout()
    plt.show()


def plot_multi_line_indexed(data1, data2, keys, title, ylabel, label_prefix1, label_prefix2):
    plt.figure(figsize=(10, 6))

    indices1 = list(range(len(data1)))
    indices2 = list(range(len(data2)))

    for key in keys:
        plt.plot(indices1,
                 [d[key] for d in data1],
                 marker='.',
                 label=f"{label_prefix1}:{key}")
        plt.plot(indices2,
                 [d[key] for d in data2],
                 marker='.',
                 label=f"{label_prefix2}:{key}")

    plt.title(title)
    plt.xlabel("Индекс метрики (порядковый номер)")
    plt.ylabel(ylabel)
    plt.legend()
    plt.grid(True)
    plt.tight_layout()
    plt.show()



def plot_xy_lines(data1, data2, xkey, ykey, title, xlabel, ylabel, label1, label2):
    plt.figure(figsize=(8, 6))

    plt.plot(
        [d[xkey] for d in data1],
        [d[ykey] for d in data1],
        marker='.',
        linestyle='-',
        label=label1
    )

    plt.plot(
        [d[xkey] for d in data2],
        [d[ykey] for d in data2],
        marker='.',
        linestyle='-',
        label=label2
    )

    plt.title(title)
    plt.xlabel(xlabel)
    plt.ylabel(ylabel)
    plt.legend()
    plt.grid(True)
    plt.tight_layout()
    plt.show()


# -------- MAIN --------
if __name__ == "__main__":
    import argparse

    parser = argparse.ArgumentParser(description="Построение графиков по метрикам из двух папок")
    parser.add_argument("folder1", help="Путь к первой папке с метриками")
    parser.add_argument("folder2", help="Путь ко второй папке с метриками")
    args = parser.parse_args()

    label1 = os.path.basename(args.folder1)
    label2 = os.path.basename(args.folder2)

    data1 = load_metrics(args.folder1)
    data2 = load_metrics(args.folder2)

    # График 1: blks_hit / blks_read
    plot_two_line_time_series(data1, data2, "blks_ratio",
                              "Postgres: blks_hit / blks_read",
                              "Доля попаданий в кэш", label1, label2)

    # График 2: Сумма TOAST таблиц
    plot_two_line_time_series(data1, data2, "toast_total",
                              "Суммарный размер TOAST таблиц",
                              "Размер в байтах", label1, label2)

    # График 3: avg_*_time_ms (8 линий)
    time_keys = [
        "avg_select_time_ms",
        "avg_insert_time_ms",
        "avg_update_time_ms",
        "avg_delete_time_ms"
    ]
    plot_multi_line_indexed(data1, data2, time_keys,
                                "Среднее время операций (мс)",
                                "мс", label1, label2)

    # График 4: зависимость avg_select_time_ms от avg_select_size_bytes
    plot_xy_lines(data1, data2,
                  "avg_select_size_bytes", "avg_select_time_ms",
                  "Зависимость времени SELECT от размера JSONB",
                  "Размер JSONB (бит)", "Время SELECT (мс)",
                  label1, label2)
