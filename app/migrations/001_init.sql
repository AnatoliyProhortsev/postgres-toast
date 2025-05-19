-- +goose Up
CREATE TABLE test_table (
    id SERIAL PRIMARY KEY,
    info jsonb
);
CREATE TABLE request_log (
    id SERIAL PRIMARY KEY,
    duration_ms INTEGER NOT NULL,
    raw_bytes BIGINT DEFAULT 0
);

-- +goose Down
DROP TABLE IF EXISTS test_table;
DROP TABLE IF EXISTS request_log;