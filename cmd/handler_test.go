package cmd_test

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/voxelost/manga-downloader/cmd"
)

func TestHandler(t *testing.T) {
	t.Skip("skipping a DEV test")

	testCases := []struct {
		name    string
		command *cobra.Command
		args    []string
	}{
		// {
		// 	name:    "testing",
		// 	command: &cobra.Command{},
		// 	args:    []string{"-o", "out", "https://mangadex.org/title/68112dc1-2b80-4f20-beb8-2f2a8716a430/dandadan"},
		// },
		{
			name:    "inmanga",
			command: &cobra.Command{},
			args:    []string{"-o", "out", "https://inmanga.com/ver/manga/Dandadan/b8a36982-7b20-4624-a39d-0e00425b0333"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cmd.Handler(tc.command, tc.args)
		})
	}
}
