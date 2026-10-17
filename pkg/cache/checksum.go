package cache

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
)

func fileSHA256(path string) (string, error) { return fileSHA256Context(context.Background(), path) }
func fileSHA256Context(ctx context.Context, path string) (string, error) {
	file, err := os.Open(path) //nolint:gosec // G304: internal validated cache payload.
	if err != nil {
		return "", err
	}
	defer func() { _ = file.Close() }()
	hash := sha256.New()
	if _, err := io.Copy(hash, checksumReader{ctx, file}); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

type checksumReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r checksumReader) Read(data []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(data)
}
