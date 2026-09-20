package operatortrust

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"

	"github.com/thebpandey/agent-team/vnext/internal/core"
	"github.com/thebpandey/agent-team/vnext/internal/store"
)

const trustLimit = 64 << 10

type key struct {
	Algorithm string `json:"algorithm"`
	KeyID     string `json:"keyId"`
	PublicKey string `json:"publicKey"`
}

type document struct {
	Schema int   `json:"schema"`
	Keys   []key `json:"keys"`
}

// Verify checks an Ed25519 signature against an ownership-validated operator
// trust store. Callers choose the path only for tests; production passes Path.
func Verify(path string, owner func(string, os.FileInfo) bool, keyID string, payload, signature []byte) error {
	info, err := os.Lstat(path)
	if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > trustLimit || owner == nil || !owner(path, info) {
		return core.ErrRevision
	}
	raw, _, err := store.New(filepath.Dir(path), core.StorageLimits{CanonicalBytes: trustLimit}).ReadFile(filepath.Base(path), trustLimit)
	if err != nil {
		return core.ErrRevision
	}
	var trust document
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&trust) != nil || !errors.Is(decoder.Decode(&struct{}{}), io.EOF) || trust.Schema != 1 || len(trust.Keys) == 0 || len(trust.Keys) > 32 || !sort.SliceIsSorted(trust.Keys, func(i, j int) bool { return trust.Keys[i].KeyID < trust.Keys[j].KeyID }) {
		return core.ErrRevision
	}
	for i, candidate := range trust.Keys {
		if candidate.KeyID == "" || i > 0 && trust.Keys[i-1].KeyID == candidate.KeyID {
			return core.ErrRevision
		}
		if candidate.KeyID != keyID {
			continue
		}
		publicKey, decodeErr := base64.StdEncoding.DecodeString(candidate.PublicKey)
		digest := sha256.Sum256(publicKey)
		if decodeErr != nil || candidate.Algorithm != "ed25519" || len(publicKey) != ed25519.PublicKeySize || hex.EncodeToString(digest[:]) != candidate.KeyID || !ed25519.Verify(publicKey, payload, signature) {
			return core.ErrRevision
		}
		return nil
	}
	return core.ErrRevision
}
