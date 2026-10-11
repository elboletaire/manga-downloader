package browser

import (
	"errors"
	"testing"
)

// fakeHost builds a browserHost with the given environment and installed
// files, so no real filesystem or PATH is touched
func fakeHost(goos string, env map[string]string, files ...string) browserHost {
	installed := map[string]bool{}
	for _, f := range files {
		installed[f] = true
	}
	return browserHost{
		goos:   goos,
		getenv: func(k string) string { return env[k] },
		isFile: func(p string) bool { return installed[p] },
		lookPath: func(name string) (string, error) {
			if installed[name] {
				return "/usr/bin/" + name, nil
			}
			return "", errors.New("not found")
		},
	}
}

func TestFindBrowser(t *testing.T) {
	winEnv := map[string]string{
		"ProgramFiles":      `C:\Program Files`,
		"ProgramFiles(x86)": `C:\Program Files (x86)`,
		"LOCALAPPDATA":      `C:\Users\me\AppData\Local`,
		"USERPROFILE":       `C:\Users\me`,
		"HOME":              "/home/me",
	}

	tests := []struct {
		name  string
		host  browserHost
		want  string
		fails bool
	}{
		{
			name: "linux chromium on PATH is picked before brave",
			host: fakeHost("linux", nil, "chromium", "brave"),
			want: "/usr/bin/chromium",
		},
		{
			name: "linux brave and edge on PATH when no chrome",
			host: fakeHost("linux", nil, "microsoft-edge", "brave-browser"),
			want: "/usr/bin/brave-browser",
		},
		{
			name: "linux edge stable",
			host: fakeHost("linux", nil, "microsoft-edge-stable"),
			want: "/usr/bin/microsoft-edge-stable",
		},
		{
			name: "linux absolute chrome path",
			host: fakeHost("linux", nil, "/usr/bin/google-chrome"),
			want: "/usr/bin/google-chrome",
		},
		{
			name:  "linux nothing installed",
			host:  fakeHost("linux", nil),
			fails: true,
		},
		{
			name: "darwin chromedp's chromium comes first",
			host: fakeHost("darwin", map[string]string{"HOME": "/Users/me"},
				"/Applications/Chromium.app/Contents/MacOS/Chromium",
				"/Applications/Brave Browser.app/Contents/MacOS/Brave Browser"),
			want: "/Applications/Chromium.app/Contents/MacOS/Chromium",
		},
		{
			name: "darwin brave in /Applications",
			host: fakeHost("darwin", map[string]string{"HOME": "/Users/me"},
				"/Applications/Brave Browser.app/Contents/MacOS/Brave Browser"),
			want: "/Applications/Brave Browser.app/Contents/MacOS/Brave Browser",
		},
		{
			name: "darwin edge in the user's Applications",
			host: fakeHost("darwin", map[string]string{"HOME": "/Users/me"},
				"/Users/me/Applications/Microsoft Edge.app/Contents/MacOS/Microsoft Edge"),
			want: "/Users/me/Applications/Microsoft Edge.app/Contents/MacOS/Microsoft Edge",
		},
		{
			name: "windows chromedp's hardcoded chrome wins over brave",
			host: fakeHost("windows", winEnv,
				`C:\Program Files\Google\Chrome\Application\chrome.exe`,
				`C:\Program Files\BraveSoftware\Brave-Browser\Application\brave.exe`),
			want: `C:\Program Files\Google\Chrome\Application\chrome.exe`,
		},
		{
			name: "windows brave from ProgramFiles",
			host: fakeHost("windows", winEnv,
				`C:\Program Files\BraveSoftware\Brave-Browser\Application\brave.exe`),
			want: `C:\Program Files\BraveSoftware\Brave-Browser\Application\brave.exe`,
		},
		{
			name: "windows edge from ProgramFiles(x86)",
			host: fakeHost("windows", winEnv,
				`C:\Program Files (x86)\Microsoft\Edge\Application\msedge.exe`),
			want: `C:\Program Files (x86)\Microsoft\Edge\Application\msedge.exe`,
		},
		{
			name: "windows brave beats edge when both are installed",
			host: fakeHost("windows", winEnv,
				`C:\Program Files (x86)\Microsoft\Edge\Application\msedge.exe`,
				`C:\Program Files\BraveSoftware\Brave-Browser\Application\brave.exe`),
			want: `C:\Program Files\BraveSoftware\Brave-Browser\Application\brave.exe`,
		},
		{
			name: "windows paths follow the environment, not C:",
			host: fakeHost("windows", map[string]string{
				"ProgramFiles":      `D:\Apps`,
				"ProgramFiles(x86)": `D:\Apps86`,
				"LOCALAPPDATA":      `D:\Local`,
			}, `D:\Local\Microsoft\Edge\Application\msedge.exe`),
			want: `D:\Local\Microsoft\Edge\Application\msedge.exe`,
		},
		{
			name: "windows chromium from LOCALAPPDATA fallback to USERPROFILE",
			host: fakeHost("windows", map[string]string{"USERPROFILE": `D:\Users\me`},
				`D:\Users\me\AppData\Local\Chromium\Application\chrome.exe`),
			want: `D:\Users\me\AppData\Local\Chromium\Application\chrome.exe`,
		},
		{
			name: "windows brave on PATH",
			host: fakeHost("windows", winEnv, "brave"),
			want: "/usr/bin/brave",
		},
		{
			name: "windows program folders are searched before PATH",
			host: fakeHost("windows", winEnv, "msedge",
				`C:\Program Files\BraveSoftware\Brave-Browser\Application\brave.exe`),
			want: `C:\Program Files\BraveSoftware\Brave-Browser\Application\brave.exe`,
		},
		{
			name:  "windows nothing installed",
			host:  fakeHost("windows", winEnv),
			fails: true,
		},
		{
			name: "override wins over installed browsers",
			host: fakeHost("linux", map[string]string{browserOverrideEnv: "/opt/edge/msedge"},
				"/opt/edge/msedge", "chromium"),
			want: "/opt/edge/msedge",
		},
		{
			name: "override can be a command on PATH",
			host: fakeHost("linux", map[string]string{browserOverrideEnv: "brave"}, "brave", "chromium"),
			want: "/usr/bin/brave",
		},
		{
			name:  "override pointing nowhere is an error, not a search",
			host:  fakeHost("linux", map[string]string{browserOverrideEnv: "/nope"}, "chromium"),
			fails: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := findBrowser(tt.host)
			if tt.fails {
				if err == nil {
					t.Fatalf("expected an error, got %q", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFindBrowserNoneIsErrNoBrowser(t *testing.T) {
	_, err := findBrowser(fakeHost("linux", nil))
	if !errors.Is(err, ErrNoBrowser) {
		t.Fatalf("got %v, want ErrNoBrowser", err)
	}
}

func TestBrowserCandidatesStartWithChromedp(t *testing.T) {
	// the first entries must stay chromedp's own list, in its order
	chromedpLinux := []string{
		"headless_shell", "headless-shell", "chromium", "chromium-browser",
		"google-chrome", "google-chrome-stable", "google-chrome-beta",
		"google-chrome-unstable", "/usr/bin/google-chrome",
		"/usr/local/bin/chrome", "/snap/bin/chromium", "chrome",
	}
	got := browserCandidates("linux", nil)
	for i, want := range chromedpLinux {
		if got[i].path != want {
			t.Errorf("linux candidate %d = %q, want %q", i, got[i].path, want)
		}
	}
}
