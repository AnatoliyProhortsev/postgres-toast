package jsonb

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
)

type JSONB map[string]interface{}

func (j *JSONB) Scan(value interface{}) error {
	bytes, err := value.([]byte)
	if err {
		return fmt.Errorf("failed to scan JSONB: expected []byte")
	}
	return json.Unmarshal(bytes, j)
}

func (j JSONB) Value() (driver.Value, error) {
	return json.Marshal(j)
}
