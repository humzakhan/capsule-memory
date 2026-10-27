package main

import (
	"errors"
	"testing"
)

type failingExportFile struct {
	syncErr  error
	closeErr error
	calls    []string
}

func (f *failingExportFile) Sync() error {
	f.calls = append(f.calls, "sync")
	return f.syncErr
}

func (f *failingExportFile) Close() error {
	f.calls = append(f.calls, "close")
	return f.closeErr
}

// Both CLI export routes share this finalization boundary. Exercise failures
// without relying on platform-specific disk-full or close-error behavior.
func TestExportFinalizationPreservesFailures(t *testing.T) {
	primary := errors.New("export interrupted")
	syncErr := errors.New("sync failed")
	closeErr := errors.New("close failed")
	for _, tc := range []struct {
		name    string
		primary error
		sync    error
		close   error
	}{
		{name: "sync", sync: syncErr},
		{name: "close", close: closeErr},
		{name: "primary and finalization", primary: primary, sync: syncErr, close: closeErr},
		{name: "success"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			file := &failingExportFile{syncErr: tc.sync, closeErr: tc.close}
			err := finalizeExport(file, tc.primary)
			for _, want := range []error{tc.primary, tc.sync, tc.close} {
				if want != nil && !errors.Is(err, want) {
					t.Fatalf("lost export failure: %v", want)
				}
			}
			if tc.primary == nil && tc.sync == nil && tc.close == nil && err != nil {
				t.Fatalf("successful export: %v", err)
			}
			if len(file.calls) != 2 || file.calls[0] != "sync" || file.calls[1] != "close" {
				t.Fatalf("finalization order: %v", file.calls)
			}
		})
	}
}
