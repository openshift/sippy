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
	CacheKey() string
}

type lineMatcher struct {
	matcher        ContentLineMatcher
	contextBefore  int
	contextAfter   int
	maxFileMatches int
}

func (m *lineMatcher) GetContentTemplate() interface{} {
	return ContentLineMatches{}
}

func (m *lineMatcher) GetCacheKey() string {
	return m.matcher.CacheKey() // ignore match/context limits for purposes of caching - cache everything we could need
}

func (m *lineMatcher) PostProcessMatch(fullArtifact JobRunArtifact) JobRunArtifact {
	artifact := fullArtifact
	if fullArtifact.MatchedContent.ContentLineMatches == nil {
		return artifact // no matches to process
	}
	clMatches := fullArtifact.MatchedContent.ContentLineMatches
	trimmed := &ContentLineMatches{
		Matches:   []ContentLineMatch{},
		Truncated: clMatches.Truncated,
	}

	// if we have more matches than requested, trim them down
	if len(clMatches.Matches) > m.maxFileMatches {
		clMatches.Matches = clMatches.Matches[:m.maxFileMatches]
		trimmed.Truncated = true
	}
	// if we have more context lines than requested, trim them down
	for _, match := range clMatches.Matches {
		beforeLines := match.Before
		if len(beforeLines) > m.contextBefore {
			beforeLines = beforeLines[len(beforeLines)-m.contextBefore:]
		}
		afterLines := match.After
		if len(afterLines) > m.contextAfter {
			afterLines = afterLines[:m.contextAfter]
		}
		trimmed.Matches = append(trimmed.Matches, ContentLineMatch{
			Before: beforeLines,
			Match:  match.Match,
			After:  afterLines,
		})
	}
	artifact.MatchedContent.ContentLineMatches = trimmed
	return artifact
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

func (slm stringLineMatcher) CacheKey() string {
	return "stringLineMatcher: " + slm.matchString
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

func (rlm regexLineMatcher) CacheKey() string {
	return "regexLineMatcher: " + rlm.matchRegex.String()
}

func (rlm regexLineMatcher) MatchLine(line string) bool {
	return rlm.matchRegex.MatchString(line)
}

// GetMatches reads lines from the provided reader and returns a ContentLineMatches and possibly an error
func (m *lineMatcher) GetMatches(reader *bufio.Reader) (MatchedContent, error) {
	matches := &ContentLineMatches{
		// NOTE: allocate enough that the slice never moves; otherwise appending causes reallocation
		// which invalidates pendingMatches pointers to the slice contents.
		Matches:   make([]ContentLineMatch, 0, maxFileMatches),
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
			return MatchedContent{}, err
		}

		// record this line in previous matches' "after" context
		for _, match := range pendingMatches {
			match.After = append(match.After, line)
		}
		if len(pendingMatches) > 0 && len(pendingMatches[0].After) >= maxFileMatches {
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
			// take a pointer to the match for later addition of "After" context
			pendingMatches = append(pendingMatches, &matches.Matches[len(matches.Matches)-1])
			if len(matches.Matches) >= maxFileMatches {
				seenMaxLines = true
			}
		}

		// record the line for later matches' "before" context
		if beforeLines = append(beforeLines, line); len(beforeLines) > maxFileMatches {
			beforeLines = beforeLines[1:] // remove lines outside the "before" window
		}
	}
	if len(matches.Matches) == 0 {
		return MatchedContent{}, nil // no matches found
	}
	return MatchedContent{ContentLineMatches: matches}, nil
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
