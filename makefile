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
	@rm -fv ./manga-downloader* *.cbz *.cbr *.epub

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
# GUI_TAGS and GUI_LDFLAGS are what the wails targets pass to `wails build`,
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

# the wails CLI, which the release builds with: `make install/wails` installs
# this version into GOBIN, and WAILS can point at another binary
WAILS ?= wails
WAILS_VERSION = v2.16.0
# the arch of the windows and linux targets (the release builds amd64 and arm64)
GOARCH ?= $(shell go env GOARCH)
# X.Y.Z for the Windows version resource, the installer and the bundle's
# Info.plist, which wails reads from gui/wails.json. Empty leaves it alone.
PRODUCT_VERSION ?=
install/wails:
	go install github.com/wailsapp/wails/v2/cmd/wails@$(WAILS_VERSION)

# wails reads only gui/wails.json, so PRODUCT_VERSION is written there for the
# build, and a trap puts the file back on exit (Ctrl+C included)
WAILS_BUILD = cd gui && bak=$$(mktemp) && cp wails.json "$$bak" && trap 'cp "$$bak" wails.json; rm -f "$$bak"' 0 && trap 'exit 130' INT TERM && { \
	$(if $(PRODUCT_VERSION),sed 's/"productVersion": "0.0.0"/"productVersion": "$(PRODUCT_VERSION)"/' "$$bak" > wails.json &&) \
	$(WAILS) build -platform $(WAILS_PLATFORM) -tags "$(GUI_TAGS)" -trimpath -skipbindings -m -nosyncgomod \
	$(WAILS_FLAGS) -ldflags "$(GUI_LDFLAGS)"; }

# the frontend is embedded into the binary, so it has to exist before go builds
build/gui/frontend:
	cd gui/frontend && pnpm install --frozen-lockfile && pnpm build

build/gui: build/gui/frontend
	cd gui && $(GUI_CGO) go build -trimpath -tags $(GUI_GO_TAGS) -ldflags="$(GUI_GO_LDFLAGS)" \
		-o ../manga-downloader-gui .

# the release's wails build (what release-gui.yaml runs), with the icon and
# manifest embedded. Output: gui/build/bin/ (the -installer.exe with NSIS=1)
# wails builds the frontend itself, as it does for the release
build/gui/windows: WAILS_PLATFORM = windows/$(GOARCH)
build/gui/windows: WAILS_FLAGS = $(if $(NSIS),-nsis)
build/gui/windows:
	$(WAILS_BUILD)

# gui/build/bin/manga-downloader-gui (WebKitGTK headers needed, see above)
build/gui/linux: WAILS_PLATFORM = linux/$(GOARCH)
build/gui/linux:
	$(WAILS_BUILD)

# gui/build/bin/Manga Downloader.app, universal (macOS only)
build/gui/darwin: WAILS_PLATFORM = darwin/universal
build/gui/darwin:
	$(WAILS_BUILD)

test:
ifdef RICHGO
	richgo test ${GOTAGS} -v ./...
else
	go test ${GOTAGS} -v ./...
endif

# gui/ is its own module, so ./... above never reaches it. The Go checks need
# the frontend built (it's embedded) and the webview's headers, as build/gui does.
# gui-test.yaml runs this target, so it's the whole GUI check, -race included
test/gui: build/gui/frontend
	cd gui && $(GUI_CGO) go vet -tags $(GUI_GO_TAGS) ./...
	cd gui && $(GUI_CGO) go test -race -tags $(GUI_GO_TAGS) ./...
	cd gui/frontend && pnpm test

# the website (website/, Vite + TS): its dependencies come from the lock file
install/website:
	cd website && pnpm install --frozen-lockfile

build/website: install/website
	cd website && pnpm build

test/website: install/website
	cd website && pnpm test

# the tests, then the build, with one install: what website-test.yaml and pages.yaml run
check/website: install/website
	cd website && pnpm test && pnpm build

# the demo build's fake grabber is only built with the demo tag (see demo/README.md)
test/demo:
	go test -tags $(BUILD_TAGS),demo -count=1 ./demo/

# checks that the archives hold images, not just exist: the smoke jobs run it on
# each target's CBZs. FILES picks others, e.g. make verify-cbz FILES="a.cbz b.cbz"
verify-cbz:
	go run ./tools/verify-cbz $(or $(FILES),*.cbz)

# one dependency per line, alphabetically sorted (grabber/html deliberately
# last), so parallel site-support branches never edit the same line
grabber: \
	grabber/1punchman \
	grabber/3asq \
	grabber/akanebanashimanga \
	grabber/allporncomic \
	grabber/anikiga \
	grabber/animeallstar1 \
	grabber/animeallstar30 \
	grabber/animexnovel \
	grabber/apcomics \
	grabber/apenasumafa \
	grabber/apothecary \
	grabber/arbxcomix \
	grabber/arhentai \
	grabber/arlas \
	grabber/arthurscan \
	grabber/asacomi \
	grabber/atsumaru \
	grabber/aurorascans \
	grabber/azorafly \
	grabber/baozimh \
	grabber/berserkkmanga \
	grabber/beyondtheataraxia \
	grabber/bibibicomic \
	grabber/bigcomics \
	grabber/bigsolo \
	grabber/blackcloveronline \
	grabber/bluelockmanga \
	grabber/bluesolo \
	grabber/bokunoheroacademia \
	grabber/booklista \
	grabber/boruto2bluevortex \
	grabber/borutotwobluevortex \
	grabber/bunmanga \
	grabber/cafecomyaoi \
	grabber/chainsawman \
	grabber/chainsawmannmanga \
	grabber/championcross \
	grabber/cocomic \
	grabber/coffeemanga \
	grabber/comici \
	grabber/comicroombase \
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
	grabber/comirela \
	grabber/comix \
	grabber/cucumbermanga \
	grabber/danke \
	grabber/datgarscanlation \
	grabber/demonslayermanga \
	grabber/divascans \
	grabber/doujinxh \
	grabber/dragonballmangaa \
	grabber/dragonballsupermanga \
	grabber/dragontranslation \
	grabber/drakecomic \
	grabber/duoscanlators \
	grabber/elftoon \
	grabber/euphoriascan \
	grabber/fanfox \
	grabber/feelweb \
	grabber/flamecomics \
	grabber/fmteam \
	grabber/galaxscanlator \
	grabber/gcomi \
	grabber/gakamangas \
	grabber/gdscans \
	grabber/genztoon \
	grabber/globalcomix \
	grabber/grabberzone \
	grabber/guya \
	grabber/hachirumi \
	grabber/hanamiheaven \
	grabber/hanayume \
	grabber/hanmokkuscan \
	grabber/harmonyscan \
	grabber/hayacomic \
	grabber/heavenlydelusion \
	grabber/heavenmanga \
	grabber/heckscans \
	grabber/hentaiscantrad \
	grabber/hentaiteca \
	grabber/hentaixcomic \
	grabber/hentaixdickgirl \
	grabber/hentaixyuri \
	grabber/hhentai \
	grabber/hijala \
	grabber/hivetoons \
	grabber/houseofotakusv2 \
	grabber/hunlightcomics \
	grabber/ichicomi \
	grabber/ichithewitch \
	grabber/infrafandub \
	grabber/inkapk \
	grabber/inmanga \
	grabber/inventariooculto \
	grabber/jnbooks \
	grabber/jestful \
	grabber/jojolands \
	grabber/jujutsukaisenmanga \
	grabber/jujutsukaisenonline \
	grabber/kaijimanga \
	grabber/kaijuno8chapters \
	grabber/kaijunochapter \
	grabber/kaliscan \
	grabber/kamisama \
	grabber/kansai \
	grabber/kaynscan \
	grabber/kazokuden \
	grabber/kimicomi \
	grabber/klikmanga \
	grabber/kokomangas \
	grabber/ksgroupscans \
	grabber/kuragebunch \
	grabber/lectordarkroomfansub \
	grabber/lectorhadeslatamtoon \
	grabber/leercapitulo \
	grabber/lelscans \
	grabber/lepoytl \
	grabber/ler999 \
	grabber/lilymanga \
	grabber/lonertl \
	grabber/luascans \
	grabber/magcomi \
	grabber/magustoon \
	grabber/mahoushoujobu \
	grabber/mangaailand \
	grabber/mangabang \
	grabber/mangacrazy \
	grabber/mangaparktv \
	grabber/mangazegra \
	grabber/manga18free \
	grabber/mangaball \
	grabber/mangabats \
	grabber/mangabu \
	grabber/mangadenizi \
	grabber/mangadex \
	grabber/mangadelsiglotvmanga \
	grabber/mangafire \
	grabber/mangaforfree \
	grabber/mangaforfreenet \
	grabber/mangafree \
	grabber/mangahere \
	grabber/mangak \
	grabber/mangalib \
	grabber/mangalivre \
	grabber/mangalt \
	grabber/mangapark \
	grabber/manganato \
	grabber/mangaowl \
	grabber/mangaplus \
	grabber/mangasushi \
	grabber/mangataro \
	grabber/mangatimesquare \
	grabber/mangatown \
	grabber/mangaxhentai \
	grabber/mangitto \
	grabber/manhuahot \
	grabber/manhuanext \
	grabber/manhwaonlinees \
	grabber/manhwaclub \
	grabber/manhwanex \
	grabber/manhwareads \
	grabber/manhwatop \
	grabber/marmota \
	grabber/mgeko \
	grabber/mikrokosmosfb \
	grabber/mkissa \
	grabber/monopolymanhua \
	grabber/montetaiscanlator \
	grabber/mrtenzus \
	grabber/namicomic \
	grabber/naver \
	grabber/nebulosascan \
	grabber/ngamenkomik05 \
	grabber/nhaduong \
	grabber/nijitranslations \
	grabber/nikkanspa \
	grabber/ninjacomics \
	grabber/nyxscans \
	grabber/octopusmanga \
	grabber/omniscientreadersviewpoint \
	grabber/omniscientreadersviewpointt \
	grabber/omniscientreaderviewpoint \
	grabber/onepunchmanmanga \
	grabber/onepunchmannmanga \
	grabber/onlinechainsawmanmanga \
	grabber/orvmanhwa \
	grabber/ourfeel \
	grabber/paradisebl \
	grabber/piacomic \
	grabber/plumacomics \
	grabber/projectsuki \
	grabber/punchonemanga \
	grabber/qimanga \
	grabber/ragnarokscanlation \
	grabber/rawdex \
	grabber/re-yume \
	grabber/readkagura \
	grabber/readspyxfamilymanga \
	grabber/rimacomiplus \
	grabber/roliascan \
	grabber/ruahapchanhday2 \
	grabber/ruridragon \
	grabber/sacachispa \
	grabber/samuraij5z \
	grabber/seraphicdeviltry \
	grabber/shadowceviri \
	grabber/shinobiscans \
	grabber/shiyurasub \
	grabber/shonenjumpplus \
	grabber/snscoeurturkey \
	grabber/solooscan \
	grabber/spyxfamilymanga \
	grabber/stonescape \
	grabber/sundaywebry \
	grabber/taiyo \
	grabber/takecomic \
	grabber/tankouhentai \
	grabber/tarotscans \
	grabber/tcb \
	grabber/templetoons \
	grabber/thebeginningaftertheendmanga \
	grabber/thebluelock \
	grabber/thetowerofgod \
	grabber/tokyorevengers \
	grabber/tonarinoyj \
	grabber/topcomicporno \
	grabber/tortugaceviri \
	grabber/trmangaoku \
	grabber/truyentranhdammyy \
	grabber/turkcemanga \
	grabber/tvymanga3 \
	grabber/tritinia \
	grabber/utoon \
	grabber/valirscans \
	grabber/vinlandsaga \
	grabber/vortexscans \
	grabber/weebcentral \
	grabber/webtoonia \
	grabber/webtoons \
	grabber/witchtoons \
	grabber/wolfscanbr \
	grabber/xsanomanga \
	grabber/yaoihub \
	grabber/yaoiscan \
	grabber/yokaiteam \
	grabber/younganimal \
	grabber/youngchampion \
	grabber/yurimoonsub \
	grabber/yurionline \
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
# grabber/browser group, but kept out of the CI smoke rotation (SMOKE_EXCLUDE)
# like every other browser grabber. Uses an old chapter (stable, not
# premium-gated like newest).
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

# azorafly is another TanstackComic site: the same chapter list sits in the
# seroval data, so a chapter well behind the newest ones proves it's read.
grabber/azorafly:
	go run . https://azorafly.com/series/a-ghostly-song 20

grabber/hijala:
	go run . https://en-hijala.com/series/double-click 252
grabber/mangataro:
	go run . https://mangataro.org/manga/one-piece 1188
# plain HTTP, one HTML document per reader page; damn_necromancer is a long strip
grabber/mangatown:
	go run . https://www.mangatown.com/manga/koi_wa_amaagari_no_you_ni/ 10
	go run . https://www.mangatown.com/manga/damn_necromancer/ 1
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
	go run . https://comic-y-ours.com/episode/12207421983368565943 1
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
# uses a real (headless) browser: the site is behind a cloudflare challenge
# that plain HTTP gets a 403 on; the series' payload is in the rendered page
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
	grabber/teamshadowi \
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
	grabber/108read \
	grabber/4porncomic \
	grabber/adonisfansub \
	grabber/afroditscans \
	grabber/akazascans \
	grabber/amangaplanet \
	grabber/anzmanga25 \
	grabber/apollcomics \
	grabber/archiviumbar \
	grabber/arcurafansub \
	grabber/arenascan \
	grabber/artlapsa \
	grabber/asialotuss \
	grabber/asterscans \
	grabber/asura \
	grabber/athreascans \
	grabber/auntmanhwa \
	grabber/bacakomik \
	grabber/bananascans \
	grabber/blmangafree \
	grabber/borutoexplorer \
	grabber/bymichiby \
	grabber/cat300 \
	grabber/csmmangafree \
	grabber/dandadanmanga \
	grabber/dandadanmangas \
	grabber/decadencescans \
	grabber/deathtollscans \
	grabber/demonicscans \
	grabber/dojing \
	grabber/doodmanga \
	grabber/doujindistrict \
	grabber/doujinlc \
	grabber/dynastyscans \
	grabber/ecchidoujin \
	grabber/eldermanga \
	grabber/erisscans \
	grabber/eskimangalar \
	grabber/evascans \
	grabber/fbsquadx \
	grabber/flashmanga \
	grabber/furyosociety \
	grabber/gachiakutas \
	grabber/gaiatoon \
	grabber/ghostoon \
	grabber/goddoujin \
	grabber/godhman \
	grabber/grimscans \
	grabber/gutsberserk \
	grabber/hentai1 \
	grabber/hentai20 \
	grabber/hentaidesi \
	grabber/hentaiorigines \
	grabber/hentaivn \
	grabber/hotcabaretscan \
	grabber/inumanga \
	grabber/izanamiscans \
	grabber/kanzenin \
	grabber/kewnscans \
	grabber/kingofshojo \
	grabber/kiryuu \
	grabber/kitsunedawn \
	grabber/kombatch \
	grabber/komikdewasa \
	grabber/komikindo \
	grabber/komikindoshop \
	grabber/komikmanga \
	grabber/komikmu \
	grabber/komikstation \
	grabber/komiku \
	grabber/gourmetsupremacy \
	grabber/kumapoi \
	grabber/lagoonscans \
	grabber/lectortaurus \
	grabber/leermanhwas \
	grabber/legionscans \
	grabber/lelmanga \
	grabber/lelscanfr \
	grabber/limonmanga \
	grabber/linkmanga \
	grabber/madaradex \
	grabber/madarascans \
	grabber/maid \
	grabber/mangalc \
	grabber/mangaberri \
	grabber/mangadistrict \
	grabber/mangaeclipse \
	grabber/mangakatana \
	grabber/mangakimi \
	grabber/mangalami \
	grabber/mangamaniacs \
	grabber/mangapill \
	grabber/mangaread \
	grabber/mangasorigines \
	grabber/mangastep \
	grabber/mangasusuku \
	grabber/mangatellers \
	grabber/mangatrend \
	grabber/mangavault \
	grabber/mangaworld \
	grabber/mangawow \
	grabber/mangazin \
	grabber/manhuabug \
	grabber/manhuaplus \
	grabber/manhuathai \
	grabber/manhwabreakup \
	grabber/manhwabuddy \
	grabber/manhwadesu \
	grabber/manhwahub \
	grabber/manhwalist \
	grabber/manhwaonline \
	grabber/mhscans \
	grabber/mihentai \
	grabber/mindafansub \
	grabber/mistscans \
	grabber/mundomanhwa \
	grabber/myreadingcomic \
	grabber/nabicix \
	grabber/narasininja \
	grabber/natsuone \
	grabber/nekoproject \
	grabber/ngomik \
	grabber/nirvanamanga \
	grabber/nocfsb \
	grabber/noranofansub \
	grabber/noromax02 \
	grabber/novelcrow \
	grabber/noxsubs \
	grabber/ntrmanga \
	grabber/nyanukafe \
	grabber/nyraxmanga \
	grabber/onma \
	grabber/oremanga \
	grabber/orionscans \
	grabber/pornhwaz \
	grabber/rackusreads \
	grabber/ravenscans \
	grabber/rawkuma \
	grabber/readyagami \
	grabber/reapertrans \
	grabber/richtoon \
	grabber/ritharscans \
	grabber/rizzfables \
	grabber/rokaricomics \
	grabber/rosyscans \
	grabber/ruyamanga2 \
	grabber/s2read \
	grabber/sakamotodays \
	grabber/sanascans \
	grabber/sasangeyou \
	grabber/scanvf \
	grabber/scythescans \
	grabber/seinagi \
	grabber/sektedoujin \
	grabber/sereinscan \
	grabber/shijiescans \
	grabber/siikomik \
	grabber/silentquill \
	grabber/singmanga \
	grabber/slowmanga \
	grabber/smuglo \
	grabber/speedmanga \
	grabber/springtoons \
	grabber/sxfmanga \
	grabber/tanukimanga \
	grabber/tcbscans \
	grabber/tenshimanga \
	grabber/thaimanga \
	grabber/thunderscans \
	grabber/tilkiscans \
	grabber/timelesstoons \
	grabber/tmomanhwa \
	grabber/toomtammanga \
	grabber/toonhunter \
	grabber/trmanga \
	grabber/tsundoku \
	grabber/uchuujinmangas \
	grabber/undertranslations \
	grabber/utsukushiibg \
	grabber/uzaymanga \
	grabber/vermanhwa \
	grabber/violetscans \
	grabber/visorotaku \
	grabber/writerscans \
	grabber/sodsaime \
	grabber/yakshacomics \
	grabber/yaoiflix \
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

# no code: recognised by their markup (Comici, TanStack, RSC comic, Toivo Lite, Guya, tcb)
grabber/allporncomic:
	go run . https://allporncomic.io/manga/living-with-a-milf/ 63
grabber/apcomics:
	go run . https://apcomics.org/manga/zillionaire-glazed-biscuits-teen-titans/ 1
grabber/asacomi:
	go run . https://asacomi.jp/series/2c82a215ef38c 1
grabber/bibibicomic:
	go run . https://bibibi-comic.com/series/25d7fb9c93708 1
grabber/booklista:
	go run . https://studio.booklista.co.jp/series/311043b819c7e 1
grabber/bunmanga:
	go run . https://bunmanga.com/manga/thrive-in-catastrophe/ 76
grabber/cafecomyaoi:
	go run . https://cafecomyaoi.com.br/manga/amor-involuntario/ 24
grabber/cocomic:
	go run . https://cocomic.co/manga/the_ogre_s_bride/ 28
grabber/coffeemanga:
	go run . https://coffeemanga.net/manga/under-the-oak-tree/ 74
grabber/comicroombase:
	go run . https://comic-room-base.com/series/04ba145f9256c 1
grabber/comirela:
	go run . https://comirela.com/series/0cafb95cfbd4e 1
grabber/cucumbermanga:
	go run . https://cucumbermanga.com/manga/dog-and-bird-cucumber/ 49
grabber/divascans:
	go run . https://divascans.org/series/comic/a-naughty-girl 29
grabber/dragontranslation:
	go run . https://dragontranslation.org/manga/plan-de-intercambio-de-madres/ 48
grabber/euphoriascan:
	go run . https://euphoriascan.com/manga/lights-camera-action/ 9
grabber/gcomi:
	go run . https://g-comi.jp/series/13dcf7ad305b6 1
grabber/gakamangas:
	go run . https://gakamangas.com/manga/the-anime-geniuses-meet/ 15
grabber/hanamiheaven:
	go run . https://hanamiheaven.org/manga/210/ 52
grabber/hanayume:
	go run . https://hanayume.com/series/18c2f6615601e 1
grabber/harmonyscan:
	go run . https://harmony-scan.fr/manga/shijou-saikyou-no-mahou-kenshi-f-rank-boukensha-ni-tensei-suru/ 31
grabber/hayacomic:
	go run . https://hayacomic.jp/series/5f62ae8f75f3b 0
grabber/hentaiscantrad:
	go run . https://hentai-scantrad.org/manga/asmodeck-jeu-du-desir/ 18
grabber/hentaiteca:
	go run . https://hentaiteca.online/manga/o-prontuario-do-climax/ 3
grabber/hentaixcomic:
	go run . https://hentaixcomic.com/manga/xmas-girls/ 0
grabber/hentaixdickgirl:
	go run . https://hentaixdickgirl.com/manga/toaru-spy-no-bunretsu-nichijou/ 0
grabber/hentaixyuri:
	go run . https://hentaixyuri.com/manga/yami-koi/ 0
grabber/hhentai:
	go run . https://hhentai.fr/manga/un-livre-sur-mon-eleve-qui-ma-poussee-chez-moi/ 0
grabber/houseofotakusv2:
	go run . https://houseofotakusv2.xyz/manga/una-vida-de-aventurero-en-la-segunda-vuelta-despues-de-derrotar-al-jefe-final/ 4
grabber/infrafandub:
	go run . https://infrafandub.com/manga/sistema-de-dragon/ 93
grabber/inventariooculto:
	go run . https://inventariooculto.com/manga/underdoctor/ 15
grabber/jnbooks:
	go run . https://comic.j-nbooks.jp/series/019d1ffb5551d 1
grabber/kaijimanga:
	go run . https://w9.kaijimanga.com/ 79
grabber/kamisama:
	go run . https://leitor.kamisama.com.br/manga/toccio-the-angel/ 0
grabber/kazokuden:
	go run . https://www.kazokuden.com/manga/yuusha-sama-wa-houshuu-ni-hitozuma-wo-gokibou-desu/ 14
grabber/kimicomi:
	go run . https://kimicomi.com/series/19a3246898734 1
grabber/klikmanga:
	go run . https://klikmanga.org/manga/sensen-senki/ 1
grabber/kokomangas:
	go run . https://kokomangas.com/manga/my-sons-ex-girlfriend-became-my-subordinate/ 2
grabber/magustoon:
	go run . https://magustoon.org/series/bitten-by-the-dog-i-abandoned 64
grabber/mahoushoujobu:
	go run . https://mahoushoujobu.com/read/manga/Magical-Trans/ 20
grabber/mangabang:
	go run . https://comics.manga-bang.com/series/010217927fd53 1
grabber/mangazegra:
	go run . https://manga-zegra.com/series/005fce4c07385 1
grabber/manga18free:
	go run . https://manga18free.com/manga/soeun/ 60
grabber/mangabu:
	go run . https://mangabu.jp/series/0045d3636febb 1
grabber/mangaforfree:
	go run . https://mangaforfree.com/manga/soeun/ 64
grabber/mangaforfreenet:
	go run . https://mangaforfree.net/manga/secret-class-a18/ 187
grabber/mangafree:
	go run . https://mangafree.info/manga/tears-on-a-withered-flower/ 67
grabber/mangalt:
	go run . https://mangalt.jp/series/1c4b716a99389 1
grabber/mangaxhentai:
	go run . https://mangaxhentai.com/manga/the-intentions-of-the-neighborhood-meeting/ 12
grabber/manhuahot:
	go run . https://manhuahot.com/manga/you-own-my-all-20/ 77
grabber/manhuanext:
	go run . https://manhuanext.com/manga/the-ultimate-prodigy/ 8
grabber/manhwaonlinees:
	go run . https://manhwa-online.com/manga/dear-00-3/ 16
grabber/manhwaclub:
	go run . https://manhwaclub.net/manga/soeun-04/ 76
grabber/manhwanex:
	go run . https://manhwanex.com/manga/top-tier-providence-secretly-cultivate-for-a-thousand-years/ 278
grabber/manhwareads:
	go run . https://manhwareads.com/manga/the-young-wife/ 15
grabber/monopolymanhua:
	go run . https://monopolymanhua.com/manga/yo-solo-derribare-este-mundo-en-caos/ 3
grabber/montetaiscanlator:
	go run . https://montetaiscanlator.xyz/manga/o-recluso-supremo/ 51
grabber/namicomic:
	go run . https://namicomic.jp/series/0122c59e09925 1
grabber/nebulosascan:
	go run . https://nebulosascan.com/manga/xadrez-azul/ 9
grabber/nhaduong:
	go run . https://nhaduong.com/manga/nhung-ten-ngoc/ 5
grabber/nikkanspa:
	go run . https://mangaspa.nikkan-spa.jp/series/0c3f3089850ed 1
grabber/ninjacomics:
	go run . https://ninjacomics.xyz/manga/o-antigo-soberano-da-eternidade/ 286
grabber/nyxscans:
	go run . https://nyxscans.com/series/the-princess-kidnapped-to-a-land-without-women 11
grabber/octopusmanga:
	go run . https://octopusmanga.com/manga/jinx-uncensor-38d2ed-octou/ 61.5
grabber/paradisebl:
	go run . https://paradise-bl.com/manga/under-the-green-light/ 39
grabber/piacomic:
	go run . https://piacomic.jp/series/029fcd22bc9fd 1
grabber/rawdex:
	go run . https://rawdex.net/manga/ts-punishment/ 5
grabber/ruridragon:
	go run . https://w2.ruridragon.com/ 24
grabber/seraphicdeviltry:
	go run . https://seraphic-deviltry.com/manga/sasakure-chronicle/ 2
grabber/shinobiscans:
	go run . https://shinobiscans.com/manga/resurrection-of-the-catastrophic-hero/ 9
grabber/tankouhentai:
	go run . https://tankouhentai.com/manga/tomodachi-no-haha-wa-boku-no-mono/ 3
grabber/tarotscans:
	go run . https://www.tarotscans.com/manga/yakudatazu-skill-ni-jinsei-o-sosogikomi-25-nen-imasara-saikyou-no-boukentan-midori-kashi-no-akira/ 27
grabber/topcomicporno:
	go run . https://topcomicporno.com/manga/viviendo-en-un-cuarto-para-coger/ 14
grabber/trmangaoku:
	go run . https://trmangaoku.com/manga/takimyildizlari-benim-ogrencilerimdi/ 36
grabber/truyentranhdammyy:
	go run . https://truyentranhdammyy.site/manga/yeu-mot-ke-ngoc/ 83
grabber/valirscans:
	go run . https://valirscans.org/series/comic/i-saved-my-favorite-from-exile-but-now-i-cant-escape-his-love 8
grabber/yaoihub:
	go run . https://yaoihub.org/manga/waterside-night/ 58
grabber/youngchampion:
	go run . https://youngchampion.jp/series/0cdd7013b23d7 1
grabber/yurionline:
	go run . https://yuri-online.com/manga/weve-been-reincarnated-theres-no-way-im-giving-you-my-mana-again/ 6

# no code: PlainHTML selector entries (themesia, Madara, Keyoapp, Astro markup)
grabber/108read:
	go run . https://www.108read.com/manga/driver-in-the-new-city/ 25
grabber/akazascans:
	go run . https://akazascans.org/manga/shadow-slave/ 6
grabber/amangaplanet:
	go run . https://www.amangaplanet.com.tr/manga/dunyada-kotu-leydi-yoktur/ 15
grabber/archiviumbar:
	go run . https://archiviumbar.com/manga/el-rey-demonio-se-retira/ 16
grabber/artlapsa:
	go run . https://artlapsa.com/series/0070c20a-6954-4c39-bfde-7df67deff2a7 19.5
grabber/asialotuss:
	go run . https://asialotuss.com/manga/el-cazador-quiere-vivir-tranquilamente 36
grabber/asterscans:
	go run . https://asterscans.com/manga/the-unfilial-son-who-will-kill-me-is-seducing-me/ 12
grabber/athreascans:
	go run . https://athreascans.com/manga/the-queens-last-tea-time/ 4
grabber/borutoexplorer:
	go run . https://leitor.borutoexplorer.com.br/manga/tartarugas-ninjas-x-naruto/ 2
grabber/bymichiby:
	go run . https://bymichiby.com/manga/hattori-sensei-no-quiere-tener-novia/ 5
grabber/cat300:
	go run . https://cat-300.com/manga/%e0%b9%82%e0%b8%a3%e0%b8%84%e0%b8%aa%e0%b8%a5%e0%b8%b1%e0%b8%9a%e0%b9%80%e0%b8%9e%e0%b8%a8-shiki-takuto-ts-revolution-ch-1-comic-mugen-tensei-2020-07/ 1
grabber/dojing:
	go run . https://dojing.net/manga/senpai-yang-imut-tidak-bisa-menolakku/ 1
grabber/doodmanga:
	go run . https://www.doodmanga.com/manga/yuusha-party-o-oida-sareta-kiyou-binbou/ 16.3
grabber/doujindistrict:
	go run . https://doujindistrict.com/read/yukemuri-nyannyan-jiken/ 1
grabber/erisscans:
	go run . https://erisscans.com/series/65e09cfaf12/ 2
grabber/flashmanga:
	go run . https://www.flash-manga.net/manga/all-football-talents-are-mine/ 53
grabber/gaiatoon:
	go run . https://gaiatoon.com/manga/i-will-become-the-villains-poison-taster/ 40
grabber/ghostoon:
	go run . https://ghostoon.com/manga/under-the-oak-tree/ 39
grabber/goddoujin:
	go run . https://god-doujin.com/manga/wireless-onahole/ 57
grabber/godhman:
	go run . https://www.godhman.net/manga/taming-an-evil-young-lady/ 15
grabber/grimscans:
	go run . https://grimscans.com/series/turn-based-wizard/ 15
grabber/hentai1:
	go run . https://hentai1.io/manga/who-cares-if-its-a-lie/ 14
grabber/hentaidesi:
	go run . https://hentaidesi.com/manga/teach-me-first-uncensored-88/ 10
grabber/hotcabaretscan:
	go run . https://hotcabaretscan.com/manga/wild-lilies-and-violets/ 16
grabber/inumanga:
	go run . https://www.inu-manga.net/manga/in-this-life-i-will-be-the-lord/ 107
grabber/izanamiscans:
	go run . https://izanamiscans.my.id/manga/it-all-starts-with-trillions-of-nether-currency/ 138
grabber/kanzenin:
	go run . https://kanzenin.info/manga/dakaretagari-no-tsuma-onna-tachi/ 12
grabber/kewnscans:
	go run . https://kewnscans.org/series/bitter-side-cranberries/ 8
grabber/kitsunedawn:
	go run . https://kitsunedawn.com/series/51df2977-a425-46ea-b5b5-785c4f1c9509 9
grabber/komikmu:
	go run . https://komikmu.top/komik/komikus-fasik/ 6
grabber/komiku:
	go run . https://komiku.org/manga/albus-changes-the-world/ 103
grabber/lectortaurus:
	go run . https://lectortaurus.com/manga/regresion-del-tanque-clase-sss/ 4
grabber/linkmanga:
	go run . https://linkmanga.com/manga/ultimate-shut-in/ 50
grabber/mangalc:
	go run . https://manga-lc.net/manga/yandere-ka-to-omottara-motto-yabe-onna-datta/ 13
grabber/mangakimi:
	go run . https://www.mangakimi.com/manga/whats-wrong-with-being-the-villainess/ 44
grabber/mangastep:
	go run . https://mangastep.com/manga/mato-seihei-no-slave/ 93
grabber/mangatrend:
	go run . https://mangatrend.org/manga/wistoria-wand-and-sword-manga/ 63
grabber/manhuabug:
	go run . https://www.manhuabug.com/manga/the-legend-god-of-war-in-the-city/ 117
grabber/manhwabreakup:
	go run . https://www.manhwabreakup.com/manga/you-flirted-with-me-now-you-want-to-run/ 1
grabber/manhwadesu:
	go run . https://manhwadesu.wiki/komik/wireless-onahole/ 62
grabber/manhwahub:
	go run . https://manhwahub.to/manhwa/someone-stop-her/ 58
grabber/manhwalist:
	go run . https://m1.manhwalist.my.id/manga/lookism/ 623
grabber/mihentai:
	go run . https://mihentai.net/manga/wear-student-underwear-and-apologize/ 1
grabber/mindafansub:
	go run . https://mindafansub.dev/manga/waterside-night-45/ 59
grabber/nekoproject:
	go run . https://nekoproject.org/manga/daija-ni-totsuida-musume/ 21
grabber/ngomik:
	go run . https://02.ngomik.cc/manga/magic-emperor/ 462
grabber/nirvanamanga:
	go run . https://nirvanamanga.com/manga/operation-true-love/ 54
grabber/nocfsb:
	go run . https://nocfsb.com/manga/zona-0-restart/ 34
grabber/noromax02:
	go run . https://noromax02.my.id/manga/scoring-the-sacred-body-of-the-ancients-from-the-get-go-bahasa-indonesia/ 88
grabber/ntrmanga:
	go run . https://www.ntr-manga.net/manga/set-up/ 59
grabber/nyanukafe:
	go run . https://nyanukafe.com/series/63f559b92c0/ 10
grabber/orionscans:
	go run . https://orion-scans.com/series/an-adventurer-skilled-at-nurturing-finds-a-little-girl-and-devotes-his-second-life-to-raising-her 9
grabber/pornhwaz:
	go run . https://www.pornhwaz.com/webtoon/secret-class/ 160
grabber/reapertrans:
	go run . https://reapertrans.com/manga/revenge-of-the-iron-blooded-sword-hound/ 79
grabber/richtoon:
	go run . https://r1.richtoon.top/manga/renacer-hacia-la-venganza/ 22
grabber/rosyscans:
	go run . https://rosyscans.com/manga/serena/ 78
grabber/s2read:
	go run . https://s2read.com/manga/who-made-me-a-princess/ 68
grabber/sasangeyou:
	go run . https://sasangeyou.net/manga/ikura-de-yaremasuka-original/ 1
grabber/shijiescans:
	go run . https://shijiescans.com/seri/solo-leveling/ 100
grabber/siikomik:
	go run . https://siikomik.id/komik/reverse-try/ 1
grabber/singmanga:
	go run . https://www.sing-manga.com/manga/your-throne/ 129
grabber/slowmanga:
	go run . https://www.slow-manga.net/manga/i-took-over-the-academy-with-a-single-sashimi-knife/ 46
grabber/speedmanga:
	go run . https://speed-manga.net/manga/the-eternal-supreme/ 257
grabber/tanukimanga:
	go run . https://www.tanuki-manga.net/manga/magic-emperor/ 459
grabber/tilkiscans:
	go run . https://www.tilkiscans.com/seri/unlu-bir-ailenin-kilic-dahisi-oldum/ 70
grabber/timelesstoons:
	go run . https://timelesstoons.org/series/i-got-married-and-gained-cute-stepsons-but-ive-been-ordered-to-kill-my-husband/ 1
grabber/toomtammanga:
	go run . https://toomtam-manga.com/manga/latna-saga-survival-of-a-sword-king/ 125
grabber/toonhunter:
	go run . https://toonhunter.com/manga/sex-stopwatch/ 77
grabber/tsundoku:
	go run . https://tsundoku.com.br/manga/um-ultimo-hurrah-os-herois-grisalhos-exploram-um-futuro-vivido/ 4
grabber/uchuujinmangas:
	go run . https://uchuujinmangas.com/manga/como-criar-villanos-correctamente/ 29
grabber/undertranslations:
	go run . https://undertranslations.com/manga/las-heroinas-de-un-galgame-que-evitaron-un-final-mortal-leyeron-mi-diario-y-descubrieron-mi-secreto/ 1
grabber/vermanhwa:
	go run . https://vermanhwa.com/manga/tocame-abajo-sin-censura/ 8
grabber/sodsaime:
	go run . https://www.xn--l3c0azab5a2gta.com/manga/magic-emperor/ 462
grabber/yakshacomics:
	go run . https://yakshacomics.com/manga/the-ultimate-wantless-godly-rich-system/ 73
grabber/yaoiflix:
	go run . https://yaoiflix.fit/manga/shut-line-19/ 58

# no code: recognised by their markup (tcb, Toivo Lite, WordPressCategory, Blogger)
grabber/akanebanashimanga:
	go run . https://akanebanashimanga.website/ 113

grabber/animexnovel:
	go run . https://www.animexnovel.com/manga/botsuraku-yotei-nanode-kajishokunin-wo-mezasu/ 15

grabber/arbxcomix:
	go run . https://arbxcomix.com/manga/%d8%af%d8%b1%d9%88%d8%b3-%d8%a7%d9%84%d9%85%d8%b2%d8%b1%d8%b9%d8%a9/ 12

grabber/berserkkmanga:
	go run . https://berserkkmanga.com/berserk-manga-chapter-386/ 193

grabber/beyondtheataraxia:
	go run . https://www.beyondtheataraxia.com/manga/witchriv/ 13

grabber/bluelockmanga:
	go run . https://w50.blue-lock-manga.com/ 186

grabber/boruto2bluevortex:
	go run . https://boruto2bluevortex.com/manga/boruto-two-blue-vortex-chapter-37/ 30

grabber/demonslayermanga:
	go run . https://demonslayermanga.us.com/demon-slayer-kimetsu-no-yaiba-manga-chapter-104/ 103

grabber/dragonballsupermanga:
	go run . https://dragonballsupermanga.com/dragon-ball-super-manga-chapter-54/ 54

grabber/galaxscanlator:
	go run . https://galaxscanlator.blogspot.com/2024/01/savage-hero.html 30

grabber/ichithewitch:
	go run . https://ichi-the-witch.com/ 33

grabber/kaijunochapter:
	go run . https://kaijunochapter.com/ 61

grabber/kaliscan:
	go run . https://kaliscan.org/manga/demon-slayer-kimetsu-no-yaiba-manga-in-colored/ 200

grabber/mangacrazy:
	go run . https://mangacrazy.net/manga/summer-vacation-uncensored/ 59

grabber/mrtenzus:
	go run . https://mrtenzus.com/manga/empregada-domestica/ 5

grabber/omniscientreadersviewpoint:
	go run . https://w2.omniscient-readers-viewpoint.online/ 164

grabber/omniscientreadersviewpointt:
	go run . https://omniscientreadersviewpointt.com/manga/omniscient-readers-viewpoint-chapter-162/ 156

grabber/onepunchmanmanga:
	go run . https://w27.onepunchman-manga.net/ 114

grabber/onepunchmannmanga:
	go run . https://onepunchmannmanga.com/one-punch-man-manga-chapter-107/ 107

grabber/onlinechainsawmanmanga:
	go run . https://onlinechainsawmanmanga.us/chainsaw-man-chapter-116/ 116

grabber/orvmanhwa:
	go run . https://orvmanhwa.online/omniscient-readers-viewpoint-manhwa-chapter-157/ 157

grabber/plumacomics:
	go run . https://plumacomics.com/manga/retorno-do-medico/ 75

grabber/punchonemanga:
	go run . https://w14.punchonemanga.com/ 121

grabber/readkagura:
	go run . https://readkagura.com/ 68

grabber/readspyxfamilymanga:
	go run . https://readspyxfamilymanga.com/manga/spy-x-family-chapter-68-1/ 68.1

grabber/thebeginningaftertheendmanga:
	go run . https://w22.thebeginningaftertheendmanga.com/manga/the-beginning-after-the-end-ch-185/ 114

grabber/thebluelock:
	go run . https://ww1.the-blue-lock.com/manga/blue-lock-chapter-213/ 182

grabber/vinlandsaga:
	go run . https://vinlandsaga.website/ 112

grabber/yaoiscan:
	go run . https://yaoiscan.org/manga/qvirk9krncwjss4/ 52


# no code: PlainHTML selector entries (themesia, Madara, manhwaonline, FoOlSlide, MangaReaderCMS and the rest)
grabber/4porncomic:
	go run . https://4porncomic.com/comic/wireless-onahole/ 63

grabber/arcurafansub:
	go run . https://arcurafansub.com/manga/a-ghostly-song/ 24

grabber/auntmanhwa:
	go run . https://auntmanhwa.com/manhwa/milk-my-strawberries-uncensored/ 30

grabber/bacakomik:
	go run . https://bacakomik.my/komik/883432-eleceed/ 212

grabber/bananascans:
	go run . https://bananascans.com/manga/one-hundred-storey-tower 3

grabber/doujinlc:
	go run . https://doujin-lc.net/doujin/%e0%b8%82%e0%b9%88%e0%b8%a1%e0%b8%82%e0%b8%b7%e0%b8%99%e0%b8%84%e0%b8%99%e0%b8%97%e0%b8%b5%e0%b9%88%e0%b8%a1%e0%b8%b2%e0%b9%81%e0%b8%a2%e0%b9%88%e0%b8%87%e0%b9%81%e0%b8%9f%e0%b8%99%e0%b8%9c%e0%b8%a1/ 3

grabber/ecchidoujin:
	go run . https://ecchi-doujin.com/doujin/soeun/ 57

grabber/hentaiorigines:
	go run . https://hentai-origines.com/manga/stop-smoking/ 57

grabber/hentaivn:
	go run . https://hentaivn.show/truyen-hentai/dong-ho-ngung-dong-thoi-gian/ 78

grabber/kiryuu:
	go run . https://v7.kiryuu.to/manga/martial-peak/ 1935

grabber/komikindoshop:
	go run . https://2.komikindo.shop/manga/mr-kims-general-shop/ 5

grabber/kumapoi:
	go run . https://kumapoi.info/manga/mom-eat-first/ 71

grabber/leermanhwas:
	go run . https://leermanhwas.com/manhwa/el-estudiante-de-rango-ss-de-la-super-academia-de-magia/ 19

grabber/legionscans:
	go run . https://legionscans.com/wp/manga/esclavo-feliz/ 41

grabber/mangamaniacs:
	go run . https://mangamaniacs.org/manga/kiss-me-if-you-can/ 31

grabber/mangatellers:
	go run . https://reader.mangatellers.gr/series/kingdoms-of-dreams/ 1

grabber/manhuathai:
	go run . https://www.manhuathai.com/manga/master-cultivation-return/ 100

grabber/mhscans:
	go run . https://mhscans.com/series/emperador-del-apocalipsis-con-mi-estandarte-convocare-a-todas-mis-discipulas/ 19

grabber/myreadingcomic:
	go run . https://myreadingcomic.com/comic/wireless-onahole/ 63

grabber/narasininja:
	go run . https://narasininja.net/komik/one-piece 611

grabber/novelcrow:
	go run . https://novelcrow.com/comic/forbidden-fruits-tal3s-of-debauchery/ 5

grabber/seinagi:
	go run . https://reader.seinagi.org.es/series/sakura_sakura_morishige/ 8

grabber/springtoons:
	go run . https://springtoons.com/comic-page/jinx-mingwa/ 53.1

grabber/tenshimanga:
	go run . https://tenshimanga.com/manga/namgung-ailesi-nin-genc-efendisi-duygusuz 26

grabber/thaimanga:
	go run . https://www.thaimanga.net/manga/isekai-de-slow-life-wo-ganbou/ 27

grabber/tmomanhwa:
	go run . https://tmomanhwa.com/manhwa/la-espada-del-chaman/ 42

grabber/utsukushiibg:
	go run . https://utsukushii-bg.com/manga/fairy-tail 138

# no code: recognised by their markup (tcb, Toivo Lite, Comici, Blogger, WordPressCategory)

grabber/1punchman:
	go run . https://1punchman.com/ 200

grabber/apenasumafa:
	go run . https://apenasuma-fa.blogspot.com/2024/06/eu-quero-ser-um-ator_15.html 140

grabber/arhentai:
	go run . https://arhentai.net/title/a-spell-that-makes-your-dick-bigger/ 1

grabber/arlas:
	go run . https://www.arlas.online/2025/05/sotsugyou-album-no-kanojotachi.html 12

grabber/arthurscan:
	go run . https://arthurscan.xyz/manga/lit-the-supreme-being/ 50

grabber/bokunoheroacademia:
	go run . https://bokunohero-academia.online/ 200

grabber/borutotwobluevortex:
	go run . https://w26.boruto-twobluevortex.com/ 20

grabber/comici:
	go run . https://comics.comici.jp/comicbro/series/d2268717a79a6 0

grabber/datgarscanlation:
	go run . https://datgarscanlation.blogspot.com/2022/04/tonikaku-kawaii.html 160

grabber/doujinxh:
	go run . https://doujinx-h.com/doujin/doraemon-bbm/ 1

grabber/duoscanlators:
	go run . https://duoscanlators.blogspot.com/2024/10/the-house-without-time.html 15

grabber/grabberzone:
	go run . https://grabber.zone/comics/sonic-bad-guys-espanol/ 3

grabber/hanmokkuscan:
	go run . https://hanmokkuscan.blogspot.com/2022/04/my-disciples-are-super-gods.html 60

grabber/heckscans:
	go run . https://heckscans.blogspot.com/2023/10/suki-koso-momo-no-jouzu-nare.html 12

grabber/hunlightcomics:
	go run . https://hunlightcomics.com/m/i-got-reincarnated-as-a-mid-boss-who-dies-at-the-start-of-a-game/ 19

grabber/inkapk:
	go run . https://inkapk.net/obras/the-female-delinquent-set-her-eyes-on-me/ 32

grabber/jujutsukaisenonline:
	go run . https://w28.jujutsukaisen-online.com/ 150

grabber/kaijuno8chapters:
	go run . https://kaijuno8chapters.com/ 13

grabber/lectordarkroomfansub:
	go run . https://lector-darkroomfansub.blogspot.com/2023/03/hosaka-san-to-miyoshi-kun.html 8

grabber/lectorhadeslatamtoon:
	go run . https://lectorhades.latamtoon.com/tmo/renacimiento-del-cultivador-urbano-inmortal/ 524

grabber/lepoytl:
	go run . https://www.lepoytl.my.id/2024/04/ogami-tsumiki-to-kinichijou.html 58

grabber/ler999:
	go run . https://ler999.blogspot.com/2023/04/albanara-of-gods.html 2

grabber/lonertl:
	go run . https://loner-tl.blogspot.com/2022/08/nigoru-hitomi-de-nani-wo-negau-highserk.html 4

grabber/mangaailand:
	go run . https://manga-ai-land.blogspot.com/2020/09/juujika-no-rokunin.html 60

grabber/mangaparktv:
	go run . https://mangapark.tv/manhwa/desharow-mermaid/ 81

grabber/marmota:
	go run . https://marmota.me/comic/ravage-2023/ 1

grabber/mikrokosmosfb:
	go run . https://mikrokosmosfb.blogspot.com/2022/07/night-song.html 20

grabber/ngamenkomik05:
	go run . https://ngamenkomik05.blogspot.com/2024/04/i-regressed-to-level-up-instead-of_22.html 50

grabber/omniscientreaderviewpoint:
	go run . https://w53.omniscientreaderviewpoint.com/ 300

grabber/ragnarokscanlation:
	go run . https://ragnarokscanlation.org/series/mokushiroku-no-yonkishi/ 255

grabber/rimacomiplus:
	go run . https://rimacomiplus.jp/digitalmargaret/series/d83eefaea85ab 1

grabber/ruahapchanhday2:
	go run . https://ruahapchanhday2.com/truyen/nhat-ky-suy-nghi-tieu-cuc/ 30

grabber/samuraij5z:
	go run . https://samurai.j5z.xyz/son/el-archimago-de-los-mil-elementos/ 591

grabber/shadowceviri:
	go run . https://shadowceviri.blogspot.com/2021/12/hajime-no-ippo-943.html 1441

grabber/shiyurasub:
	go run . https://shiyurasub.blogspot.com/2022/12/jojolion.html 50

grabber/snscoeurturkey:
	go run . https://snscoeurturkey.blogspot.com/2022/04/can-love-be-recycled.html 2

grabber/solooscan:
	go run . https://solooscan.blogspot.com/2024/03/extraindo-atributos.html 20

grabber/spyxfamilymanga:
	go run . https://spyxfamilymanga.com/spy-x-family-chapter-67-1/ 60

grabber/thetowerofgod:
	go run . https://w82.thetowerofgod.com/ 300

grabber/tokyorevengers:
	go run . https://w28.read-tokyorevengers.com/ 204

grabber/wolfscanbr:
	go run . https://wolfscanbr.blogspot.com/2023/10/minha-esposa-e-uma-cultivadora-celestial.html 6

grabber/xsanomanga:
	go run . https://www.xsano-manga.com/2026/08/217-be-blues.html 188

grabber/yokaiteam:
	go run . https://yokai-team.blogspot.com/2023/09/hunter-x-hunter-kurapika-tsuioku-hen.html 2

grabber/yurimoonsub:
	go run . https://yurimoonsub.blogspot.com/2026/03/murmur.html 10

# no code: PlainHTML selector entries (themesia, Madara, Next.js, FoOlSlide, mangavault)

grabber/blmangafree:
	go run . https://ww5.blmangafree.com/ 200

grabber/csmmangafree:
	go run . https://www.csmmangafree.com/ 60

grabber/dandadanmanga:
	go run . https://w2.dandadanmanga.net/ 246

grabber/dandadanmangas:
	go run . https://dandadanmangas.online/ 200

grabber/gachiakutas:
	go run . https://www2.gachiakutas.com/ 100

grabber/gutsberserk:
	go run . https://www.gutsberserk.com/ 384

grabber/noxsubs:
	go run . https://noxsubs.net/manga/murim-dunyasinin-psikopati/ 12

grabber/readyagami:
	go run . https://read.yagami.me/series/tower_of_god/ 150

grabber/sakamotodays:
	go run . https://sakamotodays.com.lv/ 200

grabber/scythescans:
	go run . https://scythescans.com/manga/the-ancient-sovereign-of-eternity/ 150

grabber/sxfmanga:
	go run . https://www.sxfmanga.net/ 140

grabber/visorotaku:
	go run . https://visorotaku.com/obra/al-diablo-la-ventana-de-estado-qk3x 20

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

# the rotation smoke.yaml runs twice a week: the SMOKE_COUNT targets verified
# longest ago, split round-robin over SHARDS (TARGETS="a b" overrides it).
# About a quarter of the list: every site comes round about every two weeks.
SMOKE_COUNT ?= 110
# runs on one day share a date, so each run skips the previous batch by SMOKE_COUNT
# (GITHUB_RUN_NUMBER on Actions: the shards of one run agree, the next run moves on)
SMOKE_OFFSET ?= $(GITHUB_RUN_NUMBER)
SHARD ?= 1
SHARDS ?= 1
SMOKE_RESULTS ?= smoke-results.txt

# targets CI can't run, so they stay out of the rotation (check them locally)
SMOKE_EXCLUDE =
# 403 from GitHub's runners, while residential connections work
SMOKE_EXCLUDE += mangaball mangalib silentquill bigsolo luascans hivetoons eldermanga eskimangalar limonmanga uzaymanga afroditscans mangasorigines dandadanmanga gutsberserk hanamiheaven
# Cloudflare-blocked, or rate-limited hard (429, or the app API)
SMOKE_EXCLUDE += mangitto lilymanga mangaplus
# 429, reset, 530 or a non-blog page from GitHub's runners, while they pass locally
SMOKE_EXCLUDE += ler999 lonertl mangaailand mangadelsiglotvmanga mangabats legionscans asialotuss
# headless-browser grabbers: a runner can't get past their challenges
SMOKE_EXCLUDE += comix mkissa templetoons mangafire

# every target of the grabber and grabber/html aggregates, read from this file
# so a new site is one line there, minus SMOKE_EXCLUDE
SMOKE_TARGETS = $(filter-out $(SMOKE_EXCLUDE) html,$(sort $(patsubst grabber/%,%,$(shell awk '/^grabber: \\/ || /^grabber\/html: \\/ {f = 1; next} f && /^\t/ {gsub(/[\\\t ]/, ""); print; next} {f = 0}' makefile))))

# GNU timeout bounds each target at SMOKE_LIMIT seconds; macOS has none, so its targets run unbounded
SMOKE_LIMIT ?= 300
SMOKE_TIMEOUT = $(if $(shell command -v timeout),timeout $(SMOKE_LIMIT))
# not $(MAKE) itself: a line naming it runs in full even under make -n
SMOKE_MAKE = $(MAKE)

# the targets to check, one per line, before the shard split
SMOKE_PICK = if [ -n "$(TARGETS)" ]; then printf '%s\n' $(TARGETS); \
	elif [ "$(SMOKE_COUNT)" = all ]; then printf '%s\n' $(SMOKE_TARGETS); \
	else printf '%s\n' $(SMOKE_TARGETS) | go run ./tools/site-status -oldest $(SMOKE_COUNT) -offset $$(( $(or $(SMOKE_OFFSET),0) * $(SMOKE_COUNT) )); fi
# this shard's round-robin share of the lines on its stdin
SMOKE_SHARD = awk -v n=$(SHARDS) -v s=$(SHARD) 'NF && (NR - 1) % n == s - 1'

smoke/list:
	@list=$$($(SMOKE_PICK)) || exit 1; \
	echo "$$list" | $(SMOKE_SHARD)

# runs each smoke/list target: grabber/<t> must leave CBZs that tools/verify-cbz accepts.
# Appends "<t> pass|fail" to SMOKE_RESULTS and exits 1 at the end if any failed.
# On GitHub Actions each target is a log group, a failure an ::error and a row of $GITHUB_STEP_SUMMARY.
smoke:
	@list=$$($(SMOKE_PICK)) || exit 1; \
	list=$$(echo "$$list" | $(SMOKE_SHARD)); \
	stamp=$$(mktemp); rc=$$(mktemp); fails=0; \
	[ -z "$$GITHUB_ACTIONS" ] || [ -z "$$GITHUB_STEP_SUMMARY" ] || printf '| site | result | reason |\n|---|---|---|\n' >> "$$GITHUB_STEP_SUMMARY"; \
	for t in $$list; do \
		touch $$stamp; sleep 1; \
		if [ -z "$$GITHUB_ACTIONS" ]; then \
			if $(SMOKE_TIMEOUT) $(SMOKE_MAKE) grabber/$$t && find . -maxdepth 1 -name '*.cbz' -newer $$stamp | grep -q . && find . -maxdepth 1 -name '*.cbz' -newer $$stamp -print0 | xargs -0 go run ./tools/verify-cbz; then r=pass; else r=fail; fi; \
		else \
			log=$$(mktemp); \
			echo "::group::$$t"; \
			{ $(SMOKE_TIMEOUT) $(SMOKE_MAKE) grabber/$$t; echo $$? > $$rc; \
				if [ "$$(cat $$rc)" = 0 ]; then \
					if find . -maxdepth 1 -name '*.cbz' -newer $$stamp | grep -q .; then \
						find . -maxdepth 1 -name '*.cbz' -newer $$stamp -print0 | xargs -0 go run ./tools/verify-cbz || echo 1 > $$rc; \
					else echo "no CBZ written"; echo 1 > $$rc; fi; \
				fi; } 2>&1 | tee $$log; \
			echo "::endgroup::"; \
			if [ "$$(cat $$rc)" = 0 ]; then r=pass; else r=fail; fi; \
			if [ $$r = pass ]; then row="✅ pass | |"; else \
				if [ "$$(cat $$rc)" = 124 ]; then reason="timed out after $(SMOKE_LIMIT)s"; else \
					reason=$$(grep -E -v '^make(\[[0-9]+\])?: (\*\*\* \[.*\] Error [0-9]+|(Entering|Leaving) directory)|^exit status [0-9]+$$|^OK |^[[:space:]]*$$' $$log | tail -n 1); \
				fi; \
				[ -n "$$reason" ] || reason="no output"; \
				printf '::error title=%s failed::%s\n' "$$t" "$$(printf '%s\n' "$$reason" | awk '{gsub(/%/, "%25"); gsub(/\r/, "%0D"); printf "%s", $$0}')"; \
				row="❌ fail | $$(printf '%s\n' "$$reason" | sed 's/|/\\|/g') |"; \
			fi; \
			[ -z "$$GITHUB_STEP_SUMMARY" ] || echo "| $$t | $$row" >> "$$GITHUB_STEP_SUMMARY"; \
			rm -f $$log; \
		fi; \
		[ $$r = pass ] || fails=$$((fails + 1)); \
		echo "$$t $$r" | tee -a $(SMOKE_RESULTS); \
		find . -maxdepth 1 -name '*.cbz' -newer $$stamp -delete; \
	done; \
	rm -f $$stamp $$rc; \
	exit $$((fails > 0))

# the overall result of a results file (smoke's SMOKE_RESULTS): the failing sites, then the count.
# Written to $GITHUB_STEP_SUMMARY when set, else stdout: make smoke/summary RESULTS=results.all
smoke/summary:
	@test -n "$(RESULTS)" || { echo "usage: make smoke/summary RESULTS=results.all"; exit 2; }
	@{ echo "## Site smoke tests"; echo; \
	awk '$$2 == "fail" {print "- ❌ " $$1; f++} $$2 == "pass" {p++} END {if (f) print ""; print "✅ " p + 0 " passing, ❌ " f + 0 " failing"}' $(RESULTS); \
	} >> $${GITHUB_STEP_SUMMARY:-/dev/stdout}
