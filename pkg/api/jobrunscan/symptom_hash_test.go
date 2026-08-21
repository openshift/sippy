package jobrunscan

import (
	"testing"

	"github.com/lib/pq"
	"github.com/openshift/sippy/pkg/db/models/jobrunscan"
)

func symptom(id, matcherType, matchString, filePattern string, labels ...string) jobrunscan.Symptom {
	return jobrunscan.Symptom{
		SymptomContent: jobrunscan.SymptomContent{
			ID:          id,
			MatcherType: matcherType,
			MatchString: matchString,
			FilePattern: filePattern,
			LabelIDs:    pq.StringArray(labels),
		},
	}
}

func TestComputeSymptomHash(t *testing.T) {
	tests := []struct {
		name     string
		a, b     []jobrunscan.Symptom
		wantSame bool
	}{
		{
			name:     "empty slices are equal",
			a:        nil,
			b:        []jobrunscan.Symptom{},
			wantSame: true,
		},
		{
			name: "same symptoms in same order",
			a: []jobrunscan.Symptom{
				symptom("s1", "string", "foo", "*.log", "l1"),
				symptom("s2", "regex", "bar.*", "*.txt", "l2"),
			},
			b: []jobrunscan.Symptom{
				symptom("s1", "string", "foo", "*.log", "l1"),
				symptom("s2", "regex", "bar.*", "*.txt", "l2"),
			},
			wantSame: true,
		},
		{
			name: "same symptoms in different order",
			a: []jobrunscan.Symptom{
				symptom("s2", "regex", "bar.*", "", "l2"),
				symptom("s1", "string", "foo", "", "l1"),
			},
			b: []jobrunscan.Symptom{
				symptom("s1", "string", "foo", "", "l1"),
				symptom("s2", "regex", "bar.*", "", "l2"),
			},
			wantSame: true,
		},
		{
			name:     "different match string",
			a:        []jobrunscan.Symptom{symptom("s1", "string", "foo", "", "l1")},
			b:        []jobrunscan.Symptom{symptom("s1", "string", "bar", "", "l1")},
			wantSame: false,
		},
		{
			name:     "different labels",
			a:        []jobrunscan.Symptom{symptom("s1", "string", "foo", "", "l1")},
			b:        []jobrunscan.Symptom{symptom("s1", "string", "foo", "", "l1", "l2")},
			wantSame: false,
		},
		{
			name:     "label order does not matter",
			a:        []jobrunscan.Symptom{symptom("s1", "", "", "", "l2", "l1")},
			b:        []jobrunscan.Symptom{symptom("s1", "", "", "", "l1", "l2")},
			wantSame: true,
		},
		{
			name: "extra symptom differs",
			a:    []jobrunscan.Symptom{symptom("s1", "", "", "", "l1")},
			b: []jobrunscan.Symptom{
				symptom("s1", "", "", "", "l1"),
				symptom("s2", "", "", "", "l2"),
			},
			wantSame: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hashA := computeSymptomHash(tt.a)
			hashB := computeSymptomHash(tt.b)

			if len(hashA) != 16 {
				t.Errorf("hash length = %d, want 16", len(hashA))
			}

			if (hashA == hashB) != tt.wantSame {
				t.Errorf("hashA=%s hashB=%s, wantSame=%v", hashA, hashB, tt.wantSame)
			}
		})
	}
}
