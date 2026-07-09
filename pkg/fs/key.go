package fs

import (
	"crypto/sha256"
	"encoding/hex"
	"runtime"
)

const cacheVersion byte = 1

type AppKey string

type keyInputs struct {
	cacheVersion byte
	goversion    string
	sourcePath   string
	source       []byte
	goos         string
	goarch       string
}

func GenerateAppKey(absPath string, source []byte) AppKey {
	inputs := keyInputs{
		cacheVersion: cacheVersion,
		goversion:    runtime.Version(),
		sourcePath:   absPath,
		source:       source,
		goos:         runtime.GOOS,
		goarch:       runtime.GOARCH,
	}
	return AppKey(hashStruct(inputs))
}

func hashStruct(inputs keyInputs) string {
	buf := make([]byte, 0)
	buf = append(buf, inputs.cacheVersion)
	buf = append(buf, []byte(inputs.goversion)...)
	buf = append(buf, []byte(inputs.sourcePath)...)
	buf = append(buf, inputs.source...)
	buf = append(buf, []byte(inputs.goos)...)
	buf = append(buf, []byte(inputs.goarch)...)

	sha := sha256.Sum256(buf)
	return hex.EncodeToString(sha[:])
}
