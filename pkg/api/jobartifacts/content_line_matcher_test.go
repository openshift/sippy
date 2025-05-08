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
	assert.Len(t, matches.ContentLineMatches.Matches, 1)
	assert.Equal(t, "this is a test line\n", matches.ContentLineMatches.Matches[0].Match)
}

func TestMatchLineWithRegexMatcher(t *testing.T) {
	regex := regexp.MustCompile(`test`)
	matcher := NewRegexMatcher(regex, 0, 0, 0)
	reader := bufio.NewReader(strings.NewReader("this is a test line\nanother line\n"))
	matches, err := matcher.GetMatches(reader)
	assert.NoError(t, err)
	assert.Len(t, matches.ContentLineMatches.Matches, 1)
	assert.Equal(t, "this is a test line\n", matches.ContentLineMatches.Matches[0].Match)
}

func TestMatchLineWithContext(t *testing.T) {
	matcher := NewStringMatcher("test", 1, 1, maxFileMatches)
	reader := bufio.NewReader(strings.NewReader("line before\nthis is a test line\nline after\nanother line\n"))
	matches, err := matcher.GetMatches(reader)
	assert.NoError(t, err)
	assert.Len(t, matches.ContentLineMatches.Matches, 1)
	assert.Equal(t, "this is a test line\n", matches.ContentLineMatches.Matches[0].Match)
	assert.Equal(t, []string{"line before\n"}, matches.ContentLineMatches.Matches[0].Before)
	// although we said to capture 1 line after, we actually capture up to the max and then trim them later
	assert.Equal(t, []string{"line after\n", "another line\n"}, matches.ContentLineMatches.Matches[0].After)
}

func TestLinesAfterMatches(t *testing.T) {
	matcher := NewStringMatcher("test", 0, 2, maxFileMatches)
	reader := bufio.NewReader(strings.NewReader("test before\ntest line\ntest after\nanother line\n"))
	matches, err := matcher.GetMatches(reader)
	assert.NoError(t, err)
	m := matches.ContentLineMatches.Matches
	assert.Len(t, m, 3)
	// although we said to capture 2 lines after, we actually capture up to the max and then trim them later
	assert.Equal(t, 3, len(m[0].After))
	assert.Equal(t, 2, len(m[1].After))
	assert.Equal(t, 1, len(m[2].After))
}

func TestMatchLineWithMultipleMatches(t *testing.T) {
	matcher := NewStringMatcher("test", 1, 1, maxFileMatches)
	reader := bufio.NewReader(strings.NewReader("this is a test line\nanother test line\n"))
	matches, err := matcher.GetMatches(reader)
	assert.NoError(t, err)
	lineMatches := matches.ContentLineMatches.Matches
	assert.Len(t, lineMatches, 2)
	assert.Equal(t, "this is a test line\n", lineMatches[0].Match)
	assert.Equal(t, []string{"this is a test line\n"}, lineMatches[1].Before)
	assert.Equal(t, "another test line\n", lineMatches[1].Match)
	assert.Equal(t, []string{"another test line\n"}, lineMatches[0].After)
}

func TestMatchLineWithMaxFileMatches(t *testing.T) {
	matcher := NewStringMatcher("test", 0, 0, 3)
	reader := bufio.NewReader(strings.NewReader(strings.Repeat("this is a test line\n", maxFileMatches+1)))
	matches, err := matcher.GetMatches(reader)
	assert.NoError(t, err)
	// we return up to the max matches even though we only asked for 3 (matches will be trimmed in post-processing)
	assert.Len(t, matches.ContentLineMatches.Matches, maxFileMatches)
	assert.True(t, matches.ContentLineMatches.Truncated)
}

func TestPostProcessMatch(t *testing.T) {
	matcher := &lineMatcher{
		contextBefore:  1,
		contextAfter:   1,
		maxFileMatches: 2,
	}

	fullArtifact := JobRunArtifact{
		MatchedContent: MatchedContent{&ContentLineMatches{
			Matches: []ContentLineMatch{
				{
					Before: []string{"line before 1\n", "line before 2\n"},
					Match:  "this is a test line\n",
					After:  []string{"line after 1\n", "line after 2\n"},
				},
				{
					Before: []string{"line before 3\n"},
					Match:  "another test line\n",
					After:  []string{"line after 3\n", "line after 4\n"},
				},
				{
					Before: []string{"line before 4\n"},
					Match:  "extra test line\n",
					After:  []string{"line after 5\n"},
				},
			},
			Truncated: false,
		}},
	}

	processedArtifact := matcher.PostProcessMatch(fullArtifact)
	processedMatches := processedArtifact.MatchedContent.ContentLineMatches

	assert.Len(t, processedMatches.Matches, 2)
	assert.True(t, processedMatches.Truncated)

	assert.Equal(t, []string{"line before 2\n"}, processedMatches.Matches[0].Before)
	assert.Equal(t, "this is a test line\n", processedMatches.Matches[0].Match)
	assert.Equal(t, []string{"line after 1\n"}, processedMatches.Matches[0].After)

	assert.Equal(t, []string{"line before 3\n"}, processedMatches.Matches[1].Before)
	assert.Equal(t, "another test line\n", processedMatches.Matches[1].Match)
	assert.Equal(t, []string{"line after 3\n"}, processedMatches.Matches[1].After)
}
