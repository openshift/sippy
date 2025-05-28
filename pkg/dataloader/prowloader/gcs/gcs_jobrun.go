package gcs

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"regexp"

	"cloud.google.com/go/storage"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"google.golang.org/api/iterator"

	"github.com/openshift/sippy/pkg/apis/junit"
)

const TestFailureSummaryFilePrefix = "risk-analysis"
const ClusterDataFilePrefix = "cluster-data_"
const JunitRegExStr = "\\/.*junit.*xml"
const intervalFilesRegExStr = "\\/(e2e-events|e2e-timelines).*json"

var (
	defaultRiskAnalysisSummaryFileRegEx *regexp.Regexp
	defaultClusterDataFileRegEx         *regexp.Regexp
	defaultJunitFileRegEx               *regexp.Regexp
	intervalFilesRegex                  *regexp.Regexp
)

func GetDefaultRiskAnalysisSummaryFile() *regexp.Regexp {
	if defaultRiskAnalysisSummaryFileRegEx == nil {
		defaultRiskAnalysisSummaryFileRegEx = regexp.MustCompile(fmt.Sprintf("%s.json", TestFailureSummaryFilePrefix))
	}
	return defaultRiskAnalysisSummaryFileRegEx
}

func GetDefaultClusterDataFile() *regexp.Regexp {
	if defaultClusterDataFileRegEx == nil {
		defaultClusterDataFileRegEx = regexp.MustCompile(fmt.Sprintf("%s.*json", ClusterDataFilePrefix))
	}
	return defaultClusterDataFileRegEx
}

func GetDefaultJunitFile() *regexp.Regexp {
	if defaultJunitFileRegEx == nil {
		defaultJunitFileRegEx = regexp.MustCompile(JunitRegExStr)
	}
	return defaultJunitFileRegEx
}

func GetIntervalFile() *regexp.Regexp {
	if intervalFilesRegex == nil {
		intervalFilesRegex = regexp.MustCompile(intervalFilesRegExStr)
	}
	return intervalFilesRegex
}

type GCSJobRun struct {
	// retrieval mechanisms
	bkt *storage.BucketHandle

	gcsProwJobPath string
	gcsJunitPaths  []string

	pathToContent map[string][]byte
}

func NewGCSJobRun(bkt *storage.BucketHandle, path string) *GCSJobRun {
	return &GCSJobRun{
		bkt:            bkt,
		gcsProwJobPath: path,
	}
}

func (j *GCSJobRun) SetGCSJunitPaths(paths []string) {
	j.gcsJunitPaths = paths
}

func (j *GCSJobRun) GetGCSJunitPaths() ([]string, error) {
	if len(j.gcsJunitPaths) == 0 {
		matches, err := j.FindAllMatches([]*regexp.Regexp{GetDefaultJunitFile()})
		if err != nil {
			return nil, err
		}

		if len(matches) > 0 {
			j.gcsJunitPaths = matches[0]
		}
	}

	return j.gcsJunitPaths, nil
}

func (j *GCSJobRun) GetCombinedJUnitTestSuites(ctx context.Context) (*junit.TestSuites, error) {
	testSuites := &junit.TestSuites{}
	junitPaths, err := j.GetGCSJunitPaths()
	if err != nil {
		return nil, err
	}

	for _, junitFile := range junitPaths {
		junitContent, err := j.GetContent(ctx, junitFile)
		if err != nil {
			return nil, fmt.Errorf("error getting content for jobrun %w", err)
		}
		// if the file was retrieve, but the content was empty, there is no work to be done.
		if len(junitContent) == 0 {
			continue
		}

		// try as testsuites first just in case we are one
		currTestSuites := &junit.TestSuites{}
		testSuitesErr := xml.Unmarshal(junitContent, currTestSuites)
		if testSuitesErr == nil {
			testSuites.Suites = append(testSuites.Suites, currTestSuites.Suites...)
			continue
		}

		currTestSuite := &junit.TestSuite{}
		if testSuiteErr := xml.Unmarshal(junitContent, currTestSuite); testSuiteErr != nil {
			log.WithError(testSuiteErr).Warningf("error parsing content for jobrun in file %s path %s", junitFile, j.gcsProwJobPath)
			continue
		}
		testSuites.Suites = append(testSuites.Suites, currTestSuite)
	}

	return testSuites, nil
}

func (j *GCSJobRun) GetContent(ctx context.Context, path string) ([]byte, error) {
	if len(path) == 0 {
		return nil, fmt.Errorf("missing path to GCS content for jobrun")
	}
	if content, ok := j.pathToContent[path]; ok {
		return content, nil
	}

	// Get an Object handle for the path
	obj := j.bkt.Object(path)

	// use the object attributes to try to get the latest generation to try to retrieve the data without getting a cached
	// version of data that does not match the latest content.  I don't know if this will work, but in the easy case
	// it doesn't seem to fail.
	objAttrs, err := obj.Attrs(ctx)
	if err != nil {
		return nil, fmt.Errorf("error reading GCS attributes for jobrun: %w", err)
	}
	obj = obj.Generation(objAttrs.Generation)

	// Get an io.Reader for the object.
	gcsReader, err := obj.NewReader(ctx)
	if err != nil {
		return nil, fmt.Errorf("error reading GCS content for jobrun: %w", err)
	}
	defer gcsReader.Close()

	return io.ReadAll(gcsReader)
}

func (j *GCSJobRun) ContentExists(ctx context.Context, path string) bool {
	// Get an Object handle for the path
	obj := j.bkt.Object(path)

	// if we can get the attrs then presume the object exists
	// otherwise presume it doesn't
	_, err := obj.Attrs(ctx)
	return err == nil
}

func (j *GCSJobRun) FindFirstFile(root string, filename *regexp.Regexp) []byte {
	if root == "" {
		root = j.gcsProwJobPath
	}

	it := j.bkt.Objects(context.Background(), &storage.Query{
		Prefix: root,
	})
	for {
		attrs, err := it.Next()
		if err == iterator.Done {
			break
		}

		if filename.MatchString(attrs.Name) {
			data, err := j.GetContent(context.Background(), attrs.Name)

			// if we had an error keep looking, or bail?
			if err != nil {
				log.WithError(err).Errorf("Error reading file: %s/%s", root, attrs.Name)
				return nil
			}
			return data
		}
	}

	return nil
}

// FindAllMatches takes an array of regexes
// and compares the name of the object in gcs
// with each regex for a match
// each regex that matches will get the attribute name
// in the returned matches with the index matching the regex
func (j *GCSJobRun) FindAllMatches(filenames []*regexp.Regexp) ([][]string, error) {
	if len(filenames) < 1 {
		return nil, nil
	}
	matches := make([][]string, len(filenames))

	it := j.bkt.Objects(context.Background(), &storage.Query{
		Prefix: j.gcsProwJobPath,
	})
	for {
		attrs, err := it.Next()
		if errors.Is(err, iterator.Done) {
			break
		}
		if err != nil {
			return nil, errors.Wrap(err, "error reading GCS attributes for job run")
		}

		for i, filename := range filenames {
			if matches[i] == nil {
				matches[i] = make([]string, 0)
			}
			if filename.MatchString(attrs.Name) {
				matches[i] = append(matches[i], attrs.Name)
			}
		}
	}

	return matches, nil
}
