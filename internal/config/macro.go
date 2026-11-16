package config

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// MacroSeries is one macro workbook, not an issuer. CN is the China dashboard.
type MacroSeries struct {
	ID            string `yaml:"id"`
	Name          string `yaml:"name"`
	Title         string `yaml:"title"`
	SpreadsheetID string `yaml:"spreadsheet_id"`
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
		seen[s.ID] = struct{}{}
		r.byID[s.ID] = s
		r.list = append(r.list, s)
	}
	return r, nil
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
		if strings.HasPrefix(trim, "- id:") {
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
