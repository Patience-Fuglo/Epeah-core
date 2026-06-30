//go:build enclave

package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"os"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/mdlayher/vsock"
)

type EnclaveClient struct {
	enclaveCID  uint32
	enclavePort uint32
	conn        net.Conn
}

func NewEnclaveClient(cid, port uint32) (*EnclaveClient, error) {
	conn, err := vsock.Dial(cid, port, nil)
	if err != nil {
		return nil, fmt.Errorf("secure enclave hardware link failed: %w", err)
	}
	return &EnclaveClient{
		enclaveCID:  cid,
		enclavePort: port,
		conn:        conn,
	}, nil
}

func (ec *EnclaveClient) MirrorPayloadToEnclave(ctx context.Context, payload interface{}) {
	go func() {
		data, err := json.Marshal(payload)
		if err != nil {
			return
		}
		data = append(data, '\n')

		_ = ec.conn.SetWriteDeadline(time.Now().Add(2 * time.Millisecond))
		_, _ = ec.conn.Write(data)
	}()
}

func (ec *EnclaveClient) StartEnclaveSignalListener(workerCount int, apiKey, apiSecret string) *TelemetryTracker {
	tracker := &TelemetryTracker{}
	signalChannel := make(chan []byte, 50000)

	for i := 0; i < workerCount; i++ {
		go func() {
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
		}()
	}

	go func() {
		defer close(signalChannel)
		scanner := bufio.NewScanner(ec.conn)
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

func initEnclaveConnection(apiKey, apiSecret string) (*EnclaveClient, *TelemetryTracker) {
	cid, _ := strconv.ParseUint(os.Getenv("ARBITER_ENCLAVE_CID"), 10, 32)
	if cid == 0 {
		cid = 16
	}
	ec, err := NewEnclaveClient(uint32(cid), 5005)
	if err != nil {
		log.Printf("[ENCLAVE] Nitro enclave not reachable at CID %d:5005 — running without mirror: %v", cid, err)
		return nil, nil
	}
	telemetry := ec.StartEnclaveSignalListener(16, apiKey, apiSecret)
	log.Printf("[ENCLAVE] Connected to Rust shadow engine via vsock CID %d:5005 (16 worker pool)", cid)

	go func() {
		for {
			time.Sleep(30 * time.Second)
			telemetry.PrintTelemetryReport()
		}
	}()

	return ec, telemetry
}
