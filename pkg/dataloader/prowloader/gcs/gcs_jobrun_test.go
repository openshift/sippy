package gcs

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openshift/sippy/pkg/apis/junit"
)

func TestAppendJUnitXMLAggregatesSupportedDocuments(t *testing.T) {
	combined := &junit.TestSuites{}
	appendJUnitXML(combined, []byte(`<testsuites><testsuite name="openshift-tests"><testcase name="one"/></testsuite><testsuite name="openshift-tests"><testcase name="two"/></testsuite></testsuites>`), "first.xml", "logs/job/1")
	appendJUnitXML(combined, []byte(`<testsuite name="openshift-tests"><testcase name="three"><failure>failed output</failure></testcase></testsuite>`), "second.xml", "logs/job/1")
	require.Len(t, combined.Suites, 3)
	assert.Equal(t, "one", combined.Suites[0].TestCases[0].Name)
	assert.Equal(t, "two", combined.Suites[1].TestCases[0].Name)
	assert.Equal(t, "three", combined.Suites[2].TestCases[0].Name)
	assert.Equal(t, "failed output", combined.Suites[2].TestCases[0].FailureOutput.Output)
}

func TestAppendJUnitXMLToleratesMalformedAndEmptyDocuments(t *testing.T) {
	combined := &junit.TestSuites{}
	appendJUnitXML(combined, nil, "empty.xml", "logs/job/1")
	appendJUnitXML(combined, []byte(`<testsuite`), "malformed.xml", "logs/job/1")
	assert.Empty(t, combined.Suites)
}
