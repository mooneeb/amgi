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

	return c, nil
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
