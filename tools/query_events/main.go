package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	_ "modernc.org/sqlite"
	"os"
	"path/filepath"
)

func main() {
	progData := os.Getenv("ProgramData")
	if progData == "" {
		progData = `C:\\ProgramData`
	}
	dbPath := filepath.Join(progData, "SentinelAgent", "events.db")
	if _, err := os.Stat(dbPath); err != nil {
		fmt.Fprintf(os.Stderr, "events.db not found at %s: %v\n", dbPath, err)
		os.Exit(1)
	}
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "open db error:", err)
		os.Exit(1)
	}
	defer db.Close()

	rows, err := db.Query("SELECT id, timestamp, type, payload FROM events ORDER BY id DESC LIMIT 20")
	if err != nil {
		fmt.Fprintln(os.Stderr, "query error:", err)
		os.Exit(1)
	}
	defer rows.Close()

	out := make([]map[string]any, 0)
	for rows.Next() {
		var id int64
		var ts string
		var typ string
		var payload string
		if err := rows.Scan(&id, &ts, &typ, &payload); err != nil {
			fmt.Fprintln(os.Stderr, "scan error:", err)
			continue
		}
		m := map[string]any{"id": id, "timestamp": ts, "type": typ}
		var p any
		if json.Unmarshal([]byte(payload), &p) == nil {
			m["payload"] = p
		} else {
			if len(payload) > 200 {
				m["payload"] = payload[:200] + "..."
			} else {
				m["payload"] = payload
			}
		}
		out = append(out, m)
	}
	b, _ := json.MarshalIndent(out, "", "  ")
	fmt.Println(string(b))
}
