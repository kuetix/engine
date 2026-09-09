package main

import (
	"context"
	"fmt"
	"runtime"
	"runtime/debug"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
)

// registeredToolNames is filled by registerTools so server_info can report
// exactly which tools this build exposes.
var registeredToolNames []string

// activeTransport is set in main() ("stdio" | "sse" | "http") for server_info.
var activeTransport = "stdio"

// handleServerInfo answers the server_info tool: build/version metadata plus
// runtime state, so a client can confirm which engine build it is talking to.
func handleServerInfo(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	bi := collectBuildInfo()
	return jsonResult(map[string]interface{}{
		"name":              bi.Name,
		"version":           bi.Version,
		"build_time":        bi.BuildTime,
		"engine_module":     bi.EngineModule,
		"engine_version":    bi.EngineVersion,
		"go_version":        bi.GoVersion,
		"vcs_revision":      bi.VCSRevision,
		"vcs_time":          bi.VCSTime,
		"vcs_modified":      bi.VCSModified,
		"transport":         activeTransport,
		"uptime_seconds":    int64(time.Since(startedAt).Seconds()),
		"tools":             registeredToolNames,
		"runner_bin":        runnerBin,
		"wsl_run_available": strings.TrimSpace(runnerBin) != "",
		"run_scratch_dir":   runScratchDir,
	})
}

// startedAt is stamped once at process start so server_info can report uptime.
var startedAt = time.Now()

// buildInfo is everything a client needs to tell which build of the engine
// (and therefore which WSL grammar / transition pipeline) this MCP server is
// running. It combines the -ldflags stamps (main.Version / main.BuildTime,
// set from `git describe --tags --dirty` by the Makefile) with the module and
// VCS metadata Go embeds automatically.
type buildInfo struct {
	Name          string `json:"name"`
	Version       string `json:"version"`        // git describe of the engine repo, e.g. "v1.2.0"
	BuildTime     string `json:"build_time"`     // RFC3339, or "unknown" for `go run`
	EngineModule  string `json:"engine_module"`  // "github.com/kuetix/engine"
	EngineVersion string `json:"engine_version"` // module version, or the VCS tag/revision for a local build
	GoVersion     string `json:"go_version"`
	VCSRevision   string `json:"vcs_revision,omitempty"`
	VCSTime       string `json:"vcs_time,omitempty"`
	VCSModified   bool   `json:"vcs_modified"`
}

func collectBuildInfo() buildInfo {
	bi := buildInfo{
		Name:          serverName,
		Version:       Version,
		BuildTime:     BuildTime,
		EngineModule:  "github.com/kuetix/engine",
		EngineVersion: Version,
		GoVersion:     runtime.Version(),
	}

	stampedTag := strings.HasPrefix(Version, "v") && Version != ""

	if info, ok := debug.ReadBuildInfo(); ok {
		if info.Main.Path != "" {
			bi.EngineModule = info.Main.Path
		}
		// Prefer the -ldflags `git describe` tag; only use the module version
		// Go embeds when there is no real stamp (keeps one format, not two).
		if !stampedTag && info.Main.Version != "" && info.Main.Version != "(devel)" {
			bi.EngineVersion = info.Main.Version
		}
		for _, s := range info.Settings {
			switch s.Key {
			case "vcs.revision":
				bi.VCSRevision = s.Value
			case "vcs.time":
				bi.VCSTime = s.Value
			case "vcs.modified":
				bi.VCSModified = s.Value == "true"
			}
		}
	}

	// A `go run` / un-stamped `go build` has Version == the dev default; fall
	// back to the VCS revision so the field is still meaningful.
	if (bi.Version == "" || strings.HasSuffix(bi.Version, "-dev")) && !stampedTag && bi.VCSRevision != "" {
		short := bi.VCSRevision
		if len(short) > 12 {
			short = short[:12]
		}
		bi.EngineVersion = short
		if bi.VCSModified {
			bi.EngineVersion += "-dirty"
		}
	}
	return bi
}

func (bi buildInfo) String() string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s %s\n", bi.Name, bi.Version)
	fmt.Fprintf(&b, "  engine:     %s %s\n", bi.EngineModule, bi.EngineVersion)
	fmt.Fprintf(&b, "  build time: %s\n", bi.BuildTime)
	fmt.Fprintf(&b, "  go:         %s\n", bi.GoVersion)
	if bi.VCSRevision != "" {
		dirty := ""
		if bi.VCSModified {
			dirty = " (modified)"
		}
		fmt.Fprintf(&b, "  vcs:        %s%s  %s\n", bi.VCSRevision, dirty, bi.VCSTime)
	}
	return b.String()
}
