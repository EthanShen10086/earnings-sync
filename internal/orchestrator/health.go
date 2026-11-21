package orchestrator

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// HealthReport is a lightweight readiness snapshot for the local orchestrator.
type HealthReport struct {
	Status    string            `json:"status"` // ok | degraded | fail
	CheckedAt time.Time         `json:"checked_at"`
	Checks    map[string]string `json:"checks"`
}

// HealthOpts configures what to verify.
type HealthOpts struct {
	FlowPath   string
	Tools      ToolRegistry
	RunsPath   string // optional; checks parent dir is writable
	EventsPath string
}

// CheckHealth validates flow load, required tools, and store writability.
func CheckHealth(opts HealthOpts) HealthReport {
	rep := HealthReport{
		Status:    "ok",
		CheckedAt: time.Now().UTC(),
		Checks:    map[string]string{},
	}
	fail := func(name, msg string) {
		rep.Checks[name] = "fail: " + msg
		rep.Status = "fail"
	}
	ok := func(name, msg string) {
		rep.Checks[name] = "ok: " + msg
	}
	degrade := func(name, msg string) {
		rep.Checks[name] = "degraded: " + msg
		if rep.Status == "ok" {
			rep.Status = "degraded"
		}
	}

	if opts.FlowPath == "" {
		fail("flow", "未指定 flow 路径")
	} else {
		def, err := LoadFile(opts.FlowPath)
		if err != nil {
			fail("flow", err.Error())
		} else {
			ok("flow", fmt.Sprintf("%s (%d nodes)", def.Title, len(def.Nodes)))
			tools := opts.Tools
			if tools == nil {
				tools = BuiltinTools()
			}
			missing := []string{}
			for _, n := range def.Nodes {
				if n.Type != "tool" {
					continue
				}
				if _, exists := tools[n.Tool]; !exists {
					missing = append(missing, n.Tool)
				}
			}
			if len(missing) > 0 {
				fail("tools", fmt.Sprintf("缺少工具: %v", missing))
			} else {
				ok("tools", fmt.Sprintf("%d registered", len(tools)))
			}
		}
	}

	for _, path := range []string{opts.RunsPath, opts.EventsPath} {
		if path == "" {
			continue
		}
		dir := filepath.Dir(path)
		if dir == "" || dir == "." {
			dir = "."
		}
		if err := os.MkdirAll(dir, 0o755); err != nil {
			fail("store_"+filepath.Base(path), err.Error())
			continue
		}
		probe := filepath.Join(dir, ".orchestrator-health-probe")
		if err := os.WriteFile(probe, []byte("ok"), 0o644); err != nil {
			fail("store_"+filepath.Base(path), err.Error())
			continue
		}
		_ = os.Remove(probe)
		ok("store_"+filepath.Base(path), "writable")
	}

	if len(rep.Checks) == 0 {
		degrade("general", "未配置检查项")
	}
	return rep
}
