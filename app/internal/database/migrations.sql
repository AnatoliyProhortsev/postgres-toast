DROP TABLE IF EXISTS test_table
CREATE TABLE test_table (
    id SERIAL PRIMARY KEY,
    name varchar(100),
    info jsonb
)