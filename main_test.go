package main

import (
	"encoding/xml"
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
func TestInstallProfileRejectsInvalidSettings(t *testing.T) {
	if _, err := installProfile([]byte("<settings>"), "profile", "url", "token"); err == nil {
		t.Fatal("expected invalid XML to fail")
	}
}
