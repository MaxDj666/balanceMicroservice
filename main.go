package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
)

type Transaction struct {
	UserID int     `json:"user_id"`
	Amount float64 `json:"amount"`
}

type Response struct {
	Status  string `json:"status"`
	Message string `json:"message"`
}

var db *sql.DB

func connectDB() error {
	err := godotenv.Load()
	if err != nil {
		log.Println("Warning: No .env file found, using system environment variables")
	}

	host := os.Getenv("DB_HOST")
	if host == "" {
		host = "localhost"
	}

	port := os.Getenv("DB_PORT")
	if port == "" {
		port = "5432"
	}

	user := os.Getenv("DB_USER")
	if user == "" {
		user = "postgres"
	}

	password := os.Getenv("DB_PASSWORD")
	if password == "" {
		return fmt.Errorf("DB_PASSWORD environment variable is required")
	}

	dbname := os.Getenv("DB_NAME")
	if dbname == "" {
		return fmt.Errorf("DB_NAME environment variable is required")
	}

	connStr := fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=disable",
		user, password, host, port, dbname)

	database, err := sql.Open("postgres", connStr)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}

	err = database.Ping()
	if err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}

	db = database
	log.Printf("✓ Database connection established to %s:%s/%s\n", host, port, dbname)
	return nil
}

func depositHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Only POST allowed", http.StatusMethodNotAllowed)
		return
	}

	var tx Transaction
	err := json.NewDecoder(r.Body).Decode(&tx)
	if err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	if tx.Amount <= 0 {
		http.Error(w, "Amount must be positive", http.StatusBadRequest)
		return
	}

	_, err = db.Exec(
		"INSERT INTO transactions (user_id, amount, operation_type) VALUES ($1, $2, 'DEPOSIT')",
		tx.UserID,
		tx.Amount,
	)

	if err != nil {
		log.Printf("Database error (deposit): %v\n", err)
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(Response{
		Status:  "success",
		Message: fmt.Sprintf("Deposited %.2f to user %d", tx.Amount, tx.UserID),
	})
}

func withdrawHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Only POST allowed", http.StatusMethodNotAllowed)
		return
	}

	var tx Transaction
	err := json.NewDecoder(r.Body).Decode(&tx)
	if err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	if tx.Amount <= 0 {
		http.Error(w, "Amount must be positive", http.StatusBadRequest)
		return
	}

	_, err = db.Exec(
		"INSERT INTO transactions (user_id, amount, operation_type) VALUES ($1, $2, 'WITHDRAW')",
		tx.UserID,
		tx.Amount,
	)

	if err != nil {
		log.Printf("Database error (withdraw): %v\n", err)
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(Response{
		Status:  "success",
		Message: fmt.Sprintf("Withdrawn %.2f from user %d", tx.Amount, tx.UserID),
	})
}

func main() {
	err := connectDB()
	if err != nil {
		log.Fatalf("Failed to initialize database: %v\n", err)
	}
	defer db.Close()

	http.HandleFunc("/api/deposit", depositHandler)
	http.HandleFunc("/api/withdraw", withdrawHandler)

	port := os.Getenv("SERVER_PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("Server running on http://localhost:%s\n", port)
	log.Fatalf("Server error: %v\n", http.ListenAndServe(":"+port, nil))
}
