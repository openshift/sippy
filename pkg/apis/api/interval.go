package api

import "time"

// Types from origin monitorapi package

type Condition struct {
	Level string `json:"level"`

	Locator string `json:"locator"`
	Message string `json:"message"`
}

type Locator struct {
	Type string            `json:"type"`
	Keys map[string]string `json:"keys"`
}

type Message struct {
	Reason       string            `json:"reason"`
	Cause        string            `json:"cause"`
	HumanMessage string            `json:"humanMessage"`
	Annotations  map[string]string `json:"annotations"`
}
type EventInterval struct {
	Condition

	Display           bool    `json:"display"`
	Source            string  `json:"tempSource,omitempty"`
	StructuredLocator Locator `json:"tempStructuredLocator"`
	StructuredMessage Message `json:"tempStructuredMessage"`

	From *time.Time `json:"from"`
	To   *time.Time `json:"to"`
	// Filename is the base filename we read the intervals from in gcs. If multiple,
	// that usually means one for upgrade and one for conformance portions of the job run.
	// TODO: this may need to be revisited once we're further along with the UI/new schema.
	Filename string `json:"filename"`
}

type EventIntervalList struct {
	Items                  []EventInterval `json:"items"`
	IntervalFilesAvailable []string        `json:"intervalFilesAvailable"`
}
