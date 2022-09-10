package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsSortMapByValue(t *testing.T) {
	tests := []struct {
		name          string
		input         map[string]int32
		expectedOrder []string
	}{
		{
			name: "map 1",
			input: map[string]int32{
				"bar": int32(8),
				"foo": int32(5),
				"joe": int32(9),
			},
			expectedOrder: []string{"foo", "bar", "joe"},
		},
		{
			name: "map 2",
			input: map[string]int32{
				"bar": int32(8),
				"foo": int32(9),
				"joe": int32(10),
			},
			expectedOrder: []string{"bar", "foo", "joe"},
		},
		{
			name: "map 3",
			input: map[string]int32{
				"bar": int32(8),
				"foo": int32(15),
				"joe": int32(10),
			},
			expectedOrder: []string{"bar", "joe", "foo"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pairList := SortMapByValue(tt.input)
			for i, pair := range pairList {
				assert.Equal(t, tt.expectedOrder[i], pair.Key)
				assert.Equal(t, tt.input[pair.Key], pair.Value)
			}
		})
	}
}
