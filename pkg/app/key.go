package app

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"runtime"
)

const cacheVersion byte = 1

// Key is the hex SHA-256 cache key identifying a unique build of a script.
type Key string

type keyInputs struct {
	cacheVersion byte
	goversion    string
	sourcePath   string
	goos         string
	goarch       string
	source       []byte
}

// NewKey derives the cache key for a script from its absolute path and
// bytes, plus the Go version and target GOOS/GOARCH, so any change that affects
// the built binary forces a cache miss.
func NewKey(absPath string, source []byte) Key {
	inputs := keyInputs{
		cacheVersion: cacheVersion,
		goversion:    runtime.Version(),
		sourcePath:   absPath,
		source:       source,
		goos:         runtime.GOOS,
		goarch:       runtime.GOARCH,
	}
	return Key(hashStruct(inputs))
}

func hashStruct(inputs keyInputs) string {
	h := sha256.New()

	// Write byte fields
	h.Write([]byte{inputs.cacheVersion})

	// Write length-prefixed string fields
	var n [8]byte
	stringFields := [4]string{inputs.goversion, inputs.sourcePath, inputs.goos, inputs.goarch}
	for _, field := range stringFields {
		binary.LittleEndian.PutUint64(n[:], uint64(len(field)))
		h.Write(n[:])
		h.Write([]byte(field))
	}

	// Write length-prefixed byte slice fields
	binary.LittleEndian.PutUint64(n[:], uint64(len(inputs.source)))
	h.Write(n[:])
	h.Write(inputs.source)

	return hex.EncodeToString(h.Sum(nil))
}
