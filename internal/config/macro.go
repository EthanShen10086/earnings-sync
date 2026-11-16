package config

import (
	"fmt"
	"os"
	"strings"

	"github.com/EthanShen10086/earnings-sync/internal/model"
	"gopkg.in/yaml.v3"
)

// MacroSeries is one macro workbook, not an issuer. CN is the China dashboard.
// Panels are the sheets inside it. An empty list means the monthly rate sheet only.
type MacroSeries struct {
	ID            string             `yaml:"id"`
	Name          string             `yaml:"name"`
	Title         string             `yaml:"title"`
	SpreadsheetID string             `yaml:"spreadsheet_id"`
	Panels        []model.MacroPanel `yaml:"panels,omitempty"`
}

func (s MacroSeries) SheetURL() string {
	if s.SpreadsheetID == "" {
		return ""
	}
	return "https://docs.google.com/spreadsheets/d/" + s.SpreadsheetID
}

type macroFile struct {
	Series []MacroSeries `yaml:"series"`
}

// MacroRegistry looks series up by id.
type MacroRegistry struct {
	path string
	byID map[string]MacroSeries
	list []MacroSeries
}

// LoadMacro reads the series file. A missing file is an empty registry.
func LoadMacro(path string) (*MacroRegistry, error) {
	r := &MacroRegistry{path: path, byID: map[string]MacroSeries{}}
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return r, nil
		}
		return nil, err
	}
	var f macroFile
	if err := yaml.Unmarshal(b, &f); err != nil {
		return nil, err
	}
	seen := map[string]struct{}{}
	for _, s := range f.Series {
		s.ID = strings.ToUpper(strings.TrimSpace(s.ID))
		if s.ID == "" {
			return nil, fmt.Errorf("macro series is missing id")
		}
		if _, ok := seen[s.ID]; ok {
			return nil, fmt.Errorf("duplicate macro series %s", s.ID)
		}
		if err := normalizePanels(&s); err != nil {
			return nil, err
		}
		seen[s.ID] = struct{}{}
		r.byID[s.ID] = s
		r.list = append(r.list, s)
	}
	return r, nil
}

func normalizePanels(s *MacroSeries) error {
	if len(s.Panels) == 0 {
		title := s.Title
		if title == "" {
			title = s.Name
		}
		s.Panels = []model.MacroPanel{{
			ID:     "monthly",
			Sheet:  "宏观",
			Layout: "rate",
			Title:  title,
		}}
		return nil
	}
	seenSheet := map[string]struct{}{}
	seenID := map[string]struct{}{}
	for i := range s.Panels {
		p := &s.Panels[i]
		p.ID = strings.TrimSpace(p.ID)
		p.Layout = strings.ToLower(strings.TrimSpace(p.Layout))
		if p.ID == "" {
			return fmt.Errorf("%s 有一块表没有 id", s.ID)
		}
		if _, ok := seenID[p.ID]; ok {
			return fmt.Errorf("%s 的表 id 重复：%s", s.ID, p.ID)
		}
		seenID[p.ID] = struct{}{}
		switch p.Layout {
		case "rate":
			if p.Sheet == "" {
				p.Sheet = "宏观"
			}
			if p.Title == "" {
				p.Title = s.Title
			}
		case "level":
			window, err := canonicalWindow(p.Window)
			if err != nil {
				return fmt.Errorf("%s 的 %s：%w", s.ID, p.ID, err)
			}
			p.Window = window
			if p.Sheet == "" {
				p.Sheet = "历年" + windowLabel(window)
			}
			if p.Title == "" {
				p.Title = p.Sheet
			}
			if len(p.Indicators) == 0 {
				return fmt.Errorf("%s 的 %s 没有指标列", s.ID, p.ID)
			}
			keys := map[string]struct{}{}
			for _, col := range p.Indicators {
				if col.Key == "" || col.Header == "" {
					return fmt.Errorf("%s 的 %s 有指标缺 key 或 header", s.ID, p.ID)
				}
				if _, ok := keys[col.Key]; ok {
					return fmt.Errorf("%s 的 %s 指标重复：%s", s.ID, p.ID, col.Key)
				}
				keys[col.Key] = struct{}{}
			}
			if len(p.Derived) == 0 {
				p.Derived = []string{"yoy_pct", "yoy_delta", "vs_peak_pct", "vs_peak_delta"}
			}
			for _, kind := range p.Derived {
				if derivedLabel(kind) == "" {
					return fmt.Errorf("%s 的 %s 不认识派生行 %s", s.ID, p.ID, kind)
				}
			}
			for _, ch := range p.Charts {
				if len(ch.Indicators) == 0 {
					return fmt.Errorf("%s 的 %s 有图没有指标", s.ID, p.ID)
				}
				for _, key := range ch.Indicators {
					if _, ok := keys[key]; !ok {
						return fmt.Errorf("%s 的 %s 图引用了不存在的指标 %s", s.ID, p.ID, key)
					}
				}
				if ch.Callout != "" {
					if _, ok := keys[ch.Callout]; !ok {
						return fmt.Errorf("%s 的 %s 图的 callout 不是指标列", s.ID, p.ID)
					}
				}
			}
		default:
			return fmt.Errorf("%s 的 %s layout 只能是 rate 或 level", s.ID, p.ID)
		}
		if _, ok := seenSheet[p.Sheet]; ok {
			return fmt.Errorf("%s 的工作表名重复：%s", s.ID, p.Sheet)
		}
		seenSheet[p.Sheet] = struct{}{}
	}
	return nil
}

func canonicalWindow(raw string) (string, error) {
	s := strings.TrimSpace(raw)
	s = strings.TrimPrefix(s, "历年")
	s = strings.TrimSuffix(s, "月")
	s = strings.ReplaceAll(s, "—", "-")
	s = strings.ReplaceAll(s, "–", "-")
	s = strings.ReplaceAll(s, "~", "-")
	s = strings.ReplaceAll(s, " ", "")
	if s == "" {
		return "", fmt.Errorf("水平值表要写 window，例如 1-10")
	}
	left, right, ok := strings.Cut(s, "-")
	if !ok {
		return "", fmt.Errorf("window 要写成 1-10 这样的月份区间")
	}
	a, errA := atoi(left)
	b, errB := atoi(right)
	if errA != nil || errB != nil || a < 1 || a > 12 || b < 1 || b > 12 || a > b {
		return "", fmt.Errorf("window %s 不是有效月份区间", raw)
	}
	return itoa(a) + "-" + itoa(b), nil
}

func windowLabel(window string) string {
	return window + "月"
}

func derivedLabel(kind string) string {
	switch kind {
	case "yoy_pct":
		return "同比变动 (%)"
	case "yoy_delta":
		return "同比增量"
	case "vs_peak_pct":
		return "较历年同期最高 (%)"
	case "vs_peak_delta":
		return "较历年同期最高"
	default:
		return ""
	}
}

func atoi(s string) (int, error) {
	n := 0
	if s == "" {
		return 0, fmt.Errorf("empty")
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0, fmt.Errorf("nan")
		}
		n = n*10 + int(r-'0')
	}
	return n, nil
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [4]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}

func (r *MacroRegistry) Get(id string) (MacroSeries, bool) {
	if r == nil {
		return MacroSeries{}, false
	}
	s, ok := r.byID[strings.ToUpper(strings.TrimSpace(id))]
	return s, ok
}

func (r *MacroRegistry) List() []MacroSeries {
	if r == nil {
		return nil
	}
	out := make([]MacroSeries, len(r.list))
	copy(out, r.list)
	return out
}

// SetSpreadsheet remembers a workbook and writes it back to the series file.
func (r *MacroRegistry) SetSpreadsheet(id, sheetID string) error {
	if r == nil {
		return fmt.Errorf("no macro registry")
	}
	id = strings.ToUpper(strings.TrimSpace(id))
	s, ok := r.byID[id]
	if !ok {
		return fmt.Errorf("unknown series %s", id)
	}
	s.SpreadsheetID = sheetID
	r.byID[id] = s
	for i := range r.list {
		if r.list[i].ID == id {
			r.list[i].SpreadsheetID = sheetID
		}
	}
	if r.path == "" {
		return nil
	}
	b, err := os.ReadFile(r.path)
	if err != nil {
		return err
	}
	updated, err := replaceMacroSpreadsheetID(string(b), id, sheetID)
	if err != nil {
		return err
	}
	return os.WriteFile(r.path, []byte(updated), 0o644)
}

func replaceMacroSpreadsheetID(src, id, sheetID string) (string, error) {
	lines := strings.Split(src, "\n")
	in := false
	idLine := -1
	for i, line := range lines {
		trim := strings.TrimSpace(line)
		if strings.HasPrefix(trim, "- id:") && len(line)-len(strings.TrimLeft(line, " ")) <= 2 {
			in = strings.EqualFold(strings.TrimSpace(strings.TrimPrefix(trim, "- id:")), id)
			if in {
				idLine = i
			}
			continue
		}
		if in && strings.HasPrefix(trim, "spreadsheet_id:") {
			indent := line[:len(line)-len(strings.TrimLeft(line, " "))]
			lines[i] = indent + "spreadsheet_id: " + sheetID
			return strings.Join(lines, "\n"), nil
		}
	}
	if idLine < 0 {
		return "", fmt.Errorf("series %s was not found in registry", id)
	}
	insert := "    spreadsheet_id: " + sheetID
	out := append([]string{}, lines[:idLine+1]...)
	out = append(out, insert)
	out = append(out, lines[idLine+1:]...)
	return strings.Join(out, "\n"), nil
}
