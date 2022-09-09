package util

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
)

const (
	// Release is the usually somewhat older release we're importing during e2e runs (as it has far less data)
	// to then test sippy. Needs to match what we import in the e2e sh scripts.
	Release = "4.7"

	// APIPort is the port e2e.sh launches the sippy API on. These values must be kept in sync.
	APIPort = 18080
)

func buildURL(apiPath string) string {
	return fmt.Sprintf("http://localhost:%d%s", APIPort, apiPath)
}

func SippyRequest(path string, data interface{}) error {
	res, err := http.Get(buildURL(path))
	if err != nil {
		return err
	}

	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return err
	}
	err = json.Unmarshal(body, data)
	if err != nil {
		return err
	}
	return nil
}
