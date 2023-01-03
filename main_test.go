package main

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestControllersToStart(t *testing.T) {
	tests := []struct {
		name             string
		envValue         string
		expectedStartZdb bool
		expectedStartZau bool
	}{
		{
			name:             "zau only",
			envValue:         "zau",
			expectedStartZdb: false,
			expectedStartZau: true,
		},
		{
			name:             "zdb only",
			envValue:         "zdb",
			expectedStartZdb: true,
			expectedStartZau: false,
		},
		{
			name:             "zdb and zau",
			envValue:         "zdb,zau",
			expectedStartZdb: true,
			expectedStartZau: true,
		},
		{
			name:             "empty env var",
			envValue:         "",
			expectedStartZdb: true,
			expectedStartZau: true,
		},
		{
			name:             "no valid controller specified",
			envValue:         "bla",
			expectedStartZdb: false,
			expectedStartZau: false,
		},
	}

	defer os.Unsetenv(CONTROLLERS_ENV)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.envValue != "" {
				os.Setenv(CONTROLLERS_ENV, tt.envValue)
			} else {
				os.Unsetenv(CONTROLLERS_ENV)
			}

			zdb, zau := controllersToStart()
			assert.Equal(t, zdb, tt.expectedStartZdb)
			assert.Equal(t, zau, tt.expectedStartZau)
		})
	}
}
