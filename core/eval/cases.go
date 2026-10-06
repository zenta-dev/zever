package eval

import (
	"encoding/json"
	"fmt"
	"os"
)

// LoadCases reads a golden dataset of cases from path. The loader lives
// outside tests so external runners (CI, tooling) can reuse the dataset.
func LoadCases(path string) ([]Case, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("eval: load cases %q: %w", path, err)
	}

	var cases []Case
	if err := json.Unmarshal(raw, &cases); err != nil {
		return nil, fmt.Errorf("eval: decode cases %q: %w", path, err)
	}

	return cases, nil
}
