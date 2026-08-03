package main

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const releasesURL = "https://github.com/mvn-sh/cli/releases"

func update() error {
	if version == "dev" {
		return fmt.Errorf("development builds cannot update automatically")
	}
	client := &http.Client{Timeout: 60 * time.Second}
	var release struct {
		Tag string `json:"tag_name"`
	}
	if err := getJSON(client, "https://api.github.com/repos/mvn-sh/cli/releases/latest", &release); err != nil {
		return fmt.Errorf("check latest release: %w", err)
	}
	current := "v" + strings.TrimPrefix(version, "v")
	if release.Tag == current {
		fmt.Printf("mvnsh %s is already current.\n", version)
		return nil
	}
	if release.Tag == "" {
		return fmt.Errorf("latest release has no version")
	}
	archive, err := archiveName(release.Tag)
	if err != nil {
		return err
	}
	base := releasesURL + "/download/" + release.Tag
	tmp, err := os.MkdirTemp("", "mvnsh-update-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	archivePath := filepath.Join(tmp, archive)
	if err = download(client, base+"/"+archive, archivePath); err != nil {
		return err
	}
	checksumsPath := filepath.Join(tmp, "checksums.txt")
	if err = download(client, base+"/checksums.txt", checksumsPath); err != nil {
		return err
	}
	if err = verifyChecksum(archivePath, checksumsPath, archive); err != nil {
		return err
	}
	binaryPath, err := extractBinary(archivePath, tmp)
	if err != nil {
		return err
	}
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	executable, err = filepath.EvalSymlinks(executable)
	if err != nil {
		return err
	}
	if err = replaceExecutable(executable, binaryPath); err != nil {
		return fmt.Errorf("install update: %w", err)
	}
	fmt.Printf("Updated mvnsh from %s to %s.\n", version, strings.TrimPrefix(release.Tag, "v"))
	return nil
}

func archiveName(tag string) (string, error) {
	osName := runtime.GOOS
	arch := runtime.GOARCH
	if (osName != "linux" && osName != "darwin" && osName != "windows") || (arch != "amd64" && arch != "arm64") {
		return "", fmt.Errorf("automatic updates do not support %s/%s", osName, arch)
	}
	ext := ".tar.gz"
	if osName == "windows" {
		ext = ".zip"
	}
	return fmt.Sprintf("mvnsh_%s_%s_%s%s", strings.TrimPrefix(tag, "v"), osName, arch, ext), nil
}

func getJSON(client *http.Client, url string, target any) error {
	response, err := client.Get(url)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("GitHub returned %s", response.Status)
	}
	return json.NewDecoder(response.Body).Decode(target)
}

func download(client *http.Client, url, destination string) error {
	response, err := client.Get(url)
	if err != nil {
		return fmt.Errorf("download %s: %w", filepath.Base(destination), err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("download %s: GitHub returned %s", filepath.Base(destination), response.Status)
	}
	file, err := os.Create(destination)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(file, response.Body)
	closeErr := file.Close()
	if copyErr != nil {
		return copyErr
	}
	return closeErr
}

func verifyChecksum(archivePath, checksumsPath, archive string) error {
	data, err := os.ReadFile(checksumsPath)
	if err != nil {
		return err
	}
	var expected string
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && fields[1] == archive {
			expected = fields[0]
			break
		}
	}
	if expected == "" {
		return fmt.Errorf("release checksum not found")
	}
	file, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err = io.Copy(hash, file); err != nil {
		return err
	}
	if hex.EncodeToString(hash.Sum(nil)) != strings.ToLower(expected) {
		return fmt.Errorf("checksum verification failed")
	}
	return nil
}

func extractBinary(archivePath, destination string) (string, error) {
	output := filepath.Join(destination, "mvnsh")
	if runtime.GOOS == "windows" {
		output += ".exe"
		reader, err := zip.OpenReader(archivePath)
		if err != nil {
			return "", err
		}
		defer reader.Close()
		for _, file := range reader.File {
			if filepath.Base(file.Name) == "mvnsh.exe" {
				input, openErr := file.Open()
				if openErr != nil {
					return "", openErr
				}
				defer input.Close()
				return output, writeBinary(output, input)
			}
		}
		return "", fmt.Errorf("release archive does not contain mvnsh.exe")
	}
	file, err := os.Open(archivePath)
	if err != nil {
		return "", err
	}
	defer file.Close()
	gzipReader, err := gzip.NewReader(file)
	if err != nil {
		return "", err
	}
	defer gzipReader.Close()
	tarReader := tar.NewReader(gzipReader)
	for {
		header, nextErr := tarReader.Next()
		if nextErr == io.EOF {
			break
		}
		if nextErr != nil {
			return "", nextErr
		}
		if filepath.Base(header.Name) == "mvnsh" {
			return output, writeBinary(output, tarReader)
		}
	}
	return "", fmt.Errorf("release archive does not contain mvnsh")
}

func writeBinary(path string, source io.Reader) error {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(file, source)
	closeErr := file.Close()
	if copyErr != nil {
		return copyErr
	}
	return closeErr
}

func replaceExecutable(executable, replacement string) error {
	backup := executable + ".old"
	_ = os.Remove(backup)
	if err := os.Rename(executable, backup); err != nil {
		return err
	}
	if err := os.Rename(replacement, executable); err != nil {
		_ = os.Rename(backup, executable)
		return err
	}
	_ = os.Remove(backup)
	return nil
}
