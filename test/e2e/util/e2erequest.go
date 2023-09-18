package util

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strconv"
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
	return fmt.Sprintf("http://%s%s", net.JoinHostPort(envSippyEndpoint, strconv.Itoa(port)), apiPath)
}

func SippyRequest(path string, data interface{}) error {
	res, err := http.Get(buildURL(path))
	if err != nil {
		return err
	}

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return err
	}
	err = json.Unmarshal(body, data)
	if err != nil {
		return err
	}
	return nil
}
