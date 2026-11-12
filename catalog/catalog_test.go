package catalog

import "testing"

func TestControls_Has93(t *testing.T) {
	cs, err := Controls()
	if err != nil {
		t.Fatal(err)
	}
	if len(cs) != 93 {
		t.Fatalf("expected 93 controls, got %d", len(cs))
	}
	seen := map[string]bool{}
	for _, c := range cs {
		if c.ID == "" || c.Clause == "" || c.ClauseES == "" || c.Category == "" {
			t.Errorf("incomplete control: %+v", c)
		}
		if seen[c.ID] {
			t.Errorf("duplicate control %s", c.ID)
		}
		seen[c.ID] = true
	}
}

func TestLookup(t *testing.T) {
	if c, ok := Lookup("A.8.15"); !ok || c.Category != "A.8" {
		t.Errorf("A.8.15 lookup failed: %+v %v", c, ok)
	}
	if _, ok := Lookup("A.9.99"); ok {
		t.Error("A.9.99 should not exist")
	}
}
