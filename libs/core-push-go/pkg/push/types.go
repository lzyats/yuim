package push

import "time"

type Message struct {
	Title   string            `json:"title"`
	Body    string            `json:"body"`
	Data    map[string]string `json:"data,omitempty"`
	Targets []string          `json:"targets,omitempty"`
}

type Result struct {
	OK       bool      `json:"ok"`
	Provider string    `json:"provider"`
	Body     string    `json:"body,omitempty"`
	Error    string    `json:"error,omitempty"`
	At       time.Time `json:"at"`
}
