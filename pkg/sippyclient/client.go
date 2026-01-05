package sippyclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	DefaultServerURL = "http://localhost:8080"
)

// Client is a client for the Sippy API. It is intended to be imported by golang-based clients
// (e.g. cloud functions and cmdline tools) that need to access sippy APIs,
// which is why there may be no uses of these methods within sippy itself.
type Client struct {
	BaseURL    string
	HTTPClient *http.Client
	Token      string
}

// Option is a functional option for configuring the client
type Option func(*Client)

// WithServerURL sets the server URL for the client
func WithServerURL(url string) Option {
	return func(c *Client) {
		c.BaseURL = strings.TrimSuffix(url, "/")
	}
}

// WithToken sets the authentication token for the client
func WithToken(token string) Option {
	return func(c *Client) {
		c.Token = token
	}
}

// WithHTTPClient sets a custom HTTP client
func WithHTTPClient(httpClient *http.Client) Option {
	return func(c *Client) {
		c.HTTPClient = httpClient
	}
}

// New creates a new Sippy API client
func New(opts ...Option) *Client {
	client := &Client{
		BaseURL: DefaultServerURL,
		HTTPClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}

	for _, opt := range opts {
		opt(client)
	}

	return client
}

// Get performs a GET request to the specified path and decodes the JSON response
func (c *Client) Get(ctx context.Context, path string, result interface{}) error {
	url := c.BaseURL + path

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("unexpected status code %d: %s", resp.StatusCode, string(body))
	}

	if err := json.NewDecoder(resp.Body).Decode(result); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}

	return nil
}

// Post performs a POST request to the specified path with the given body
func (c *Client) Post(ctx context.Context, path string, body, result interface{}) error {
	return c.doJSONRequest(ctx, http.MethodPost, path, body, result)
}

// Put performs a PUT request to the specified path with the given body
func (c *Client) Put(ctx context.Context, path string, body, result interface{}) error {
	return c.doJSONRequest(ctx, http.MethodPut, path, body, result)
}

// Delete performs a DELETE request to the specified path
func (c *Client) Delete(ctx context.Context, path string) error {
	url := c.BaseURL + path

	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("unexpected status code %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// doJSONRequest performs a request with a JSON body
func (c *Client) doJSONRequest(ctx context.Context, method, path string, body, result interface{}) error {
	url := c.BaseURL + path

	var bodyReader io.Reader
	if body != nil {
		bodyBytes, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("failed to marshal request body: %w", err)
		}
		bodyReader = bytes.NewReader(bodyBytes)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("unexpected status code %d: %s", resp.StatusCode, string(respBody))
	}

	if result != nil {
		if err := json.NewDecoder(resp.Body).Decode(result); err != nil {
			return fmt.Errorf("failed to decode response: %w", err)
		}
	}

	return nil
}
