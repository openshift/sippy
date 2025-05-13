package api

import "time"

// Types originally from origin monitorapi package
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
	Level             string  `json:"level"`
	Display           bool    `json:"display"`
	Source            string  `json:"source,omitempty"`
	StructuredLocator Locator `json:"locator"`
	StructuredMessage Message `json:"message"`

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
	JobRunURL              string          `json:"jobRunURL"`
}

// LegacyEventInterval is the previous temporary schema we used before we completed the port to the new API.
// We fall back to using this if we cannot parse the new schema (because locator/message are still strings in that file),
// then convert to the new format and return from the API.
type LegacyEventInterval struct {
	Level             string  `json:"level"`
	Locator           string  `json:"locator"`
	Message           string  `json:"message"`
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

type LegacyEventIntervalList struct {
	Items                  []LegacyEventInterval `json:"items"`
	IntervalFilesAvailable []string              `json:"intervalFilesAvailable"`
}
