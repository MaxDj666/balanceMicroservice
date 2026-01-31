package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"
)

type ZAPAlert struct {
	AlertName      string `json:"alert_name"`
	Risk           string `json:"risk"`
	Confidence     string `json:"confidence"`
	Description    string `json:"description"`
	Solution       string `json:"solution"`
	Reference      string `json:"reference"`
	CWEID          int    `json:"cwe_id"`
	WASCID         int    `json:"wasc_id"`
	URL            string `json:"url"`
	Param          string `json:"param"`
	Attack         string `json:"attack"`
	Evidence       string `json:"evidence"`
	UserID         int    `json:"user_id"`
	Timestamp      string `json:"timestamp"`
	RequestHeaders string `json:"request_headers"`
}

type ZAPNotifier struct {
	zapEnabled  bool
	zapAPIURL   string
	zapAPIKey   string
	httpClient  *http.Client
	serviceName string
}

func NewZAPNotifier() *ZAPNotifier {
	zapEnabled := os.Getenv("ZAP_ENABLED") == "true" || os.Getenv("ZAP_ENABLED") == "1"
	zapAPIURL := os.Getenv("ZAP_API_URL")
	if zapAPIURL == "" {
		zapAPIURL = "http://zaproxy:8090"
	}
	zapAPIKey := os.Getenv("ZAP_API_KEY")

	return &ZAPNotifier{
		zapEnabled: zapEnabled,
		zapAPIURL:  zapAPIURL,
		zapAPIKey:  zapAPIKey,
		httpClient: &http.Client{
			Timeout: 5 * time.Second,
		},
		serviceName: "balance-service",
	}
}

func (zn *ZAPNotifier) NotifyRaceConditionPrevented(userID int, endpoint, method string, metadata map[string]interface{}) {
	if !zn.zapEnabled {
		return
	}

	alert := ZAPAlert{
		AlertName:      "Race Condition Prevention Triggered",
		Risk:           "Informational",
		Confidence:     "Certain",
		Description:    "A race condition was detected and prevented by distributed locking mechanism",
		Solution:       "Ensure proper synchronization mechanisms are in place for concurrent access to shared resources",
		Reference:      "https://owasp.org/www-community/vulnerabilities/Race_Conditions",
		CWEID:          362, // CWE-362: Concurrent Execution using Shared Resource with Improper Synchronization
		WASCID:         33,  // WASC-33: HTTP Response Splitting
		URL:            fmt.Sprintf("%s://%s%s", "http", zn.serviceName, endpoint),
		Param:          "user_id",
		Attack:         fmt.Sprintf("Concurrent %s requests for user %d", method, userID),
		Evidence:       "Distributed lock acquisition failed (Redlock)",
		UserID:         userID,
		Timestamp:      time.Now().UTC().Format(time.RFC3339),
		RequestHeaders: fmt.Sprintf("Service: %s, Endpoint: %s", zn.serviceName, endpoint),
	}

	// Add metadata if provided
	if metadata != nil {
		if data, err := json.Marshal(metadata); err == nil {
			alert.Evidence += fmt.Sprintf(" | Metadata: %s", string(data))
		}
	}

	go func() {
		if err := zn.sendAlert(alert); err != nil {
			log.Printf("Failed to send ZAP alert: %v", err)
		} else {
			log.Printf("ZAP notified about prevented race condition for user %d", userID)
		}
	}()
}

func (zn *ZAPNotifier) sendAlert(alert ZAPAlert) error {
	// Method 1: Send to ZAP API endpoint (if ZAP is configured to receive alerts)
	apiURL := fmt.Sprintf("%s/JSON/alert/action/addAlert", zn.zapAPIURL)

	payload := map[string]interface{}{
		"apikey":      zn.zapAPIKey,
		"messageId":   "1",
		"risk":        alert.Risk,
		"confidence":  alert.Confidence,
		"description": alert.Description,
		"param":       alert.Param,
		"attack":      alert.Attack,
		"evidence":    alert.Evidence,
		"url":         alert.URL,
		"cweId":       alert.CWEID,
		"wascId":      alert.WASCID,
		"solution":    alert.Solution,
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal alert: %w", err)
	}

	req, err := http.NewRequest("POST", apiURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := zn.httpClient.Do(req)
	if err != nil {
		// If ZAP API is not available, try alternative method
		return zn.sendToZAPLogger(alert)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("zap api returned status: %d", resp.StatusCode)
	}

	return nil
}

func (zn *ZAPNotifier) sendToZAPLogger(alert ZAPAlert) error {
	// Method 2: Send to a custom endpoint that ZAP can monitor
	logURL := fmt.Sprintf("%s/JSON/core/action/newAlert", zn.zapAPIURL)

	payload := map[string]interface{}{
		"apikey": zn.zapAPIKey,
		"alert":  alert,
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", logURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := zn.httpClient.Do(req)
	if err != nil {
		// Last resort: log to stdout in structured format
		log.Printf("ZAP_ALERT: %+v", alert)
		return err
	}
	defer resp.Body.Close()

	return nil
}
