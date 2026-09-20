package deploy

import (
	"strconv"
	"strings"

	"github.com/thebpandey/agent-team/vnext/internal/core"
)

type DeployRequest struct {
	Action                   string
	RunID, Target, ProfileID string
	BatchSize                int
	Resume                   bool
}

func ParseDeployArgs(args []string) (DeployRequest, error) {
	if len(args) == 0 || args[0] != "deploy" {
		return DeployRequest{}, ErrAction
	}
	request := DeployRequest{Action: "deploy"}
	seen := map[string]bool{}
	for i := 1; i < len(args); {
		flag := args[i]
		if flag == "--resume" {
			if seen[flag] {
				return DeployRequest{}, core.ErrSettings
			}
			seen[flag], request.Resume = true, true
			i++
			continue
		}
		if i+1 >= len(args) || seen[flag] || unsafeDeployValue(args[i+1]) {
			return DeployRequest{}, core.ErrSettings
		}
		value := args[i+1]
		seen[flag] = true
		switch flag {
		case "--run":
			request.RunID = value
		case "--target":
			request.Target = value
		case "--profile":
			request.ProfileID = value
		case "--batch-size":
			size, err := strconv.Atoi(value)
			if err != nil || size < 1 || size > 100 || strconv.Itoa(size) != value {
				return DeployRequest{}, core.ErrBatch
			}
			request.BatchSize = size
		default:
			return DeployRequest{}, ErrAction
		}
		i += 2
	}
	if strings.EqualFold(request.Target, "production") {
		return DeployRequest{}, core.ErrSettings
	}
	return request, nil
}

func ResolveDeployRun(request DeployRequest, candidates []string) (DeployRequest, error) {
	if request.RunID != "" {
		for _, id := range candidates {
			if id == request.RunID {
				return request, nil
			}
		}
		return DeployRequest{}, core.ErrSettings
	}
	if len(candidates) != 1 || unsafeDeployValue(candidates[0]) {
		return DeployRequest{}, core.ErrSettings
	}
	request.RunID = candidates[0]
	return request, nil
}

func ApplyDeployDefaults(request DeployRequest, target, profile string) (DeployRequest, error) {
	if request.Target == "" {
		request.Target = target
	}
	if request.ProfileID == "" {
		request.ProfileID = profile
	}
	if unsafeDeployValue(request.Target) || unsafeDeployValue(request.ProfileID) || strings.EqualFold(request.Target, "production") {
		return DeployRequest{}, core.ErrSettings
	}
	return request, nil
}

func unsafeDeployValue(value string) bool {
	return value == "" || strings.TrimSpace(value) != value || strings.ContainsAny(value, ";&|$`\x00\r\n")
}
