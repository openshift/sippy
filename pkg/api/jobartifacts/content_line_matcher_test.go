package jobartifacts

import (
	"bufio"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMatchLineWithStringMatcher(t *testing.T) {
	matcher := NewStringMatcher("test", 0, 0, maxFileMatches)
	reader := bufio.NewReader(strings.NewReader("this is a test line\nanother line\n"))
	matches, err := matcher.GetMatches(reader)
	assert.NoError(t, err)
	assert.Len(t, matches.(ContentLineMatches).Matches, 1)
	assert.Equal(t, "this is a test line\n", matches.(ContentLineMatches).Matches[0].Match)
}

func TestMatchLineWithRegexMatcher(t *testing.T) {
	regex := regexp.MustCompile(`test`)
	matcher := NewRegexMatcher(regex, 0, 0, 0)
	reader := bufio.NewReader(strings.NewReader("this is a test line\nanother line\n"))
	matches, err := matcher.GetMatches(reader)
	assert.NoError(t, err)
	assert.Len(t, matches.(ContentLineMatches).Matches, 1)
	assert.Equal(t, "this is a test line\n", matches.(ContentLineMatches).Matches[0].Match)
}

func TestMatchLineWithContext(t *testing.T) {
	matcher := NewStringMatcher("test", 1, 1, maxFileMatches)
	reader := bufio.NewReader(strings.NewReader("line before\nthis is a test line\nline after\nanother line\n"))
	matches, err := matcher.GetMatches(reader)
	assert.NoError(t, err)
	assert.Len(t, matches.(ContentLineMatches).Matches, 1)
	assert.Equal(t, "this is a test line\n", matches.(ContentLineMatches).Matches[0].Match)
	assert.Equal(t, []string{"line before\n"}, matches.(ContentLineMatches).Matches[0].Before)
	assert.Equal(t, []string{"line after\n"}, matches.(ContentLineMatches).Matches[0].After)
}

func TestMatchLineWithMultipleMatches(t *testing.T) {
	matcher := NewStringMatcher("test", 1, 1, maxFileMatches)
	reader := bufio.NewReader(strings.NewReader("this is a test line\nanother test line\n"))
	matches, err := matcher.GetMatches(reader)
	assert.NoError(t, err)
	lineMatches := matches.(ContentLineMatches).Matches
	assert.Len(t, lineMatches, 2)
	assert.Equal(t, "this is a test line\n", lineMatches[0].Match)
	assert.Equal(t, []string{"this is a test line\n"}, lineMatches[1].Before)
	assert.Equal(t, "another test line\n", lineMatches[1].Match)
	assert.Equal(t, []string{"another test line\n"}, lineMatches[0].After)
}

func TestMatchLineWithMaxFileMatches(t *testing.T) {
	matcher := NewStringMatcher("test", 0, 0, 3)
	reader := bufio.NewReader(strings.NewReader(strings.Repeat("this is a test line\n", 3+1)))
	matches, err := matcher.GetMatches(reader)
	assert.NoError(t, err)
	assert.Len(t, matches.(ContentLineMatches).Matches, 3)
	assert.True(t, matches.(ContentLineMatches).Truncated)
}
