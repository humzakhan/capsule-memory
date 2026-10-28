package enrollment

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"

	"github.com/humzakhan/capsule-memory/internal/strictjson"
)

func ReadPrivate(path string, max int, out any) error {
	f, err := os.Open(path)
	if err != nil {
		return errors.New("open private enrollment file failed")
	}
	defer f.Close()
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
		return errors.New("enrollment file must be regular and private (0600)")
	}
	b, err := io.ReadAll(io.LimitReader(f, int64(max)+1))
	if err != nil || strictjson.Decode(b, max, out) != nil {
		return ErrInvalid
	}
	return nil
}

// WritePrivate atomically publishes one new private file. Existing state is
// never overwritten, including on a lost response or interrupted preparation.
func WritePrivate(path string, value any) error {
	b, err := json.Marshal(value)
	if err != nil || len(b) > MaxBody {
		return ErrInvalid
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".enrollment-*")
	if err != nil {
		return errors.New("enrollment destination cannot be created")
	}
	defer os.Remove(f.Name())
	_, writeErr := f.Write(b)
	syncErr := f.Sync()
	closeErr := f.Close()
	if err = errors.Join(writeErr, syncErr, closeErr); err != nil {
		return err
	}
	// Link the synced inode with create-once semantics: there is no truncated
	// identity at the final pathname and a retry cannot replace another key.
	if err = os.Link(f.Name(), path); err != nil {
		return errors.New("enrollment destination exists or atomic publication failed")
	}
	d, err := os.Open(filepath.Dir(path))
	if err != nil {
		return err
	}
	return errors.Join(d.Sync(), d.Close())
}
