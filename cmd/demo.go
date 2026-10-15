//go:build demo

package cmd

import "github.com/elboletaire/manga-downloader/demo"

// the demo build answers the network from the recorded fixtures
func init() {
	demo.Install()
}
