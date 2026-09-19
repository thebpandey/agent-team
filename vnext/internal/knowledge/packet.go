package knowledge

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/thebpandey/agent-team/vnext/internal/core"
)

// PacketDigest returns the stable digest of the full immutable assignment
// packet, including its envelope and resource snapshot.
func PacketDigest(packet core.AssignmentPacket) (string, error) {
	if err := validatePacket(packet); err != nil {
		return "", err
	}
	encoded, err := json.Marshal(packet)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(encoded)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

// ValidatePacket confirms a packet still has the immutable digest that was
// assigned to the team; callers must create a new packet for any change.
func ValidatePacket(packet core.AssignmentPacket, digest string) error {
	if !strings.HasPrefix(digest, "sha256:") || len(digest) != len("sha256:")+64 {
		return fmt.Errorf("%w: invalid packet digest", core.ErrPath)
	}
	actual, err := PacketDigest(packet)
	if err != nil {
		return err
	}
	if actual != digest {
		return fmt.Errorf("%w: assignment packet digest mismatch", ErrConflict)
	}
	return nil
}

func validatePacket(packet core.AssignmentPacket) error {
	if err := validateEnvelope(packet.RecordEnvelope); err != nil {
		return err
	}
	for name, value := range map[string]string{"task": string(packet.Task), "team": string(packet.Team), "spec revision": packet.SpecRevision, "queue fingerprint": packet.QueueFingerprint, "owner": packet.Owner, "worktree": packet.Worktree, "base": packet.Base, "next action": packet.NextAction} {
		if value == "" {
			return fmt.Errorf("%w: packet %s is required", core.ErrRevision, name)
		}
		if err := validateText(value, name); err != nil {
			return err
		}
	}
	if err := validateSegment(string(packet.Task)); err != nil {
		return err
	}
	if err := validateSegment(string(packet.Team)); err != nil {
		return err
	}
	if packet.ReceiptPath != "" {
		if err := validatePointer(packet.ReceiptPath); err != nil {
			return err
		}
	}
	return nil
}
