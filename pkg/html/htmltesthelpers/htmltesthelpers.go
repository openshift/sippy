package htmltesthelpers

import (
	"bytes"
	"io"
	"net/http/httptest"
	"strings"
	"testing"
)

// Collection of helpers and fixtures used for HTML tests

type recordedFunc func(*httptest.ResponseRecorder)

func AssertHTTPResponseContains(t *testing.T, expectedContents []string, testFunc recordedFunc) {
	t.Helper()

	contentBuf, err := getHTTPResponseContents(testFunc)
	if err != nil {
		t.Error(err)
	}

	contents := contentBuf.String()

	for _, item := range expectedContents {
		if !strings.Contains(contents, item) {
			t.Errorf("expected result to contain: %s", item)
		}
	}
}

func PrintHTML(t *testing.T, testFunc recordedFunc) {
	contentBuf, err := getHTTPResponseContents(testFunc)
	if err != nil {
		t.Error(err)
	}

	t.Log(contentBuf.String())
}

func getHTTPResponseContents(testFunc recordedFunc) (*bytes.Buffer, error) {
	// Because several of the HTML functions write to an http.ResponseWriter, we
	// use an httptest.ResponseRecorder to read the written response.
	recorder := httptest.NewRecorder()

	testFunc(recorder)

	result := recorder.Result()
	defer result.Body.Close()

	buf := bytes.NewBufferString("")

	if _, err := io.Copy(buf, result.Body); err != nil {
		return buf, err
	}

	return buf, nil
}
