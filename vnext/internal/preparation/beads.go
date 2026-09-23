package preparation

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"time"
)

const beadsReleaseURL = "https://github.com/gastownhall/beads/releases/download/v1.3.0/"
const archiveLimit = 128 << 20
const binaryLimit = 256 << 20

type fetcher interface {
	fetch(context.Context, string, int64) ([]byte, error)
}

func installBeads(ctx context.Context, dest, goos, arch string, fetch fetcher) error {
	platform := goos + "_" + arch
	switch platform {
	case "linux_amd64", "linux_arm64", "darwin_amd64", "darwin_arm64", "windows_amd64", "windows_arm64", "freebsd_amd64", "android_arm64":
	default:
		return fmt.Errorf("no verified Beads v1.3.0 release recipe for %s; install from %s", platform, "https://github.com/gastownhall/beads")
	}
	ext := ".tar.gz"
	binary := "bd"
	if goos == "windows" {
		ext = ".zip"
		binary = "bd.exe"
	}
	asset := "beads_1.3.0_" + platform + ext
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	checksums, err := fetch.fetch(ctx, beadsReleaseURL+"checksums.txt", 1<<20)
	if err != nil {
		return err
	}
	expected := ""
	for _, line := range strings.Split(string(checksums), "\n") {
		parts := strings.Fields(line)
		if len(parts) == 2 && strings.TrimPrefix(parts[1], "*") == asset {
			if expected != "" {
				return errors.New("ambiguous Beads release checksum")
			}
			expected = parts[0]
		}
	}
	digest, err := hex.DecodeString(expected)
	if err != nil || len(digest) != sha256.Size {
		return errors.New("missing or invalid Beads release checksum")
	}
	archive, err := fetch.fetch(ctx, beadsReleaseURL+asset, archiveLimit)
	if err != nil {
		return err
	}
	actual := sha256.Sum256(archive)
	if !bytes.Equal(actual[:], digest) {
		return errors.New("Beads release checksum mismatch")
	}
	var data []byte
	if goos == "windows" {
		zr, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
		if err != nil {
			return err
		}
		for _, f := range zr.File {
			if f.Name == binary {
				if data != nil || !f.Mode().IsRegular() {
					return errors.New("invalid Beads binary archive entry")
				}
				r, err := f.Open()
				if err != nil {
					return err
				}
				data, err = readBounded(r, binaryLimit)
				_ = r.Close()
				if err != nil {
					return err
				}
			}
		}
	} else {
		gz, err := gzip.NewReader(bytes.NewReader(archive))
		if err != nil {
			return err
		}
		defer gz.Close()
		tr := tar.NewReader(io.LimitReader(gz, binaryLimit+1))
		for {
			h, err := tr.Next()
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				return err
			}
			if h.Name == binary {
				if data != nil || h.Typeflag != tar.TypeReg || h.Size > binaryLimit {
					return errors.New("invalid Beads binary archive entry")
				}
				data, err = readBounded(tr, binaryLimit)
				if err != nil {
					return err
				}
			}
		}
	}
	if len(data) == 0 {
		return errors.New("verified release archive has no Beads executable")
	}
	return publishExecutable(dest, data)
}

func readBounded(r io.Reader, limit int64) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(r, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, errors.New("dependency download exceeds size limit")
	}
	return data, nil
}

func (n nativeRunner) fetch(ctx context.Context, rawURL string, limit int64) ([]byte, error) {
	u, err := url.Parse(rawURL)
	if err != nil || u.Scheme != "https" || u.Host != "github.com" || !allowedReleaseURL(rawURL) {
		return nil, errors.New("dependency download source is not allowlisted")
	}
	client := http.Client{Timeout: 5 * time.Minute}
	if n.client != nil {
		client = *n.client
	}
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) > 5 || req.URL.Scheme != "https" || req.URL.User != nil {
			return errors.New("dependency download redirect rejected")
		}
		switch req.URL.Host {
		case "github.com", "release-assets.githubusercontent.com", "objects.githubusercontent.com":
			return nil
		default:
			return errors.New("dependency download redirect host rejected")
		}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("download %s: request failed", filepath.Base(u.Path))
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download %s: HTTP %d", filepath.Base(u.Path), resp.StatusCode)
	}
	if resp.ContentLength > limit {
		return nil, errors.New("dependency download exceeds size limit")
	}
	return readBounded(resp.Body, limit)
}
