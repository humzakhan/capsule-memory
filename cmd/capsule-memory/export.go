package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/humzakhan/capsule-memory/internal/app"
	"github.com/humzakhan/capsule-memory/internal/daemon"
	"github.com/humzakhan/capsule-memory/internal/memory"
	"github.com/humzakhan/capsule-memory/internal/session"
)

// exportFile is the file finalization boundary. Stdout is caller-owned and is
// neither synced nor closed by export.
type exportFile interface {
	Sync() error
	Close() error
}

func finalizeExport(file exportFile, primary error) error {
	// Close must run even after Sync fails. Preserve every failure, including
	// the primary export error, so a partial file cannot look successful.
	syncErr := file.Sync()
	closeErr := file.Close()
	if syncErr != nil {
		syncErr = fmt.Errorf("sync export file: %w", syncErr)
	}
	if closeErr != nil {
		closeErr = fmt.Errorf("close export file: %w", closeErr)
	}
	return errors.Join(primary, syncErr, closeErr)
}

func exportTo(out string, write func(io.Writer) error) (resultErr error) {
	if out == "" {
		return write(os.Stdout)
	}
	file, err := os.OpenFile(out, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	defer func() {
		resultErr = finalizeExport(file, resultErr)
	}()
	return write(file)
}

func exportIPC(ctx context.Context, client *daemon.Client, params app.Params) error {
	return exportTo(params.Out, func(w io.Writer) error {
		return exportIPCPages(ctx, client, params, w)
	})
}

func exportIPCPages(ctx context.Context, client *daemon.Client, params app.Params, w io.Writer) error {
	params.From = 0
	params.MaxBytes = 256 << 10
	for {
		var page session.ExportPage
		if err := client.Call(ctx, "admin.session.export", params, &page); err != nil {
			return err
		}
		if _, err := io.WriteString(w, page.JSONL); err != nil {
			return err
		}
		if page.Done {
			return nil
		}
		if page.Next <= params.From {
			return memory.ErrInvalid
		}
		params.From = page.Next
		params.Snapshot = page.Snapshot
	}
}
