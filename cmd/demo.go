//go:build demo

package cmd

import "github.com/elboletaire/manga-downloader/demo"

// the demo build serves the tapes' series from fake data, not the network
func init() {
	demo.Install()
}
