package jobrunannotator

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"path"
	"sort"
	"strings"

	"cloud.google.com/go/storage"
	"github.com/openshift/sippy/pkg/api/jobartifacts"
	"github.com/openshift/sippy/pkg/db/models/jobrunscan"
	"github.com/yuin/goldmark"
	"google.golang.org/api/iterator"
	g "maragu.dev/gomponents"
	h "maragu.dev/gomponents/html"
)

const BucketLabelsPrefix = "artifacts/job_labels/"

// JobRunBucketLabelContainer is a schema-specifying container for writing a label file to the prow bucket.
// for incompatible schema changes just add another version with a different type and tag
type JobRunBucketLabelContainer struct {
	V1 *JobRunBucketLabel `json:"symptom_label_v1,omitempty"`
}

// JobRunBucketLabel represents a single label applied to a job run per a symptom.
type JobRunBucketLabel struct {
	Symptom jobrunscan.SymptomContent `json:"symptom"`
	Label   jobrunscan.LabelContent   `json:"label"`
	// path of the matched file relative to the top of the bucket
	FileMatch string `json:"file_match"`
	// the first file text that matched the symptom, if relevant for the matcher (otherwise empty)
	TextMatch string `json:"text_match,omitempty"`
	// metadata for where to write this
	Bucket string `json:"-"`
	// directory for the job run within the bucket: logs/.../<job-name>/<buildId>/
	JobRunPath string `json:"-"`
}

func (x JobRunBucketLabel) WriteJSONToBucket(ctx context.Context, client *storage.Client) error {
	baseName := path.Base(x.FileMatch)
	hasher := sha256.New()
	hasher.Write([]byte(x.FileMatch))
	labelFile := x.Label.ID + "-" + baseName + "-" + hex.EncodeToString(hasher.Sum(nil)) + ".json"

	bucketWriter := client.Bucket(x.Bucket).Object(x.JobRunPath + BucketLabelsPrefix + labelFile).NewWriter(ctx)
	encoder := json.NewEncoder(bucketWriter)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(JobRunBucketLabelContainer{V1: &x}); err != nil {
		_ = bucketWriter.Close()
		return err
	}

	return bucketWriter.Close()
}

// WriteHTMLSummaryToBucket reads all label JSON files from artifacts/job_labels/ and generates
// an HTML summary file that Spyglass can display. Exported for use by cloud function.
func WriteHTMLSummaryToBucket(ctx context.Context, bucket *storage.BucketHandle, jobRunPath string) (int, error) {
	labelDir := jobRunPath + BucketLabelsPrefix // bucket directory for labels and summary

	// List all JSON files in the job_labels directory
	labels, err := readBucketJSONLabels(ctx, bucket, jobRunPath, labelDir)
	if err != nil || len(labels) == 0 {
		// error reading or no labels found, don't create the summary file
		return 0, err
	}

	// Write the HTML summary file
	writer := bucket.Object(labelDir + "label-summary.html").NewWriter(ctx)
	writer.ContentType = "text/html"
	if _, err = writer.Write([]byte(generateHTMLSummary(labels))); err != nil {
		_ = writer.Close() // close writer but only report failure to write
		return 0, err
	}

	return len(labels), writer.Close()
}

// readBucketJSONLabels reads all label JSON files from the bucket as a map by label ID.
func readBucketJSONLabels(
	ctx context.Context, bucket *storage.BucketHandle,
	jobRunPath, labelDir string,
) (map[string][]JobRunBucketLabelContainer, error) {

	labelMap := make(map[string][]JobRunBucketLabelContainer)
	it := bucket.Objects(ctx, &storage.Query{
		Delimiter: "/",
		MatchGlob: labelDir + "*.json",
	})

	for {
		attrs, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return labelMap, err
		}

		// Read and parse the label file
		reader, err := bucket.Object(attrs.Name).NewReader(ctx)
		if err != nil {
			return labelMap, err
		}

		data, err := io.ReadAll(reader)
		_ = reader.Close() // attempt to close, but only report actual failure to read
		if err != nil {
			return labelMap, err
		}

		var container JobRunBucketLabelContainer
		if err := json.Unmarshal(data, &container); err != nil {
			return labelMap, err
		}
		if container.V1 == nil {
			continue // skip invalid or mis-versioned labels
		}
		container.V1.JobRunPath = jobRunPath

		labelMap[container.V1.Label.ID] = append(labelMap[container.V1.Label.ID], container)
	}
	return labelMap, nil
}

// generateHTMLSummary creates an HTML document from the label data
func generateHTMLSummary(labelMap map[string][]JobRunBucketLabelContainer) string {

	// Get sorted label IDs
	var labelIDs []string
	for id := range labelMap {
		labelIDs = append(labelIDs, id)
	}
	sort.Strings(labelIDs)

	// Build HTML document
	title := g.Text("Symptoms matched this job run with one label.")
	if count := len(labelMap); count > 1 {
		title = g.Textf("Symptoms matched this job run with %d labels.", count)
	}
	doc := h.Doctype(
		h.HTML(h.Lang("en"),
			h.Head(
				h.Meta(h.Charset("UTF-8")),
				h.Meta(h.Name("viewport"), h.Content("width=device-width, initial-scale=1.0")),
				h.TitleEl(title),
				styleElement(),
			),
			h.Body(
				g.Group(labelSections(labelMap, labelIDs)),
			),
		),
	)

	var sb strings.Builder
	_ = doc.Render(&sb)
	return sb.String()
}

// styleElement returns the CSS styles for the HTML document
func styleElement() g.Node {
	return h.StyleEl(g.Raw(`
        body {
            font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "Helvetica Neue", Arial, sans-serif;
            line-height: 1.6;
            color: #fff;
            max-width: 1200px;
            margin: 0 auto;
            padding: 20px;
            background-color: #111111;
        }
        .label-section {
            background: #222222;
            border-radius: 8px;
            padding: 20px;
            margin-bottom: 20px;
            box-shadow: 0 2px 4px rgba(0,0,0,0.3);
        }
        .label-header {
            display: flex;
            align-items: center;
            margin-bottom: 15px;
        }
        .label-name {
            font-size: 1.3em;
            font-weight: bold;
            color: #fff;
        }
        .symptom-info {
            background: #212121;
            border-left: 4px solid #FF5277;
            padding: 10px;
            margin: 5px 0;
            border-radius: 4px;
        }
        .info-row {
            margin: 4px 0;
        }
        .info-label {
            font-weight: 600;
            color: #fff;
            display: inline-block;
            min-width: 120px;
        }
        .info-value {
            color: #ccc;
        }
        .code {
            font-family: "Courier New", Courier, monospace;
            background: #111111;
            padding: 2px 6px;
            border-radius: 3px;
            font-size: 0.9em;
            color: #fff;
        }
        .text-match {
            background: #2a2a2a;
            border-left: 4px solid #FF5277;
            padding: 10px;
            margin: 10px 0;
            border-radius: 4px;
            font-family: monospace;
            white-space: pre-wrap;
            word-break: break-all;
            color: #fff;
        }

		/* content styling mainly intended for markdown content */
        a {
            color: #FF5277;
            text-decoration: underline;
        }
        p {
            margin: 0.5em 0;
        }
        ul, ol {
            margin: 0.5em 0;
            padding-left: 2em;
        }
        li {
            margin: 0.25em 0;
        }
        code {
            font-family: "Courier New", Courier, monospace;
            background: #111111;
            padding: 2px 6px;
            border-radius: 3px;
            font-size: 0.9em;
            color: #fff;
        }
        pre {
            background: #111111;
            padding: 10px;
            border-radius: 4px;
            overflow-x: auto;
            border-left: 4px solid #BBBBBB;
        }
        pre code {
            background: none;
            padding: 0;
        }
        blockquote {
            border-left: 4px solid #FF5277;
            padding-left: 1em;
            margin: 0.5em 0;
            color: #ccc;
        }
    `))
}

// labelSections generates all label section elements
func labelSections(labelMap map[string][]JobRunBucketLabelContainer, labelIDs []string) []g.Node {
	sections := make([]g.Node, 0, len(labelIDs))
	for _, labelID := range labelIDs {
		sections = append(sections, labelSection(labelMap[labelID]))
	}
	return sections
}

// markdownToHTML converts markdown text to HTML
func markdownToHTML(md string) (string, error) {
	var buf bytes.Buffer
	if err := goldmark.Convert([]byte(md), &buf); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// labelSection creates a section for a single label with all its instances
func labelSection(instances []JobRunBucketLabelContainer) g.Node {
	if len(instances) == 0 || instances[0].V1 == nil {
		return nil
	}

	firstLabel := instances[0].V1
	markDownEl := h.P(g.Text(firstLabel.Label.Explanation))
	if markdown, mdErr := markdownToHTML(firstLabel.Label.Explanation); mdErr == nil {
		markDownEl = h.Div(g.Raw(markdown))
	}
	return h.Div(h.Class("label-section"),
		h.Div(h.Class("label-header"),
			h.Span(h.Class("label-name"), g.Text(firstLabel.Label.LabelTitle)),
		),
		g.If(firstLabel.Label.Explanation != "", markDownEl),
		g.Group(matchInstances(instances)),
	)
}

// matchInstances generates elements for all match instances
func matchInstances(instances []JobRunBucketLabelContainer) []g.Node {
	nodes := make([]g.Node, 0, len(instances))
	for _, instance := range instances {
		if instance.V1 == nil {
			continue
		}
		nodes = append(nodes, matchInstance(instance.V1))
	}
	return nodes
}

// matchInstance creates an element for a single match instance
func matchInstance(label *JobRunBucketLabel) g.Node {
	relativePath := strings.TrimPrefix(label.FileMatch, label.JobRunPath)
	return h.Div(h.Class("symptom-info"),
		h.Div(h.Class("info-row"),
			infoRow("Symptom:", g.Text(label.Symptom.Summary)),
			infoRow("Matched file:",
				h.A(
					h.Href(jobartifacts.ArtifactURLFor(label.FileMatch)),
					h.Target("_blank"),
					h.Class("info-value code"), g.Text(relativePath),
				),
			),
		),
		g.If(label.TextMatch != "", h.Div(h.Class("text-match"), g.Text(label.TextMatch))),
	)
}

// infoRow creates a row with a label and value
func infoRow(labelText string, value g.Node) g.Node {
	return h.Div(h.Class("info-row"),
		h.Span(h.Class("info-label"), g.Text(labelText)),
		value,
	)
}
