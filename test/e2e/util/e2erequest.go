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
	"time"
)

const (
	// Needs to match the releases in config/seed-views.yaml
	Release     = "4.22"
	BaseRelease = "4.21"

	// APIPort is the port e2e.sh launches the sippy API on. These values must be kept in sync.
	APIPort = 18080
)

func BuildE2EURL(apiPath string) string {
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
	return SippyGetAbsolute(BuildE2EURL(path), data)
}

func SippyGetAbsolute(url string, data interface{}) error {
	client := &http.Client{Timeout: 30 * time.Second}
	req, err := client.Get(url) //nolint:gosec // G107: URL is either from BuildE2EURL (hardcoded localhost) or a server-returned HATEOAS link in e2e tests
	if err != nil {
		return err
	}
	defer req.Body.Close()

	body, err := io.ReadAll(req.Body)
	if err != nil {
		return err
	}
	if req.StatusCode != http.StatusOK {
		return fmt.Errorf("sippy API request failed with code %d: %s", req.StatusCode, string(body))
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

	req, err := http.NewRequest(method, BuildE2EURL(path), bodyReader)
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Forwarded-User", "developer")
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req) //nolint:gosec // G704: URL is constructed from test helper's hardcoded localhost base URL
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("sippy API request failed with code %d: %s", resp.StatusCode, string(body))
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
