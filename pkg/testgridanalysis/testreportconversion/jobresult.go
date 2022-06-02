package testreportconversion

import (
	"math"
)

func IsNeverStableOrTechPreview(variants []string) bool {
	for _, variant := range variants {
		if variant == "never-stable" || variant == "techpreview" {
			return true
		}
	}

	return false
}

// ConvertNaNToZero prevents attempts to marshal the NaN zero-value of a float64 in go by converting to 0.
func ConvertNaNToZero(f float64) float64 {
	if math.IsNaN(f) {
		return 0.0
	}

	return f
}
