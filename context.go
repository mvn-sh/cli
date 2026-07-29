package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type repositoryContext struct {
	Slug       string `json:"slug"`
	Name       string `json:"name"`
	Visibility string `json:"visibility"`
}
type teamContext struct {
	Slug         string              `json:"slug"`
	Name         string              `json:"name"`
	Repositories []repositoryContext `json:"repositories"`
}

func loadContext(apiURL, token string) ([]teamContext, error) {
	request, err := http.NewRequest(http.MethodGet, strings.TrimRight(apiURL, "/")+"/v1/cli/context", nil)
	if err != nil {
		return nil, err
	}
	request.SetBasicAuth("token", token)
	client := http.Client{Timeout: 15 * time.Second}
	response, err := client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("connect to mvn.sh: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		var problem struct {
			Error string `json:"error"`
		}
		_ = json.NewDecoder(response.Body).Decode(&problem)
		if problem.Error == "" {
			problem.Error = response.Status
		}
		return nil, fmt.Errorf("login failed: %s", problem.Error)
	}
	var teams []teamContext
	if err = json.NewDecoder(response.Body).Decode(&teams); err != nil {
		return nil, err
	}
	return teams, nil
}
