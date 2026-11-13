package core_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/project-openubl/xhandler-go"
	"github.com/project-openubl/xhandler-go/internal/conformance"
	"github.com/project-openubl/xhandler-go/internal/core"
)

const corpus = "../../testdata/conformance"

func TestConformance(t *testing.T) {
	for _, c := range conformance.Cases(t, corpus, "") {
		t.Run(c.Op+"/"+c.Name, func(t *testing.T) {
			t.Parallel()
			request, _ := json.Marshal(map[string]any{"op": c.Op, "input": c.Input})
			raw, err := core.InvokeRaw(context.Background(), request)
			if err != nil {
				t.Fatal(err)
			}
			var response struct {
				Ok    any `json:"ok"`
				Error *struct {
					Kind string `json:"kind"`
				} `json:"error"`
			}
			if err := json.Unmarshal(raw, &response); err != nil {
				t.Fatal(err)
			}

			if c.Expected.Error != nil {
				if response.Error == nil || response.Error.Kind != c.Expected.Error.Kind {
					t.Fatalf("expected error %q, got %s", c.Expected.Error.Kind, raw)
				}
				return
			}
			var expected any
			if err := json.Unmarshal(c.Expected.Ok, &expected); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(response.Ok, expected) {
				t.Fatalf("unexpected result:\n%s", raw)
			}
		})
	}
}

func TestVersionMatchesSyncedCore(t *testing.T) {
	var version string
	if err := core.Invoke(context.Background(), "version", nil, &version); err != nil {
		t.Fatal(err)
	}
	synced, err := os.ReadFile("VERSION")
	if err != nil {
		t.Fatal(err)
	}
	if version != strings.TrimSpace(string(synced)) || version != core.Version {
		t.Fatalf("core reports %q, VERSION is %q", version, synced)
	}
}

func TestErrorsAreTyped(t *testing.T) {
	err := core.Invoke(context.Background(), "nope", nil, nil)
	var xerr *xhandler.Error
	if !errors.As(err, &xerr) || xerr.Kind != xhandler.ErrInvalidRequest {
		t.Fatalf("expected an invalid_request error, got %v", err)
	}
}

func TestConcurrentCalls(t *testing.T) {
	var wg sync.WaitGroup
	for range 32 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			var version string
			if err := core.Invoke(context.Background(), "version", nil, &version); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
}
