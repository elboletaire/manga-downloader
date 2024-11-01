package cmd

import (
	"regexp"

	"github.com/fatih/color"
)

const (
	CobraUse   = "manga-downloader [flags] [url] [ranges]"
	CobraShort = "Helps you download mangas from websites to CBZ files"
	CobraLong  = `With manga-downloader you can easily convert/download web based mangas to CBZ files.

You only need to specify the URL of the manga and the chapters you want to download as a range.

Note the URL must be of the index of the manga, not a single chapter.`
)

var (
	CobraExample = colorizeHelp(`  manga-downloader https://inmanga.com/ver/manga/Fire-Punch/17748683-8986-4628-934a-e94a47fe5d59

Would ask you if you want to download all chapters of Fire Punch (1-83).

  manga-downloader https://inmanga.com/ver/manga/Dr-Stone/d9e47ba6-7dfc-401d-a21c-19326c2ea45f 1-10

Would download chapters 1 to 10 of Dr. Stone from inmanga.com.

  manga-downloader https://inmanga.com/ver/manga/Dr-Stone/d9e47ba6-7dfc-401d-a21c-19326c2ea45f 1-10,12,15-20

Would download chapters 1 to 10, 12 and 15 to 20 of Dr. Stone from inmanga.com.

  manga-downloader --language es https://mangadex.org/title/e7eabe96-aa17-476f-b431-2497d5e9d060/black-clover 10-20

Would download chapters 10 to 20 of Black Clover from mangadex.org in Spanish.

  manga-downloader --language es --bundle https://mangadex.org/title/e7eabe96-aa17-476f-b431-2497d5e9d060/black-clover 10-20

It would also download chapters 10 to 20 of Black Clover from mangadex.org in Spanish, but in this case would bundle them into a single file.

Note arguments aren't really positional, you can specify them in any order:

  manga-downloader --language es 10-20 https://mangadex.org/title/e7eabe96-aa17-476f-b431-2497d5e9d060/black-clover --bundle`)
)

func colorizeHelp(help string) string {
	yellowRegex := regexp.MustCompile(`manga-downloader|nada`)
	help = yellowRegex.ReplaceAllStringFunc(help, func(s string) string {
		return color.YellowString(s)
	})

	grayRegex := regexp.MustCompile(`http[^ ]*|\d+-[\d,-]+`)
	help = grayRegex.ReplaceAllStringFunc(help, func(s string) string {
		return color.HiBlackString(s)
	})

	blueRegex := regexp.MustCompile(`((--language|--bundle)( es)?)`)
	help = blueRegex.ReplaceAllStringFunc(help, func(s string) string {
		return color.HiBlueString(s)
	})

	return help
}
