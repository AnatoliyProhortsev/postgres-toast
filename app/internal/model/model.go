package model

type Row struct {
	ID   int    `json:"id" db:"id"`
	Name string `json:"name" db:"name"`
	Info JSONB  `json:"info" db:"info"`
}
