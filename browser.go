package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"time"
)

type browserCredentials struct{ Token, Team, Repository string }

func browserLogin(appURL string) (browserCredentials, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return browserCredentials{}, err
	}
	defer listener.Close()
	random := make([]byte, 24)
	if _, err = rand.Read(random); err != nil {
		return browserCredentials{}, err
	}
	state := base64.RawURLEncoding.EncodeToString(random)
	callback := "http://" + listener.Addr().String() + "/callback"
	result := make(chan browserCredentials, 1)
	mux := http.NewServeMux()
	server := &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	mux.HandleFunc("GET /callback", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("state") != state {
			http.Error(w, "Invalid authorization state.", http.StatusBadRequest)
			return
		}
		credentials := browserCredentials{Token: r.URL.Query().Get("token"), Team: r.URL.Query().Get("team"), Repository: r.URL.Query().Get("repository")}
		if credentials.Token == "" || credentials.Team == "" || credentials.Repository == "" {
			http.Error(w, "Authorization was not completed.", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, "<h1>mvn.sh login complete</h1><p>You can close this window and return to your terminal.</p>")
		select {
		case result <- credentials:
		default:
		}
	})
	go server.Serve(listener)
	authorize := appURL + "/cli/authorize?callback=" + url.QueryEscape(callback) + "&state=" + url.QueryEscape(state)
	fmt.Println("Opening your browser to authorize mvn.sh…")
	fmt.Println(authorize)
	if err = openBrowser(authorize); err != nil {
		fmt.Println("Open the URL above in your browser.")
	}
	select {
	case credentials := <-result:
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = server.Shutdown(ctx)
		return credentials, nil
	case <-time.After(5 * time.Minute):
		return browserCredentials{}, fmt.Errorf("browser authorization timed out")
	}
}
func openBrowser(target string) error {
	commands := browserCommands(target)
	var lastErr error
	for _, command := range commands {
		if _, err := exec.LookPath(command[0]); err != nil {
			lastErr = err
			continue
		}
		if err := exec.Command(command[0], command[1:]...).Run(); err == nil {
			return nil
		} else {
			lastErr = err
		}
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("no browser opener is available")
	}
	return lastErr
}

func browserCommands(target string) [][]string {
	if browser := os.Getenv("BROWSER"); browser != "" {
		return append([][]string{{browser, target}}, platformBrowserCommands(target)...)
	}
	return platformBrowserCommands(target)
}

func platformBrowserCommands(target string) [][]string {
	switch runtime.GOOS {
	case "darwin":
		return [][]string{{"open", target}}
	case "windows":
		return [][]string{{"rundll32", "url.dll,FileProtocolHandler", target}}
	default:
		return [][]string{{"xdg-open", target}, {"gio", "open", target}, {"sensible-browser", target}}
	}
}
