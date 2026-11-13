// Package conformance reads the corpus that every xhandler binding must pass
// (testdata/conformance, synced from xhandler-rust).
package conformance

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Case is one corpus file: <op>/<name>.json.
type Case struct {
	Op       string
	Name     string
	Input    json.RawMessage `json:"input"`
	Expected struct {
		Ok    json.RawMessage `json:"ok"`
		Error *struct {
			Kind string `json:"kind"`
		} `json:"error"`
	} `json:"expected"`
}

// Cases returns the cases under dir (the corpus root), optionally only those
// of the given op.
func Cases(t *testing.T, dir, op string) []Case {
	t.Helper()
	var cases []Case
	err := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() || filepath.Ext(path) != ".json" {
			return err
		}
		relative, _ := filepath.Rel(dir, path)
		caseOp, name, _ := strings.Cut(filepath.ToSlash(relative), "/")
		if op != "" && caseOp != op {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		c := Case{Op: caseOp, Name: strings.TrimSuffix(name, ".json")}
		if err := json.Unmarshal(data, &c); err != nil {
			return err
		}
		cases = append(cases, c)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(cases) == 0 {
		t.Fatalf("no conformance cases in %s (run make sync)", dir)
	}
	return cases
}
