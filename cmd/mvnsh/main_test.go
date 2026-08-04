package main

import (
	"encoding/xml"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstallProfileCreatesAndUpdatesManagedProfile(t *testing.T) {
	input := []byte("<settings><mirrors><mirror><id>central</id></mirror></mirrors></settings>")
	first, err := installProfile(input, "mvn-sh-acme", "https://acme.mvn.sh/releases", "mvn_old")
	if err != nil {
		t.Fatal(err)
	}
	second, err := installProfile(first, "mvn-sh-acme", "https://acme.mvn.sh/releases", "mvn_new")
	if err != nil {
		t.Fatal(err)
	}
	text := string(second)
	if strings.Count(text, "<server><id>mvn-sh-acme</id>") != 1 || strings.Contains(text, "mvn_old") || !strings.Contains(text, "mvn_new") {
		t.Fatalf("profile was not updated safely: %s", text)
	}
	var root struct{ XMLName xml.Name }
	if err = xml.Unmarshal(second, &root); err != nil || root.XMLName.Local != "settings" {
		t.Fatalf("invalid generated XML: %v", err)
	}
	if !strings.Contains(text, "<mirrors>") {
		t.Fatal("existing settings were discarded")
	}
}
func TestInstallProfilePreservesCredentialsForOtherRepositories(t *testing.T) {
	input := []byte("<settings></settings>")
	releases, err := installProfile(input, "mvn-sh-acme-releases", "https://acme.mvn.sh/releases", "mvn_releases")
	if err != nil {
		t.Fatal(err)
	}
	both, err := installProfile(releases, "mvn-sh-acme-snapshots", "https://acme.mvn.sh/snapshots", "mvn_snapshots")
	if err != nil {
		t.Fatal(err)
	}
	text := string(both)
	for _, expected := range []string{
		"<server><id>mvn-sh-acme-releases</id>",
		"<server><id>mvn-sh-acme-snapshots</id>",
		"mvn_releases",
		"mvn_snapshots",
	} {
		if !strings.Contains(text, expected) {
			t.Fatalf("missing %q after installing both repositories: %s", expected, text)
		}
	}
}

func TestConfigureProjectUpdatesMavenProject(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "pom.xml")
	input := []byte(`<project xmlns="http://maven.apache.org/POM/4.0.0"><modelVersion>4.0.0</modelVersion></project>`)
	if err := os.WriteFile(path, input, 0644); err != nil {
		t.Fatal(err)
	}

	args := []string{"--project", dir, "--team", "acme", "--repository", "releases"}
	if err := configureProject(args); err != nil {
		t.Fatal(err)
	}
	// Running the command again must update its managed entry, not duplicate it.
	if err := configureProject(args); err != nil {
		t.Fatal(err)
	}

	configured, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(configured)
	if strings.Count(text, "<!-- mvnsh:project-repository-mvn-sh-acme-releases:start -->") != 1 ||
		strings.Count(text, "<!-- mvnsh:distribution-management-mvn-sh-acme-releases:start -->") != 1 {
		t.Fatalf("expected one managed repository and distribution configuration, got: %s", text)
	}
	if !strings.Contains(text, "<distributionManagement>") || !strings.Contains(text, "<url>https://acme.mvn.sh/releases</url>") {
		t.Fatalf("configured repository URL is missing: %s", text)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0644 {
		t.Fatalf("build file permissions changed: mode=%v", info.Mode().Perm())
	}
}

func TestConfigurePOMPreservesProjectAndIsIdempotent(t *testing.T) {
	input := []byte(`<project xmlns="http://maven.apache.org/POM/4.0.0"><modelVersion>4.0.0</modelVersion><repositories><repository><id>central</id></repository></repositories></project>`)
	first, err := configurePOM(input, "mvn-sh-acme-releases", "https://acme.mvn.sh/releases")
	if err != nil {
		t.Fatal(err)
	}
	second, err := configurePOM(first, "mvn-sh-acme-releases", "https://acme.mvn.sh/releases")
	if err != nil {
		t.Fatal(err)
	}
	text := string(second)
	if strings.Count(text, "<!-- mvnsh:project-repository-mvn-sh-acme-releases:start -->") != 1 ||
		strings.Count(text, "<!-- mvnsh:distribution-management-mvn-sh-acme-releases:start -->") != 1 ||
		!strings.Contains(text, "<id>central</id>") {
		t.Fatalf("unexpected configured POM: %s", text)
	}
	var root struct{ XMLName xml.Name }
	if err = xml.Unmarshal(second, &root); err != nil || root.XMLName.Local != "project" {
		t.Fatalf("invalid configured POM: %v", err)
	}
}

func TestConfigureGradleUsesEnvironmentCredential(t *testing.T) {
	first, err := configureGradle([]byte("plugins { id 'java' }\n"), "mvn-sh-acme-releases", "https://acme.mvn.sh/releases", false)
	if err != nil {
		t.Fatal(err)
	}
	second, err := configureGradle(first, "mvn-sh-acme-releases", "https://acme.mvn.sh/releases", false)
	if err != nil {
		t.Fatal(err)
	}
	text := string(second)
	if strings.Count(text, "<!-- mvnsh:project-repository-mvn-sh-acme-releases:start -->") != 1 ||
		strings.Count(text, "<!-- mvnsh:distribution-management-mvn-sh-acme-releases:start -->") != 1 ||
		!strings.Contains(text, "publishing {") || !strings.Contains(text, "environmentVariable('MVN_TOKEN')") {
		t.Fatalf("unexpected configured Gradle file: %s", text)
	}
}

func TestInstallProfileRejectsInvalidSettings(t *testing.T) {
	if _, err := installProfile([]byte("<settings>"), "profile", "url", "token"); err == nil {
		t.Fatal("expected invalid XML to fail")
	}
}
