package sets

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewString(t *testing.T) {
	tests := []struct {
		name     string
		items    []string
		wantLen  int
		wantList []string
	}{
		{
			name:     "empty",
			items:    nil,
			wantLen:  0,
			wantList: []string{},
		},
		{
			name:     "single item",
			items:    []string{"a"},
			wantLen:  1,
			wantList: []string{"a"},
		},
		{
			name:     "multiple items",
			items:    []string{"c", "a", "b"},
			wantLen:  3,
			wantList: []string{"a", "b", "c"},
		},
		{
			name:     "duplicates are collapsed",
			items:    []string{"a", "b", "a", "b", "a"},
			wantLen:  2,
			wantList: []string{"a", "b"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := NewString(tt.items...)
			assert.Equal(t, tt.wantLen, s.Len())
			assert.Equal(t, tt.wantList, s.List())
		})
	}
}

func TestInsert(t *testing.T) {
	tests := []struct {
		name      string
		initial   []string
		insert    []string
		wantLen   int
		wantItems []string
	}{
		{
			name:      "insert into empty set",
			initial:   nil,
			insert:    []string{"x"},
			wantLen:   1,
			wantItems: []string{"x"},
		},
		{
			name:      "insert new item",
			initial:   []string{"a"},
			insert:    []string{"b"},
			wantLen:   2,
			wantItems: []string{"a", "b"},
		},
		{
			name:      "insert duplicate does not increase length",
			initial:   []string{"a", "b"},
			insert:    []string{"a"},
			wantLen:   2,
			wantItems: []string{"a", "b"},
		},
		{
			name:      "insert multiple with overlap",
			initial:   []string{"a"},
			insert:    []string{"a", "b", "c"},
			wantLen:   3,
			wantItems: []string{"a", "b", "c"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := NewString(tt.initial...)
			s.Insert(tt.insert...)
			assert.Equal(t, tt.wantLen, s.Len())
			assert.Equal(t, tt.wantItems, s.List())
		})
	}
}

func TestDelete(t *testing.T) {
	tests := []struct {
		name      string
		initial   []string
		delete    []string
		wantLen   int
		wantItems []string
	}{
		{
			name:      "delete existing item",
			initial:   []string{"a", "b", "c"},
			delete:    []string{"b"},
			wantLen:   2,
			wantItems: []string{"a", "c"},
		},
		{
			name:      "delete non-existent item does not panic",
			initial:   []string{"a"},
			delete:    []string{"z"},
			wantLen:   1,
			wantItems: []string{"a"},
		},
		{
			name:      "delete from empty set does not panic",
			initial:   nil,
			delete:    []string{"x"},
			wantLen:   0,
			wantItems: []string{},
		},
		{
			name:      "delete all items",
			initial:   []string{"a", "b"},
			delete:    []string{"a", "b"},
			wantLen:   0,
			wantItems: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := NewString(tt.initial...)
			s.Delete(tt.delete...)
			assert.Equal(t, tt.wantLen, s.Len())
			assert.Equal(t, tt.wantItems, s.List())
		})
	}
}

func TestHas(t *testing.T) {
	tests := []struct {
		name    string
		initial []string
		check   string
		want    bool
	}{
		{
			name:    "item present",
			initial: []string{"a", "b"},
			check:   "a",
			want:    true,
		},
		{
			name:    "item absent",
			initial: []string{"a", "b"},
			check:   "c",
			want:    false,
		},
		{
			name:    "empty set",
			initial: nil,
			check:   "a",
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := NewString(tt.initial...)
			assert.Equal(t, tt.want, s.Has(tt.check))
		})
	}
}

func TestHasAll(t *testing.T) {
	tests := []struct {
		name    string
		initial []string
		check   []string
		want    bool
	}{
		{
			name:    "all present",
			initial: []string{"a", "b", "c"},
			check:   []string{"a", "c"},
			want:    true,
		},
		{
			name:    "some missing",
			initial: []string{"a", "b"},
			check:   []string{"a", "z"},
			want:    false,
		},
		{
			name:    "empty args returns true (vacuous truth)",
			initial: []string{"a"},
			check:   nil,
			want:    true,
		},
		{
			name:    "empty set with empty args returns true",
			initial: nil,
			check:   nil,
			want:    true,
		},
		{
			name:    "empty set with non-empty args returns false",
			initial: nil,
			check:   []string{"a"},
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := NewString(tt.initial...)
			assert.Equal(t, tt.want, s.HasAll(tt.check...))
		})
	}
}

func TestHasAny(t *testing.T) {
	tests := []struct {
		name    string
		initial []string
		check   []string
		want    bool
	}{
		{
			name:    "one match",
			initial: []string{"a", "b", "c"},
			check:   []string{"z", "b"},
			want:    true,
		},
		{
			name:    "no match",
			initial: []string{"a", "b"},
			check:   []string{"x", "y"},
			want:    false,
		},
		{
			name:    "empty args returns false",
			initial: []string{"a"},
			check:   nil,
			want:    false,
		},
		{
			name:    "empty set returns false",
			initial: nil,
			check:   []string{"a"},
			want:    false,
		},
		{
			name:    "empty set with empty args returns false",
			initial: nil,
			check:   nil,
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := NewString(tt.initial...)
			assert.Equal(t, tt.want, s.HasAny(tt.check...))
		})
	}
}

func TestDifference(t *testing.T) {
	tests := []struct {
		name string
		s1   []string
		s2   []string
		want []string
	}{
		{
			name: "standard asymmetric: s1 minus s2",
			s1:   []string{"a", "b", "c"},
			s2:   []string{"b", "c", "d"},
			want: []string{"a"},
		},
		{
			name: "reverse direction: s2 minus s1",
			s1:   []string{"b", "c", "d"},
			s2:   []string{"a", "b", "c"},
			want: []string{"d"},
		},
		{
			name: "disjoint sets",
			s1:   []string{"a", "b"},
			s2:   []string{"c", "d"},
			want: []string{"a", "b"},
		},
		{
			name: "identical sets",
			s1:   []string{"a", "b"},
			s2:   []string{"a", "b"},
			want: []string{},
		},
		{
			name: "empty s1",
			s1:   nil,
			s2:   []string{"a"},
			want: []string{},
		},
		{
			name: "empty s2",
			s1:   []string{"a", "b"},
			s2:   nil,
			want: []string{"a", "b"},
		},
		{
			name: "both empty",
			s1:   nil,
			s2:   nil,
			want: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s1 := NewString(tt.s1...)
			s2 := NewString(tt.s2...)
			result := s1.Difference(s2)
			assert.Equal(t, tt.want, result.List())
		})
	}
}

func TestDifferenceIsAsymmetric(t *testing.T) {
	s1 := NewString("a", "b", "c")
	s2 := NewString("b", "c", "d", "e")

	d1 := s1.Difference(s2)
	d2 := s2.Difference(s1)

	assert.Equal(t, []string{"a"}, d1.List())
	assert.Equal(t, []string{"d", "e"}, d2.List())
	assert.False(t, d1.Equal(d2), "Difference must be asymmetric")
}

func TestUnion(t *testing.T) {
	tests := []struct {
		name string
		s1   []string
		s2   []string
		want []string
	}{
		{
			name: "disjoint",
			s1:   []string{"a", "b"},
			s2:   []string{"c", "d"},
			want: []string{"a", "b", "c", "d"},
		},
		{
			name: "overlapping",
			s1:   []string{"a", "b", "c"},
			s2:   []string{"b", "c", "d"},
			want: []string{"a", "b", "c", "d"},
		},
		{
			name: "identical",
			s1:   []string{"a", "b"},
			s2:   []string{"a", "b"},
			want: []string{"a", "b"},
		},
		{
			name: "empty s1",
			s1:   nil,
			s2:   []string{"a"},
			want: []string{"a"},
		},
		{
			name: "empty s2",
			s1:   []string{"a"},
			s2:   nil,
			want: []string{"a"},
		},
		{
			name: "both empty",
			s1:   nil,
			s2:   nil,
			want: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s1 := NewString(tt.s1...)
			s2 := NewString(tt.s2...)
			result := s1.Union(s2)
			assert.Equal(t, tt.want, result.List())
		})
	}
}

func TestUnionIsCommutative(t *testing.T) {
	s1 := NewString("a", "b")
	s2 := NewString("c", "d")
	assert.True(t, s1.Union(s2).Equal(s2.Union(s1)))
}

func TestIntersection(t *testing.T) {
	tests := []struct {
		name string
		s1   []string
		s2   []string
		want []string
	}{
		{
			name: "standard overlap",
			s1:   []string{"a", "b", "c"},
			s2:   []string{"b", "c", "d"},
			want: []string{"b", "c"},
		},
		{
			name: "disjoint",
			s1:   []string{"a", "b"},
			s2:   []string{"c", "d"},
			want: []string{},
		},
		{
			name: "identical",
			s1:   []string{"a", "b"},
			s2:   []string{"a", "b"},
			want: []string{"a", "b"},
		},
		{
			name: "empty s1",
			s1:   nil,
			s2:   []string{"a"},
			want: []string{},
		},
		{
			name: "empty s2",
			s1:   []string{"a"},
			s2:   nil,
			want: []string{},
		},
		{
			name: "both empty",
			s1:   nil,
			s2:   nil,
			want: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s1 := NewString(tt.s1...)
			s2 := NewString(tt.s2...)
			result := s1.Intersection(s2)
			assert.Equal(t, tt.want, result.List())
		})
	}
}

func TestIntersectionOptimizationPath(t *testing.T) {
	// The implementation walks the smaller set. Verify both code paths
	// produce the same result regardless of which set is smaller.
	small := NewString("b", "c")           // len=2
	large := NewString("a", "b", "c", "d") // len=4

	t.Run("s1 smaller than s2", func(t *testing.T) {
		result := small.Intersection(large)
		assert.Equal(t, []string{"b", "c"}, result.List())
	})

	t.Run("s1 larger than s2", func(t *testing.T) {
		result := large.Intersection(small)
		assert.Equal(t, []string{"b", "c"}, result.List())
	})

	t.Run("commutative", func(t *testing.T) {
		assert.True(t, small.Intersection(large).Equal(large.Intersection(small)))
	})
}

func TestIsSuperset(t *testing.T) {
	tests := []struct {
		name string
		s1   []string
		s2   []string
		want bool
	}{
		{
			name: "proper superset",
			s1:   []string{"a", "b", "c"},
			s2:   []string{"a", "b"},
			want: true,
		},
		{
			name: "equal sets are supersets of each other",
			s1:   []string{"a", "b"},
			s2:   []string{"a", "b"},
			want: true,
		},
		{
			name: "not a superset",
			s1:   []string{"a", "b"},
			s2:   []string{"a", "c"},
			want: false,
		},
		{
			name: "empty set is superset of empty set",
			s1:   nil,
			s2:   nil,
			want: true,
		},
		{
			name: "any set is superset of empty set",
			s1:   []string{"a"},
			s2:   nil,
			want: true,
		},
		{
			name: "empty set is not superset of non-empty set",
			s1:   nil,
			s2:   []string{"a"},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s1 := NewString(tt.s1...)
			s2 := NewString(tt.s2...)
			assert.Equal(t, tt.want, s1.IsSuperset(s2))
		})
	}
}

func TestEqual(t *testing.T) {
	tests := []struct {
		name string
		s1   []string
		s2   []string
		want bool
	}{
		{
			name: "identical contents",
			s1:   []string{"a", "b", "c"},
			s2:   []string{"c", "b", "a"},
			want: true,
		},
		{
			name: "different sizes",
			s1:   []string{"a", "b"},
			s2:   []string{"a"},
			want: false,
		},
		{
			name: "same length but different contents",
			s1:   []string{"a", "b"},
			s2:   []string{"a", "c"},
			want: false,
		},
		{
			name: "both empty",
			s1:   nil,
			s2:   nil,
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s1 := NewString(tt.s1...)
			s2 := NewString(tt.s2...)
			assert.Equal(t, tt.want, s1.Equal(s2))
		})
	}
}

func TestList(t *testing.T) {
	tests := []struct {
		name  string
		items []string
		want  []string
	}{
		{
			name:  "sorted output",
			items: []string{"c", "a", "b"},
			want:  []string{"a", "b", "c"},
		},
		{
			name:  "already sorted",
			items: []string{"a", "b", "c"},
			want:  []string{"a", "b", "c"},
		},
		{
			name:  "empty set returns empty slice",
			items: nil,
			want:  []string{},
		},
		{
			name:  "single item",
			items: []string{"x"},
			want:  []string{"x"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := NewString(tt.items...)
			assert.Equal(t, tt.want, s.List())
		})
	}
}

func TestListIsDeterministic(t *testing.T) {
	s := NewString("z", "a", "m", "b", "y")
	first := s.List()
	for i := 0; i < 100; i++ {
		assert.Equal(t, first, s.List(), "List() must return deterministic sorted order")
	}
}

func TestUnsortedList(t *testing.T) {
	s := NewString("a", "b", "c")
	result := s.UnsortedList()
	assert.Len(t, result, 3)
	assert.ElementsMatch(t, []string{"a", "b", "c"}, result)
}

func TestPopAny(t *testing.T) {
	t.Run("pop from non-empty set", func(t *testing.T) {
		s := NewString("a", "b", "c")
		val, ok := s.PopAny()
		require.True(t, ok)
		assert.NotEmpty(t, val)
		assert.False(t, s.Has(val), "popped item must be removed from the set")
		assert.Equal(t, 2, s.Len())
	})

	t.Run("pop from empty set", func(t *testing.T) {
		s := NewString()
		val, ok := s.PopAny()
		assert.False(t, ok)
		assert.Equal(t, "", val)
	})

	t.Run("pop all items", func(t *testing.T) {
		s := NewString("x", "y")
		seen := map[string]bool{}
		for s.Len() > 0 {
			val, ok := s.PopAny()
			require.True(t, ok)
			seen[val] = true
		}
		assert.Equal(t, map[string]bool{"x": true, "y": true}, seen)
		_, ok := s.PopAny()
		assert.False(t, ok)
	})
}

func TestLen(t *testing.T) {
	tests := []struct {
		name  string
		items []string
		want  int
	}{
		{"empty", nil, 0},
		{"one", []string{"a"}, 1},
		{"three", []string{"a", "b", "c"}, 3},
		{"duplicates don't count", []string{"a", "a", "a"}, 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := NewString(tt.items...)
			assert.Equal(t, tt.want, s.Len())
		})
	}
}

func TestStringKeySet(t *testing.T) {
	t.Run("from map[string]int", func(t *testing.T) {
		m := map[string]int{"foo": 1, "bar": 2, "baz": 3}
		s := StringKeySet(m)
		assert.Equal(t, 3, s.Len())
		assert.Equal(t, []string{"bar", "baz", "foo"}, s.List())
	})

	t.Run("from map[string]string", func(t *testing.T) {
		m := map[string]string{"a": "x", "b": "y"}
		s := StringKeySet(m)
		assert.Equal(t, []string{"a", "b"}, s.List())
	})

	t.Run("from empty map", func(t *testing.T) {
		m := map[string]bool{}
		s := StringKeySet(m)
		assert.Equal(t, 0, s.Len())
		assert.Equal(t, []string{}, s.List())
	})

	t.Run("panics with non-map argument", func(t *testing.T) {
		assert.Panics(t, func() {
			StringKeySet("not a map")
		})
	})

	t.Run("panics with slice argument", func(t *testing.T) {
		assert.Panics(t, func() {
			StringKeySet([]string{"a", "b"})
		})
	})
}

func TestInsertReturnsSelf(t *testing.T) {
	s := NewString()
	returned := s.Insert("a", "b")
	assert.True(t, s.Equal(returned), "Insert should return the same set for chaining")
}

func TestDeleteReturnsSelf(t *testing.T) {
	s := NewString("a", "b")
	returned := s.Delete("a")
	assert.True(t, s.Equal(returned), "Delete should return the same set for chaining")
}
