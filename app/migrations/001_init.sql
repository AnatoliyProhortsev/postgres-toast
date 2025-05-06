-- +goose Up
CREATE TABLE test_table (
    id SERIAL PRIMARY KEY,
    name varchar(100),
    info jsonb
);
CREATE TABLE request_log (
    id SERIAL PRIMARY KEY,
    request_type INTEGER NOT NULL,
    duration_ms INTEGER NOT NULL,
    raw_bytes INTEGER
);

-- +goose Down
DROP TABLE IF EXISTS test_table;
DROP TABLE IF EXISTS request_log;