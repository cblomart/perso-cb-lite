package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
)

const baseURL = "https://api.coinbase.com/api/v3/brokerage"

// makeRequest makes an authenticated HTTP request to the Coinbase API
func (c *CoinbaseClient) makeRequest(ctx context.Context, method, endpoint string, body interface{}) ([]byte, error) {
	// Track request count
	atomic.AddInt64(&c.requestCount, 1)

	// Extract the path for JWT URI construction (exclude query parameters)
	path := endpoint
	if idx := strings.Index(endpoint, "?"); idx != -1 {
		path = endpoint[:idx]
	}

	fullPath := "/api/v3/brokerage" + path

	jwt, err := c.createJWT(ctx, method, fullPath)
	if err != nil {
		return nil, fmt.Errorf("failed to create JWT: %w", err)
	}

	// Prepare request body
	var bodyReader io.Reader
	var bodyBytes []byte
	if body != nil {
		bodyBytes, err = json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request body: %w", err)
		}
		bodyReader = bytes.NewReader(bodyBytes)
	}

	// Create request
	url := baseURL + endpoint
	req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	req.Header.Set("Authorization", "Bearer "+jwt)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	// Debug: Log request details (skip for health checks)
	if os.Getenv("LOG_LEVEL") == "DEBUG" && endpoint != "/health" && ctx.Value(healthCheckKey) != true {
		c.logger.Printf("=== REQUEST DUMP ===")
		c.logger.Printf("Method: %s", method)
		c.logger.Printf("URL: %s", url)
		c.logger.Printf("Headers:")
		for key, values := range req.Header {
			for _, value := range values {
				if key == "Authorization" {
					c.logger.Printf("  %s: Bearer %s", key, jwt)
				} else {
					c.logger.Printf("  %s: %s", key, value)
				}
			}
		}
		if body != nil {
			bodyPretty, _ := json.MarshalIndent(body, "", "  ")
			c.logger.Printf("Body: %s", string(bodyPretty))
		} else {
			c.logger.Printf("Body: <empty>")
		}
		c.logger.Printf("==================")
	}

	// Make request using our optimized HTTP client
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to make request: %w", err)
	}
	defer resp.Body.Close()

	// Read response
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	// Debug: Log response details (skip for health checks)
	if os.Getenv("LOG_LEVEL") == "DEBUG" && endpoint != "/health" && ctx.Value(healthCheckKey) != true {
		c.logger.Printf("=== RESPONSE DUMP ===")
		c.logger.Printf("Status: %s", resp.Status)
		c.logger.Printf("Status Code: %d", resp.StatusCode)
		c.logger.Printf("Headers:")
		for key, values := range resp.Header {
			for _, value := range values {
				c.logger.Printf("  %s: %s", key, value)
			}
		}

		// Read and log response body if present
		if resp.Body != nil {
			bodyBytes, err := io.ReadAll(resp.Body)
			if err == nil && len(bodyBytes) > 0 {
				c.logger.Printf("Body: %s", string(bodyBytes))
			}
			// Recreate the response body for potential future use
			resp.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
		}
		c.logger.Printf("==================")
	}

	// Check status code
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	return respBody, nil
}
