package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/bsm/redislock"
	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/redis/go-redis/v9"
)

var (
	port int
	db   *sql.DB

	redisClient  *redis.Client
	locker       *redislock.Client
	redisEnabled bool

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
	flag.IntVar(&port, "port", 8081, "port to listen on")
}

func initDBTable() error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var tableExists bool
	err := db.QueryRowContext(ctx, `
        SELECT EXISTS (
            SELECT FROM information_schema.tables 
            WHERE table_schema = 'public' 
            AND table_name = 'transactions'
        )
    `).Scan(&tableExists)

	if err != nil {
		return fmt.Errorf("failed to check table existence: %w", err)
	}

	if !tableExists {
		log.Println("Creating transactions table...")

		createTableSQL := `
        CREATE TABLE transactions (
            id SERIAL PRIMARY KEY,
            user_id INTEGER NOT NULL,
            amount DECIMAL(10, 2) NOT NULL,
            operation_type VARCHAR(10) NOT NULL CHECK (operation_type IN ('DEPOSIT', 'WITHDRAW')),
            created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
        );

        CREATE INDEX idx_transactions_user_id ON transactions(user_id);
        CREATE INDEX idx_transactions_created_at ON transactions(created_at);
        `

		_, err := db.ExecContext(ctx, createTableSQL)
		if err != nil {
			return fmt.Errorf("failed to create transactions table: %w", err)
		}
		log.Println("Transactions table created")
	} else {
		log.Println("Transactions table already exists")
	}

	return nil
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
	log.Printf("Database connection established to %s:%s/%s\n", host, port, dbname)
	return nil
}

func connectRedis() error {
	addr := os.Getenv("REDIS_ADDR")
	if addr == "" {
		addr = "localhost:6379"
	}

	dbNum := 0
	if dbStr := os.Getenv("REDIS_DB"); dbStr != "" {
		if v, err := strconv.Atoi(dbStr); err == nil {
			dbNum = v
		}
	}

	password := os.Getenv("REDIS_PASSWORD")

	redisClient = redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: password,
		DB:       dbNum,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := redisClient.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("failed to connect to redis: %w", err)
	}

	locker = redislock.New(redisClient)

	log.Printf("Redis connection established to %s DB=%d\n", addr, dbNum)
	return nil
}

func depositHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Only POST allowed", http.StatusMethodNotAllowed)
		return
	}

	var tx Transaction
	if err := json.NewDecoder(r.Body).Decode(&tx); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	if tx.Amount <= 0 {
		http.Error(w, "Amount must be positive", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	err := withUserLock(ctx, tx.UserID, func(ctx context.Context) error {
		// Здесь я могу делать всё, что должно быть атомарным
		_, err := db.ExecContext(
			ctx,
			"INSERT INTO transactions (user_id, amount, operation_type) VALUES ($1, $2, 'DEPOSIT')",
			tx.UserID,
			tx.Amount,
		)

		if err != nil {
			return fmt.Errorf("database error (deposit): %w", err)
		}

		return nil
	})

	if err != nil {
		log.Printf("deposit error: %v\n", err)
		// Стоит ли различать ошибки блокировки и бизнес-ошибки?
		http.Error(w, "Conflict or database error", http.StatusConflict)
		return
	}

	writeJSONResponse(w, http.StatusOK, Response{
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
	if err := json.NewDecoder(r.Body).Decode(&tx); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	if tx.Amount <= 0 {
		http.Error(w, "Amount must be positive", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	err := withUserLock(ctx, tx.UserID, func(ctx context.Context) error {
		// Проверка баланса (работает как с Redis, так и без)
		var balance float64
		row := db.QueryRowContext(ctx, `
            SELECT COALESCE(SUM(
                CASE WHEN operation_type = 'DEPOSIT' THEN amount
                     WHEN operation_type = 'WITHDRAW' THEN -amount
                END
            ), 0) AS balance
            FROM transactions
            WHERE user_id = $1
        `, tx.UserID)

		if err := row.Scan(&balance); err != nil {
			return fmt.Errorf("failed to get balance: %w", err)
		}

		if balance < tx.Amount {
			return fmt.Errorf("insufficient funds: balance=%.2f, withdraw=%.2f", balance, tx.Amount)
		}

		_, err := db.ExecContext(
			ctx,
			"INSERT INTO transactions (user_id, amount, operation_type) VALUES ($1, $2, 'WITHDRAW')",
			tx.UserID,
			tx.Amount,
		)
		if err != nil {
			return fmt.Errorf("database error (withdraw): %w", err)
		}

		return nil
	})

	if err != nil {
		log.Printf("withdraw error: %v\n", err)
		// Стоит ли различать ошибки блокировки и бизнес-ошибки?
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}

	writeJSONResponse(w, http.StatusOK, Response{
		Status:  "success",
		Message: fmt.Sprintf("Withdrawn %.2f from user %d", tx.Amount, tx.UserID),
	})
}

func withUserLock(ctx context.Context, userID int, fn func(ctx context.Context) error) error {
	// Если Redis отключен, просто выполняем функцию без блокировки
	if !redisEnabled {
		return fn(ctx)
	}

	// Если Redis включен, используем Redlock
	if locker == nil {
		return fmt.Errorf("locker is not initialized")
	}

	key := fmt.Sprintf("lock:user:%d", userID)

	// Время жизни блокировки
	ttl := 5 * time.Second

	lock, err := locker.Obtain(ctx, key, ttl, &redislock.Options{
		RetryStrategy: redislock.LinearBackoff(100 * time.Millisecond),
	})
	if errors.Is(err, redislock.ErrNotObtained) {
		return fmt.Errorf("could not obtain lock for user %d", userID)
	}
	if err != nil {
		return fmt.Errorf("failed to obtain lock: %w", err)
	}

	defer func() {
		if err := lock.Release(ctx); err != nil {
			log.Printf("failed to release lock for user %d: %v", userID, err)
		}
	}()

	// Выполняем критическую секцию
	return fn(ctx)
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

func writeJSONResponse(w http.ResponseWriter, statusCode int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		log.Printf("Failed to encode JSON response: %v", err)
	}
}

func main() {
	log.Println("=== Starting Balance Microservice v2.1 ===")
	flag.Parse()

	if err := connectDB(); err != nil {
		log.Fatalf("Failed to initialize database: %v\n", err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			log.Printf("Error closing database connection: %v", err)
		}
	}()

	redisEnabledStr := os.Getenv("REDIS_ENABLED")
	redisEnabled = redisEnabledStr == "true" || redisEnabledStr == "1" || redisEnabledStr == "yes"

	if redisEnabled {
		log.Println("Redis Redlock mode ENABLED")
		if err := connectRedis(); err != nil {
			log.Fatalf("Failed to initialize redis: %v\n", err)
		}
		defer func() {
			if err := redisClient.Close(); err != nil {
				log.Printf("Error closing redis client: %v", err)
			}
		}()
	} else {
		log.Println("Redis Redlock mode DISABLED - using fallback (no distributed locking)")
	}

	if err := initDBTable(); err != nil {
		log.Printf("Warning: Failed to initialize database table: %v\n", err)
		log.Println("Continuing without table initialization...")
	}

	metrics.requestTime = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name: "prom_request_time",
		Help: "Time it has taken to retrieve the metrics",
	}, []string{"time"})

	if err := prometheus.Register(metrics.requestTime); err != nil {
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
