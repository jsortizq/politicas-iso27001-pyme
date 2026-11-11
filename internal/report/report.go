// Package report handles JSON export of audit results and compliance reports.
package report

import (
	"encoding/json"
	"os"
	"time"
)

const version = "1.1.0"

// Export writes a structured JSON report to the given path.
// The data map is augmented with tool metadata (name, version, timestamp).
func Export(path string, command string, data map[string]interface{}) error {
	data["tool"] = "iso-cli"
	data["version"] = version
	data["command"] = command
	data["timestamp"] = time.Now().Format(time.RFC3339)

	out, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, out, 0644)
}

// Version returns the current tool version string.
func Version() string {
	return version
}
