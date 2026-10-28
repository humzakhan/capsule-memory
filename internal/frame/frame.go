// Package frame encodes independently compressed, padded schema-v2 revisions.
package frame

import (
	"bytes"
	"compress/flate"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"unicode/utf8"

	"github.com/humzakhan/capsule-memory/internal/strictjson"
)

const (
	Magic                    = "\x89CMF"
	Version1            byte = 1
	CodecIdentity       byte = 0
	CodecDeflate        byte = 1
	ContentRevisionJSON byte = 1
	HeaderSize               = 16
	Quantum                  = 4096
	MaxFrame                 = 1 << 20
	MaxDecoded               = 1 << 20
	ChunkHardMax             = 896 << 10
)

var (
	ErrUnsupported = errors.New("unsupported frame version, codec, content or flags")
	ErrMalformed   = errors.New("malformed frame")
	ErrTooLarge    = errors.New("frame exceeds size bounds")
)

func IsFramed(b []byte) bool { return len(b) >= 4 && string(b[:4]) == Magic }
func Encode(content byte, decoded []byte) ([]byte, error) {
	if content != ContentRevisionJSON {
		return nil, ErrUnsupported
	}
	if len(decoded) == 0 || !utf8.Valid(decoded) {
		return nil, ErrMalformed
	}
	if len(decoded) > ChunkHardMax {
		return nil, ErrTooLarge
	}
	if strictjson.Unique(decoded) != nil {
		return nil, ErrMalformed
	}
	var logical map[string]json.RawMessage
	var version int
	if json.Unmarshal(decoded, &logical) != nil || json.Unmarshal(logical["schema_version"], &version) != nil || version != 2 {
		return nil, ErrMalformed
	}
	var body bytes.Buffer
	w, err := flate.NewWriter(&body, flate.DefaultCompression)
	if err != nil {
		return nil, err
	}
	if _, err = w.Write(decoded); err != nil {
		w.Close()
		return nil, err
	}
	if err = w.Close(); err != nil {
		return nil, err
	}
	n := (HeaderSize + body.Len() + Quantum - 1) / Quantum * Quantum
	if n > MaxFrame {
		return nil, ErrTooLarge
	}
	out := make([]byte, n)
	copy(out, Magic)
	out[4] = Version1
	out[5] = CodecDeflate
	out[6] = content
	binary.BigEndian.PutUint32(out[8:12], uint32(len(decoded)))
	binary.BigEndian.PutUint32(out[12:16], uint32(body.Len()))
	copy(out[16:], body.Bytes())
	return out, nil
}
func Decode(b []byte) (byte, []byte, error) {
	if len(b) > MaxFrame {
		return 0, nil, ErrTooLarge
	}
	if len(b) < HeaderSize || len(b)%Quantum != 0 || !IsFramed(b) {
		return 0, nil, ErrMalformed
	}
	if b[4] != Version1 || b[5] > CodecDeflate || b[6] != ContentRevisionJSON || b[7] != 0 {
		return 0, nil, ErrUnsupported
	}
	n := int(binary.BigEndian.Uint32(b[8:12]))
	m := int(binary.BigEndian.Uint32(b[12:16]))
	if n > MaxDecoded {
		return 0, nil, ErrTooLarge
	}
	if n < 1 || m < 1 || m > len(b)-HeaderSize || len(b)-HeaderSize-m >= Quantum {
		return 0, nil, ErrMalformed
	}
	for _, v := range b[HeaderSize+m:] {
		if v != 0 {
			return 0, nil, ErrMalformed
		}
	}
	body := b[HeaderSize : HeaderSize+m]
	out := make([]byte, n)
	if b[5] == CodecIdentity {
		if m != n {
			return 0, nil, ErrMalformed
		}
		copy(out, body)
	} else {
		br := bytes.NewReader(body)
		r := flate.NewReader(br)
		defer r.Close()
		limited := io.LimitReader(r, int64(n)+1)
		if _, err := io.ReadFull(limited, out); err != nil {
			return 0, nil, ErrMalformed
		}
		var extra [1]byte
		k, err := limited.Read(extra[:])
		if k != 0 || err != io.EOF || br.Len() != 0 {
			return 0, nil, ErrMalformed
		}
	}
	if !utf8.Valid(out) || strictjson.Unique(out) != nil {
		return 0, nil, ErrMalformed
	}
	var logical map[string]json.RawMessage
	var version int
	if json.Unmarshal(out, &logical) != nil || json.Unmarshal(logical["schema_version"], &version) != nil {
		return 0, nil, ErrMalformed
	}
	if version > 2 {
		return 0, nil, ErrUnsupported
	}
	if version != 2 {
		return 0, nil, ErrMalformed
	}
	return b[6], out, nil
}
