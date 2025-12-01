package main

import (
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	port    int
	db      *sql.DB
	metrics = struct {
		counter     prometheus.Counter
		gauge       prometheus.Gauge
		histogram   prometheus.Histogram
		summary     prometheus.Summary
		requestTime *prometheus.HistogramVec
	}{
		counter: prometheus.NewCounter(
			prometheus.CounterOpts{
				Namespace: "app",
				Name:      "demo_counter",
				Help:      "This is demo counter",
			}),
		gauge: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Namespace: "app",
				Name:      "demo_gauge",
				Help:      "This is demo gauge",
			}),
		histogram: prometheus.NewHistogram(
			prometheus.HistogramOpts{
				Namespace: "app",
				Name:      "demo_histogram",
				Help:      "This is demo histogram",
			}),
		summary: prometheus.NewSummary(
			prometheus.SummaryOpts{
				Namespace: "app",
				Name:      "demo_summary",
				Help:      "This is demo summary",
			}),
	}
)

type Transaction struct {
	UserID int     `json:"user_id"`
	Amount float64 `json:"amount"`
}

type Response struct {
	Status  string `json:"status"`
	Message string `json:"message"`
}

func init() {
	flag.IntVar(&port, "port", 8080, "port to listen on")
}

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

func newHandlerWithHistogram(handler http.Handler, histogram *prometheus.HistogramVec) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		start := time.Now()
		status := http.StatusOK

		defer func() {
			histogram.WithLabelValues(fmt.Sprintf("%d", status)).Observe(time.Since(start).Seconds())
		}()

		if req.Method == http.MethodGet {
			handler.ServeHTTP(w, req)
			return
		}
		status = http.StatusBadRequest

		w.WriteHeader(status)
	})
}

func updateMetrics() {
	for {
		metrics.counter.Add(rand.Float64() * 5)
		metrics.gauge.Add(rand.Float64()*15 - 5)
		metrics.histogram.Observe(rand.Float64() * 10)
		metrics.summary.Observe(rand.Float64() * 10)

		time.Sleep(time.Second)
	}
}

func main() {
	flag.Parse()

	err := connectDB()
	if err != nil {
		log.Fatalf("Failed to initialize database: %v\n", err)
	}
	defer db.Close()

	metrics.requestTime = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name: "prom_request_time",
		Help: "Time it has taken to retrieve the metrics",
	}, []string{"time"})

	err = prometheus.Register(metrics.requestTime)
	if err != nil {
		log.Printf("Failed to register histogram: %v\n", err)
	}

	prometheus.MustRegister(metrics.counter)
	prometheus.MustRegister(metrics.gauge)
	prometheus.MustRegister(metrics.histogram)
	prometheus.MustRegister(metrics.summary)

	go updateMetrics()

	http.Handle("/metrics", newHandlerWithHistogram(promhttp.Handler(), metrics.requestTime))
	http.HandleFunc("/api/deposit", depositHandler)
	http.HandleFunc("/api/withdraw", withdrawHandler)

	serverPort := strconv.Itoa(port)
	if envPort := os.Getenv("SERVER_PORT"); envPort != "" {
		serverPort = envPort
	}

	addr := ":" + serverPort
	log.Printf("Server running on http://localhost:%s\n", serverPort)
	log.Printf("Metrics available at http://localhost:%s/metrics\n", serverPort)
	log.Fatal(http.ListenAndServe(addr, nil))
}
