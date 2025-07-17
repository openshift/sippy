package util

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
)

const (
	// Needs to match what we import in the e2e sh scripts and the
	Release = "4.14"

	// APIPort is the port e2e.sh launches the sippy API on. These values must be kept in sync.
	APIPort = 18080
)

func buildURL(apiPath string) string {
	envSippyAPIPort := os.Getenv("SIPPY_API_PORT")
	envSippyEndpoint := os.Getenv("SIPPY_ENDPOINT")

	var port = APIPort
	if len(envSippyAPIPort) > 0 {
		val, err := strconv.Atoi(envSippyAPIPort)
		if err == nil {
			port = val
		}
	}
	if len(envSippyEndpoint) == 0 {
		envSippyEndpoint = "localhost"
	}
	url := fmt.Sprintf("http://%s%s", net.JoinHostPort(envSippyEndpoint, strconv.Itoa(port)), apiPath)
	return url
}

func SippyGet(path string, data interface{}) error {
	req, err := http.Get(buildURL(path))
	if err != nil {
		return err
	}

	body, err := io.ReadAll(req.Body)
	if err != nil {
		return err
	}
	if req.StatusCode != http.StatusOK {
		return fmt.Errorf("Sippy API request failed with code %d: %s", req.StatusCode, string(body))
	}
	err = json.Unmarshal(body, data)
	if err != nil {
		return err
	}
	return nil
}

// sippyMutatingRequest performs a generic HTTP request with JSON body to the Sippy API
func sippyMutatingRequest(method, path string, bodyData, responseData interface{}) error {
	var bodyReader io.Reader
	if bodyData != nil {
		bodyBytes, err := json.Marshal(bodyData)
		if err != nil {
			return err
		}
		bodyReader = strings.NewReader(string(bodyBytes))
	}

	req, err := http.NewRequest(method, buildURL(path), bodyReader)
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Forwarded-User", "developer")
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("Sippy API request failed with code %d: %s", resp.StatusCode, string(body))
	}

	if responseData != nil {
		err = json.Unmarshal(body, responseData)
		if err != nil {
			return err
		}
	}
	return nil
}

func SippyPost(path string, bodyData, responseData interface{}) error {
	return sippyMutatingRequest(http.MethodPost, path, bodyData, responseData)
}

func SippyPut(path string, bodyData, responseData interface{}) error {
	return sippyMutatingRequest(http.MethodPut, path, bodyData, responseData)
}

func SippyDelete(path string) error {
	return sippyMutatingRequest(http.MethodDelete, path, nil, nil)
}
