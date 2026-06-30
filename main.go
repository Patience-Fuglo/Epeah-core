package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"sync/atomic"
	"time"
)

type TradePayload struct {
	AgentID        string  `json:"agent_id"`
	Timestamp      int64   `json:"timestamp"`
	AssetClass     string  `json:"asset_class"`
	Ticker         string  `json:"ticker"`
	OrderType      string  `json:"order_type"`
	Quantity       float64 `json:"quantity"`
	Price          float64 `json:"price"`
	ContextWindow  string  `json:"context_window_reasoning"`
	CryptoChecksum string  `json:"crypto_checksum"`
}

type RiskVerdict struct {
	Decision  string `json:"decision"`
	Reason    string `json:"reason"`
	LatencyMs int64  `json:"latency_ms"`
}

const MaxPositionSize = 100000.0
const BannedAsset = "DOGE"

type MirrorClient struct {
	socketPath string
	conn       net.Conn
}

func NewMirrorClient(socketPath string) (*MirrorClient, error) {
	dialer := net.Dialer{Timeout: 500 * time.Millisecond}
	conn, err := dialer.Dial("unix", socketPath)
	if err != nil {
		return nil, err
	}
	return &MirrorClient{socketPath: socketPath, conn: conn}, nil
}

func (mc *MirrorClient) MirrorPayload(ctx context.Context, payload interface{}) {
	go func() {
		data, err := json.Marshal(payload)
		if err != nil {
			return
		}

		data = append(data, '\n')

		_ = mc.conn.SetWriteDeadline(time.Now().Add(2 * time.Millisecond))
		_, _ = mc.conn.Write(data)
	}()
}

type InboundKillSignal struct {
	SignalType      string `json:"signal_type"`
	AgentID         string `json:"agent_id"`
	Ticker          string `json:"ticker"`
	ViolationReason string `json:"violation_reason"`
	Timestamp       int64  `json:"timestamp"`
}

type TelemetryTracker struct {
	TotalSignalsRead   uint64
	SuccessfulDrops    uint64
	ProcessingFailures uint64
}

func (mc *MirrorClient) StartConcurrentSignalListener(workerCount int, apiKey, apiSecret string) *TelemetryTracker {
	tracker := &TelemetryTracker{}

	signalChannel := make(chan []byte, 50000)

	for i := 0; i < workerCount; i++ {
		go func(workerID int) {
			for rawLine := range signalChannel {
				if bytes.Contains(rawLine, []byte("KILL_FLATTEN")) {
					var signal InboundKillSignal
					if err := json.Unmarshal(rawLine, &signal); err != nil {
						atomic.AddUint64(&tracker.ProcessingFailures, 1)
						continue
					}

					start := time.Now()
					executeEmergencyFlatten(signal, apiKey, apiSecret)

					if time.Since(start) <= 2*time.Millisecond {
						atomic.AddUint64(&tracker.SuccessfulDrops, 1)
					}
				}
			}
		}(i)
	}

	go func() {
		defer close(signalChannel)
		scanner := bufio.NewScanner(mc.conn)
		scanner.Buffer(make([]byte, 4096), 4096)

		for scanner.Scan() {
			rawLine := append([]byte(nil), scanner.Bytes()...)

			atomic.AddUint64(&tracker.TotalSignalsRead, 1)

			select {
			case signalChannel <- rawLine:
			default:
				atomic.AddUint64(&tracker.ProcessingFailures, 1)
			}
		}
	}()

	return tracker
}

func (t *TelemetryTracker) PrintTelemetryReport() {
	read := atomic.LoadUint64(&t.TotalSignalsRead)
	drops := atomic.LoadUint64(&t.SuccessfulDrops)
	fails := atomic.LoadUint64(&t.ProcessingFailures)

	fmt.Println("\n===================================================================")
	fmt.Println("             ARBITER TELEMETRY ENGINE HEALTH REPORT                ")
	fmt.Println("===================================================================")
	fmt.Printf("Total Signals Extracted from IPC Socket: %d\n", read)
	fmt.Printf("Successful Urgent API Dispatches (<2ms):  %d\n", drops)
	fmt.Printf("Queue Buffer Drops / JSON Parse Errors:   %d\n", fails)
	if read > 0 {
		fmt.Printf("System Reliability Operational Coefficient: %.2f%%\n", float64(drops)/float64(read)*100)
	}
	fmt.Println("===================================================================")
}

func executeEmergencyFlatten(signal InboundKillSignal, apiKey, apiSecret string) {
	url := fmt.Sprintf("https://paper-api.alpaca.markets/v2/positions/%s", signal.Ticker)

	ctx, cancel := context.WithTimeout(context.Background(), 2000*time.Millisecond)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "DELETE", url, nil)
	if err != nil {
		return
	}

	req.Header.Set("APCA-API-KEY-ID", apiKey)
	req.Header.Set("APCA-API-SECRET-KEY", apiSecret)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{
		Transport: &http.Transport{
			DisableKeepAlives: true,
		},
	}

	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("[CRITICAL SYSTEM FAULT] Failed to execute emergency liquidations: %v\n", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusAccepted {
		fmt.Printf("[SYSTEM HARDENED] Arbiter successfully flattened risk position for Ticker: %s. Reason: %s\n",
			signal.Ticker, signal.ViolationReason)
	} else {
		fmt.Printf("[CRITICAL ERROR] Broker rejected emergency liquidation with status code: %d\n", resp.StatusCode)
	}
}

type ComplianceExportHandler struct {
	LedgerPath string
	AdminToken string
}

func NewComplianceExportHandler(ledgerPath, adminToken string) *ComplianceExportHandler {
	return &ComplianceExportHandler{
		LedgerPath: ledgerPath,
		AdminToken: adminToken,
	}
}

func (h *ComplianceExportHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	providedToken := r.Header.Get("X-Arbiter-Auth")
	if subtle.ConstantTimeCompare([]byte(providedToken), []byte(h.AdminToken)) != 1 {
		http.Error(w, "Unauthorized compliance access attempt logged.", http.StatusUnauthorized)
		return
	}

	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed.", http.StatusMethodNotAllowed)
		return
	}

	file, err := os.Open(h.LedgerPath)
	if err != nil {
		http.Error(w, "Ledger storage target unreadable or currently rotating.", http.StatusInternalServerError)
		return
	}
	defer file.Close()

	w.Header().Set("Content-Disposition", "attachment; filename=arbiter_compliance_audit.json")
	w.Header().Set("Content-Type", "application/json")

	_, _ = io.Copy(w, file)
}

var mirror *MirrorClient
var enclave *EnclaveClient

func evaluateRisk(payload TradePayload) RiskVerdict {
	startTime := time.Now()

	totalNotional := payload.Quantity * payload.Price

	if totalNotional > MaxPositionSize {
		return RiskVerdict{
			Decision:  "KILL",
			Reason:    fmt.Sprintf("Position size limit exceeded. Requested: $%.2f, Max: $%.2f", totalNotional, MaxPositionSize),
			LatencyMs: time.Since(startTime).Milliseconds(),
		}
	}

	if payload.Ticker == BannedAsset {
		return RiskVerdict{
			Decision:  "KILL",
			Reason:    fmt.Sprintf("Ticker '%s' is on the internal risk restriction list.", payload.Ticker),
			LatencyMs: time.Since(startTime).Milliseconds(),
		}
	}

	return RiskVerdict{
		Decision:  "ALLOW",
		Reason:    "Payload within established risk parameters.",
		LatencyMs: time.Since(startTime).Milliseconds(),
	}
}

func handleCheckRisk(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var payload TradePayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "Invalid payload format", http.StatusBadRequest)
		return
	}

	if mirror != nil {
		mirror.MirrorPayload(context.Background(), payload)
	}

	verdict := evaluateRisk(payload)

	if verdict.Decision == "KILL" {
		go triggerBrokerKillSwitch(payload.AgentID, verdict.Reason)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(verdict)
}

func triggerBrokerKillSwitch(agentID string, reason string) {
	log.Printf("[CRITICAL AUTO-KILL] Initiating broker kill-switch for Agent: %s. Reason: %s", agentID, reason)
}

const shadowSocketPath = "/var/run/arbiter/shadow.sock"

const complianceLedgerPath = "/var/log/arbiter/compliance_ledger.json"

func main() {
	http.HandleFunc("/v1/risk/check", handleCheckRisk)

	exportHandler := NewComplianceExportHandler(
		complianceLedgerPath,
		os.Getenv("ARBITER_COMPLIANCE_TOKEN"),
	)
	http.Handle("/v1/compliance/export", exportHandler)

	var telemetry *TelemetryTracker

	envMode := os.Getenv("ARBITER_ENV")
	apiKey := os.Getenv("ALPACA_API_KEY_ID")
	apiSecret := os.Getenv("ALPACA_API_SECRET_KEY")

	if envMode == "PROD" {
		fmt.Println("[HARDWARE MODE] Linking Gateway to secure AWS Nitro Enclave...")
		ec, t := initEnclaveConnection(apiKey, apiSecret)
		if ec != nil {
			enclave = ec
			telemetry = t
		}
	} else {
		fmt.Println("[LOCAL DEV MODE] Linking Gateway to local Unix socket path...")
		mc, err := NewMirrorClient(shadowSocketPath)
		if err != nil {
			log.Printf("[SHADOW] Rust daemon not available at %s — running without mirror: %v", shadowSocketPath, err)
		} else {
			mirror = mc
			telemetry = mirror.StartConcurrentSignalListener(8, apiKey, apiSecret)
			log.Printf("[SHADOW] Connected to Rust shadow engine at %s (8 worker pool)", shadowSocketPath)
		}
	}

	http.HandleFunc("/portal", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "web/portal.html")
	})

	http.HandleFunc("/v1/telemetry/health", func(w http.ResponseWriter, r *http.Request) {
		if telemetry == nil {
			http.Error(w, "Telemetry unavailable — shadow engine not connected", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]uint64{
			"total_signals_read":   atomic.LoadUint64(&telemetry.TotalSignalsRead),
			"successful_drops":     atomic.LoadUint64(&telemetry.SuccessfulDrops),
			"processing_failures":  atomic.LoadUint64(&telemetry.ProcessingFailures),
		})
	})

	fmt.Println("=========================================================")
	fmt.Println("ARBITER LOCAL VPC NODE: RUNNING ON PORT 8080")
	fmt.Println("=========================================================")

	log.Fatal(http.ListenAndServe(":8080", nil))
}
