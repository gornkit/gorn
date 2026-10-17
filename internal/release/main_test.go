package main

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestVersionContract(t *testing.T) {
	for _, v := range []string{"v1.0.0", "v1.0.0-rc.1", "v1.10.3"} {
		if !validVersion(v) {
			t.Errorf("rejected %q", v)
		}
	}
	for _, v := range []string{"", "v1", "v1.0", "1.0.0", "v2.0.0", "v1.0.0+foo", "v1.00.0", "v1.0.0\n"} {
		if validVersion(v) {
			t.Errorf("accepted %q", v)
		}
	}
}

func TestArchiveRoundTrip(t *testing.T) {
	for _, target := range targets {
		t.Run(target.OS+"-"+target.Arch, func(t *testing.T) {
			binary := []byte{0, 1, 2, 255}
			timestamp := time.Unix(1792250000, 0).UTC()
			packed, err := pack(target, timestamp, binary, []byte("license"), []byte("install"))
			if err != nil {
				t.Fatal(err)
			}
			again, err := pack(target, timestamp, binary, []byte("license"), []byte("install"))
			if err != nil || !bytes.Equal(packed, again) {
				t.Fatal("archive packaging is not deterministic", err)
			}
			files, err := unpack(target, packed)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(files[0].data, binary) || files[1].name != "LICENSE" || files[2].name != "INSTALL.txt" {
				t.Fatal(files)
			}
			if _, err = unpack(target, packed[:len(packed)/2]); err == nil {
				t.Fatal("accepted truncated archive")
			}
		})
	}
}

func TestArchiveRejectsUnsafeMember(t *testing.T) {
	for _, name := range []string{"../gorn.exe", "/gorn.exe", "gorn.exe"} {
		var buffer bytes.Buffer
		writer := zip.NewWriter(&buffer)
		for _, entryName := range []string{name, "LICENSE", "INSTALL.txt"} {
			entry, err := writer.Create(entryName)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = entry.Write([]byte("x")); err != nil {
				t.Fatal(err)
			}
		}
		if err := writer.Close(); err != nil {
			t.Fatal(err)
		}
		// Even the expected member name fails: Create leaves non-executable mode.
		if _, err := unpack(target{"windows", "amd64"}, buffer.Bytes()); err == nil {
			t.Fatal("accepted unsafe name or incorrect executable mode", name)
		}
	}
}

func TestChecksumMembership(t *testing.T) {
	var lines []string
	for _, target := range targets {
		lines = append(lines, strings.Repeat("a", 64)+"  "+archiveName("v1.0.0", target))
	}
	lines = append(lines, strings.Repeat("b", 64)+"  BUILD_INFO.json", strings.Repeat("c", 64)+"  RELEASE_NOTES.md")
	good := strings.Join(lines, "\n") + "\n"
	if _, err := parseSums(good); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{good + lines[0] + "\n", strings.Replace(good, "BUILD_INFO.json", "../BUILD_INFO.json", 1), strings.Replace(good, strings.Repeat("a", 64), "no-hash", 1), strings.Join(lines[1:], "\n")} {
		if _, err := parseSums(bad); err == nil {
			t.Fatal("accepted invalid checksum list", bad)
		}
	}
}

func TestPublicVerificationNeutralizesPrivatePatterns(t *testing.T) {
	for _, key := range []string{"GOPRIVATE", "GONOSUMDB", "GONOPROXY", "GOINSECURE"} {
		t.Setenv(key, "*")
	}
	t.Setenv("GOSUMDB", "off")
	t.Setenv("GOAUTH", "netrc")
	env := isolatedEnvironment(t.TempDir())
	values := map[string]string{}
	for _, entry := range env {
		key, value, _ := strings.Cut(entry, "=")
		values[strings.ToUpper(key)] = value
	}
	for _, key := range []string{"GOPRIVATE", "GONOSUMDB", "GONOPROXY", "GOINSECURE"} {
		if values[key] != "" {
			t.Fatal(key, values[key])
		}
	}
	if values["GOSUMDB"] != "sum.golang.org" || values["GOPROXY"] != "https://proxy.golang.org,direct" || values["GOAUTH"] != "off" {
		t.Fatal(values)
	}
}

func TestPublishedVerificationRejectsWrongProvenance(t *testing.T) {
	for _, preview := range []bool{true, false} {
		t.Run(fmt.Sprint(preview), func(t *testing.T) {
			dir := t.TempDir()
			info := buildInfo{Schema: 1, Version: "v1.0.0", Revision: strings.Repeat("0", 40), Preview: preview, Artifacts: make([]artifact, len(targets))}
			encoded, err := json.Marshal(info)
			if err != nil {
				t.Fatal(err)
			}
			if err = os.WriteFile(filepath.Join(dir, "BUILD_INFO.json"), encoded, 0o600); err != nil {
				t.Fatal(err)
			}
			if err = os.WriteFile(filepath.Join(dir, "RELEASE_NOTES.md"), []byte("notes"), 0o600); err != nil {
				t.Fatal(err)
			}
			var sums strings.Builder
			fmt.Fprintf(&sums, "%s  BUILD_INFO.json\n%s  RELEASE_NOTES.md\n", digest(encoded), digest([]byte("notes")))
			for _, target := range targets {
				fmt.Fprintf(&sums, "%s  %s\n", strings.Repeat("a", 64), archiveName("v1.0.0", target))
			}
			if err = os.WriteFile(filepath.Join(dir, "SHA256SUMS.txt"), []byte(sums.String()), 0o600); err != nil {
				t.Fatal(err)
			}
			err = verify(context.Background(), "v1.0.0", dir, true)
			want := "revision differs"
			if preview {
				want = "preview assets"
			}
			if err == nil || !strings.Contains(err.Error(), want) {
				t.Fatalf("wanted %q, got %v", want, err)
			}
		})
	}
}
