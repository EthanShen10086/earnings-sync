package monorepo_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// TestLayoutGuards keeps the deepened monorepo shape honest without requiring
// mobile SDKs on CI (make test stays go-only).
func TestLayoutGuards(t *testing.T) {
	root := repoRoot(t)
	required := []string{
		"apps/api/README.md",
		"apps/api/Makefile",
		"apps/web/index.html",
		"apps/web/app.js",
		"apps/web/README.md",
		"apps/mobile-ios/project.yml",
		"apps/mobile-ios/EarningsSync/ContentView.swift",
		"apps/mobile-ios/EarningsSync.xcodeproj/project.pbxproj",
		"apps/mobile-android/settings.gradle.kts",
		"apps/mobile-android/gradlew",
		"apps/mobile-android/gradle/wrapper/gradle-wrapper.properties",
		"apps/mobile-android/app/build.gradle.kts",
		"apps/mobile-android/app/src/main/java/com/earningssync/mobile/ApiClient.kt",
		"apps/mobile-android/app/src/main/java/com/earningssync/mobile/MainActivity.kt",
		"packages/api-client/index.js",
		"packages/api-types/openapi-lite.json",
		"packages/swift-api-client/Package.swift",
		"docs/monorepo.md",
		"docs/mobile.md",
		"docs/coexistence-acceptance.md",
		"docs/api-contract.md",
	}
	for _, rel := range required {
		path := filepath.Join(root, rel)
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("missing %s: %v", rel, err)
		}
	}

	openapi, err := os.ReadFile(filepath.Join(root, "packages/api-types/openapi-lite.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(openapi, []byte(`"2026-11-22.1"`)) {
		t.Fatal("openapi-lite.json should pin contract 2026-11-22.1")
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	dir := wd
	for i := 0; i < 8; i++ {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	t.Fatal("go.mod not found from", wd)
	return ""
}
