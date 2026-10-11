package browser

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

// browserOverrideEnv names the environment variable holding the path of the
// browser to use. When set, the search is skipped.
const browserOverrideEnv = "MANGA_DOWNLOADER_BROWSER"

// browserHost is what the browser lookup needs from the system. It's injected
// so the candidate order of every OS can be tested without the real filesystem.
type browserHost struct {
	goos     string
	getenv   func(string) string
	isFile   func(string) bool
	lookPath func(string) (string, error)
}

// systemHost is the browserHost of the machine we're running on
func systemHost() browserHost {
	return browserHost{
		goos:   runtime.GOOS,
		getenv: os.Getenv,
		isFile: func(p string) bool {
			info, err := os.Stat(p)
			return err == nil && !info.IsDir()
		},
		lookPath: exec.LookPath,
	}
}

// browserCandidate is one place a browser may be: an absolute path to check,
// or a bare name to look up on PATH
type browserCandidate struct {
	path   string
	onPATH bool
}

func absCandidate(path string) browserCandidate {
	return browserCandidate{path: path}
}

func pathCandidate(name string) browserCandidate {
	return browserCandidate{path: name, onPATH: true}
}

// findBrowser returns the path of the browser to launch: the override from
// MANGA_DOWNLOADER_BROWSER, else the first candidate found, else ErrNoBrowser
func findBrowser(host browserHost) (string, error) {
	if override := host.getenv(browserOverrideEnv); override != "" {
		// like a shell: a bare name (e.g. "brave") is a command on PATH, anything
		// with a path separator is a file path
		if !strings.ContainsAny(override, `/\`) {
			if found, err := host.lookPath(override); err == nil {
				return found, nil
			}
			return "", fmt.Errorf("%s=%q is not a command on PATH", browserOverrideEnv, override)
		}
		if !host.isFile(override) {
			return "", fmt.Errorf("%s=%q is not a file", browserOverrideEnv, override)
		}
		return override, nil
	}

	for _, c := range browserCandidates(host.goos, host.getenv) {
		if c.onPATH {
			if found, err := host.lookPath(c.path); err == nil {
				return found, nil
			}
		} else if host.isFile(c.path) {
			return c.path, nil
		}
	}

	return "", ErrNoBrowser
}

// browserCandidates lists where to look for a browser on goos, in order. The
// first entries are chromedp's own list (its findExecPath), so a machine where
// chromedp finds a browser today keeps getting the same one. The rest are
// Chromium-family browsers chromedp doesn't look for.
func browserCandidates(goos string, getenv func(string) string) []browserCandidate {
	switch goos {
	case "darwin":
		list := []browserCandidate{
			absCandidate("/Applications/Chromium.app/Contents/MacOS/Chromium"),
			absCandidate("/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"),
		}
		dirs := []string{"/Applications"}
		if home := getenv("HOME"); home != "" {
			dirs = append(dirs, home+"/Applications")
		}
		apps := []string{
			"Google Chrome.app/Contents/MacOS/Google Chrome",
			"Chromium.app/Contents/MacOS/Chromium",
			"Brave Browser.app/Contents/MacOS/Brave Browser",
			"Microsoft Edge.app/Contents/MacOS/Microsoft Edge",
		}
		for _, dir := range dirs {
			for _, app := range apps {
				list = append(list, absCandidate(dir+"/"+app))
			}
		}
		return list

	case "windows":
		userProfile := getenv("USERPROFILE")
		localAppData := getenv("LOCALAPPDATA")
		if localAppData == "" {
			localAppData = winPath(userProfile, `AppData\Local`)
		}
		list := []browserCandidate{
			pathCandidate("chrome"),
			pathCandidate("chrome.exe"), // in case PATHEXT is misconfigured
			absCandidate(`C:\Program Files (x86)\Google\Chrome\Application\chrome.exe`),
			absCandidate(`C:\Program Files\Google\Chrome\Application\chrome.exe`),
			absCandidate(winPath(userProfile, `AppData\Local\Google\Chrome\Application\chrome.exe`)),
			absCandidate(winPath(userProfile, `AppData\Local\Chromium\Application\chrome.exe`)),
		}
		// the program folders come from the environment, never a hardcoded drive
		bases := []string{getenv("ProgramFiles"), getenv("ProgramFiles(x86)"), localAppData}
		rels := []string{
			`Google\Chrome\Application\chrome.exe`,
			`BraveSoftware\Brave-Browser\Application\brave.exe`,
			`Microsoft\Edge\Application\msedge.exe`,
			`Chromium\Application\chrome.exe`,
		}
		for _, rel := range rels {
			for _, base := range bases {
				list = append(list, absCandidate(winPath(base, rel)))
			}
		}
		return append(list, pathCandidate("brave"), pathCandidate("msedge"))

	default:
		return []browserCandidate{
			pathCandidate("headless_shell"),
			pathCandidate("headless-shell"),
			pathCandidate("chromium"),
			pathCandidate("chromium-browser"),
			pathCandidate("google-chrome"),
			pathCandidate("google-chrome-stable"),
			pathCandidate("google-chrome-beta"),
			pathCandidate("google-chrome-unstable"),
			absCandidate("/usr/bin/google-chrome"),
			absCandidate("/usr/local/bin/chrome"),
			absCandidate("/snap/bin/chromium"),
			pathCandidate("chrome"),
			pathCandidate("brave-browser"),
			pathCandidate("brave"),
			pathCandidate("microsoft-edge"),
			pathCandidate("microsoft-edge-stable"),
		}
	}
}

// winPath joins a Windows folder and a relative path. An empty folder yields an
// empty path, which never exists, so unset variables are simply skipped.
func winPath(base, rel string) string {
	if base == "" {
		return ""
	}
	return strings.TrimRight(base, `\`) + `\` + rel
}
