package fs

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"runtime"
)

// cacheVersion is bumped whenever the cache key schema changes in a way that
// invalidates existing cached binaries.
const cacheVersion byte = 1

// AppKey is the hex-encoded SHA-256 cache key for a compiled script.
// It encodes the combination of inputs that fully determine the output binary:
// the cache version, Go toolchain version, script path, source bytes, and
// target OS/arch.
type AppKey string

// keyInputs is the set of inputs that make up the cache key. It is serialized
// to JSON before hashing so that the hash is stable across Go versions and
// encoding changes.
type keyInputs struct {
	CacheVersion byte   `json:"cache_version"`
	GoVersion    string `json:"go_version"`
	SourcePath   string `json:"source_path"`
	Source       []byte `json:"source"`
	GOOS         string `json:"goos"`
	GOARCH       string `json:"goarch"`
}

// GenerateAppKey returns the AppKey for the given script path and source bytes.
// The key is deterministic: the same inputs always produce the same key.
func GenerateAppKey(absPath string, source []byte) AppKey {
	inputs := keyInputs{
		CacheVersion: cacheVersion,
		GoVersion:    runtime.Version(),
		SourcePath:   absPath,
		Source:       source,
		GOOS:         runtime.GOOS,
		GOARCH:       runtime.GOARCH,
	}

	data, err := json.Marshal(inputs)
	if err != nil {
		// json.Marshal of a plain struct with only basic types never fails;
		// panic here would indicate a programming error, not a runtime condition.
		panic("fs: GenerateAppKey: json.Marshal failed: " + err.Error())
	}

	sum := sha256.Sum256(data)
	return AppKey(hex.EncodeToString(sum[:]))
}
