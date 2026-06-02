package tui

import (
	"fmt"
	"os/exec"
	"regexp"
	"runtime"
	"strconv"
	"strings"

	"github.com/stackriot/iwatch/internal/buffer"
	"github.com/stackriot/iwatch/internal/config"
)

var urlPattern = regexp.MustCompile(`https?://[^\s"']+`)
var portPattern = regexp.MustCompile(`(?i)(?:localhost|127\.0\.0\.1|0\.0\.0\.0)[: ](\d{2,5})`)

func (a *App) observeDevFlow(line buffer.Line) {
	streamID := baseStreamID(line.Source)
	if streamID == "" {
		return
	}
	role := a.streamRole(streamID)
	switch role {
	case "backend":
		if backendURL, backendPort := extractURLAndPort(line); backendURL != "" || backendPort != 0 {
			next := backendURL
			if next == "" && backendPort != 0 {
				next = fmt.Sprintf("http://127.0.0.1:%d", backendPort)
			}
			if next != "" && next != a.backendURL {
				a.backendURL = next
				a.eventsPane.Append("backend url: " + a.backendURL)
				a.ensureViteEnvAndStart()
			}
		}
	case "vite":
		if viteURL, _ := extractURLAndPort(line); viteURL != "" && viteURL != a.viteURL {
			a.viteURL = trimTrailingPunctuation(viteURL)
			a.eventsPane.Append("vite url: " + a.viteURL)
		}
	}
}

func (a *App) streamRole(id string) string {
	for _, s := range a.allStreamConfigs() {
		if s.ID == id {
			return strings.ToLower(strings.TrimSpace(s.Role))
		}
	}
	return ""
}

func (a *App) streamIDForRole(role string) (string, bool) {
	role = strings.ToLower(strings.TrimSpace(role))
	for _, s := range a.allStreamConfigs() {
		if strings.ToLower(strings.TrimSpace(s.Role)) == role && s.ID != "" {
			return s.ID, true
		}
	}
	return "", false
}

func (a *App) ensureViteEnvAndStart() {
	if a.streams == nil || a.backendURL == "" {
		return
	}
	viteID, ok := a.streamIDForRole("vite")
	if !ok {
		return
	}

	// Build a runtime override to inject BACKEND_URL/BACKEND_PORT.
	var base config.StreamConfig
	found := false
	for _, s := range a.allStreamConfigs() {
		if s.ID == viteID {
			base = s
			found = true
			break
		}
	}
	if !found {
		return
	}
	override := base
	if override.Env == nil {
		override.Env = map[string]string{}
	} else {
		env := make(map[string]string, len(override.Env)+2)
		for k, v := range override.Env {
			env[k] = v
		}
		override.Env = env
	}
	override.Env["BACKEND_URL"] = a.backendURL
	if port := portFromURL(a.backendURL); port != 0 {
		override.Env["BACKEND_PORT"] = strconv.Itoa(port)
	}
	a.runtimeStreams[viteID] = override
	if !containsString(a.runtimeStreamOrder, viteID) {
		a.runtimeStreamOrder = append(a.runtimeStreamOrder, viteID)
	}

	a.applyActiveStreams()

	// Start Vite on-demand if it is active but not running.
	status, ok := a.streamStatus(viteID)
	if ok && status.Active && !status.Running {
		_ = a.streams.Start(viteID)
	}
}

func baseStreamID(source string) string {
	// supervisor uses "<id>:stdout" / "<id>:stderr" for process streams.
	if id, _, ok := strings.Cut(source, ":"); ok {
		return id
	}
	return source
}

func extractURLAndPort(line buffer.Line) (string, int) {
	// Prefer structured fields
	if url, ok := line.RawFields["url"]; ok && url != "" {
		return trimTrailingPunctuation(url), portFromURL(url)
	}
	if portStr, ok := line.RawFields["port"]; ok {
		if n, err := strconv.Atoi(strings.TrimSpace(portStr)); err == nil {
			return "", n
		}
	}
	// Fallback to regex on raw line text
	if url := firstURL(line.Text); url != "" {
		return url, portFromURL(url)
	}
	if port := firstPort(line.Text); port != 0 {
		return "", port
	}
	return "", 0
}

func firstURL(text string) string {
	m := urlPattern.FindString(text)
	return trimTrailingPunctuation(m)
}

func firstPort(text string) int {
	m := portPattern.FindStringSubmatch(text)
	if len(m) != 2 {
		return 0
	}
	n, _ := strconv.Atoi(m[1])
	return n
}

func portFromURL(value string) int {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0
	}
	// Cheap parse: find last ':' before optional path.
	hostPort := value
	if idx := strings.Index(hostPort, "://"); idx >= 0 {
		hostPort = hostPort[idx+3:]
	}
	hostPort = strings.TrimLeft(hostPort, "/")
	if idx := strings.Index(hostPort, "/"); idx >= 0 {
		hostPort = hostPort[:idx]
	}
	if idx := strings.LastIndex(hostPort, ":"); idx >= 0 && idx+1 < len(hostPort) {
		n, _ := strconv.Atoi(hostPort[idx+1:])
		return n
	}
	return 0
}

func trimTrailingPunctuation(value string) string {
	return strings.TrimRight(value, ".,);]")
}

func openURLCmd(url string) error {
	url = strings.TrimSpace(url)
	if url == "" {
		return nil
	}
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("open", url).Start()
	case "linux":
		return exec.Command("xdg-open", url).Start()
	case "windows":
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	default:
		return exec.Command("open", url).Start()
	}
}

