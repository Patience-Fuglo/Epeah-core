//go:build !enclave

package main

type EnclaveClient struct{}

func initEnclaveConnection(apiKey, apiSecret string) (*EnclaveClient, *TelemetryTracker) {
	return nil, nil
}
