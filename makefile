ifdef CI_COMMIT_REF_NAME
BRANCH_OR_TAG := $(CI_COMMIT_REF_NAME)
else
BRANCH_OR_TAG := develop
endif

VERSION := $(shell git rev-parse --short HEAD)
GOLDFLAGS += -X 'github.com/elboletaire/manga-downloader/cmd.Version=$(VERSION)'
GOLDFLAGS += -X 'github.com/elboletaire/manga-downloader/cmd.Tag=$(BRANCH_OR_TAG)'
GOFLAGS = -ldflags="$(GOLDFLAGS)"
# nodynamic forces gen2brain/avif to use its embedded libavif/dav1d WASM build
# (run under wazero, no cgo) instead of dlopen'ing whatever libavif happens to
# be installed on the machine, so page conversion decodes identically here, in
# CI and in the released binaries. Keep it in sync with the build_flags in
# .github/workflows/release.yaml, which is the only build path that doesn't go
# through this makefile.
GOTAGS = -tags nodynamic
RICHGO := $(shell command -v richgo 2> /dev/null)

clean:
	@rm -fv ./manga-downloader* *.cbz

install:
	go mod download

build: clean test build/unix

build/all: clean test build/unix build/win

build/unix:
	CGO_ENABLED=0 go build -o manga-downloader ${GOTAGS} ${GOFLAGS} .

build/win:
	GOOS=windows go build -o manga-downloader.exe ${GOTAGS} ${GOFLAGS} .

test:
ifdef RICHGO
	richgo test ${GOTAGS} -v ./...
else
	go test ${GOTAGS} -v ./...
endif

# one dependency per line, alphabetically sorted (grabber/html deliberately
# last), so parallel site-support branches never edit the same line
grabber: \
	grabber/atsumaru \
	grabber/aurorascans \
	grabber/baozimh \
	grabber/bigsolo \
	grabber/bluesolo \
	grabber/comix \
	grabber/danke \
	grabber/fanfox \
	grabber/flamecomics \
	grabber/fmteam \
	grabber/gdscans \
	grabber/genztoon \
	grabber/guya \
	grabber/hijala \
	grabber/inmanga \
	grabber/jestful \
	grabber/kaynscan \
	grabber/leercapitulo \
	grabber/luascans \
	grabber/mangaball \
	grabber/mangabats \
	grabber/mangadenizi \
	grabber/mangadex \
	grabber/mangafire \
	grabber/mangahere \
	grabber/mangak \
	grabber/mangalib \
	grabber/mangalivre \
	grabber/mangasushi \
	grabber/mangataro \
	grabber/mangitto \
	grabber/mgeko \
	grabber/mkissa \
	grabber/projectsuki \
	grabber/qimanga \
	grabber/roliascan \
	grabber/sacachispa \
	grabber/stonescape \
	grabber/taiyo \
	grabber/tcb \
	grabber/teamshadowi \
	grabber/tritinia \
	grabber/utoon \
	grabber/vortexscans \
	grabber/weebcentral \
	grabber/witchtoons \
	grabber/html

grabber/inmanga:
	go run . https://inmanga.com/ver/manga/One-Piece/dfc7ecb5-e9b3-4aa5-a61b-a498993cd935 1187

# note: use a language without an official publisher (i.e. not es/en/fr...):
# licensed translations get replaced by pageless mangaplus stubs on mangadex
grabber/mangadex:
	go run . https://mangadex.org/title/a1c7c817-4e59-43b7-9365-09675a149a6f/one-piece --language ca 1187 --bundle

grabber/mangabats:
	go run . https://www.mangabats.com/manga/after-the-possessor-left 1

# uses a real (headless) browser: the SPA signs every JSON API call with a
# per-session, cloudflare-challenge-gated vrf token, so we render the pages,
# let them make the signed calls and intercept the responses; images then
# download over plain HTTP. No --browser-visible needed, so not in the
# grabber/browser group, but kept out of the CI smoke matrix like every other
# browser grabber. Uses an old chapter (stable, not premium-gated like newest).
grabber/mangafire:
	go run . https://mangafire.to/title/dkw-one-piece 1050

grabber/fanfox:
	go run . https://fanfox.net/manga/chainsaw_man/ 232

grabber/mangak:
	go run . https://mangak.io/a-baby-cat-who-commands-the-dog-clan 30

grabber/mgeko:
	go run . https://www.mgeko.cc/manga/solo-leveling-mg1/ 198
grabber/mangaball:
	go run . https://mangaball.net/title-detail/baki-gaiden-shin-chiharu-6a5ffe5d90273b5b995225d2/ 1
grabber/mangalib:
	go run . https://mangalib.me/ru/manga/206--one-piece 1188
grabber/mangadenizi:
	go run . https://www.mangadenizi.net/manga/one-piece 1188

grabber/qimanga:
	go run . https://qimanga.com/series/4190634673-eleceed 2

# aurorascans.com is a rebrand alias that 301-redirects path-for-path to
# qimanga.com (via qimanhwa.com), handled by the same Qimanga grabber
grabber/aurorascans:
	go run . https://aurorascans.com/series/4190634673-nano-machine 322
# baozimh.com (包子漫画) needs its own grabber: the Chinese "第N话" chapter
# numbers aren't parsed by the PlainHTML regexes. Use an older chapter - the
# newest ones can be premium/early-access gated. Chapter 100 is a multi-part
# chapter (split across 0_99.html + 0_99_2.html, 77 pages after the overlap is
# deduped), so this target also exercises the 下一頁 part-following logic.
grabber/baozimh:
	go run . https://www.baozimh.com/comic/zhihuojintiandelunhuiqishi-leehyunminiankanara 100
grabber/bluesolo:
	go run . https://bluesolo.org/comics/frieren 147
# use a chapter with price 0 (recent chapters are often premium/paywalled)
grabber/luascans:
	go run . https://luacomic.org/series/even-today-the-ranker-dreams-of-retirement 56
grabber/mangitto:
	go run . https://mangtto.com/manga/chainsaw-man 232
grabber/projectsuki:
	go run . https://projectsuki.com/book/159270 233

grabber/tcb:
	go run . https://lhtranslation.net/manga/gaikotsu-kishi-sama-tadaima-isekai-e-o-dekake-chuu/ 71

# another Madara wordpress site, matched by the generic Tcb grabber (no code
# changes needed: same ajax/chapters endpoint and reading-content markup)
grabber/mangasushi:
	go run . https://mangasushi.org/manga/lonely-attack-on-the-different-world/ 324
# another Madara/wp-manga site, handled by the same Tcb grabber; this one
# groups chapters under "Volume N" wrappers (exercises the wp-manga-chapter
# scoping fix)
grabber/gdscans:
	go run . https://gdscans.com/manga/a-rank-boukensha-no-slow-life/ 50
# mangalivre.tv/mangalivre.net shut down (redirects to a "support official
# sources" closure page); mangalivre.to is the actively-updated successor.
# Its markup is a customized Madara theme, matched by the existing Tcb
# grabber with zero new code.
grabber/mangalivre:
	go run . https://mangalivre.to/manga/chainsaw-man-pt-br/ 232
# tritinia.org is a plain Madara wordpress site, already matched by the
# generic Tcb grabber (no new code needed)
grabber/tritinia:
	go run . https://tritinia.org/manga/blue-period/ 64

grabber/flamecomics:
	go run . https://flamecomics.xyz/series/154 104

# reader pages only show one page at a time; images are fetched per-page from
# an obfuscated (packed js) chapterfun.ashx endpoint, so this is its own
# grabber rather than a PlainHTML selector
grabber/mangahere:
	go run . https://www.mangahere.cc/manga/kengan_omega/ 363
grabber/jestful:
	go run . https://jestful.net/hwms-jitsu-wa-ore-saikyou-deshita.html 150

grabber/weebcentral:
	go run . https://weebcentral.com/series/01J76XYDXH7KT6AABVG3JAT3ZP/Shangri-La-Frontier 274

# uses a real (headless) browser just to toggle the reader's "load all pages"
# client-side preference, no --browser-visible needed (no cloudflare here)
grabber/leercapitulo:
	go run . https://www.leercapitulo.co/manga/0cj9hhn6di/kingdom/ 883

grabber/guya:
	go run . https://guya.moe/read/manga/Kaguya-Wants-To-Be-Confessed-To/ 281
# atsu.moe is a react SPA, but its json api is wide open to plain HTTP
grabber/atsumaru:
	go run . https://atsu.moe/manga/2VgNt 97
# 2VgNt is translated by three groups (Alpha, Gamma and Thunder), all of which
# have a chapter 97, so these two exercise --scanlator against a real ambiguity.
# They're deliberately out of the `grabber` aggregate to keep it to one download
# per site; run them by hand when touching the scanlator selection.
grabber/atsumaru/scanlator:
	go run . --scanlator alpha https://atsu.moe/manga/2VgNt 97
# expect one file per group, each tagged with the group name in its filename
grabber/atsumaru/all:
	go run . --scanlator all https://atsu.moe/manga/2VgNt 97
# use a chapter that's not one of the newest few (those can be paywalled
# behind coins/early access) so the smoke test doesn't flake as new chapters
# release
grabber/vortexscans:
	go run . https://vortexscans.org/series/archmage-curriculum 20

# same guyamoe platform as guya.moe, different instance/domain
grabber/danke:
	go run . https://danke.moe/read/manga/100-girlfriends/ 251
# chapter pages are hosted on imgchest.com, not bigsolo.org itself
grabber/bigsolo:
	go run . https://bigsolo.org/wind-breaker 222
grabber/fmteam:
	go run . https://fmteam.fr/comics/batuque 157
# note: the newest 1-2 chapters can be "early access" (locked behind coins),
# pick a chapter a bit behind the tip if this starts 404ing on pages
grabber/genztoon:
	go run . https://genzupdates.com/series/the-return-of-the-legendary-genius-ranker/ 33
grabber/kaynscan:
	go run . https://kaynscan.org/series/heavenly-demon-cultivation-simulation 191
grabber/hijala:
	go run . https://en-hijala.com/series/double-click 252
grabber/mangataro:
	go run . https://mangataro.org/manga/one-piece 1188
grabber/roliascan:
	go run . https://roliascan.com/manga/no-marriage/ 77
grabber/sacachispa:
	go run . https://sacachispa.site/series/boku-no-seito-wa-otona-gal 14
grabber/teamshadowi:
	go run . https://www.team-shadowi.com/series/the-regressed-mercenary-has-a-plan 98
grabber/taiyo:
	go run . https://taiyo.moe/media/000bdf97-407f-4ca8-95a1-ee2a3114e73a 82
grabber/stonescape:
	go run . https://stonescape.xyz/series/only-see-you 21
grabber/utoon:
	go run . https://www.utoon.us/en/manga/gachiakuta.html 171
# uses a real (headless) browser: the chapter list/reader JSON API responses
# are AES-encrypted client-side and the reader lazily mounts pages on scroll;
# no cloudflare here, no --browser-visible needed
grabber/comix:
	go run . https://comix.to/title/e1wlg-the-scandal-maker-has-returned 24
# reader route 403s cloudflare on direct navigation, but not through the
# app's own client-side (SPA) routing, which is what the grabber uses; no
# --browser-visible needed, the challenge is never actually triggered
grabber/mkissa:
	go run . https://mkissa.to/manga/eaYBWW65WLabNiLEi 59

# sites needing a real browser: not part of the `grabber` target since they
# open a Chrome window and may require solving an interactive challenge
# (cloudflare). Run them one by one and solve the challenge if prompted.
grabber/browser: \
	grabber/dragontea \
	grabber/drakecomic \
	grabber/kappabeast \
	grabber/mangahub \
	grabber/mangakakalot \
	grabber/mangapark \
	grabber/manhuatop \
	grabber/manhuaus \
	grabber/natomanga \
	grabber/setsuscans \
	grabber/sushiscan \
	grabber/toongod \
	grabber/toonily

grabber/toonily:
	go run . --browser-visible https://toonily.com/serie/the-beginning-after-the-end-ea2130e6/ 244

grabber/toongod:
	go run . --browser-visible https://www.toongod.org/webtoon/solo-leveling/ 200

grabber/dragontea:
	go run . --browser-visible https://dragontea.ink/novel/it-all-starts-with-trillions-of-nether-currency/ 290

grabber/kappabeast:
	go run . --browser-visible https://kappabeast.com/series/tekkarian 2

grabber/sushiscan:
	go run . --browser-visible https://sushiscan.net/catalogue/mushoku-tensei/ 17

grabber/mangakakalot:
	go run . --browser-visible https://www.mangakakalot.gg/manga/akuyaku-reijou-kara-no-kareinaru-tenshin-aisare-heroine-anthology-comic 1

grabber/natomanga:
	go run . --browser-visible https://www.natomanga.com/manga/rebirth-from-0-to-1 205.9

grabber/manhuaus:
	go run . --browser-visible https://manhuaus.com/manga/solo-leveling-ragnarok/ 68

grabber/mangahub:
	go run . --browser-visible https://mangahub.io/manga/one-piece_142 1188
grabber/manhuatop:
	go run . --browser-visible https://manhuatop.org/manhua/chronicles-of-the-martial-gods-return/ 168
grabber/drakecomic:
	go run . --browser-visible https://drakecomic.org/manga/beast-evolution/ 70
grabber/setsuscans:
	go run . --browser-visible https://setsuscans.com/manga/a-story-about-accidentally-finding-out-a-girls-secret-at-school/ 13

# the whole domain went behind a cloudflare JS challenge (Aug 2026), which
# headless never passes: only the series page is rendered, and the clearance it
# harvests carries the chapters API and the ~200 page images per chapter over
# plain HTTP. --browser-visible just skips the pointless 30s headless probe.
grabber/mangapark:
	go run . --browser-visible https://mangapark.page/series/rowdy-reunion 41

grabber/html: \
	grabber/asmotoon \
	grabber/asura \
	grabber/deathtollscans \
	grabber/demonicscans \
	grabber/dynastyscans \
	grabber/elftoon \
	grabber/furyosociety \
	grabber/hivetoons \
	grabber/lagoonscans \
	grabber/madarascans \
	grabber/mangakatana \
	grabber/mangapill \
	grabber/mangaread \
	grabber/manhuaplus \
	grabber/mistscans \
	grabber/rawkuma \
	grabber/ritharscans \
	grabber/rokaricomics \
	grabber/sanascans \
	grabber/silentquill \
	grabber/tcbscans \
	grabber/templetoons \
	grabber/thunderscans \
	grabber/violetscans \
	grabber/writerscans \
	grabber/zonatmo

grabber/tcbscans:
	go run . https://tcbonepiecechapters.com/mangas/5/one-piece 1100

grabber/asura:
	go run . https://asurascans.com/comics/absolute-regression-f886a8af 1

grabber/zonatmo:
	go run . https://zonatmo.org/library/manga/31322/one-piece 1188

grabber/mangapill:
	go run . https://mangapill.com/manga/2/one-piece 1188

grabber/demonicscans:
	go run . https://demonicscans.org/manga/Return-of-the-Mount-Hua-Sect 172

grabber/mangakatana:
	go run . https://mangakatana.com/manga/dandadan.25818 241
grabber/rawkuma:
	go run . https://rawkuma.net/manga/jujutsu-kaisen/ 271
grabber/dynastyscans:
	go run . https://dynasty-scans.com/series/please_bully_me_miss_villainess 162
grabber/mangaread:
	go run . https://www.mangaread.org/manga/one-piece/ 1188
grabber/manhuaplus:
	go run . https://manhuaplus.com/manga/tales-of-demons-and-gods01/ 522.6
grabber/silentquill:
	go run . https://www.silentquill.net/i-was-invited-to-join-the-country-as-an-otherworldly-warrior-but-i-refused-and-decided-to-start-as-a-soldier/ 59
grabber/hivetoons:
	go run . https://hivetoons.org/series/eleceed 410
grabber/templetoons:
	go run . https://templetoons.com/comic/bl-antidote 88
grabber/deathtollscans:
	go run . https://reader.deathtollscans.net/series/kakushigoto/ 30
grabber/furyosociety:
	go run . https://furyosociety.com/series/a-bout/ 3
grabber/elftoon:
	go run . https://elftoon.com/manga/god-level-assassin-im-the-shadow/ 123
grabber/asmotoon:
	go run . https://asmotoon.com/series/official-adultery/ 1
grabber/madarascans:
	go run . https://madarascans.org/series/my-disciples-are-all-big-villains/ 481
grabber/lagoonscans:
	go run . https://lagoonscans.com/manga/crimson-reset/ 52
grabber/rokaricomics:
	go run . https://rokaricomics.com/manga/caregivers-alliance/ 5
# note: use a free chapter (older ones); recent chapters are often coin-gated
grabber/ritharscans:
	go run . https://ritharscans.com/series/0f32cefc-20a7-4337-aed0-fa78f832012f 19
grabber/sanascans:
	go run . https://sanascans.com/series/my-beloved-daughter-is-a-villainess 11
grabber/thunderscans:
	go run . https://en-thunderscans.com/comics/full-time-awakening/ 108
grabber/violetscans:
	go run . https://violetscans.org/comics/after-all-ill-marry-you/ 21
# witchscans.com rebranded to witchtoons.net: new platform (Next.js, chapters
# and pages in the RSC payload), so it has its own grabber instead of a
# plainhtml selector entry
grabber/witchtoons:
	go run . https://witchtoons.net/series/comic/i-am-a-cultivation-big-shot 600
grabber/writerscans:
	go run . https://writerscans.com/series/640ecb8f2bb/ 45
# note: chapters get locked behind a coin/early-access paywall right after
# release; pick a chapter a few behind the latest one to keep it free
grabber/mistscans:
	go run . https://mistscans.com/series/i-was-thrown-into-an-unfamiliar-manga/ 33
