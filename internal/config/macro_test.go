package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadMacroAndRememberSheet(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "macro.yaml")
	body := []byte("series:\n  - id: cn\n    name: 中国宏观经济\n    title: 汇总\n    spreadsheet_id: \"\"\n")
	if err := os.WriteFile(path, body, 0o644); err != nil {
		t.Fatal(err)
	}
	reg, err := LoadMacro(path)
	if err != nil {
		t.Fatal(err)
	}
	s, ok := reg.Get("cn")
	if !ok || s.ID != "CN" || s.SheetURL() != "" {
		t.Fatalf("%+v", s)
	}
	if err := reg.SetSpreadsheet("CN", "sheet-1"); err != nil {
		t.Fatal(err)
	}
	again, err := LoadMacro(path)
	if err != nil {
		t.Fatal(err)
	}
	s, ok = again.Get("CN")
	if !ok || s.SpreadsheetID != "sheet-1" || s.SheetURL() == "" {
		t.Fatalf("%+v", s)
	}
}
