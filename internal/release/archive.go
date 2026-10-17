package main

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"time"
)

type fileData struct {
	name string
	data []byte
	mode fs.FileMode
}

const maxAssetBytes = 128 << 20

func pack(t target, timestamp time.Time, binary, license, install []byte) ([]byte, error) {
	files := []fileData{{binaryName(t), binary, 0o755}, {"LICENSE", license, 0o644}, {"INSTALL.txt", install, 0o644}}
	var buffer bytes.Buffer
	if t.OS == "windows" {
		writer := zip.NewWriter(&buffer)
		for _, file := range files {
			header := &zip.FileHeader{Name: file.name, Method: zip.Deflate, Modified: timestamp}
			header.SetMode(file.mode)
			entry, err := writer.CreateHeader(header)
			if err != nil {
				return nil, err
			}
			if _, err = entry.Write(file.data); err != nil {
				return nil, err
			}
		}
		if err := writer.Close(); err != nil {
			return nil, err
		}
	} else {
		gz := gzip.NewWriter(&buffer)
		writer := tar.NewWriter(gz)
		for _, file := range files {
			if err := writer.WriteHeader(&tar.Header{Name: file.name, Mode: int64(file.mode), Size: int64(len(file.data)), ModTime: timestamp, Typeflag: tar.TypeReg}); err != nil {
				return nil, err
			}
			if _, err := writer.Write(file.data); err != nil {
				return nil, err
			}
		}
		if err := writer.Close(); err != nil {
			return nil, err
		}
		if err := gz.Close(); err != nil {
			return nil, err
		}
	}
	return buffer.Bytes(), nil
}

// unpack recognizes only our three flat regular files; no archive path reaches
// filesystem operations. Size limits bound even malformed compressed input.
func unpack(t target, data []byte) ([]fileData, error) {
	var files []fileData
	if t.OS == "windows" {
		reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
		if err != nil {
			return nil, err
		}
		for _, entry := range reader.File {
			if len(files) >= 3 || !entry.Mode().IsRegular() {
				return nil, errors.New("unexpected archive entry")
			}
			r, e := entry.Open()
			if e != nil {
				return nil, e
			}
			content, e := io.ReadAll(io.LimitReader(r, maxAssetBytes+1))
			closeErr := r.Close()
			if e != nil {
				return nil, e
			}
			if closeErr != nil {
				return nil, closeErr
			}
			files = append(files, fileData{entry.Name, content, entry.Mode()})
		}
	} else {
		gz, err := gzip.NewReader(bytes.NewReader(data))
		if err != nil {
			return nil, err
		}
		defer func() { _ = gz.Close() }()
		reader := tar.NewReader(gz)
		for {
			entry, e := reader.Next()
			if errors.Is(e, io.EOF) {
				break
			}
			if e != nil {
				return nil, e
			}
			if len(files) >= 3 || entry.Typeflag != tar.TypeReg || entry.Size > maxAssetBytes {
				return nil, errors.New("unexpected archive entry")
			}
			content, e := io.ReadAll(io.LimitReader(reader, maxAssetBytes+1))
			if e != nil {
				return nil, e
			}
			files = append(files, fileData{entry.Name, content, fs.FileMode(entry.Mode)}) // #nosec G115 -- only low permission bits are used below.
		}
	}
	if len(files) != 3 {
		return nil, errors.New("expected exactly three archive files")
	}
	expected := []string{binaryName(t), "LICENSE", "INSTALL.txt"}
	for i, file := range files {
		mode := fs.FileMode(0o644)
		if i == 0 {
			mode = 0o755
		}
		if file.name != expected[i] || len(file.data) > maxAssetBytes || file.mode.Perm() != mode {
			return nil, fmt.Errorf("unexpected name, size or mode for archive entry %q", file.name)
		}
	}
	return files, nil
}
