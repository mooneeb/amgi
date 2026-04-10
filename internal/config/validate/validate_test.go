package validate

import (
	"testing"
)

func TestValidateConfig(t *testing.T) {
	tests := []struct {
		name    string
		config  string
		wantErr bool
	}{
		{
			name:    "valid config",
			config:  "testdata/correct.yaml",
			wantErr: false,
		},
		{
			name:    "invalid config",
			config:  "testdata/incorrect.yaml",
			wantErr: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := ValidateConfig(test.config)
			if (err != nil) != test.wantErr {
				t.Errorf("ValidateConfig(%s) = %v, want %v", test.config, err, test.wantErr)
			}
		})
	}
}
