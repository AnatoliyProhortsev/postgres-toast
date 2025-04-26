import requests
import random
import json

# Конфигурация
URL = "http://localhost:8080/addRow"  # Адрес микросервиса
NUM_REQUESTS = 1000  # Количество запросов

for i in range(NUM_REQUESTS):
    payload = {
        "id": i,
        "name": f"User_{i}",
        "info": {
            "age": random.randint(18, 90),
            "city": random.choice(["Moscow", "Paris", "Berlin", "Tokyo", "New York"]),
            "active": random.choice([True, False])
        }
    }

    try:
        response = requests.post(URL, json=payload)
        if response.status_code == 201:
            print(f"[{i}] OK")
        else:
            print(f"[{i}] Error {response.status_code}: {response.text}")
    except Exception as e:
        print(f"[{i}] Exception: {e}")
