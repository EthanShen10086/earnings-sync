package observability

import (
	"bufio"
	"os"
	"strings"
)

// AlertRule is a minimal view of a Prometheus rule for /v1/ops/alerts wiring.
type AlertRule struct {
	Alert  string `json:"alert"`
	Expr   string `json:"expr,omitempty"`
	For    string `json:"for,omitempty"`
	Source string `json:"source"` // file path
}

// LoadAlertRules parses a subset of Prometheus rule YAML without a full YAML dep:
// it looks for lines starting with "- alert:" and optional expr/for under them.
func LoadAlertRules(path string) ([]AlertRule, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var out []AlertRule
	var cur *AlertRule
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimRight(sc.Text(), " \t")
		trim := strings.TrimSpace(line)
		if strings.HasPrefix(trim, "- alert:") {
			if cur != nil {
				out = append(out, *cur)
			}
			name := strings.TrimSpace(strings.TrimPrefix(trim, "- alert:"))
			name = strings.Trim(name, `"'`)
			cur = &AlertRule{Alert: name, Source: path}
			continue
		}
		if cur == nil {
			continue
		}
		if strings.HasPrefix(trim, "expr:") {
			cur.Expr = strings.TrimSpace(strings.TrimPrefix(trim, "expr:"))
			continue
		}
		if strings.HasPrefix(trim, "for:") {
			cur.For = strings.TrimSpace(strings.TrimPrefix(trim, "for:"))
			continue
		}
		// Next rule group item that is not alert ends current.
		if strings.HasPrefix(trim, "- alert:") {
			continue
		}
	}
	if cur != nil {
		out = append(out, *cur)
	}
	return out, sc.Err()
}
