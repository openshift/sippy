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

// sippyMutatingRequest performs a JSON request as the default e2e user, failing on any non-200
// status and decoding the response into responseData when it is non-nil.
func sippyMutatingRequest(method, path string, bodyData, responseData interface{}) error {
	status, body, err := SippyRequestWithHeaders(method, path, map[string]string{ForwardedUserHeader: DefaultUser}, bodyData)
	if err != nil {
		return err
	}
	if status != http.StatusOK {
		return fmt.Errorf("sippy API request failed with code %d: %s", status, string(body))
	}
	if responseData != nil {
		return json.Unmarshal(body, responseData)
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

const (
	// ForwardedUserHeader carries the authenticated principal, as the oauth proxy sets it in production.
	ForwardedUserHeader string = "X-Forwarded-User"
	// ActingForHeader lets a trusted delegator name the user it acts for.
	ActingForHeader string = "X-Acting-For"
	// DefaultUser is the principal the default mutating helpers authenticate as.
	DefaultUser string = "developer"

	// TrustedDelegator and SecondTrustedDelegator are the principals scripts/e2e.sh and
	// e2e-scripts/sippy-e2e-sippy-e2e-test-commands.sh pass to --trusted-delegator. These values
	// must be kept in sync.
	TrustedDelegator       string = "system:serviceaccount:sippy-e2e:sippy-mcp"
	SecondTrustedDelegator string = "sippy-e2e-second-delegator"
)

// SippyRequestWithHeaders sends a JSON request with exactly the supplied headers (no default
// X-Forwarded-User), returning the status code and body so tests can assert on rejections. It is the
// single place e2e helpers build and send mutating requests.
func SippyRequestWithHeaders(method, path string, headers map[string]string, bodyData interface{}) (int, []byte, error) {
	var bodyReader io.Reader
	if bodyData != nil {
		bodyBytes, err := json.Marshal(bodyData)
		if err != nil {
			return 0, nil, err
		}
		bodyReader = strings.NewReader(string(bodyBytes))
	}
	req, err := http.NewRequest(method, BuildE2EURL(path), bodyReader)
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	for name, value := range headers {
		req.Header.Set(name, value)
	}
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req) //nolint:gosec // G704: URL is constructed from test helper's hardcoded localhost base URL
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	return resp.StatusCode, body, err
}
