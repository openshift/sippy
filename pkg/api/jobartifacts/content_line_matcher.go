package jobartifacts

import (
	"bufio"
	"errors"
	"io"
	"net/http"
	"regexp"
	"strings"

	"github.com/openshift/sippy/pkg/util/param"
	"github.com/openshift/sippy/pkg/util/sets"
)

// ContentLineMatcher is an interface for matching lines in a text artifact file.
type ContentLineMatcher interface {
	MatchLine(line string) bool
}

type lineMatcher struct {
	matcher        ContentLineMatcher
	contextBefore  int
	contextAfter   int
	maxFileMatches int
}

// NewStringMatcher creates a ContentMatcher that matches lines containing the specified string
// as well as (optionally) recording context lines for the match.
func NewStringMatcher(matchString string, contextBefore, contextAfter, maxFileMatches int) ContentMatcher {
	return &lineMatcher{
		matcher:        stringLineMatcher{matchString: matchString},
		contextBefore:  contextBefore,
		contextAfter:   contextAfter,
		maxFileMatches: maxFileMatches,
	}
}

type stringLineMatcher struct {
	matchString string
}

func (slm stringLineMatcher) MatchLine(line string) bool {
	return strings.Contains(line, slm.matchString)
}

// NewRegexMatcher creates a ContentMatcher that matches lines against a regex
// as well as (optionally) recording context lines for the match.
func NewRegexMatcher(matchRegex *regexp.Regexp, contextBefore, contextAfter, maxMatches int) ContentMatcher {
	return &lineMatcher{
		matcher:        regexLineMatcher{matchRegex: matchRegex},
		contextBefore:  contextBefore,
		contextAfter:   contextAfter,
		maxFileMatches: maxMatches,
	}
}

type regexLineMatcher struct {
	matchRegex *regexp.Regexp
}

func (rlm regexLineMatcher) MatchLine(line string) bool {
	return rlm.matchRegex.MatchString(line)
}

// GetMatches reads lines from the provided reader and returns a ContentLineMatches and possibly an error
func (m *lineMatcher) GetMatches(reader *bufio.Reader) (interface{}, error) {
	matches := ContentLineMatches{
		Matches:   []ContentLineMatch{},
		Truncated: false,
	}
	beforeLines := make([]string, 0, m.contextBefore) // lines before the match for context
	var pendingMatches []*ContentLineMatch            // matches waiting to record more "after" context lines
	seenMaxLines := false

	for !matches.Truncated || len(pendingMatches) > 0 {
		// scan the file line by line; however, if lines are too long, ReadString
		// breaks them up instead of failing, so these may not be complete lines.
		line, err := reader.ReadString('\n')
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return matches, err
		}

		// record this line in previous matches' "after" context
		for _, match := range pendingMatches {
			match.After = append(match.After, line)
		}
		if len(pendingMatches) > 0 && len(pendingMatches[0].After) >= m.contextAfter {
			pendingMatches = pendingMatches[1:] // remove from pending once "after" context is full
		}

		// record the match (or the truncation if we already have enough)
		if m.matcher.MatchLine(line) {
			if seenMaxLines {
				matches.Truncated = true
				continue
			}
			matches.Matches = append(matches.Matches, ContentLineMatch{
				Before: beforeLines,
				Match:  line,
				After:  []string{},
			})
			if m.contextAfter > 0 { // if we want "after" context, track matches waiting for it
				pendingMatches = append(pendingMatches, &matches.Matches[len(matches.Matches)-1])
			}
			if len(matches.Matches) >= m.maxFileMatches {
				seenMaxLines = true
			}
		}

		// record the line for later matches' "before" context
		if beforeLines = append(beforeLines, line); len(beforeLines) > m.contextBefore {
			beforeLines = beforeLines[1:] // remove lines outside the "before" window
		}
	}
	return matches, nil
}

// ParseLineMatcherParams parses common request parameters used to construct a ContentLineMatcher.
func ParseLineMatcherParams(req *http.Request) (beforeContext, afterContext, maxMatches int, errs map[string]error) {
	errs = map[string]error{}
	params := map[string]int{}
	for name := range sets.NewString("beforeContext", "afterContext", "maxFileMatches") {
		if value, err := param.ReadUint(req, name, maxFileMatches); err != nil {
			errs[name] = err
		} else {
			params[name] = value
		}
	}
	beforeContext = params["beforeContext"]
	afterContext = params["afterContext"]
	maxMatches = params["maxFileMatches"]
	if maxMatches == 0 {
		maxMatches = maxFileMatches
	}
	return
}
