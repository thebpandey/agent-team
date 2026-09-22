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
	"os"
	"strings"
	"time"
)

type pinnedArchive struct {
	URL, Member, SHA256 string
}

// Digests are pinned from the official GitHub release asset manifests, not
// accepted from a mutable response during installation. See SOURCES.md.
var toolArchives = map[string]map[string]pinnedArchive{
	"uv": {
		"linux_amd64":   uvArchive("x86_64-unknown-linux-musl", "6401c4665d8fa2a9893e087c91f585430738e3170f5398a1141483efb4a93310"),
		"linux_arm64":   uvArchive("aarch64-unknown-linux-musl", "a6096da273d548cb9f277d237a01ac7344a39ef0f455c0e148e4dc9737c1596b"),
		"darwin_amd64":  uvArchive("x86_64-apple-darwin", "8dcf05a8c809bb3c471d2b614788ba27a6e41298fc8c31ac84b5f4339fd468e5"),
		"darwin_arm64":  uvArchive("aarch64-apple-darwin", "85f00cbdc6dd3e97eba4c31b4d014375a9fdfe8f570023b84e5102fc3456896b"),
		"windows_amd64": uvArchive("x86_64-pc-windows-msvc", "a252121d5b59398fcb137c6ea448176459a44010f33f67e0072305a637119ca7"),
		"windows_arm64": uvArchive("aarch64-pc-windows-msvc", "3e1aa6849d77f0e00dc865e4afab5c5b32de053e21fe35bf5ad5cec3734ec976"),
	},
	"rg": {
		"linux_amd64":   rgArchive("x86_64-unknown-linux-musl", "33e15bcf1624b25cdd2a55813a47a2f95dbe126268203e76aa6a585d1e7b149c"),
		"linux_arm64":   rgArchive("aarch64-unknown-linux-musl", "800b1e7206afe799dfb5a6901f23147cfaabe0e52210538100f61e86e1740915"),
		"darwin_amd64":  rgArchive("x86_64-apple-darwin", "af7825fcc69a2afc7a7aea55fc9af90e26421d8f20fe59df32e233c0b8a231c1"),
		"darwin_arm64":  rgArchive("aarch64-apple-darwin", "3750b2e93f37e0c692657da574d7019a101c0084da05a790c83fd335bad973e4"),
		"windows_amd64": rgArchive("x86_64-pc-windows-msvc", "71b2fef860abe467217a538ff31de02f5258807c0129f771846f87bd029aafc5"),
		"windows_arm64": rgArchive("aarch64-pc-windows-msvc", "e4abca10c3a64ebea742667dd7009449d49403db5460dd6873e389fa2945360f"),
	},
	"ast-grep": {
		"linux_amd64":   astArchive("x86_64-unknown-linux-gnu", "f8ac830881339d1edee6b2652f54798c0f4da5a827f2db38a08ee31117783ce8"),
		"linux_arm64":   astArchive("aarch64-unknown-linux-gnu", "b39cfbc58da4b869a88b8a4bc57bd5deb0d24541e704cf7c257da7b53ec81c8f"),
		"darwin_amd64":  astArchive("x86_64-apple-darwin", "b2ffd26f42810340326a9e8a084bdc3647a8795c1a3f21fc06bd7bef3c7c5b2c"),
		"darwin_arm64":  astArchive("aarch64-apple-darwin", "6d2279dea5bea2ad79c66ea93f5fe54ba926e398a8a26de76c56db68fe59eac6"),
		"windows_amd64": astArchive("x86_64-pc-windows-msvc", "3751b7d6be7fd39a80df1180ffe7e053903dcf92d3190e5a8336ff1746af9059"),
		"windows_arm64": astArchive("aarch64-pc-windows-msvc", "5da748848c4cad2a1e7f41e27b58871714dd2c0908632f9874cc2833b92a21ca"),
	},
	"lean-ctx": {
		"linux_amd64":   leanArchive("x86_64-unknown-linux-musl", "4dd50b64f64811daa7798c8ac92c2d93a8179f0e0139e0f332a8c3cadb9537ec"),
		"linux_arm64":   leanArchive("aarch64-unknown-linux-musl", "fc1980d7d891bc1954d666e1ea683feeba4d3a12b0d1e29d04aad265751026c3"),
		"darwin_amd64":  leanArchive("x86_64-apple-darwin", "ebe13f471247590bb3ca043b05f5f0420f218ba49d9ab492922444df00794289"),
		"darwin_arm64":  leanArchive("aarch64-apple-darwin", "a1dac301a77234520729f1451af7959d6bdc1db56856123028398cb06d670615"),
		"windows_amd64": leanArchive("x86_64-pc-windows-msvc", "74cf6cddaa42650bd29c21acfcb58e7b9d89b62986e4434f9b45130e1caa68f6"),
	},
}

func uvArchive(target, hash string) pinnedArchive {
	base := "uv-" + target
	ext, member := ".tar.gz", base+"/uv"
	if strings.Contains(target, "windows") {
		ext, member = ".zip", "uv.exe"
	}
	return pinnedArchive{"https://github.com/astral-sh/uv/releases/download/0.12.17/" + base + ext, member, hash}
}

func rgArchive(target, hash string) pinnedArchive {
	base := "ripgrep-15.2.0-" + target
	ext, binary := ".tar.gz", "rg"
	if strings.Contains(target, "windows") {
		ext, binary = ".zip", "rg.exe"
	}
	return pinnedArchive{"https://github.com/BurntSushi/ripgrep/releases/download/15.2.0/" + base + ext, base + "/" + binary, hash}
}

func astArchive(target, hash string) pinnedArchive {
	binary := "ast-grep"
	if strings.Contains(target, "windows") {
		binary += ".exe"
	}
	return pinnedArchive{"https://github.com/ast-grep/ast-grep/releases/download/0.45.3/app-" + target + ".zip", binary, hash}
}

func leanArchive(target, hash string) pinnedArchive {
	ext, binary := ".tar.gz", "lean-ctx"
	if strings.Contains(target, "windows") {
		ext, binary = ".zip", "lean-ctx.exe"
	}
	return pinnedArchive{"https://github.com/yvgude/lean-ctx/releases/download/v3.10.2/lean-ctx-" + target + ext, binary, hash}
}

func allowedReleaseURL(rawURL string) bool {
	if strings.HasPrefix(rawURL, beadsReleaseURL) {
		return true
	}
	for _, platforms := range toolArchives {
		for _, archive := range platforms {
			if rawURL == archive.URL {
				return true
			}
		}
	}
	return false
}

func installReleaseTool(ctx context.Context, name, dest, goos, arch string, fetch fetcher) error {
	if name == "beads" {
		return installBeads(ctx, dest, goos, arch, fetch)
	}
	archive, ok := toolArchives[name][goos+"_"+arch]
	if !ok {
		return fmt.Errorf("no verified %s release recipe for %s/%s; use the upstream installation guidance", name, goos, arch)
	}
	return installPinnedArchive(ctx, dest, archive, fetch)
}

func installPinnedArchive(ctx context.Context, dest string, archive pinnedArchive, fetch fetcher) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	expected, err := hex.DecodeString(archive.SHA256)
	if err != nil || len(expected) != sha256.Size {
		return errors.New("invalid pinned release checksum")
	}
	data, err := fetch.fetch(ctx, archive.URL, archiveLimit)
	if err != nil {
		return err
	}
	actual := sha256.Sum256(data)
	if !bytes.Equal(actual[:], expected) {
		return errors.New("pinned release checksum mismatch")
	}
	binary, err := singleArchiveMember(data, archive)
	if err != nil {
		return err
	}
	// Never replace an existing installation or follow a destination symlink.
	f, err := os.OpenFile(dest, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0700)
	if err != nil {
		return err
	}
	_, writeErr := f.Write(binary)
	closeErr := f.Close()
	if writeErr != nil || closeErr != nil {
		_ = os.Remove(dest)
		return errors.Join(writeErr, closeErr)
	}
	return nil
}

// Extract exactly one fixed member to memory; archive names never become paths
// on disk. Bound uncompressed TAR work as well as the selected binary's size.
func singleArchiveMember(data []byte, archive pinnedArchive) ([]byte, error) {
	var binary []byte
	if strings.HasSuffix(archive.URL, ".zip") {
		zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
		if err != nil {
			return nil, err
		}
		for _, entry := range zr.File {
			if entry.Name != archive.Member {
				continue
			}
			if binary != nil || !entry.Mode().IsRegular() || entry.UncompressedSize64 > binaryLimit {
				return nil, errors.New("invalid executable archive entry")
			}
			r, err := entry.Open()
			if err != nil {
				return nil, err
			}
			binary, err = readBounded(r, binaryLimit)
			_ = r.Close()
			if err != nil {
				return nil, err
			}
		}
	} else {
		gz, err := gzip.NewReader(bytes.NewReader(data))
		if err != nil {
			return nil, err
		}
		defer gz.Close()
		tr := tar.NewReader(io.LimitReader(gz, binaryLimit+1))
		for {
			entry, err := tr.Next()
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				return nil, err
			}
			if entry.Name != archive.Member {
				continue
			}
			if binary != nil || entry.Typeflag != tar.TypeReg || entry.Size > binaryLimit {
				return nil, errors.New("invalid executable archive entry")
			}
			binary, err = readBounded(tr, binaryLimit)
			if err != nil {
				return nil, err
			}
		}
	}
	if len(binary) == 0 {
		return nil, errors.New("release archive has no nonempty executable")
	}
	return binary, nil
}
