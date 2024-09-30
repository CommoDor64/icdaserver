package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDASService_JSONRPC(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(jsonRPCHandler))
	defer server.Close()

	startRequest := map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  "das_startChunkedStore",
		"params":  map[string]interface{}{},
		"id":      1,
	}

	sendChunkRequest := map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  "das_sendChunk",
		"params": map[string]interface{}{
			"chunkId": 1,
			"message": "This is a test chunk",
		},
		"id": 2,
	}

	commitRequest := map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  "das_commitChunkedStore",
		"params": map[string]interface{}{
			"batchId": 1,
		},
		"id": 3,
	}

	startResponse := sendJSONRPCRequest(t, server.URL, startRequest)
	batchId, ok := startResponse["result"].(map[string]interface{})["batchId"].(float64)
	if !ok || batchId == 0 {
		t.Fatalf("Expected a valid batchId in start chunked store response")
	}

	sendChunkRequest["params"].(map[string]interface{})["batchId"] = batchId

	sendChunkResponse := sendJSONRPCRequest(t, server.URL, sendChunkRequest)
	if sendChunkResponse["error"] != nil {
		t.Fatalf("Failed to send chunk: %v", sendChunkResponse["error"])
	}

	commitRequest["params"].(map[string]interface{})["batchId"] = batchId

	commitResponse := sendJSONRPCRequest(t, server.URL, commitRequest)
	if commitResponse["error"] != nil {
		t.Fatalf("Failed to commit chunked store: %v", commitResponse["error"])
	}

	dataHash, ok := commitResponse["result"].(map[string]interface{})["dataHash"].(string)
	if !ok || dataHash == "" {
		t.Fatalf("Expected a valid data hash in commit chunked store response")
	}
}

func sendJSONRPCRequest(t *testing.T, url string, request map[string]interface{}) map[string]interface{} {
	requestBody, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("Failed to marshal request: %v", err)
	}

	resp, err := http.Post(url, "application/json", bytes.NewBuffer(requestBody))
	if err != nil {
		t.Fatalf("Failed to send request: %v", err)
	}
	defer resp.Body.Close()

	var response map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	return response
}
