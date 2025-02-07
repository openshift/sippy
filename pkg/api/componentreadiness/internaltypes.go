package componentreadiness

// TestWithVariantsKey connects the core unique db testID string to a set of variants.
// Used to serialize/deserialize as a map key when we pass test status around.
type TestWithVariantsKey struct {
	TestID string `json:"test_id"`

	// Proposed, need to serialize to use as map key
	Variants map[string]string `json:"variants"`
}
