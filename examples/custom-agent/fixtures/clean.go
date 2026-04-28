//go:build ignore
// +build ignore

// This file is a fixture for the security-reviewer agent eval.
// It is excluded from the module build via the build tag above.

package main

import (
	"database/sql"
	"fmt"
	"html/template"
	"log"
	"net/http"

	_ "github.com/mattn/go-sqlite3"
)

// User represents a user record.
type User struct {
	ID   int
	Name string
}

// getUser fetches a user by ID using parameterized queries (safe).
func getUser(db *sql.DB, userID int) (*User, error) {
	row := db.QueryRow("SELECT id, name FROM users WHERE id = ?", userID)

	var u User
	if err := row.Scan(&u.ID, &u.Name); err != nil {
		return nil, fmt.Errorf("querying user %d: %w", userID, err)
	}
	return &u, nil
}

// profileHandler renders the user profile with proper HTML escaping.
func profileHandler(db *sql.DB) http.HandlerFunc {
	tmpl := template.Must(template.New("profile").Parse(`
		<!DOCTYPE html>
		<html>
		<head><title>Profile</title></head>
		<body>
			<h1>{{.Name}}</h1>
			<p>User ID: {{.ID}}</p>
		</body>
		</html>
	`))

	return func(w http.ResponseWriter, r *http.Request) {
		user, err := getUser(db, 1)
		if err != nil {
			http.Error(w, "user not found", http.StatusNotFound)
			return
		}
		if err := tmpl.Execute(w, user); err != nil {
			log.Printf("template error: %v", err)
		}
	}
}

func main() {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	http.HandleFunc("/profile", profileHandler(db))
	log.Fatal(http.ListenAndServe(":8080", nil))
}
