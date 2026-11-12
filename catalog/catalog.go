// Package catalog exposes the reference catalog of ISO/IEC 27001:2022
// Annex A controls. The catalog is central reference data shared by all
// organizations; each organization records its own assessment in the SoA.
package catalog

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"sync"
)

//go:embed iso27001/2022/annex-a-controls.json
var annexA []byte

// Control is one Annex A control. Titles are original wording, not ISO text.
type Control struct {
	ID       string `json:"id"`        // e.g. "A.5.1"
	Clause   string `json:"clause"`    // title in English
	ClauseES string `json:"clause_es"` // title in Spanish
	Category string `json:"category"`  // "A.5" … "A.8"
}

// Standard identifies the catalog version.
const Standard = "ISO/IEC 27001:2022"

var (
	once     sync.Once
	controls []Control
	byID     map[string]Control
	loadErr  error
)

func load() {
	var doc struct {
		Controls []Control `json:"controls"`
	}
	if err := json.Unmarshal(annexA, &doc); err != nil {
		loadErr = fmt.Errorf("catalog: parse annex A: %w", err)
		return
	}
	controls = doc.Controls
	byID = make(map[string]Control, len(controls))
	for _, c := range controls {
		byID[c.ID] = c
	}
}

// Controls returns the Annex A controls in catalog order.
func Controls() ([]Control, error) {
	once.Do(load)
	return controls, loadErr
}

// Lookup returns the control with the given ID.
func Lookup(id string) (Control, bool) {
	once.Do(load)
	c, ok := byID[id]
	return c, ok
}
