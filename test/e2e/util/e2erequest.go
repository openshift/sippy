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
	// config/e2e-openshift.yaml.
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
	err = json.Unmarshal(body, data)
	if err != nil {
		return err
	}
	return nil
}

func SippyPost(path string, bodyData interface{}, responseData interface{}) error {
	bodyBytes, err := json.Marshal(bodyData)
	req, err := http.Post(buildURL(path), "application/json", strings.NewReader(string(bodyBytes)))
	if err != nil {
		return err
	}

	body, err := io.ReadAll(req.Body)
	if err != nil {
		return err
	}
	err = json.Unmarshal(body, responseData)
	if err != nil {
		return err
	}
	return nil
}

func SippyPut(path string, bodyData interface{}, responseData interface{}) error {
	bodyBytes, err := json.Marshal(bodyData)
	req, err := http.NewRequest(http.MethodPut, buildURL(path), strings.NewReader(string(bodyBytes)))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("Sippy API returned status code %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	err = json.Unmarshal(body, responseData)
	if err != nil {
		return err
	}
	return nil
}
