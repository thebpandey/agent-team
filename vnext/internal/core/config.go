package core

import "fmt"

type Config struct {
	Schema  int           `json:"schema"`
	Runtime RuntimeConfig `json:"runtime"`
	Tracker TrackerConfig `json:"tracker"`
	Limits  Limits        `json:"limits"`
	Storage StorageLimits `json:"storage"`
}

type RuntimeConfig struct {
	Kind                 string `json:"kind"`
	WorkerContractDigest string `json:"workerContractDigest"`
}

type TrackerConfig struct {
	Kind string `json:"kind"`
	Path string `json:"path"`
}

type Limits struct {
	ParallelTeams       int `json:"parallelTeams"`
	TeamQueueMax        int `json:"teamQueueMax"`
	ProjectTaskCapacity int `json:"projectTaskCapacity"`
	DevServers          int `json:"devServers"`
	BrowserSessions     int `json:"browserSessions"`
}

type StorageLimits struct {
	TrackerBytes       int64 `json:"trackerBytes"`
	CanonicalBytes     int64 `json:"canonicalBytes"`
	HandoffWarnBytes   int64 `json:"handoffWarnBytes"`
	HandoffHardBytes   int64 `json:"handoffHardBytes"`
	ArgumentBytes      int64 `json:"argumentBytes"`
	StagingBytes       int64 `json:"stagingBytes"`
	TrackerWarnPercent int   `json:"trackerWarnPercent"`
}

func DefaultConfig() Config {
	return Config{
		Schema:  1,
		Runtime: RuntimeConfig{Kind: "native"},
		Tracker: TrackerConfig{Kind: "tasks-md", Path: "TASKS.md"},
		Limits:  Limits{ParallelTeams: 4, TeamQueueMax: 8, ProjectTaskCapacity: 1000, DevServers: 2, BrowserSessions: 2},
		Storage: StorageLimits{TrackerBytes: 2 << 20, CanonicalBytes: 16 << 20, HandoffWarnBytes: 192 << 10, HandoffHardBytes: 256 << 10, ArgumentBytes: 1 << 20, StagingBytes: 10 << 20, TrackerWarnPercent: 90},
	}
}

func ApplySettings(c Config, settings map[string]string) (Config, error) {
	for key, value := range settings {
		switch key {
		case "runtime.kind":
			if value != "native" {
				return Config{}, fmt.Errorf("%w: runtime.kind=%s", ErrSettings, value)
			}
			c.Runtime.Kind = value
		case "tracker.kind":
			if value != "tasks-md" && value != "beads" {
				return Config{}, fmt.Errorf("%w: tracker.kind=%s", ErrSettings, value)
			}
			c.Tracker.Kind = value
		case "tracker.path":
			if value == "" {
				return Config{}, fmt.Errorf("%w: tracker.path", ErrSettings)
			}
			c.Tracker.Path = value
		default:
			return Config{}, fmt.Errorf("%w: unknown setting %s", ErrSettings, key)
		}
	}
	return c, nil
}
