package selfupdate

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	buildinfo "github.com/rchehd/phpwarden"
	"github.com/rchehd/phpwarden/internal/toolset"
)

const MaxArchive = 128 << 20

func checksum(data []byte, name string) (string, error) {
	value := ""
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		if len(fields) != 2 {
			return "", fmt.Errorf("invalid SHA256SUMS entry")
		}
		sum, err := hex.DecodeString(fields[0])
		if err != nil || len(sum) != 32 {
			return "", fmt.Errorf("invalid SHA256 checksum")
		}
		if strings.TrimPrefix(fields[1], "*") == name {
			if value != "" {
				return "", fmt.Errorf("duplicate archive checksum")
			}
			value = strings.ToLower(fields[0])
		}
	}
	if value == "" {
		return "", fmt.Errorf("archive is absent from SHA256SUMS")
	}
	return value, nil
}

func verifyArchive(data []byte, expected, apiDigest string) error {
	actual := fmt.Sprintf("%x", sha256.Sum256(data))
	if actual != expected || apiDigest != "" && apiDigest != "sha256:"+actual {
		return fmt.Errorf("release archive checksum mismatch; the installed CLI was not changed")
	}
	return nil
}

func unpack(data []byte, tag, platform string) ([]byte, buildinfo.Info, error) {
	var metadata buildinfo.Info
	compressed := bytes.NewReader(data)
	zip, err := gzip.NewReader(compressed)
	if err != nil {
		return nil, metadata, fmt.Errorf("invalid release gzip archive")
	}
	defer zip.Close()
	zip.Multistream(false)
	archive := tar.NewReader(zip)
	files := map[string][]byte{}
	var total int64
	for {
		header, err := archive.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, metadata, fmt.Errorf("invalid release tar archive")
		}
		allowed := map[string]int64{"phpwarden": MaxArchive, "release.json": 1 << 20, "compatibility.json": 1 << 20, "README.md": 1 << 20, "COMMIT": 1024, "THIRD_PARTY_NOTICES.txt": 1 << 20, "dependencies.json": 1 << 20}
		limit, ok := allowed[header.Name]
		if !ok || header.Typeflag != tar.TypeReg || header.Size < 1 || header.Size > limit || files[header.Name] != nil {
			return nil, metadata, fmt.Errorf("unexpected, duplicate or oversized archive entry %q", header.Name)
		}
		total += header.Size
		if total > MaxArchive+4<<20 {
			return nil, metadata, fmt.Errorf("unpacked release exceeds limit")
		}
		value, err := io.ReadAll(io.LimitReader(archive, limit+1))
		if err != nil || int64(len(value)) != header.Size {
			return nil, metadata, fmt.Errorf("truncated archive entry")
		}
		files[header.Name] = value
	}
	tail, err := io.ReadAll(io.LimitReader(zip, (1<<20)+1))
	if err != nil || len(tail) > 1<<20 || len(bytes.Trim(tail, "\x00")) != 0 || compressed.Len() != 0 {
		return nil, metadata, fmt.Errorf("release gzip checksum is invalid")
	}
	decoder := json.NewDecoder(bytes.NewReader(files["release.json"]))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&metadata) != nil {
		return nil, metadata, fmt.Errorf("invalid release metadata")
	}
	if _, err := decoder.Token(); err != io.EOF {
		return nil, metadata, fmt.Errorf("trailing release metadata")
	}
	if metadata.Version != tag || metadata.Platform != platform || metadata.Distribution != "standalone" || len(metadata.Commit) != 40 || strings.TrimSpace(string(files["COMMIT"])) != metadata.Commit {
		return nil, metadata, fmt.Errorf("release identity or platform does not match")
	}
	if commit, err := hex.DecodeString(metadata.Commit); err != nil || len(commit) != 20 {
		return nil, metadata, fmt.Errorf("invalid release commit")
	}
	if files["phpwarden"] == nil || files["compatibility.json"] == nil {
		return nil, metadata, fmt.Errorf("release omitted its CLI or compatibility manifest")
	}
	if fmt.Sprintf("%x", sha256.Sum256(files["compatibility.json"])) != metadata.ManifestSHA256 {
		return nil, metadata, fmt.Errorf("release manifest checksum mismatch")
	}
	if _, err := toolset.ParseManifest(files["compatibility.json"]); err != nil {
		return nil, metadata, err
	}
	var identity struct {
		Release string `json:"release"`
	}
	json.Unmarshal(files["compatibility.json"], &identity)
	if identity.Release != tag {
		return nil, metadata, fmt.Errorf("compatibility manifest belongs to a different release")
	}
	return files["phpwarden"], metadata, nil
}
