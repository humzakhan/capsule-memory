package filetail

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"strconv"
	"time"

	"github.com/humzakhan/capsule-memory/internal/capture"
)

// RewindCheckpoint fingerprints at most two 64 KiB windows at a previously read
// line boundary. Reopening uses the approved tree's path checks, and the opened
// descriptor must still match the batch's exact file snapshot before and after
// fingerprinting. A changed source is retryable; no cursor is acknowledged.
func RewindCheckpoint(ctx context.Context, config Config, stream capture.Stream, batch capture.Checkpoint, offset int64) (capture.Checkpoint, error) {
	end, err := strconv.ParseInt(batch.Position, 10, 64)
	if err != nil || offset < 0 || offset > end || batch.FileIdentity == "" || batch.ObservedModified == "" {
		return capture.Checkpoint{}, capture.ErrInvalid
	}
	path, err := Path(config.Root, stream.Key)
	if err != nil {
		return capture.Checkpoint{}, err
	}
	file, err := os.Open(path)
	if err != nil {
		return capture.Checkpoint{}, capture.ErrUnavailable
	}
	defer file.Close()
	matches := func() bool {
		info, err := file.Stat()
		return err == nil && info.Mode().IsRegular() && Identity(info) == batch.FileIdentity &&
			info.Size() == batch.ObservedSize && info.ModTime().UTC().Format(time.RFC3339Nano) == batch.ObservedModified
	}
	if !matches() {
		return capture.Checkpoint{}, capture.ErrUnavailable
	}
	size := offset
	if size > 64<<10 {
		size = 64 << 10
	}
	fingerprint := func(start int64) (string, error) {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		data := make([]byte, size)
		if _, err := file.ReadAt(data, start); err != nil && !(err == io.EOF && size == 0) {
			return "", capture.ErrUnavailable
		}
		digest := sha256.Sum256(data)
		return hex.EncodeToString(digest[:]), nil
	}
	prefix, err := fingerprint(0)
	if err != nil {
		return capture.Checkpoint{}, err
	}
	tail, err := fingerprint(offset - size)
	if err != nil {
		return capture.Checkpoint{}, err
	}
	if !matches() {
		return capture.Checkpoint{}, capture.ErrUnavailable
	}
	batch.Position = strconv.FormatInt(offset, 10)
	batch.PrefixDigest = prefix
	batch.TailDigest = tail
	batch.Discarding = false
	batch.CaughtUp = false
	return batch, nil
}
