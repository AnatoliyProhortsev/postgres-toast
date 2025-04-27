-- +goose Up
CREATE TABLE test_table (
    id SERIAL PRIMARY KEY,
    name varchar(100),
    info jsonb
);

-- +goose Down
DROP TABLE IF EXISTS test_table;