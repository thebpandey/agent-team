package install

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/thebpandey/agent-team/vnext/internal/core"
	"github.com/thebpandey/agent-team/vnext/internal/operatortrust"
	"github.com/thebpandey/agent-team/vnext/internal/store"
)

const legacyAuthorityReceiptPath = ".agent-team/v8/authority.json"

var legacyAuthorityTrustStorePath = systemLegacyAuthorityTrustStorePath()
var legacyAuthorityTrustStoreOwner = systemLegacyAuthorityTrustStoreOwner

type legacyAuthorityEvidence struct {
	ID     string `json:"id"`
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

type legacyAuthorityReceipt struct {
	Schema              int                     `json:"schema"`
	Project             string                  `json:"project"`
	TargetRevision      string                  `json:"targetRevision"`
	ApprovalID          string                  `json:"approvalId"`
	ApprovalSHA256      string                  `json:"approvalSha256"`
	ApprovalSignerKeyID string                  `json:"approvalSignerKeyId"`
	ApprovalSignature   legacyAuthorityEvidence `json:"approvalSignature"`
	Authorization       struct {
		Source string `json:"source"`
	} `json:"authorization"`
	HostInventories []LegacyHostInventory `json:"hostInventories"`
}

type signedLegacyHostApproval struct {
	Schema          int                   `json:"schema"`
	ID              string                `json:"id"`
	Project         string                `json:"project"`
	TargetRevision  string                `json:"targetRevision"`
	SignerKeyID     string                `json:"signerKeyId"`
	Signature       string                `json:"signature"`
	HostInventories []LegacyHostInventory `json:"hostInventories"`
}

func verifyLegacyProjectAuthority(ctx context.Context, project, expectedReceiptSHA string) ([]LegacyHostInventory, error) {
	canonical, err := canonicalLegacyProject(ctx, project)
	if err != nil || canonical != project || !validSHA256(expectedReceiptSHA) {
		return nil, core.ErrRevision
	}
	receiptRaw, _, err := store.New(project, core.StorageLimits{CanonicalBytes: installJournalLimit}).ReadFile(legacyAuthorityReceiptPath, installJournalLimit)
	if err != nil || digestContent(receiptRaw) != expectedReceiptSHA {
		return nil, core.ErrRevision
	}
	var receipt legacyAuthorityReceipt
	if decodeStrictLegacyAuthority(receiptRaw, &receipt) != nil || receipt.Schema != 1 || receipt.Project != project || receipt.Authorization.Source == "" || receipt.ApprovalID == "" || !validSHA256(receipt.ApprovalSHA256) || receipt.ApprovalSignerKeyID == "" || len(receipt.HostInventories) == 0 {
		return nil, core.ErrRevision
	}
	payloadIdentity, payload, err := stableLegacyIdentity(receipt.Authorization.Source, installJournalLimit)
	if err != nil || payloadIdentity.SHA256 != receipt.ApprovalSHA256 {
		return nil, core.ErrRevision
	}
	var approval signedLegacyHostApproval
	if json.Unmarshal(payload, &approval) != nil || approval.Schema != 1 || approval.ID != receipt.ApprovalID || approval.Project != project || approval.TargetRevision != receipt.TargetRevision || approval.SignerKeyID != receipt.ApprovalSignerKeyID || approval.Signature != "" || !exactLegacyInventories(approval.HostInventories, receipt.HostInventories) {
		return nil, core.ErrRevision
	}
	if receipt.ApprovalSignature.ID != approval.ID+"-signature" || !filepath.IsAbs(receipt.ApprovalSignature.Path) {
		return nil, core.ErrRevision
	}
	_, signatureRaw, err := stableLegacyIdentity(receipt.ApprovalSignature.Path, legacyReceiptLimit)
	if err != nil || receipt.ApprovalSignature.SHA256 != "" && digestContent(signatureRaw) != receipt.ApprovalSignature.SHA256 {
		return nil, core.ErrRevision
	}
	signature := signatureRaw
	if len(signature) != 64 {
		signature, err = base64.StdEncoding.DecodeString(string(bytes.TrimSpace(signatureRaw)))
		if err != nil {
			return nil, core.ErrRevision
		}
	}
	if operatortrust.Verify(legacyAuthorityTrustStorePath, legacyAuthorityTrustStoreOwner, approval.SignerKeyID, payload, signature) != nil {
		return nil, core.ErrRevision
	}
	return append([]LegacyHostInventory(nil), receipt.HostInventories...), nil
}

func canonicalLegacyProject(ctx context.Context, project string) (string, error) {
	if ctx == nil || ctx.Err() != nil || project == "" || !filepath.IsAbs(project) || filepath.Clean(project) != project {
		return "", core.ErrPath
	}
	resolved, err := filepath.EvalSymlinks(project)
	if err != nil || !sameHostPath(resolved, project) {
		return "", core.ErrPath
	}
	output, err := exec.CommandContext(ctx, "git", "-C", project, "rev-parse", "--show-toplevel").Output()
	root := filepath.Clean(strings.TrimSpace(string(output)))
	if err != nil || !sameHostPath(root, project) {
		return "", core.ErrPath
	}
	return project, nil
}

func exactLegacyInventories(left, right []LegacyHostInventory) bool {
	return reflect.DeepEqual(normalizeLegacyInventories(left), normalizeLegacyInventories(right))
}

func normalizeLegacyInventories(values []LegacyHostInventory) []LegacyHostInventory {
	result := append([]LegacyHostInventory(nil), values...)
	for index := range result {
		result[index].Handlers = append([]LegacyHandlerInventory(nil), result[index].Handlers...)
		for handler := range result[index].Handlers {
			compact := new(bytes.Buffer)
			if json.Compact(compact, result[index].Handlers[handler].Handler) == nil {
				result[index].Handlers[handler].Handler = compact.Bytes()
			}
		}
	}
	return result
}

func decodeStrictLegacyAuthority(raw []byte, destination any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return core.ErrRevision
	}
	return nil
}

func systemLegacyAuthorityTrustStorePath() string { return operatortrust.Path() }
func systemLegacyAuthorityTrustStoreOwner(path string, info os.FileInfo) bool {
	return operatortrust.Owner(path, info)
}
