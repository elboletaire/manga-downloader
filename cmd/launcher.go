// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package cmd

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

// guiName is the desktop app's executable, shipped next to the CLI
const guiName = "manga-downloader-gui"

const guiReleasesURL = "https://github.com/elboletaire/manga-downloader/releases"

// uiCmd starts the desktop app. The GUI is a separate binary (it needs cgo and
// a windowing stack), so this only launches it.
var uiCmd = &cobra.Command{
	Use:   "ui",
	Short: "Opens the desktop app",
	Long: `Opens the desktop app (manga-downloader-gui), which is shipped as a separate
download. It has to sit next to this program or be on your PATH.`,
	Args: cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		path, ok := findGUI()
		if !ok {
			printGUINotFound()
			exit(1)
		}
		if err := launchGUI(path); err != nil {
			color.Red("Error starting %s: %s", path, err)
			exit(1)
		}
	},
}

// shouldOpenFromExplorer reports whether a run should behave as a GUI launcher
// instead of a CLI invocation: only a bare run that Explorer started counts.
// Anything with arguments, or started from a terminal, is an ordinary CLI run.
func shouldOpenFromExplorer(args []string, fromExplorer bool) bool {
	return len(args) == 0 && fromExplorer
}

// openFromExplorer handles a double-clicked CLI. With the GUI present it opens
// it; without it, the console would vanish before the help could be read, so
// the help is shown and the window waits for Enter.
func openFromExplorer() {
	if path, ok := findGUI(); ok {
		if err := launchGUI(path); err != nil {
			color.Red("Error starting %s: %s", path, err)
			waitForEnter()
			exit(1)
		}
		exit(0)
	}

	rootCmd.Help()
	fmt.Println()
	color.Yellow("This is a command-line tool. Open a terminal in this folder and run it from there,")
	color.Yellow("for example: manga-downloader https://example.com/manga/title 1-10")
	fmt.Println()
	printGUINotFound()
	waitForEnter()
	exit(1)
}

// findGUI looks for the GUI next to the running CLI, then on PATH
func findGUI() (string, bool) {
	if exe, err := os.Executable(); err == nil {
		// follow symlinks so a PATH-linked CLI still finds the GUI beside the real binary
		if real, err := filepath.EvalSymlinks(exe); err == nil {
			exe = real
		}
		if path, ok := findGUIIn(filepath.Dir(exe)); ok {
			return path, true
		}
	}

	path, err := exec.LookPath(guiFileName())
	return path, err == nil
}

// findGUIIn looks for the GUI inside dir. On macOS the CLI sits next to the
// app bundle, so the bundle's executable is checked too.
func findGUIIn(dir string) (string, bool) {
	candidates := []string{filepath.Join(dir, guiFileName())}
	if runtime.GOOS == "darwin" {
		candidates = append(candidates, filepath.Join(dir, "Manga Downloader.app", "Contents", "MacOS", guiFileName()))
	}

	for _, c := range candidates {
		if fi, err := os.Stat(c); err == nil && !fi.IsDir() {
			return c, true
		}
	}

	return "", false
}

func guiFileName() string {
	if runtime.GOOS == "windows" {
		return guiName + ".exe"
	}

	return guiName
}

// launchGUI starts the GUI detached and without waiting for it, so the CLI can exit at once
func launchGUI(path string) error {
	c := exec.Command(path)
	detach(c)
	if err := c.Start(); err != nil {
		return err
	}

	return c.Process.Release()
}

func printGUINotFound() {
	color.Red("%s was not found next to this program or on your PATH.", guiFileName())
	fmt.Printf("Download the desktop app from %s\n", guiReleasesURL)
}

// waitForEnter keeps a double-clicked console window open until the user reads it
func waitForEnter() {
	fmt.Print("\nPress Enter to close...")
	_, _ = bufio.NewReader(os.Stdin).ReadString('\n')
}

func init() {
	rootCmd.AddCommand(uiCmd)
}
