package main

import (
	"encoding/xml"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"golang.org/x/term"
)

var slugPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,61}[a-z0-9]$`)

func main() {
	if len(os.Args) < 2 || os.Args[1] != "login" {
		usage()
		os.Exit(2)
	}
	if err := login(os.Args[2:]); err != nil {
		fmt.Fprintln(os.Stderr, "mvnsh:", err)
		os.Exit(1)
	}
}
func usage() {
	fmt.Fprintln(os.Stderr, "Usage: mvnsh login [--team TEAM] [--repository REPOSITORY] [--settings PATH]\n\nInstalls an authenticated mvn.sh profile in Maven settings.xml.")
}
func login(args []string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	flags := flag.NewFlagSet("login", flag.ContinueOnError)
	team := flags.String("team", "", "team slug (interactive when omitted)")
	repository := flags.String("repository", "", "repository slug (interactive when omitted)")
	profile := flags.String("profile", "", "Maven profile ID (default: mvn-sh-TEAM)")
	settings := flags.String("settings", filepath.Join(home, ".m2", "settings.xml"), "Maven settings path")
	baseURL := flags.String("base-url", "https://%s.mvn.sh", "team URL format")
	apiURL := flags.String("api-url", "https://api.mvn.sh", "mvn.sh API URL")
	appURL := flags.String("app-url", "https://mvn.sh", "mvn.sh browser application URL")
	tokenFlag := flags.String("token", "", "access token (prefer MVN_TOKEN or the secure prompt)")
	if err := flags.Parse(args); err != nil {
		return err
	}
	*team = strings.ToLower(strings.TrimSpace(*team))
	*repository = strings.ToLower(strings.TrimSpace(*repository))
	token := strings.TrimSpace(*tokenFlag)
	if token == "" {
		token = strings.TrimSpace(os.Getenv("MVN_TOKEN"))
	}
	if token == "" {
		credentials, loginErr := browserLogin(*appURL)
		if loginErr != nil {
			return loginErr
		}
		token, *team, *repository = credentials.Token, credentials.Team, credentials.Repository
	}
	if !strings.HasPrefix(token, "mvn_") {
		return errors.New("invalid access token")
	}
	if *team == "" || *repository == "" {
		if !term.IsTerminal(int(os.Stdin.Fd())) {
			return errors.New("--team and --repository are required without an interactive terminal")
		}
		teams, contextErr := loadContext(*apiURL, token)
		if contextErr != nil {
			return contextErr
		}
		if *team == "" {
			choices := make([]choice, 0, len(teams))
			for _, item := range teams {
				choices = append(choices, choice{label: item.Name + " (" + item.Slug + ")", value: item.Slug})
			}
			*team, contextErr = selectChoice("Select a team", choices)
			if contextErr != nil {
				return contextErr
			}
		}
		var repositories []repositoryContext
		for _, item := range teams {
			if item.Slug == *team {
				repositories = item.Repositories
				break
			}
		}
		if *repository == "" {
			choices := make([]choice, 0, len(repositories))
			for _, item := range repositories {
				choices = append(choices, choice{label: item.Name + " · " + item.Visibility, value: item.Slug})
			}
			*repository, contextErr = selectChoice("Select a repository", choices)
			if contextErr != nil {
				return contextErr
			}
		}
	}
	if !slugPattern.MatchString(*team) || !slugPattern.MatchString(*repository) {
		return errors.New("team and repository must be valid slugs")
	}
	if *profile == "" {
		*profile = "mvn-sh-" + *team
	}
	url := fmt.Sprintf(*baseURL, *team) + "/" + *repository
	content, err := readSettings(*settings)
	if err != nil {
		return err
	}
	updated, err := installProfile(content, *profile, url, token)
	if err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Dir(*settings), 0700); err != nil {
		return err
	}
	if _, err = os.Stat(*settings); err == nil {
		if err = copyFile(*settings, *settings+".bak"); err != nil {
			return fmt.Errorf("backup settings: %w", err)
		}
	}
	if err = atomicWrite(*settings, updated); err != nil {
		return err
	}
	fmt.Printf("Installed Maven profile %q in %s\nRepository: %s\nBackup: %s.bak\n", *profile, *settings, url, *settings)
	return nil
}

func readSettings(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return []byte("<settings xmlns=\"http://maven.apache.org/SETTINGS/1.2.0\">\n</settings>\n"), nil
	}
	return data, err
}
func installProfile(input []byte, id, url, token string) ([]byte, error) {
	var check struct{ XMLName xml.Name }
	if err := xml.Unmarshal(input, &check); err != nil {
		return nil, fmt.Errorf("existing settings.xml is invalid: %w", err)
	}
	text := string(input)
	key := safeID(id)
	text = removeManaged(text, "server-"+key)
	text = removeManaged(text, "profile-"+key)
	text = removeManaged(text, "active-"+key)
	server := managed("server-"+key, "<server><id>"+escape(id)+"</id><username>token</username><password>"+escape(token)+"</password></server>")
	profile := managed("profile-"+key, "<profile><id>"+escape(id)+"</id><repositories><repository><id>"+escape(id)+"</id><url>"+escape(url)+"</url><releases><enabled>true</enabled></releases><snapshots><enabled>true</enabled></snapshots></repository></repositories><pluginRepositories><pluginRepository><id>"+escape(id)+"</id><url>"+escape(url)+"</url></pluginRepository></pluginRepositories></profile>")
	active := managed("active-"+key, "<activeProfile>"+escape(id)+"</activeProfile>")
	var err error
	if text, err = insertSection(text, "servers", server); err != nil {
		return nil, err
	}
	if text, err = insertSection(text, "profiles", profile); err != nil {
		return nil, err
	}
	if text, err = insertSection(text, "activeProfiles", active); err != nil {
		return nil, err
	}
	return []byte(text), nil
}
func managed(key, value string) string {
	return "\n    <!-- mvnsh:" + key + ":start -->\n    " + value + "\n    <!-- mvnsh:" + key + ":end -->\n"
}
func removeManaged(s, key string) string {
	start := "<!-- mvnsh:" + key + ":start -->"
	end := "<!-- mvnsh:" + key + ":end -->"
	for {
		a := strings.Index(s, start)
		if a < 0 {
			return s
		}
		b := strings.Index(s[a:], end)
		if b < 0 {
			return s
		}
		s = s[:a] + s[a+b+len(end):]
	}
}
func insertSection(s, section, value string) (string, error) {
	close := "</" + section + ">"
	if i := strings.LastIndex(s, close); i >= 0 {
		return s[:i] + value + s[i:], nil
	}
	i := strings.LastIndex(s, "</settings>")
	if i < 0 {
		return "", errors.New("settings.xml has no closing settings element")
	}
	block := "\n  <" + section + ">" + value + "  </" + section + ">\n"
	return s[:i] + block + s[i:], nil
}
func safeID(v string) string { return regexp.MustCompile(`[^a-zA-Z0-9_-]`).ReplaceAllString(v, "-") }
func escape(v string) string {
	var b strings.Builder
	_ = xml.EscapeText(&b, []byte(v))
	return b.String()
}
func atomicWrite(path string, data []byte) error {
	temp, err := os.CreateTemp(filepath.Dir(path), "settings-*.xml")
	if err != nil {
		return err
	}
	name := temp.Name()
	defer os.Remove(name)
	if err = temp.Chmod(0600); err == nil {
		_, err = temp.Write(data)
	}
	if closeErr := temp.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return os.Rename(name, path)
}
func copyFile(from, to string) error {
	data, err := os.ReadFile(from)
	if err != nil {
		return err
	}
	return os.WriteFile(to, data, 0600)
}
