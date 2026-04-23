package validate

import (
	"fmt"
	"os"

	"github.com/kaptinlin/jsonschema"
	"github.com/mooneeb/amgi/internal/config"
	"github.com/mooneeb/amgi/internal/schema"
	"gopkg.in/yaml.v3"
)

// ValidateConfig validates user config to ensure it conforms to the schema
// and matches all required field constraints. This function is designed to be
// used by users and CI to validate config files before usage.
func ValidateConfig(configPath string) error {

	_, err := ParseAndValidateConfig(configPath)
	return err
}

func ParseAndValidateConfig(configPath string) (*config.Config, error) {
	yc, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("read config yaml: %w", err)
	}

	var doc map[string]any
	if err := yaml.Unmarshal(yc, &doc); err != nil {
		return nil, fmt.Errorf("parse yaml: %w", err)
	}

	if err := validateConfigSchema(doc); err != nil {
		return nil, err
	}

	c, err := unmarshalConfig(yc)
	if err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}

	if err := validateSemantics(c); err != nil {
		return nil, err
	}

	return c, nil
}

// validateSemantics enforces cross-field rules that JSON Schema can't express.
// Kept separate from the schema pass so error messages stay specific to the
// rule rather than surfacing as opaque schema failures.
//
// Current rules:
//  1. Each (ownerName, repoName) pair must appear at most once across all
//     Owner stanzas. Duplicate owner names are LEGAL (needed to express "same
//     GitHub owner, different mode per repo"), but a duplicate (owner, repo)
//     tuple is ambiguous — the resolver would not know which stanza's mode,
//     filters, or marvin_config_id applies.
func validateSemantics(c *config.Config) error {
	seen := make(map[[2]string]bool, 0)
	for _, owner := range c.GitHub.Owners {
		for _, repo := range owner.Repositories {
			key := [2]string{owner.Name, repo.Name}
			if seen[key] {
				return fmt.Errorf("semantic validation failed: duplicate (owner=%q, repo=%q) pair across Owner stanzas — each (owner, repo) combination must be unique",
					owner.Name, repo.Name)
			}
			seen[key] = true
		}
	}
	return nil
}

// ValidateConfigSchema validates the config against the schema
func validateConfigSchema(y map[string]any) error {
	compiler := jsonschema.NewCompiler()
	jsonSchema, err := compiler.Compile(schema.Schema)
	if err != nil {
		return fmt.Errorf("compile schema: %w", err)
	}

	result := jsonSchema.Validate(y)
	if result.IsValid() {
		return nil
	}

	de := result.DetailedErrors()
	return fmt.Errorf("schema validation failed\n%v", de)
}

// unmarshalConfig unmarshals the user config into a Config struct
func unmarshalConfig(yc []byte) (*config.Config, error) {
	var c config.Config
	if err := yaml.Unmarshal(yc, &c); err != nil {
		return nil, err
	}
	return &c, nil
}
