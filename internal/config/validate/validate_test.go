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
		{
			// Schema-valid but semantically ambiguous: same (owner, repo) tuple
			// listed under two Owner stanzas. Must fail in validateSemantics
			// with a specific duplicate-tuple error, not a generic schema error.
			name:    "duplicate (owner, repo) tuple across stanzas",
			config:  "testdata/duplicate_owner_repo.yaml",
			wantErr: true,
		},
		{
			// Same owner name appearing in two stanzas is LEGAL when the repos
			// differ — this is the canonical shape for mixed-mode setups under
			// a single GitHub owner. Validator must not reject this.
			name:    "same owner name in two stanzas with different repos",
			config:  "testdata/same_owner_two_stanzas.yaml",
			wantErr: false,
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
