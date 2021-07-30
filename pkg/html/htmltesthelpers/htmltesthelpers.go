package htmltesthelpers

import (
	"bytes"
	"fmt"
	"io"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

// Collection of helpers and fixtures used for HTML tests

type recordedFunc func(*httptest.ResponseRecorder)

func AssertHTTPResponseContains(t *testing.T, expectedContents []string, testFunc recordedFunc) {
	t.Helper()

	// Because several of the HTML functions write to an http.ResponseWriter, we
	// use an httptest.ResponseRecorder to read the written response.
	recorder := httptest.NewRecorder()

	testFunc(recorder)

	result := recorder.Result()
	defer result.Body.Close()

	buf := bytes.NewBufferString("")

	if _, err := io.Copy(buf, result.Body); err != nil {
		t.Fatal(err)
	}

	contents := buf.String()

	fmt.Println(contents)

	for _, item := range expectedContents {
		if !strings.Contains(contents, item) {
			t.Errorf("expected result to contain: %s", item)
		}
	}
}

func WriteHTMLToFile(filename string, testFunc recordedFunc) {
	recorder := httptest.NewRecorder()

	testFunc(recorder)

	result := recorder.Result()
	defer result.Body.Close()

	file, err := os.Create(filename)
	if err != nil {
		panic(err)
	}

	defer file.Close()

	if _, err := io.Copy(file, result.Body); err != nil {
		panic(err)
	}
}
