package main

import (
	"encoding/xml"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/term"
)

func configureProject(args []string) error {
	flags := flag.NewFlagSet("configure", flag.ContinueOnError)
	team := flags.String("team", "", "team slug")
	repository := flags.String("repository", "", "repository slug")
	project := flags.String("project", ".", "project directory")
	kind := flags.String("type", "auto", "project type: auto, maven, or gradle")
	id := flags.String("id", "", "repository ID (default: mvn-sh-TEAM-REPOSITORY)")
	baseURL := flags.String("base-url", "https://%s.mvn.sh", "team URL format")
	apiURL := flags.String("api-url", "https://api.mvn.sh", "mvn.sh API URL")
	appURL := flags.String("app-url", "https://mvn.sh", "mvn.sh browser application URL")
	tokenFlag := flags.String("token", "", "access token used to select a repository (prefer MVN_TOKEN)")
	if err := flags.Parse(args); err != nil {
		return err
	}
	*team = strings.ToLower(strings.TrimSpace(*team))
	*repository = strings.ToLower(strings.TrimSpace(*repository))
	if *team == "" || *repository == "" {
		if !term.IsTerminal(int(os.Stdin.Fd())) {
			return errors.New("--team and --repository are required without an interactive terminal")
		}
		token := strings.TrimSpace(*tokenFlag)
		if token == "" {
			token = strings.TrimSpace(os.Getenv("MVN_TOKEN"))
		}
		if token == "" {
			credentials, loginErr := browserLogin(*appURL)
			if loginErr != nil {
				return loginErr
			}
			*team, *repository = credentials.Team, credentials.Repository
		} else {
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
			if *repository == "" {
				var choices []choice
				for _, item := range teams {
					if item.Slug == *team {
						for _, repo := range item.Repositories {
							choices = append(choices, choice{label: repo.Name + " · " + repo.Visibility, value: repo.Slug})
						}
					}
				}
				*repository, contextErr = selectChoice("Select a repository", choices)
				if contextErr != nil {
					return contextErr
				}
			}
		}
	}
	*team = strings.ToLower(strings.TrimSpace(*team))
	*repository = strings.ToLower(strings.TrimSpace(*repository))
	if !slugPattern.MatchString(*team) || !slugPattern.MatchString(*repository) {
		return errors.New("team and repository must be valid slugs")
	}
	if *id == "" {
		*id = "mvn-sh-" + *team + "-" + *repository
	}
	url := fmt.Sprintf(*baseURL, *team) + "/" + *repository
	path, detected, err := projectBuildFile(*project, *kind)
	if err != nil {
		return err
	}
	input, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var output []byte
	if detected == "maven" {
		output, err = configurePOM(input, *id, url)
	} else {
		output, err = configureGradle(input, *id, url, strings.HasSuffix(path, ".kts"))
	}
	if err != nil {
		return err
	}
	if string(output) == string(input) {
		fmt.Printf("%s is already configured for %s\n", path, url)
		return nil
	}
	if err = atomicWriteProject(path, output); err != nil {
		return err
	}
	fmt.Printf("Configured %s repository %q in %s\n", detected, *id, path)
	return nil
}

func projectBuildFile(dir, kind string) (string, string, error) {
	if kind != "auto" && kind != "maven" && kind != "gradle" {
		return "", "", errors.New("--type must be auto, maven, or gradle")
	}
	candidates := []struct{ name, kind string }{{"pom.xml", "maven"}, {"build.gradle.kts", "gradle"}, {"build.gradle", "gradle"}}
	for _, candidate := range candidates {
		if kind != "auto" && kind != candidate.kind {
			continue
		}
		path := filepath.Join(dir, candidate.name)
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			return path, candidate.kind, nil
		}
	}
	return "", "", fmt.Errorf("no %s build file found in %s", kind, dir)
}

func configurePOM(input []byte, id, url string) ([]byte, error) {
	var root struct{ XMLName xml.Name }
	if err := xml.Unmarshal(input, &root); err != nil || root.XMLName.Local != "project" {
		if err == nil {
			err = errors.New("root element is not project")
		}
		return nil, fmt.Errorf("invalid pom.xml: %w", err)
	}
	text := removeManaged(string(input), "project-repository-"+safeID(id))
	text = removeManaged(text, "distribution-management-"+safeID(id))
	entry := managed("project-repository-"+safeID(id), "<repository><id>"+escape(id)+"</id><url>"+escape(url)+"</url><releases><enabled>true</enabled></releases><snapshots><enabled>true</enabled></snapshots></repository>")
	distribution := managed("distribution-management-"+safeID(id), "<distributionManagement><repository><id>"+escape(id)+"</id><url>"+escape(url)+"</url></repository><snapshotRepository><id>"+escape(id)+"</id><url>"+escape(url)+"</url></snapshotRepository></distributionManagement>")
	projectClose := strings.LastIndex(text, "</project>")
	if projectClose < 0 {
		return nil, errors.New("pom.xml has no closing project element")
	}
	text = text[:projectClose] + distribution + text[projectClose:]
	close := "</repositories>"
	if index := strings.LastIndex(text, close); index >= 0 {
		return []byte(text[:index] + entry + text[index:]), nil
	}
	index := strings.LastIndex(text, "</project>")
	return []byte(text[:index] + "\n  <repositories>" + entry + "  </repositories>\n" + text[index:]), nil
}

func configureGradle(input []byte, id, url string, kotlin bool) ([]byte, error) {
	key := "project-repository-" + safeID(id)
	text := removeManaged(string(input), key)
	var repository string
	if kotlin {
		repository = "repositories {\n    maven {\n        name = \"" + gradleEscape(id) + "\"\n        url = uri(\"" + gradleEscape(url) + "\")\n        credentials {\n            username = \"token\"\n            password = providers.environmentVariable(\"MVN_TOKEN\").orNull\n        }\n    }\n}"
	} else {
		repository = "repositories {\n    maven {\n        name = '" + gradleEscape(id) + "'\n        url = uri('" + gradleEscape(url) + "')\n        credentials {\n            username = 'token'\n            password = providers.environmentVariable('MVN_TOKEN').orNull\n        }\n    }\n}"
	}
	var publishing string
	if kotlin {
		publishing = "publishing {\n    repositories {\n        maven {\n            name = \"" + gradleEscape(id) + "\"\n            url = uri(\"" + gradleEscape(url) + "\")\n            credentials {\n                username = \"token\"\n                password = providers.environmentVariable(\"MVN_TOKEN\").orNull\n            }\n        }\n    }\n}"
	} else {
		publishing = "publishing {\n    repositories {\n        maven {\n            name = '" + gradleEscape(id) + "'\n            url = uri('" + gradleEscape(url) + "')\n            credentials {\n                username = 'token'\n                password = providers.environmentVariable('MVN_TOKEN').orNull\n            }\n        }\n    }\n}"
	}
	text = removeManaged(text, "distribution-management-"+safeID(id))
	return []byte(strings.TrimRight(text, "\n") + "\n" + managed(key, repository) + managed("distribution-management-"+safeID(id), publishing)), nil
}

func gradleEscape(value string) string {
	return strings.NewReplacer("\\", "\\\\", "'", "\\'", "\"", "\\\"").Replace(value)
}

func atomicWriteProject(path string, data []byte) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	temp, err := os.CreateTemp(filepath.Dir(path), ".mvnsh-*")
	if err != nil {
		return err
	}
	name := temp.Name()
	defer os.Remove(name)
	if err = temp.Chmod(info.Mode().Perm()); err == nil {
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
