# The single source of the build flags: the release workflows build through
# these targets, or read the variables with `make -s print-VAR`.

# both can be overridden, as the release workflows do (make VERSION=… TAG=…)
VERSION ?= $(shell git rev-parse --short HEAD 2> /dev/null)
TAG ?= $(or $(CI_COMMIT_REF_NAME),develop)

# nodynamic forces gen2brain/avif to use its embedded libavif/dav1d WASM build
# (run under wazero, no cgo) instead of dlopen'ing whatever libavif happens to
# be installed, so page conversion decodes identically everywhere.
BUILD_TAGS = nodynamic
GOTAGS = -tags $(BUILD_TAGS)
# -s -w drop the symbol table and DWARF (about a third of the size; panic traces
# use the runtime's pclntab, which stays). -trimpath keeps local paths out.
GO_LDFLAGS = -s -w \
	-X 'github.com/elboletaire/manga-downloader/cmd.Version=$(VERSION)' \
	-X 'github.com/elboletaire/manga-downloader/cmd.Tag=$(TAG)'
GOBUILD = CGO_ENABLED=0 go build -trimpath $(GOTAGS) -ldflags="$(GO_LDFLAGS)"
# set by the caller when cross-compiling (GOOS=windows make build/release)
EXE = $(if $(filter windows,$(GOOS)),.exe)
RICHGO := $(shell command -v richgo 2> /dev/null)

clean:
	@rm -fv ./manga-downloader* *.cbz *.epub

install:
	go mod download

build: clean test build/unix

build/all: clean test build/unix build/win

build/unix:
	$(GOBUILD) -o manga-downloader .

build/win:
	GOOS=windows $(GOBUILD) -o manga-downloader.exe .

# the binary the VHS tapes in assets/demos run: it serves their series from the
# fake data in demo/series (see demo/README.md), so release builds never carry
# them. Named like build/unix, which is where the tapes look for it.
build/demo: GOTAGS = -tags $(BUILD_TAGS),demo
build/demo:
	$(GOBUILD) -o manga-downloader .
	@echo "demo build: serves demo/series and never touches the network"

# what the release builds for each GOOS/GOARCH (from the environment, the host's
# by default): manga-downloader, or manga-downloader.exe for windows
build/release:
	$(GOBUILD) -o manga-downloader$(EXE) .

# prints a variable; fails if undefined, to catch typos in the workflows
print-%: ; $(if $(filter undefined,$(origin $*)),$(error $* is not defined),$(info $($*)))@:

# the desktop app, deliberately not part of build: it needs the platform's
# webview (WebKitGTK on Linux) and Node + pnpm for the frontend. It's written
# next to the CLI, which is where `manga-downloader ui` looks for it.
# GUI_TAGS and GUI_LDFLAGS are what release-gui.yaml hands to `wails build`,
# which adds the desktop,production tags (without them the app refuses to
# start) and -s -w itself; plain go build gets them from GUI_GO_*. webkit2_41
# is for current Linux distros; the other platforms ignore it.
GUI_TAGS = $(BUILD_TAGS),webkit2_41
GUI_LDFLAGS = -X 'main.Version=$(VERSION)' -X 'main.Tag=$(TAG)'
GUI_GO_TAGS = desktop,production,$(GUI_TAGS)
GUI_GO_LDFLAGS = -s -w $(GUI_LDFLAGS)
# Wails' macOS frontend uses UTType but leaves linking UniformTypeIdentifiers
# to `wails build`, so a plain go build has to ask for it.
ifeq ($(shell uname -s),Darwin)
GUI_CGO = CGO_LDFLAGS="$(CGO_LDFLAGS) -framework UniformTypeIdentifiers"
endif

# the frontend is embedded into the binary, so it has to exist before go builds
build/gui/frontend:
	cd gui/frontend && pnpm install --frozen-lockfile && pnpm build

build/gui: build/gui/frontend
	cd gui && $(GUI_CGO) go build -trimpath -tags $(GUI_GO_TAGS) -ldflags="$(GUI_GO_LDFLAGS)" \
		-o ../manga-downloader-gui .

# cross-compiled from any host, without cgo: Wails' Windows webview is pure Go.
# -H windowsgui keeps the console from opening behind the window. The release
# uses `wails build` instead, which also embeds the icon and manifest.
build/gui/windows: build/gui/frontend
	cd gui && CGO_ENABLED=0 GOOS=windows go build -trimpath -tags $(GUI_GO_TAGS) \
		-ldflags="$(GUI_GO_LDFLAGS) -H windowsgui" -o ../manga-downloader-gui.exe .

test:
ifdef RICHGO
	richgo test ${GOTAGS} -v ./...
else
	go test ${GOTAGS} -v ./...
endif

# gui/ is its own module, so ./... above never reaches it. The Go tests need
# the frontend built (it's embedded) and the webview's headers, as build/gui does
test/gui: build/gui/frontend
	cd gui && $(GUI_CGO) go test -tags $(GUI_GO_TAGS) ./...
	cd gui/frontend && pnpm test

# the demo build's fake grabber is only built with the demo tag (see demo/README.md)
test/demo:
	go test -tags $(BUILD_TAGS),demo -count=1 ./demo/

# one dependency per line, alphabetically sorted (grabber/html deliberately
# last), so parallel site-support branches never edit the same line
grabber: \
	grabber/3asq \
	grabber/anikiga \
	grabber/animeallstar1 \
	grabber/animeallstar30 \
	grabber/apothecary \
	grabber/atsumaru \
	grabber/aurorascans \
	grabber/azorafly \
	grabber/baozimh \
	grabber/bigcomics \
	grabber/bigsolo \
	grabber/blackcloveronline \
	grabber/bluesolo \
	grabber/chainsawman \
	grabber/chainsawmannmanga \
	grabber/championcross \
	grabber/comicaction \
	grabber/comicborder \
	grabber/comicdays \
	grabber/comicearthstar \
	grabber/comicgardo \
	grabber/comicgrowl \
	grabber/comicheros \
	grabber/comicogyaaa \
	grabber/comicride \
	grabber/comicryu \
	grabber/comicseasons \
	grabber/comictrail \
	grabber/comicwalker \
	grabber/comicyours \
	grabber/comiczenon \
	grabber/comix \
	grabber/danke \
	grabber/dragonballmangaa \
	grabber/drakecomic \
	grabber/elftoon \
	grabber/fanfox \
	grabber/feelweb \
	grabber/flamecomics \
	grabber/fmteam \
	grabber/gdscans \
	grabber/genztoon \
	grabber/globalcomix \
	grabber/guya \
	grabber/hachirumi \
	grabber/heavenlydelusion \
	grabber/heavenmanga \
	grabber/hijala \
	grabber/hivetoons \
	grabber/ichicomi \
	grabber/inmanga \
	grabber/jestful \
	grabber/jojolands \
	grabber/jujutsukaisenmanga \
	grabber/kansai \
	grabber/kaynscan \
	grabber/ksgroupscans \
	grabber/kuragebunch \
	grabber/leercapitulo \
	grabber/lelscans \
	grabber/lilymanga \
	grabber/luascans \
	grabber/magcomi \
	grabber/mangaball \
	grabber/mangabats \
	grabber/mangadenizi \
	grabber/mangadex \
	grabber/mangadelsiglotvmanga \
	grabber/mangafire \
	grabber/mangahere \
	grabber/mangak \
	grabber/mangalib \
	grabber/mangalivre \
	grabber/mangapark \
	grabber/manganato \
	grabber/mangaowl \
	grabber/mangaplus \
	grabber/mangasushi \
	grabber/mangataro \
	grabber/mangatimesquare \
	grabber/mangatown \
	grabber/mangitto \
	grabber/manhwatop \
	grabber/mgeko \
	grabber/mkissa \
	grabber/naver \
	grabber/nijitranslations \
	grabber/ourfeel \
	grabber/projectsuki \
	grabber/qimanga \
	grabber/re-yume \
	grabber/roliascan \
	grabber/sacachispa \
	grabber/shonenjumpplus \
	grabber/stonescape \
	grabber/sundaywebry \
	grabber/taiyo \
	grabber/takecomic \
	grabber/tcb \
	grabber/teamshadowi \
	grabber/templetoons \
	grabber/tonarinoyj \
	grabber/tortugaceviri \
	grabber/turkcemanga \
	grabber/tvymanga3 \
	grabber/tritinia \
	grabber/utoon \
	grabber/vortexscans \
	grabber/weebcentral \
	grabber/webtoonia \
	grabber/webtoons \
	grabber/witchtoons \
	grabber/younganimal \
	grabber/yurionair \
	grabber/zinmanga \
	grabber/html

grabber/inmanga:
	go run . https://inmanga.com/ver/manga/One-Piece/dfc7ecb5-e9b3-4aa5-a61b-a498993cd935 1187

# note: use a language without an official publisher (i.e. not es/en/fr...):
# licensed translations get replaced by pageless mangaplus stubs on mangadex
grabber/mangadex:
	go run . https://mangadex.org/title/a1c7c817-4e59-43b7-9365-09675a149a6f/one-piece --language ca 1187 --bundle

grabber/mangabats:
	go run . https://www.mangabats.com/manga/after-the-possessor-left 30

grabber/manganato:
	go run . https://www.manganato.gg/manga/apotheosis 2

# uses a real (headless) browser: the SPA signs every JSON API call with a
# per-session, cloudflare-challenge-gated vrf token, so we render the pages,
# let them make the signed calls and intercept the responses; images then
# download over plain HTTP. No --browser-visible needed, so not in the
# grabber/browser group, but kept out of the CI smoke matrix like every other
# browser grabber. Uses an old chapter (stable, not premium-gated like newest).
# The two runs cover both chapter-list pager shapes (#169): One Piece has the
# windowed pager with "Next page" arrows; the second series is short enough
# (3 pages) that the site renders numbered buttons only, and its chapter 1
# lives on the last page — the run exits 1 unless every page was harvested.
grabber/mangafire:
	go run . https://mangafire.to/title/dkw-one-piece 1050
	go run . https://mangafire.to/title/9062q-itoko-no-onee-chan-ni-amaechau 1

grabber/fanfox:
	go run . https://fanfox.net/manga/chainsaw_man/ 232

# an old chapter of a long series on purpose: the series page only embeds the
# 50 newest chapters, so this exercises the full-list API fetch
grabber/mangak:
	go run . https://mangak.io/nano-machine 150

grabber/mgeko:
	go run . https://www.mgeko.cc/manga/solo-leveling-mg1/ 198
grabber/mangaball:
	go run . https://mangaball.com/title-detail/baki-gaiden-shin-chiharu-6a5ffe5d90273b5b995225d2/ 1
grabber/mangalib:
	go run . https://mangalib.me/ru/manga/206--one-piece 1188
grabber/mangadenizi:
	go run . https://www.mangadenizi.net/manga/one-piece 1188

# Manga Plus talks to the app API, which rate limits hard (see AGENTS.md).
# Any chapter of this series is free; an old one keeps the target stable.
grabber/mangaplus:
	go run . https://mangaplus.shueisha.co.jp/titles/100274 67

grabber/qimanga:
	go run . https://qimanga.com/series/4190634673-eleceed 2

# aurorascans.com is a rebrand alias that 301-redirects path-for-path to
# qimanga.com (via qimanhwa.com), handled by the same Qimanga grabber
grabber/aurorascans:
	go run . https://aurorascans.com/series/4190634673-nano-machine 322
# baozimh.com (包子漫画) needs its own grabber: the Chinese "第N话" chapter
# numbers aren't parsed by the PlainHTML regexes. baozimh.com itself now 403s,
# so this targets its www.twmanga.com mirror. Use an older chapter - the
# newest ones can be premium/early-access gated. Chapter 100 is a multi-part
# chapter (split across 0_99.html + 0_99_2.html, 77 pages after the overlap is
# deduped), so this target also exercises the 下一頁 part-following logic.
grabber/baozimh:
	go run . https://www.twmanga.com/comic/zhihuojintiandelunhuiqishi-leehyunminiankanara 100
grabber/bluesolo:
	go run . https://bluesolo.org/comics/frieren 147
# single-manga WordPress site, rotating wN. subdomains; use a chapter slightly
# behind the newest to keep the target stable as new chapters release
grabber/chainsawman:
	go run . https://w2.chainsawman-chapter.online 230
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
# Madara under /read-N/ (not /manga/), matched by the generic Tcb grabber
grabber/mangaowl:
	go run . https://mangaowl.io/read-1/beyond-the-limit/ 1
# a Madara site whose reader encrypts the page list (wp-manga-chapter-images-protection)
grabber/gourmetsupremacy:
	go run . https://gourmetsupremacy.com/project/hikikomori-princess-marriage/ 10,10.5
# another Madara/wp-manga site, handled by the same Tcb grabber; this one
# groups chapters under "Volume N" wrappers (exercises the wp-manga-chapter
# scoping fix)
grabber/gdscans:
	go run . https://gdscans.com/manga/a-rank-boukensha-no-slow-life/ 50
# a Madara site whose ajax/chapters answers with the series page, so the Tcb
# grabber reads the list through admin-ajax.php (Arabic)
grabber/nijitranslations:
	go run . https://niji-translations.com/manga/adachi-and-shimamura/ 41
# another Madara site, handled by the Tcb grabber; its one-shots have no number
grabber/anikiga:
	go run . https://anikiga.com/manga/alma/ 5

# Madara and themesia sites whose series URLs PlainHTML claims (chapter URLs
# can go through Tcb), no code of their own
grabber/apollcomics:
	go run . https://apollcomics.es/manga/las-manos-sanadoras-del-doctor/ 40-41
grabber/fbsquadx:
	go run . https://fbsquadx.com/manga/minmotion-syndrome/ 60-61
grabber/kombatch:
	go run . https://kombatch.cc/manga/secret-class/ 318-319
grabber/mundomanhwa:
	go run . https://mundomanhwa.com/manga/primavera-para-blossom/ 28-29

# another Madara wordpress site (Turkish). Its series page lists every row
# under "<title> N" titles, which the PlainHTML li.wp-manga-chapter entry
# can't number, so PlainHTML declines it and the Tcb grabber reads the full
# ajax/chapters list instead.
grabber/tortugaceviri:
	go run . https://tortugaceviri.com/manga/berserk/ 326

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

# Arabic Madara wordpress site (3asq.org redirects here), matched by the
# generic Tcb grabber with no code changes
grabber/3asq:
	go run . https://3asq.online/manga/berserk/ 6

# WordPress category of chapter posts (plain HTTP), matched by markup and read
# through the REST API; 1195 rather than the newest chapter, which may still be a teaser
grabber/animeallstar30:
	go run . https://animeallstar30.com/category/one-piece-espanol/ 1195

# Blogger, matched by its markup: a series is the blog's search for its title
# (the posts carry no label per series). Two chapters a few behind the newest.
grabber/animeallstar1:
	go run . "https://www.animeallstar1.com/search?q=One+Piece" 1188-1189

# WordPress category clones of animeallstar30 (plain HTTP): a chapter post
# resolves to its series, so the URL is a post a few behind the newest.
grabber/blackcloveronline:
	go run . https://blackcloveronline.com/manga/black-clover-chapter-100/ 100

grabber/chainsawmannmanga:
	go run . https://chainsawmannmanga.com/chainsaw-man-manga-chapter-100/ 100

grabber/dragonballmangaa:
	go run . https://dragonballmangaa.com/category/all-chapters/ 10

grabber/jujutsukaisenmanga:
	go run . https://jujutsukaisenmanga.pro/jujutsu-kaisen-chapter-100/ 100

grabber/tvymanga3:
	go run . https://tvymanga3.com/chainsaw-man-100/ 100

# Blogger blog, series search by title as for animeallstar1
grabber/mangadelsiglotvmanga:
	go run . "https://www.mangadelsiglotvmanga.com.mx/search?q=Kagurabachi" 130

# Blogger blog whose chapter posts keep the pages in a hidden textarea
# (script-rendered reader); a chapter post resolves to its series.
grabber/re-yume:
	go run . https://www.re-yume.my.id/2026/09/shangri-la-frontier-chapter-281.html 280-281

# Madara wordpress site whose series URLs live under /gl/ instead of /manga/;
# the generic Tcb grabber matches them through the same ajax/chapters endpoint.
# Chapter 2 is not the newest (6 episodes), and it sits in the list-style reader.
grabber/lilymanga:
	go run . https://lilymanga.net/gl/a-love-hate-chord/ 2

grabber/flamecomics:
	go run . https://flamecomics.xyz/series/154 104

# reader pages only show one page at a time; images are fetched per-page from
# an obfuscated (packed js) chapterfun.ashx endpoint, so this is its own
# grabber rather than a PlainHTML selector
grabber/mangahere:
	go run . https://www.mangahere.cc/manga/kengan_omega/ 363
grabber/apothecary:
	go run . https://the-apothecary-diaries.com 85
grabber/jestful:
	go run . https://jestful.net/hwms-jitsu-wa-ore-saikyou-deshita.html 150

grabber/jojolands:
	go run . https://jojolandsmanga.com 39

grabber/heavenlydelusion:
	go run . https://heavenly-delusion.com 83

grabber/heavenmanga:
	go run . https://heavenmanga.com/manga/black-clover 388

grabber/weebcentral:
	go run . https://weebcentral.com/series/01J76XYDXH7KT6AABVG3JAT3ZP/Shangri-La-Frontier 274

# plain HTTP only, the reader renders every page in the markup (no browser)
grabber/leercapitulo:
	go run . https://www.leercapitulo.co/manga/0cj9hhn6di/kingdom/ 883

grabber/lelscans:
	go run . https://lelscans.net/lecture-en-ligne-one-piece 1100

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
# expect one file per group: their names only differ by a version suffix
# (v2, v3), as the group isn't in the default file name
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
# recognised by its series API, not its domain
grabber/hachirumi:
	go run . https://hachirumi.com/read/manga/a-boy-and-his-crossdressing-older-brother/ 1
# chapter pages are hosted on imgchest.com, not bigsolo.org itself
grabber/bigsolo:
	go run . https://bigsolo.org/wind-breaker 222
grabber/fmteam:
	go run . https://fmteam.fr/comics/batuque 157
# note: the newest 1-2 chapters can be "early access" (locked behind coins),
# pick a chapter a bit behind the tip if this starts 404ing on pages
grabber/genztoon:
	go run . https://genztoons.org/series/the-return-of-the-legendary-genius-ranker/ 33
# licensed comics over a JSON API; paid issues are listed as locked, so the target
# uses a free series (10 is a short 13-page issue)
grabber/globalcomix:
	go run . https://globalcomix.com/c/acts-one-nine 10
# drakecomic.org moved wholesale to drakecomic.net (same RSC platform as
# witchtoons.net/kaynscans.com); the old domain still 301s /manga/{slug}/
# here and the grabber remaps old URLs in code. No browser needed anymore.
grabber/drakecomic:
	go run . https://drakecomic.net/series/comic/beast-evolution 70
# kaynscan.org moved to kaynscans.com (same RSC platform as witchtoons.net);
# the old domain still 301s here and the grabber remaps old URLs in code
grabber/kaynscan:
	go run . https://kaynscans.com/series/comic/heavenly-demon-cultivation-simulation 191
# hivetoons only server-renders the ~20 newest chapter anchors ("Load more"
# does the rest client-side), so the grabber reads the full list out of the
# page's TanStack (seroval) data; use a chapter well behind that window so the
# smoke run proves the embedded list is what's being read
grabber/hivetoons:
	go run . https://hivetoons.org/series/eleceed 200

# azorafly is another tanstackComic site: the same chapter list sits in the
# seroval data, so a chapter well behind the newest ones proves it's read.
grabber/azorafly:
	go run . https://azorafly.com/series/a-ghostly-song 20

grabber/hijala:
	go run . https://en-hijala.com/series/double-click 252
grabber/mangataro:
	go run . https://mangataro.org/manga/one-piece 1188
# plain HTTP, one HTML document per reader page
grabber/mangatown:
	go run . https://www.mangatown.com/manga/koi_wa_amaagari_no_you_ni/ 10
grabber/roliascan:
	go run . https://roliascan.com/manga/no-marriage/ 77
grabber/sacachispa:
	go run . https://sacachispa.site/manga/3ba3f7c1-4655-488f-87c5-bf1d797007c1/tsutte-tabetai-gal-sawa-san 19
# GigaViewer: an episode URL downloads its whole series, episode 2 is free
grabber/shonenjumpplus:
	go run . https://shonenjumpplus.com/episode/17107419589696004710 2
# GigaViewer hosts, served by the grabber of shonenjumpplus
grabber/tonarinoyj:
	go run . https://tonarinoyj.jp/episode/12207421984093802437 1
grabber/comicdays:
	go run . https://comic-days.com/episode/10834108156628505403 1
grabber/kuragebunch:
	go run . https://kuragebunch.com/episode/10834108156628843112 1
grabber/magcomi:
	go run . https://magcomi.com/episode/12207421984022901697 56
grabber/comicgardo:
	go run . https://comic-gardo.com/episode/10044607041200756622 1
grabber/comiczenon:
	go run . https://comic-zenon.com/episode/12207421984318179863 695
grabber/comicaction:
	go run . https://comic-action.com/episode/10044607041217599247 3
grabber/comicborder:
	go run . https://comicborder.com/episode/12207421984304147260 1
grabber/comictrail:
	go run . https://comic-trail.com/episode/12207421984031179174 45
# KadoComi: a series URL, episode 1 is free
grabber/comicwalker:
	go run . https://comic-walker.com/detail/KC_000061_S 1
grabber/comicearthstar:
	go run . https://comic-earthstar.com/episode/12207421983368356635 1
grabber/comicogyaaa:
	go run . https://comic-ogyaaa.com/episode/12207421984209269805 7
grabber/sundaywebry:
	go run . https://www.sunday-webry.com/episode/12207421983409579527 1
grabber/comicseasons:
	go run . https://comic-seasons.com/episode/12207421983368290655 0
grabber/comicyours:
	go run . https://comic-y-ours.com/episode/12207421983368565943 56
# Comici, the viewer of several publishers: an episode URL downloads its whole series
grabber/championcross:
	go run . https://championcross.jp/episodes/02c083f070a2e 1
grabber/comicgrowl:
	go run . https://comic-growl.com/episodes/134898aec09ea 19
grabber/comicheros:
	go run . https://heros-web.com/episodes/c1ffef58c01d8 1
grabber/comicride:
	go run . https://comicride.jp/episodes/eedb40a2c4f53 1
grabber/comicryu:
	go run . https://comic-ryu.jp/episodes/c56f063e17d3d 1
grabber/takecomic:
	go run . https://takecomic.jp/episodes/3984bbf3a37b6 0
grabber/bigcomics:
	go run . https://bigcomics.jp/series/7e7e188909a6e 1
grabber/kansai:
	go run . https://kansai.mag-garden.co.jp/series/dfadc73bc5c8d 1
grabber/younganimal:
	go run . https://younganimal.com/series/7ad02dc9743d0 1
grabber/ourfeel:
	go run . https://ourfeel.jp/episode/12207421983347996105 1
grabber/feelweb:
	go run . https://feelweb.jp/episode/10044607041223436639 1
grabber/ichicomi:
	go run . https://ichicomi.com/episode/12207421983348275548 2
grabber/mangatimesquare:
	go run . https://mangatime-square.com/episode/12207421983656601022 1
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
# uses a real (headless) browser: the whole site sits behind a cloudflare
# Turnstile challenge that plain HTTP is redirected to, and headless clears it
# in a few seconds; the reader's page list is in the RSC payload, and the
# images then download over plain HTTP. No --browser-visible needed.
grabber/templetoons:
	go run . https://templetoons.com/comic/bl-antidote 88
# reader route 403s cloudflare on direct navigation, but not through the
# app's own client-side (SPA) routing, which is what the grabber uses; no
# --browser-visible needed, the challenge is never actually triggered
grabber/mkissa:
	go run . https://mkissa.to/manga/eaYBWW65WLabNiLEi 59

# bestChallenge/challenge titles use the same API; 183559 is a free series
grabber/naver:
	go run . 'https://comic.naver.com/webtoon/list?titleId=183559' 2-3

# sites needing a real browser: not part of the `grabber` target since they
# open a Chrome window and may require solving an interactive challenge
# (cloudflare). Run them one by one and solve the challenge if prompted.
grabber/browser: \
	grabber/asmotoon \
	grabber/dragontea \
	grabber/kappabeast \
	grabber/mangahub \
	grabber/mangakakalot \
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

grabber/asmotoon:
	go run . --browser-visible https://asmotoon.com/tadaima-taikichuu-nitsuki 1-2

grabber/dragontea:
	go run . --browser-visible https://dragontea.ink/novel/it-all-starts-with-trillions-of-nether-currency/ 290

grabber/kappabeast:
	go run . --browser-visible https://kappabeast.com/series/tekkarian 2

grabber/sushiscan:
	go run . --browser-visible https://sushiscan.net/catalogue/mushoku-tensei/ 17

# manganelo family (Manganelo grabber): the chapters list comes from the
# /api/manga/{slug}/chapters JSON API — the series page DOM only renders the
# newest 50 rows, so the old selector-driven scrape silently truncated long
# series. The browser only clears the cloudflare challenge on the series
# page once; the API, reader pages and images all go over plain HTTP. The
# smoke chapters are old ones the truncated DOM never listed.
grabber/mangakakalot:
	go run . --browser-visible https://www.mangakakalot.gg/manga/apotheosis 2

grabber/natomanga:
	go run . --browser-visible https://www.natomanga.com/manga/rebirth-from-0-to-1 2

grabber/manhuaus:
	go run . --browser-visible https://manhuaus.com/manga/solo-leveling-ragnarok/ 68

grabber/mangahub:
	go run . --browser-visible https://mangahub.io/manga/one-piece_142 1188
grabber/manhuatop:
	go run . --browser-visible https://manhuatop.org/manhua/chronicles-of-the-martial-gods-return/ 168
grabber/setsuscans:
	go run . --browser-visible https://setsuscans.com/manga/a-story-about-accidentally-finding-out-a-girls-secret-at-school/ 13

# mangapark.cc serves plain HTML with no browser required. Chapters API
# paginates with ?lang=LANG&offset=N; images are on an external CDN.
grabber/mangapark:
	go run . https://mangapark.cc/manga/shonan-junai-gumi 266-267

# ksgroupscans, manhwatop, turkcemanga and yurionair are Madara sites matched
# by the generic Tcb grabber (same ajax/chapters endpoint)
grabber/ksgroupscans:
	go run . https://ksgroupscans.com/manga/buta-kizoku-wa-mirai-wo-kiri-hiraku-you-desu/ 20
grabber/manhwatop:
	go run . https://manhwatop.com/manga/defense-breaker/ 60
grabber/turkcemanga:
	go run . https://turkcemanga.com/manga/the-lady-with-a-mask/ 50
grabber/yurionair:
	go run . https://yurionair.top/manga/adachi-to-shimamura/ 46

# plain HTTP, chapters from a paginated JSON api, images in data-src
grabber/zinmanga:
	go run . https://www.zinmanga.net/manga/academy-idolmaster-gold-rush 12

# adonisfansub, nyraxmanga and rackusreads run the themesia theme. nyraxmanga
# marks paid chapters with a PLUS badge: chapters 116-122 of this series are
# (the CLI leaves them out), so 110 is a free one
grabber/adonisfansub:
	go run . https://manga.adonisfansub.com/manga/tensei-shitara-ken-deshita/ 26
grabber/nyraxmanga:
	go run . https://nyraxmanga.com/manga/for-my-lost-love/ 110
grabber/rackusreads:
	go run . https://rackusreads.com/manga/vs-hankeunbit/ 21
# mangaeclipse, mangawow and ruyamanga2 are Madara sites with the chapter list
# in the series page. ruyamanga2 skips chapters 41-61 of this series
grabber/mangaeclipse:
	go run . https://mangaeclipse.com/manga/great-heaven-sovereign/ 54
grabber/mangawow:
	go run . https://mangawow.org/manga/eleceed/ 220
grabber/ruyamanga2:
	go run . https://www.ruyamanga2.com/manga/felaket-buyucusu/ 70
grabber/natsuone:
	go run . https://natsu.one/manga/ao-ashi/ 400
# FoOlSlide readers. Lilyreader restarts the chapter numbers in every volume
# of the long series, so this smaller one has unique numbers
grabber/noranofansub:
	go run . https://www.noranofansub.com/lector/series/your-royal-highness-is-happy/ 38
grabber/smuglo:
	go run . https://manga.smuglo.li/series/aikatsu-secret-story/ 4

grabber/html: \
	grabber/adonisfansub \
	grabber/afroditscans \
	grabber/anzmanga25 \
	grabber/apollcomics \
	grabber/arenascan \
	grabber/asura \
	grabber/decadencescans \
	grabber/deathtollscans \
	grabber/demonicscans \
	grabber/dynastyscans \
	grabber/eldermanga \
	grabber/eskimangalar \
	grabber/evascans \
	grabber/fbsquadx \
	grabber/furyosociety \
	grabber/hentai20 \
	grabber/kingofshojo \
	grabber/kombatch \
	grabber/komikdewasa \
	grabber/komikindo \
	grabber/komikmanga \
	grabber/komikstation \
	grabber/gourmetsupremacy \
	grabber/lagoonscans \
	grabber/lelmanga \
	grabber/lelscanfr \
	grabber/limonmanga \
	grabber/madaradex \
	grabber/madarascans \
	grabber/maid \
	grabber/mangaberri \
	grabber/mangadistrict \
	grabber/mangaeclipse \
	grabber/mangakatana \
	grabber/mangalami \
	grabber/mangapill \
	grabber/mangaread \
	grabber/mangasorigines \
	grabber/mangasusuku \
	grabber/mangavault \
	grabber/mangaworld \
	grabber/mangawow \
	grabber/mangazin \
	grabber/manhuaplus \
	grabber/manhwabuddy \
	grabber/manhwaonline \
	grabber/mistscans \
	grabber/mundomanhwa \
	grabber/nabicix \
	grabber/natsuone \
	grabber/noranofansub \
	grabber/nyraxmanga \
	grabber/onma \
	grabber/oremanga \
	grabber/rackusreads \
	grabber/ravenscans \
	grabber/rawkuma \
	grabber/ritharscans \
	grabber/rizzfables \
	grabber/rokaricomics \
	grabber/ruyamanga2 \
	grabber/sanascans \
	grabber/scanvf \
	grabber/sektedoujin \
	grabber/sereinscan \
	grabber/silentquill \
	grabber/smuglo \
	grabber/tcbscans \
	grabber/thunderscans \
	grabber/trmanga \
	grabber/uzaymanga \
	grabber/violetscans \
	grabber/writerscans \
	grabber/zonatmo

grabber/tcbscans:
	go run . https://tcbonepiecechapters.com/mangas/5/one-piece 1100

# --skip-pages end to end. Chapter 1146's last page 404s on the site's end, so
# it's undownloadable without the flag: a run that produces no cbz at all is
# the failure to watch for, not a short one. Kept out of the `grabber:`
# aggregate on purpose - it's a flag check, not site coverage.
skip-pages:
	go run . --skip-pages=-1 https://tcbonepiecechapters.com/mangas/5/one-piece 1146
	go run ./tools/verify-cbz "One Piece 1146 - A Stirring in the Stillness.cbz"

grabber/arenascan:
	go run . https://arenascan.com/manga/academys-genius-swordmaster/ 80
grabber/asura:
	go run . https://asurascans.com/comics/absolute-regression-f886a8af 1

grabber/trmanga:
	go run . https://trmanga.com/webtoon/no-place-for-the-fake-princess 50

grabber/uzaymanga:
	go run . https://uzaymanga.com/manga/kunlun-daginin-seytani-ustasi 10

# eldermanga, eskimangalar and limonmanga share uzaymanga's platform
grabber/eldermanga:
	go run . https://eldermanga.com/manga/agac-olarak-yeniden-dogdum 10

grabber/eskimangalar:
	go run . https://eskimangalar.com/manga/alemlerin-otesindeki-olumsuz 10

grabber/limonmanga:
	go run . https://limonmanga.com/manga/en-guclu-suikastci-baska-dunyada 10

grabber/manhwaonline:
	go run . https://manhwaonline.org/manga/a-wonderful-new-world/ 150

grabber/zonatmo:
	go run . https://zonatmo.org/library/manga/31322/one-piece 1188

grabber/mangapill:
	go run . https://mangapill.com/manga/2/one-piece 1188

grabber/demonicscans:
	go run . https://demonicscans.org/manga/Return-of-the-Mount-Hua-Sect 172

grabber/mangakatana:
	go run . https://mangakatana.com/manga/dandadan.25818 241
grabber/ravenscans:
	go run . https://ravenscans.org/manga/academys-genius-swordmaster/ 79
grabber/rawkuma:
	go run . https://rawkuma.net/manga/jujutsu-kaisen/ 271
grabber/dynastyscans:
	go run . https://dynasty-scans.com/series/please_bully_me_miss_villainess 162
grabber/mangaread:
	go run . https://www.mangaread.org/manga/one-piece/ 1188
grabber/mangasorigines:
	go run . https://mangas-origines.fr/oeuvre/girl-crush/ 30-31
# its image CDN caches random origin 404s, so pages opt into the cache-busting retry
grabber/mangazin:
	go run . https://mangazin.org/manga/the-cold-blooded-warrior/ 75-76
grabber/manhuaplus:
	go run . https://manhuaplus.com/manga/tales-of-demons-and-gods01/ 522.6
grabber/silentquill:
	go run . https://silentquill.net/series/i-was-invited-to-join-the-country-as-an-otherworldly-warrior-but-i-refused-and-decided-to-start-as-a-soldier/ 59
grabber/decadencescans:
	go run . https://reader.decadencescans.com/manga/ashi-girl/ 1
grabber/mangalami:
	go run . https://mangalami.com/chainsaw-man-%e0%b8%95%e0%b8%ad%e0%b8%99%e0%b8%97%e0%b8%b5%e0%b9%88-202/ 200-201
grabber/deathtollscans:
	go run . https://reader.deathtollscans.net/series/kakushigoto/ 30
grabber/evascans:
	go run . https://evascans.net/series/about-things-that-havent-occurred-yet/ 13
grabber/furyosociety:
	go run . https://furyosociety.com/series/a-bout/ 3
# elftoon.com moved to elftoon.net (same RSC platform as witchtoons.net); the
# old domain still 301s /manga/{slug}/ and the grabber remaps old URLs in code
grabber/elftoon:
	go run . https://elftoon.net/series/comic/god-level-assassin-im-the-shadow 123
grabber/madaradex:
	go run . https://madaradex.org/title/boss-give-me-your-daughter-uncensored/ 70
grabber/madarascans:
	go run . https://madarascans.net/series/my-disciples-are-all-big-villains/ 481
grabber/maid:
	go run . https://www.maid.my.id/manga/watashi-no-oshi-wa-akuyaku-reijou/ 50
grabber/mangaberri:
	go run . https://mangaberri.com/title/dai-dark 37
grabber/kingofshojo:
	go run . https://kingofshojo.com/manga/although-i-was-a-condemned-witch-everyone-dotes-on-me-after-regressing/ 14
grabber/komikindo:
	go run . https://komikindo.ch/komik/155895-nano-machine/ 98
grabber/komikmanga:
	go run . https://komikmanga.com/manga/Unrivaled-Spear-Demon 55
grabber/komikstation:
	go run . https://komikstation.org/manga/boukun-wo-teki-ni-mawashita-akujo-desu-ga-bankaisa-sete-itadakimasu/ 10
grabber/mangavault:
	go run . https://mangavault.lat/manga/control-mental-f7d784 20
grabber/mangaworld:
	go run . https://www.mangaworld.mx/manga/1683/kingdom 400
grabber/lagoonscans:
	go run . https://lagoonscans.com/manga/crimson-reset/ 52
grabber/lelmanga:
	go run . https://www.lelmanga.com/manga/one-piece 1100
grabber/lelscanfr:
	go run . https://www.lelscanfr.com/manga/one-piece 700
grabber/mangasusuku:
	go run . https://mangasusuku.com/komik/get-out/ 10
grabber/rizzfables:
	go run . https://rizzfables.com/series/r2311170-an-s-rank-hunters-healing-journey-with-monsters 3
grabber/rokaricomics:
	go run . https://rokaricomics.com/manga/caregivers-alliance/ 5
# note: use a free chapter (older ones); recent chapters are often coin-gated
grabber/ritharscans:
	go run . https://ritharscans.com/series/0f32cefc-20a7-4337-aed0-fa78f832012f 19
grabber/sanascans:
	go run . https://sanascans.com/series/my-beloved-daughter-is-a-villainess 11
grabber/scanvf:
	go run . https://www.scan-vf.net/jujutsu-kaisen-modulo 10
# same CMS as scan-vf.net, with plain-number chapter URLs
grabber/anzmanga25:
	go run . https://www.anzmanga25.com/manga/1cfg7v-kagurabachi 100
grabber/onma:
	go run . https://onma.top/manga/black-clover 300
grabber/oremanga:
	go run . https://www.oremanga.net/series/blue-lock/ 300
grabber/thunderscans:
	go run . https://en-thunderscans.com/comics/full-time-awakening/ 108
grabber/violetscans:
	go run . https://violetscans.org/comics/after-all-ill-marry-you/ 21
# a free, early episode (episode_no 2 is Season 1 Ep. 1, 52 pages); the
# list is paginated, so this also walks the whole episode list
grabber/webtoons:
	go run . 'https://www.webtoons.com/en/fantasy/tower-of-god/list?title_no=95' 2
# former diamondfansub.com (which redirects here): Madara under /seri/, matched by
# the generic Tcb grabber
grabber/webtoonia:
	go run . https://webtoonia.fun/seri/ask-bagimlisi/ 5
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

# PlainHTML selector entries, no code: Madara, manhwaonline, Turkish and themesia markup
grabber/afroditscans:
	go run . https://afroditscans.com/manga/kont-un-gizli-hizmetcisi 1
grabber/hentai20:
	go run . https://hentai20.io/manga/stop-smoking/ 1
grabber/komikdewasa:
	go run . https://komikdewasa.art/komik/no-sensor-mertua-doyan-tubuh-menantu/ 1
grabber/mangadistrict:
	go run . https://mangadistrict.com/series/professors-alt-account/ 1
grabber/manhwabuddy:
	go run . https://manhwabuddy.com/manhwa/a-flood-of-stories-raw/ 1
grabber/nabicix:
	go run . https://www.nabicix.com/manga/mutlak-kilic-hissi/ 1
grabber/sektedoujin:
	go run . https://sektedoujin.cc/manga/secret-class/ 1
grabber/sereinscan:
	go run . https://sereinscan.com/manga/sinif-arkadasi/ 1

# runs the given smoke targets (names without "grabber/") and records their
# results in the README's "Last verified" column, e.g.
#   make site-status TARGETS="toonily toongod"
# A target passes when it exits 0 and tools/verify-cbz accepts the new archives
# (found with -print0, as their names hold spaces).
site-status:
	@test -n "$(TARGETS)" || { echo "usage: make site-status TARGETS=\"toonily toongod\""; exit 2; }
	@results=$$(mktemp); stamp=$$(mktemp); \
	for t in $(TARGETS); do \
		touch $$stamp; sleep 1; \
		if $(MAKE) grabber/$$t && find . -maxdepth 1 -name '*.cbz' -newer $$stamp -print0 | xargs -0 go run ./tools/verify-cbz; then \
			echo "$$t pass" >> $$results; \
		else \
			echo "$$t fail" >> $$results; \
		fi; \
		find . -maxdepth 1 -name '*.cbz' -newer $$stamp -delete; \
	done; \
	cat $$results; \
	go run ./tools/site-status -results $$results -date $$(date -u +%F); \
	rm -f $$results $$stamp
