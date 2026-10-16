// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package grabber

import (
	"encoding/json"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"unicode"

	"github.com/PuerkitoBio/goquery"
	"github.com/elboletaire/manga-downloader/http"
	"github.com/elboletaire/manga-downloader/notice"
)

// plainHTMLSelectors is the master list of site selectors used by PlainHTML sites.
// Order is critical: entries that could shadow later ones must come first.
// This list is used by both Test() (to identify sites on series pages) and
// resolveSeriesURL() (to test chapter URLs and find working series pages).
var plainHTMLSelectors = []SiteSelector{
	// lelscanfr.com (French): the series page lists its chapters in
	// #chapters-list, split over ?page=N (One Piece: six pages). The reader
	// renders every page in #chapter-container, lazy ones in data-src.
	{
		Title:      "h2.text-2xl",
		TitleNoise: "span",
		Rows:       "#chapters-list a[href*=\"/manga/\"]",
		Chapter:    "div.flex.gap-2 span",
		Image:      "#chapter-container img.chapter-image",
		Paginated:  true,
	},
	// rizzfables.com: its #chapterlist markup is shared with several themes
	// (en-thunderscans below, and the #readerarea one), which would match
	// first and read the reader's avatar and banner <img>s. Rows are
	// restricted to the site's own chapter links, so this entry can't
	// shadow any other site and goes first.
	{
		Title:        "h1.entry-title",
		Rows:         `#chapterlist li[data-num] a[href*="rizzfables.com/chapter/"]`,
		Chapter:      ".chapternum",
		ChapterTitle: ".chapternum",
		Image:        "#readerarea > img",
	},
	// tcbonepiecechapters.com (former tcbscans.com)
	{
		Title:        "h1",
		Rows:         "main .mx-auto .grid .col-span-2 a",
		Chapter:      ".font-bold",
		ChapterTitle: ".text-gray-500",
		// picture.fixed-ratio is tcb-specific; the broader "picture img"
		// would match other sites and let the SeriesLink fire on them.
		Image:      "picture.fixed-ratio img",
		SeriesLink: `a[href*="/mangas/"]`,
	},
	// asurascans.com (former asuratoon.com)
	{
		Title:        "h1",
		Rows:         `a[data-astro-prefetch][href*="/chapter/"]`,
		Chapter:      "span.font-medium",
		ChapterTitle: "span.font-medium",
		Image:        "img[data-page-index]",
		// early_access_until can be a past date on a free chapter, so only
		// is_premium counts
		LockedIsland: func(c map[string]any) bool { return c["is_premium"] == true },
	},
	// sanascans.com: same Astro-based reader template family as
	// asurascans.com, but chapter URLs use "/chapter-N" (hyphen) instead
	// of "/chapter/N" (slash), so the two selectors don't collide. The
	// series page has several decorative <h1> stat labels (Status, Type,
	// Chapters, Last update) alongside the real title, so a plain "h1"
	// selector would concatenate all of them - scope it to the one with
	// itemprop="name". Recent chapters are often coin-locked (paid early
	// access) and render with zero page images (no error, just an empty
	// chapter) - pick an older/free chapter when testing.
	{
		Title:        `h1[itemprop="name"]`,
		Rows:         `a[data-astro-prefetch][href*="/chapter-"]`,
		Chapter:      "span.font-medium",
		ChapterTitle: "span.font-medium",
		Image:        "img[data-reader-page-image]",
		LockedIsland: func(c map[string]any) bool {
			return c["isLocked"] == true && c["isShortLinkLocked"] != true
		},
	},
	// zonatmo.org (TuMangaOnline, former zonatmo.com). A chapter row lists
	// each upload in its own sub-row, with the uploader's group in a badge
	// (none on some uploads), so every upload is its own release.
	{
		Title:        "h1.element-title",
		Rows:         "li.upload-link",
		Chapter:      ".chapter-number",
		ChapterTitle: ".chapter-number",
		Link:         ".chapter-detail a.btn-primary",
		Image:        "img.reader-image",
		SeriesLink:   `a[href*="/library/manga/"]`,
		Uploads:      ".chapter-detail > div > div",
		UploadLink:   "a.btn-primary",
		UploadGroup:  `a[href*="/groups/"]`,
		IDRe:         `/view_uploads/(\d+)`,
	},
	// mangapill.com: each chapter row is a plain <a> with the chapter
	// number as its own text (no dedicated child selector), hence the
	// empty Chapter/ChapterTitle (see FetchChapters)
	{
		Title:      "h1",
		Rows:       "#chapters [data-filter-list] a",
		Image:      "img.js-page",
		SeriesLink: `a[data-hotkey="m"]`,
	},
	// demonicscans.org and komikmanga.com share the list markup; the latter
	// has .reader-page images, no reader <h1> (.reader-back) and rows that
	// end in the release date.
	{
		Title:             "h1",
		Rows:              "#chapters-list li",
		Link:              "a",
		Image:             "img.imgholder, .reader-page img",
		SeriesLink:        "h1 a, a.reader-back",
		ChapterTitleStrip: `\s*\d{4}-\d{2}-\d{2}$`,
	},
	// mangakatana.com: reader page images never land in the HTML as real
	// <img src>/data-src (they stay as data-src="#" placeholders), they
	// are only assigned client-side from an obfuscated JS array (see
	// getPlainHTMLImageURL), so this Image selector is mostly unused
	{
		Title:        "h1.heading",
		Rows:         ".chapters tr",
		Chapter:      ".chapter a",
		ChapterTitle: ".chapter a",
		Link:         ".chapter a",
		Image:        "#imgs img",
	},
	// rawkuma.net: raw (Japanese) manga, Kiru WordPress theme. The reader
	// page's full page list is server-rendered as plain <img src="..."> tags
	// inside a `[data-image-data]` section, so no browser is needed. Plain
	// "h1" is ambiguous: an info-sidebar "Last Updates" label is also an <h1>
	// earlier in the DOM, so the title selector must be scoped to the real
	// series title. Chapter numbers come from the row's text ("Chapter
	// 29.6"), never from the URL's postid.
	{
		Title:        `h1[itemprop="name"]`,
		Rows:         "#chapter-list [data-chapter-number]",
		Chapter:      "span",
		ChapterTitle: "span",
		Link:         "a",
		Image:        "[data-image-data] img",
	},
	// dynasty-scans.com: chapters are <a class="name"> rows inside a
	// <dl class="chapter-list">; volume headers are sibling <dt>s so a
	// plain descendant selector already skips them. The row is the link
	// itself (empty Chapter/ChapterTitle/Link, like mangapill above).
	// Reader pages only place the first page's <img> in the DOM; the
	// rest come from a `var pages = [...]` JSON blob handled in
	// getPlainHTMLImageURL, so Image here is just a fallback.
	{
		Title:      ".tag-title b",
		Rows:       "dl.chapter-list dd a.name",
		Image:      "#reader img",
		SeriesLink: `h3#chapter-title a[href^="/series/"]`,
	},
	// mangaread.org & manhuaplus.com: plain-HTTP Madara wordpress themes
	// (not cloudflare-walled, unlike the toongod/dragontea/manhuaus group
	// in plainhtmlbrowser.go). The full chapter list ships in the series
	// page HTML (no ajax pagination) and reader pages are single-page.
	// manhuaplus prunes old chapters (pruned URLs soft-404 to the series
	// page with HTTP 200); the list is still complete for what's
	// downloadable, so nothing to work around here.
	{
		// scoped: some themes add a summary <h1> that would join the title
		Title:        ".post-title h1",
		TitleNoise:   ".manga-title-badges",
		Rows:         "li.wp-manga-chapter",
		Chapter:      "a",
		ChapterTitle: "a",
		Link:         "a",
		Image:        "div.reading-content img",
		// paid rows carry Madara's login lock
		Locked: "span.required-login",
		// a reader page lists the chapters but not the series title
		SeriesLink:       `.c-breadcrumb a[href*="/manga/"]`,
		FollowSeriesLink: true,
	},
	// mangas-origines.fr: Madara-derived theme with its own chapter list (the
	// ajax endpoint is walled) under /oeuvre/. Reader images lazy-load
	// through data-src.
	{
		Title:        "h1.ori-sr-title",
		Rows:         "div.ori-chl-row",
		Chapter:      ".ori-chl-nom-long",
		ChapterTitle: ".ori-chl-nom-long",
		Link:         "a.ori-chl-corps",
		Image:        "img.wp-manga-chapter-img",
	},
	// nyraxmanga.com: themesia markup (below) whose paid rows carry a "PLUS"
	// badge and no images logged out. Rows are scoped to the site's own
	// links so Locked can't flag another themesia site.
	{
		Title:        `h1[itemprop="name"]`,
		Rows:         `#chapterlist li:has(.chapternum):has(a[href*="nyraxmanga.com/"])`,
		Chapter:      ".chapternum",
		ChapterTitle: ".chapternum",
		Link:         "a",
		Image:        "#readerarea img",
		Locked:       ".plus-badge",
	},
	// mangastream/themesia WordPress themes reachable over plain HTTP
	// (same markup as the cloudflare-gated sushiscan.net in
	// plainhtmlbrowser.go): lagoonscans.com, rokaricomics.com and
	// violetscans.org (witchscans.com moved to
	// witchtoons.net on a different platform; see the witchtoons
	// grabber). Reader pages embed all pages in a ts_reader.run(...)
	// blob, already handled by getPlainHTMLImageURL, so Image is just a
	// fallback here. Some of these coin-paywall their most recent
	// chapters (no href / no images in the HTML); the :has(a[href]) guard
	// on Rows skips those locked rows entirely so they don't produce
	// bogus BaseUrl() entries in the chapter list. Rows requires
	// .chapternum so a bare #chapterlist wrapper on an unrelated site
	// (e.g. mangahere.cc) can't match this entry, and Title is scoped by
	// itemprop because some of these (violetscans) also render decorative
	// <h1> stat labels (Type/Status/Views).
	{
		// kombatch.cc prints the title in a span instead of an h1
		Title:        `h1[itemprop="name"], span.entry-title`,
		Rows:         "#chapterlist li:has(.chapternum):has(a[href])",
		Chapter:      ".chapternum",
		ChapterTitle: ".chapternum",
		Link:         "a",
		Image:        "#readerarea img",
		// the coin icon rokaricomics.com draws on its paid rows; the svg keeps
		// a bare text-gold colour class on another theme from flagging a row
		Locked: "span.text-gold svg",
	},
	// furyosociety.com: French scanlation site on the FoOlSlide engine.
	// Each chapter is listed twice (a desktop <div> and a mobile <a>);
	// only the mobile <a> is itself the reader link, so scoping Rows to
	// it avoids duplicate chapters. The reader page renders every page
	// image server-side (no ajax pagination), so a plain selector works.
	// Reader pages link back to the series via .fs-read-comic-link.
	{
		Title:        "h1.fs-comic-title",
		Rows:         ".fs-chapter-list a.element.mobile",
		Chapter:      ".title-grp .title",
		ChapterTitle: ".name",
		Image:        ".fs-reader-page-container img",
		SeriesLink:   ".fs-read-comic-link a",
	},
	// reader.deathtollscans.net: FoOlSlide reader. The reader page only
	// renders the current page's <img class="open">, but embeds every
	// page's URL in a `var pages = [...]` JSON blob (see
	// getPlainHTMLImageURL), so the Image selector below is just a
	// fallback/documentation of what a single rendered page looks like
	// (noranofansub.com renders empty img.page-N placeholders instead).
	{
		Title:        "h1.title",
		Rows:         ".list .element",
		Chapter:      ".title a",
		ChapterTitle: ".title a",
		Link:         ".title a",
		Image:        `img.open, img[class^="page-"]`,
		SeriesLink:   "h1.tbtitle a",
	},
	// madarascans.net (formerly .com/.org, which now 301-redirect to .net):
	// mangareader WordPress theme. Chapter rows are <li data-num> elements
	// containing an <a> link; paywalled/locked chapters use a modal trigger
	// instead of an href, so `a[href]` on Rows picks only free chapters.
	// Images are loaded via admin-ajax.php (not in the HTML).
	// Free reader pages carry a div.allc link back to the series; locked
	// reader pages use a.btn-back-enhanced instead. No Image guard: the images
	// never appear in the HTML, so one would always suppress the SeriesLink.
	{
		Title:        "h1.entry-title",
		Rows:         "li[data-num] a[href]",
		Chapter:      ".chapternum",
		ChapterTitle: ".chapternum",
		SeriesLink:   "div.allc a, a.btn-back-enhanced",
	},
	// trmanga.com: Turkish webtoon site. Its chapter table is #chapters
	// too, so this entry must sit before the #chapters ones below (they'd
	// claim it with no chapter text). Rows are the links in the first
	// column, labelled "#124 .Bölüm" (number first, see
	// parseChapterNumber); the reader's images are lazy data-src
	// attributes, except the first one which also carries a src.
	{
		Title:             "h1.movie__title",
		Rows:              "tr.wt-ch-row td.ps-0 a",
		ChapterTitleStrip: `^#?\d+(\.\d+)?\s*\.?\s*Bölüm$`,
		Image:             "#images img.reader-img",
	},
	// ritharscans.com: chapters are wrapped in div.chapter-card inside
	// #chapters (unlike mistscans/writerscans where <a> is a direct child),
	// so Rows uses the card wrapper to tell the two platforms apart. Reader
	// pages parse images out of an Alpine.js `immersiveReader(...)` blob.
	// Chapter reader pages carry a `class="group"` link back to the series;
	// FollowSeriesLink ensures both series and chapter URLs give the same title.
	{
		Title:            "h1",
		Rows:             "#chapters div.chapter-card a",
		Chapter:          ".text-sm.truncate",
		ChapterTitle:     ".text-sm.truncate",
		Image:            `[x-data*="immersiveReader"]`,
		SeriesLink:       `a.group[href*="/series/"]`,
		FollowSeriesLink: true,
		Locked:           ".chapter-lock-badge",
	},
	// writerscans.com & mistscans.com (meowing.org platform): each chapter
	// row is a plain <a> direct child of #chapters (the number in a child
	// span, see FetchChapters/Chapter); reader page images are lazy-loaded
	// (a placeholder `src` and a `uid` attribute), see the
	// getPlainHTMLImageURL template-literal fallback. Image must stay scoped
	// to the reader: a bare "img" also picks up the header's icon <img>s,
	// which have no uid and turn into bare uploads/ URLs that 403. It's a
	// union so it matches both sites' reader markup. Reader pages nest the
	// series title in `<h1><span><a href="/series/…">`, so FollowSeriesLink
	// redirects chapter URLs to the series page for a clean title.
	{
		Title:            "h1",
		Rows:             "#chapters a",
		Chapter:          ".text-sm.truncate",
		ChapterTitle:     ".text-sm.truncate",
		Image:            "img.myImage, #pages img",
		SeriesLink:       `h1 a[href^="/series/"]`,
		FollowSeriesLink: true,
		// paid rows overlay a lock icon (lock-open is a different icon)
		Locked: `img[src*="material-symbols:lock.svg"]`,
	},
	// en-thunderscans.com: mangastream/themesia theme (reader page uses
	// ts_reader.run, already handled generically). Some recent chapters
	// are coin-locked premium content whose row has no href (just a
	// data-bs-toggle modal trigger); the `a[href]` filter on Rows keeps
	// those out of the chapter list entirely instead of yielding bogus
	// URLs. Title uses a specific class because the page has several
	// unrelated bare <h1> tags (Type, Status, Released...) whose text
	// would otherwise get concatenated in.
	{
		Title:        "h1.entry-title",
		Rows:         "#chapterlist li[data-num] a[href]",
		Chapter:      ".chapternum",
		ChapterTitle: ".chapternum",
		Image:        ".readercontent img",
	},
	// silentquill.net (Next.js App Router, server-rendered): every chapter
	// is a plain <a> row linking to /{slug}-chapter-N/, while the series
	// page's own <a> links to /series/... (recommendations) are excluded.
	// The reader's page list is in the RSC payload, see the "pages"
	// branch of getPlainHTMLImageURL. Reader pages link back via a
	// /series/{slug} link (not the bare /series/ listing).
	// No Image: the RSC reader renders no <img> tags in SSR HTML, so an
	// Image guard would suppress the SeriesLink on reader pages.
	{
		Title:        "h1",
		Rows:         `a.grid[href*="-chapter-"]:not([href^="/series/"])`,
		Chapter:      "span.text-base",
		ChapterTitle: "span.text-base",
		SeriesLink:   `a[href^="/series/"]:not([href="/series/"])`,
	},
	// mangaberri.com: the chapter section has Chapters, Volumes and RAW
	// tabs, each in its own panel. Only #chapters-panel is taken (the
	// Volumes and RAW panels would double count or mix in untranslated
	// releases). The series page also carries an ad-blocker <h1> in the
	// page's access wall, so the title is scoped to the .detail block.
	{
		Title:        ".detail h1",
		Rows:         "#chapters-panel a[data-chapter-group]",
		Chapter:      "strong",
		ChapterTitle: "strong",
		Image:        ".reader-pages img",
		SeriesLink:   ".reader-heading a",
	},
	// mangaworld.mx (Italian): server-rendered, unquoted attributes are
	// fine for goquery. Chapter rows are <a class="chap"> inside one
	// <div class="chapter"> per chapter, the number in a span ("Capitolo
	// 835"). The reader renders a single page per chapter unless it gets
	// ?style=list, which lists every page as img.page-image.
	{
		Title:        "h1",
		Rows:         "div.chapter a.chap",
		Chapter:      "span",
		ChapterTitle: "span",
		Image:        "img.page-image",
		ChapterQuery: "style=list",
	},
	// mangavault.lat (Spanish, Astro): the series page lists every chapter
	// as a plain link in one <ul> (the "Cap. N" label is a span), and the
	// reader's pages are plain <img> tags on media.mangavault.lat
	{
		Title:        "h1",
		Rows:         "ul.divide-y a",
		Chapter:      "span",
		ChapterTitle: "span",
		Image:        `img[src*="media.mangavault.lat"]`,
	},
	// scan-vf.net (French, MyMangaReaderCMS): each chapter is an <h5> row
	// whose link text ("Jujutsu Kaisen Modulo 25") has no chapter keyword,
	// so the number comes from the "chapitre-25" URL. The reader keeps the
	// real page in data-src, padded with spaces, behind a placeholder src
	// (getPlainHTMLImageURL trims it).
	// The same CMS runs anzmanga25.com, kangaryu-team.fr and onma.top: their
	// URLs are /manga/{slug}/{number} and each theme puts the series title in
	// its own element. The rows' download links go through an ad shortener.
	{
		Title:        `h2[style*="inline-block"], .titre-projets, .panel-heading`,
		Rows:         "h5.chapter-title-rtl",
		Link:         "a",
		Chapter:      "a",
		ChapterTitle: "em",
		Image:        "img.img-responsive",
		NumberRe:     `/manga/[^/]+/(\d+(?:\.\d+)?)/?$`,
	},
	// uzaymanga.com, eldermanga.com, eskimangalar.com and limonmanga.com:
	// one Turkish manga platform, chapter URLs are /manga/{slug}/{n}-bolum-oku.
	// The series page repeats the newest and oldest chapters in larger
	// cards (no .w-10 number badge), so only the list rows are taken. The
	// row's span carries the series title and "{n}. Bölüm Manga Oku", which
	// parseChapterNumber reads. Reader pages are plain <img> tags with an
	// "... Sayfa N" alt.
	{
		Title:             "h1.text-3xl",
		Rows:              `a[href*="-bolum-oku"]:has(.w-10)`,
		Chapter:           "span.font-bold",
		ChapterTitle:      "span.font-bold",
		ChapterTitleStrip: `^\d+(\.\d+)?\.?\s*Bölüm( Manga Oku)?$`,
		Image:             `img[alt*="sayfa"]`,
	},
	// manhwaonline.org: English manhwa on a custom WordPress theme (not
	// Madara's ajax chapters, so tcb can't serve it). The full list ships
	// in one ul.chapter-list of plain links, and reader pages are plain
	// <img> tags inside .reading-content.
	{
		Title:        "h1.main-info-title",
		Rows:         "ul.chapter-list li a",
		Chapter:      ".chapter-name",
		ChapterTitle: ".chapter-name",
		Image:        ".reading-content img",
	},
	// komikindo.ch: custom WordPress theme. The full list is #chapter_list
	// (the "Chapter Awal" box above it has different markup). Reader pages
	// sit in #chimg-auh, apart from the blogger and .lol ad banners around it.
	// The reader's #chapter_list is a short dropdown (≤5 recent chapters),
	// not the full list; the series link in div.nextprev leads to the full one.
	{
		Title:            `h1[itemprop="name"]`,
		Rows:             "#chapter_list li:has(.lchx)",
		Link:             "a",
		Chapter:          ".lchx",
		ChapterTitle:     ".lchx",
		Image:            "#chimg-auh img",
		SeriesLink:       `div.nextprev a[href*="/komik/"]`,
		FollowSeriesLink: true,
	},
	// maid.my.id: Indonesian WordPress theme. The reader lazy-loads its pages
	// from any host (imgbox.com, imgchest.com) through data-lazy-src (src is a
	// placeholder), and the same pages come again as plain <img>s without that
	// attribute, so only the lazy ones are taken. The reader's community banner
	// is hosted on the site itself and is left out by the host filter.
	// The reader also renders a short chapter list (.series-chapterlist);
	// #chapnav holds the link back to the full series page.
	// oremanga.net (Thai) runs the same series theme: its reader keeps the
	// pages in .reader-area-main and links the series from the side menu.
	{
		Title:            ".series-titlex h2",
		Rows:             ".series-chapterlist li",
		Link:             "a",
		Chapter:          "a",
		ChapterTitle:     "a",
		Image:            `.reader-area img[data-lazy-src]:not([data-lazy-src*="maid.my.id"]), .reader-area-main img`,
		SeriesLink:       `#chapnav a[href*="/manga/"], .coverz a[href*="/series/"]`,
		FollowSeriesLink: true,
	},
}

// PlainHTML is a grabber for any plain HTML page (with no ajax pagination whatsoever)
type PlainHTML struct {
	*Grabber
	doc  *goquery.Document
	rows *goquery.Selection
	site SiteSelector
}

func NewPlainHTML(g *Grabber) *PlainHTML {
	return &PlainHTML{Grabber: g}
}

type SiteSelector struct {
	Title        string
	Rows         string
	Link         string
	Chapter      string
	ChapterTitle string
	// Image is a CSS selector for an element that only appears on reader/chapter
	// pages for this site. It is used as a guard for SeriesLink in
	// seriesURLCandidates: a SeriesLink is only applied when Image also matches,
	// preventing one site's SeriesLink selector from firing on another site's page.
	Image string
	// ChapterQuery, when set, is appended to every chapter URL before it is
	// fetched (e.g. "style=list" for readers that render one page at a time
	// unless asked for the whole list)
	ChapterQuery string
	// TitleNoise is a selector for elements inside Title to leave out, such as
	// a "[Manga]" type badge nested in the heading
	TitleNoise string
	// ChapterTitleStrip is a regexp removing site-specific noise from a
	// sanitized chapter title (e.g. "^#?\d+\s*\.?\s*Bölüm$" for a bare number)
	ChapterTitleStrip string
	// SeriesLink is a CSS selector for finding a link to the series page on
	// reader/chapter pages. Used during chapter URL resolution as the first
	// candidate before generic link extraction.
	SeriesLink string
	// FollowSeriesLink, when true, makes Test() follow the SeriesLink even
	// when the page already has readable chapter rows. Use for sites whose
	// reader pages contain a partial chapter list: the link leads to the
	// series page with the full list.
	FollowSeriesLink bool
	// Uploads, when set, selects one sub-row per upload in a chapter row, for sites that
	// list several groups' releases under one row. A row with no sub-rows falls back to
	// its own link.
	Uploads string
	// UploadLink is the link to an upload's reader, searched inside its
	// sub-row (Uploads), where Link isn't
	UploadLink string
	// UploadGroup selects the group(s) of one upload, searched inside its sub-row, so it
	// must not match another upload's group. Several matches join with ", ".
	UploadGroup string
	// IDRe is a regexp whose first group captures a release's ID from its
	// URL. Without it, or when it doesn't match, the ID is the URL's path.
	IDRe string
	// NumberRe is a regexp whose first group captures the chapter number from
	// a row's link, tried before the row's text, for URLs that carry it.
	NumberRe string
	// Locked is a CSS selector searched inside each row: a match means the
	// listing marks the chapter as paid (Chapter.Locked)
	Locked string
	// Paginated, when set, reads the chapter list's pages after the first
	// (?page=N) until one has no rows, for series listings split over pages.
	Paginated bool
	// LockedIsland is for Astro sites that state the lock only in the series
	// page's island props: it reports whether a chapter object there (one with
	// a "number" and a "slug") is paid; rows with that number become Locked.
	LockedIsland func(chapter map[string]any) bool
}

// PlainHTMLChapter represents a PlainHTML Chapter
type PlainHTMLChapter struct {
	Chapter
	URL string
}

// Test returns true if the URL is a valid grabber URL
func (m *PlainHTML) Test() (bool, error) {
	body, err := http.Get(http.RequestParams{
		URL:          m.URL,
		DisableRetry: true, // identification probe: a 5xx means "not this site"
	})
	if err != nil {
		return false, err
	}
	m.doc, err = goquery.NewDocumentFromReader(body)
	if err != nil {
		return false, err
	}

	// Order matters: some selectors share markup, and shadowing is intentional.
	for _, selector := range plainHTMLSelectors {
		rows := m.doc.Find(selector.Rows)
		if rows.Length() > 0 {
			m.rows = rows
			m.site = selector
			break
		}
	}

	if m.rows == nil || m.rows.Length() == 0 {
		// No rows found; IdentifySite calls resolveSeriesURL as a final
		// fallback (after every other grabber has declined) to avoid extra
		// fetches for URLs that a domain-matched grabber further down the list
		// would serve.
		return false, nil
	}

	// FollowSeriesLink: some reader pages embed a short chapter list that
	// matches the selector but is incomplete. Follow the SeriesLink to the
	// full series page even though rows were found.
	if m.site.FollowSeriesLink && m.site.SeriesLink != "" {
		parsedURL, _ := url.Parse(m.URL)
		if link := m.doc.Find(m.site.SeriesLink).First(); link.Length() > 0 {
			if href, ok := link.Attr("href"); ok {
				resolved := resolveURLRelative(href, parsedURL)
				if resolved != "" && resolved != m.URL {
					if m.testCandidateSeriesURL(resolved) {
						m.URL = resolved
						return true, nil
					}
				}
			}
		}
	}

	// a matched entry whose rows carry no readable chapter number (titles
	// like "Berserk 326" have no keyword) would list nothing, so decline and
	// let a later grabber (tcb for Madara) take the site
	readable := false
	m.rows.Each(func(_ int, s *goquery.Selection) {
		if _, ok := m.rowChapterNumber(s); ok {
			readable = true
		}
	})

	return readable, nil
}

// resolveSeriesURL tries to find and load a series URL when the current page
// has no chapter rows. It fetches up to 3 candidate URLs and tests each against
// the site selectors. If a candidate works, updates m.URL, m.doc, m.rows and
// m.site, and returns true.
func (m *PlainHTML) resolveSeriesURL() bool {
	candidates := seriesURLCandidates(m.doc, m.URL, plainHTMLSelectors)
	for i, candidate := range candidates {
		if i >= 3 {
			// Cap fetches at 3
			break
		}
		if m.testCandidateSeriesURL(candidate) {
			m.URL = candidate
			return true
		}
	}
	return false
}

// testCandidateSeriesURL fetches and tests a candidate series URL against
// the selectors. Returns true if a selector matches readable chapter rows.
func (m *PlainHTML) testCandidateSeriesURL(candidateURL string) bool {
	body, err := http.Get(http.RequestParams{
		URL:          candidateURL,
		DisableRetry: true, // identification probe: a 5xx means "not this site"
	})
	if err != nil {
		return false
	}
	defer body.Close()
	doc, err := goquery.NewDocumentFromReader(body)
	if err != nil {
		return false
	}

	// Test each selector against the candidate page.
	// Pass the selector explicitly so rowChapterNumber can use it directly.
	for _, selector := range plainHTMLSelectors {
		rows := doc.Find(selector.Rows)
		if rows.Length() == 0 {
			continue
		}

		// Check if any row has a readable chapter number
		readable := false
		rows.Each(func(_ int, s *goquery.Selection) {
			if _, ok := checkRowChapterNumber(s, selector); ok {
				readable = true
			}
		})

		if readable {
			m.doc = doc
			m.rows = rows
			m.site = selector
			return true
		}
	}
	return false
}

// numberRegexps caches the compiled SiteSelector.NumberRe patterns, which are
// checked once per row.
var numberRegexps sync.Map

func numberRe(pattern string) *regexp.Regexp {
	if re, ok := numberRegexps.Load(pattern); ok {
		return re.(*regexp.Regexp)
	}
	re := regexp.MustCompile(pattern)
	numberRegexps.Store(pattern, re)
	return re
}

// checkRowChapterNumber is a helper that reads chapter numbers from a row,
// given an explicit selector (rather than relying on m.site).
// This is used by testCandidateSeriesURL to avoid relying on m.site being set.
func checkRowChapterNumber(s *goquery.Selection, selector SiteSelector) (float64, bool) {
	if selector.NumberRe != "" {
		u := s.AttrOr("href", "")
		if selector.Link != "" {
			u = s.Find(selector.Link).AttrOr("href", "")
		}
		if m := numberRe(selector.NumberRe).FindStringSubmatch(u); m != nil {
			if n, err := strconv.ParseFloat(m[1], 64); err == nil {
				return n, true
			}
		}
	}

	chapterText := textWithoutNoise(s)
	if selector.Chapter != "" {
		chapterText = textWithoutNoise(s.Find(selector.Chapter))
	}
	if number, ok := parseChapterNumber(chapterText); ok {
		return number, true
	}

	u := s.AttrOr("href", "")
	if selector.Link != "" {
		u = s.Find(selector.Link).AttrOr("href", "")
	}
	match := urlChapterNumberRe.FindStringSubmatch(u)
	if len(match) > 0 {
		if n, err := strconv.ParseFloat(match[1], 64); err == nil {
			return n, true
		}
	}

	return 0, false
}

// rowChapterNumber returns the chapter number of a row, read from its text
// or, failing that, from its reader URL. FetchChapters and Test share it, so
// Test only claims rows FetchChapters can actually number.
func (m PlainHTML) rowChapterNumber(s *goquery.Selection) (float64, bool) {
	return checkRowChapterNumber(s, m.site)
}

// Ttitle returns the manga title
func (m PlainHTML) FetchTitle() (string, error) {
	title := m.doc.Find(m.site.Title)
	if m.site.TitleNoise != "" {
		title = title.Clone()
		title.Find(m.site.TitleNoise).Remove()
	}
	return sanitizeTitle(textWithoutNoise(title)), nil
}

// textWithoutNoise returns a selection's text with common non-title noise
// stripped out first: <small> blocks (mangahub.io nests both a huge list of
// alternate-script/language titles in its <h1> — long enough to blow past
// filesystem filename limits — and a relative/absolute chapter date, e.g.
// "2 weeks ago", in every chapter row) and badge-like labels (e.g.
// mangahub's "Hot" tag, class "manga-label"), and the nested date span of
// maid.my.id's chapter rows (class "date").
func textWithoutNoise(s *goquery.Selection) string {
	clone := s.Clone()
	clone.Find("small, .manga-label, .date").Remove()
	return clone.Text()
}

// seriesURLCandidates extracts potential series URLs from a reader/chapter page.
// It tries: SeriesLink selector (if any), breadcrumb links, reader back-links,
// and same-host parent paths. Returns the ordered list of candidates for
// testCandidateSeriesURL to verify.
func seriesURLCandidates(doc *goquery.Document, pageURL string, selectors []SiteSelector) []string {
	var candidates []string
	parsedURL, _ := url.Parse(pageURL)
	if parsedURL == nil {
		return candidates
	}

	// SeriesLink selectors: when a selector has an Image field, only apply
	// its SeriesLink if that Image selector also matches the page, confirming
	// the page belongs to that site. Without this guard, a site's SeriesLink
	// can fire on another site's page (e.g. tcb's a[href*="/mangas/"]
	// matching mangapill's navigation links and exhausting the fetch cap).
	for _, selector := range selectors {
		if selector.SeriesLink == "" {
			continue
		}
		if selector.Image != "" && doc.Find(selector.Image).Length() == 0 {
			continue
		}
		doc.Find(selector.SeriesLink).Each(func(i int, s *goquery.Selection) {
			if href, ok := s.Attr("href"); ok {
				resolvedURL := resolveURLRelative(href, parsedURL)
				if resolvedURL != "" && resolvedURL != pageURL {
					candidates = append(candidates, resolvedURL)
				}
			}
		})
	}

	// Breadcrumb links
	breadcrumbSelectors := []string{
		".breadcrumb a",
		`[itemtype*="BreadcrumbList"] a`,
		`nav[aria-label*="readcrumb" i] a`,
		"ol.breadcrumb a",
	}
	for _, sel := range breadcrumbSelectors {
		doc.Find(sel).Each(func(i int, s *goquery.Selection) {
			if href, ok := s.Attr("href"); ok {
				resolvedURL := resolveURLRelative(href, parsedURL)
				if resolvedURL != "" && resolvedURL != pageURL {
					candidates = append(candidates, resolvedURL)
				}
			}
		})
	}

	// Reader back-links (themesia/mangastream)
	backLinkSelectors := []string{
		".allc a",
		".headpost a",
	}
	for _, sel := range backLinkSelectors {
		doc.Find(sel).Each(func(i int, s *goquery.Selection) {
			if href, ok := s.Attr("href"); ok {
				resolvedURL := resolveURLRelative(href, parsedURL)
				if resolvedURL != "" && resolvedURL != pageURL {
					candidates = append(candidates, resolvedURL)
				}
			}
		})
	}

	// Same-host parent paths: links whose path is a parent of current path
	normalizedCurrentPath := strings.TrimSuffix(parsedURL.Path, "/")
	doc.Find("a").Each(func(i int, s *goquery.Selection) {
		if href, ok := s.Attr("href"); ok {
			linkURL, _ := url.Parse(href)
			if linkURL == nil || linkURL.Host == "" {
				// Root-relative or relative path: resolve against the page URL.
				linkURL, _ = parsedURL.Parse(href)
				if linkURL == nil {
					return
				}
			}
			normalizedLinkHost := strings.TrimPrefix(linkURL.Host, "www.")
			normalizedCurrentHost := strings.TrimPrefix(parsedURL.Host, "www.")
			if normalizedLinkHost == normalizedCurrentHost && linkURL.Path != "" {
				normalizedLinkPath := strings.TrimSuffix(linkURL.Path, "/")
				// Is this a parent path? Current must start with link path + "/"
				if normalizedLinkPath != normalizedCurrentPath &&
					normalizedLinkPath != "/" &&
					strings.HasPrefix(normalizedCurrentPath, normalizedLinkPath+"/") {
					fullURL := linkURL.String()
					if fullURL != "" && fullURL != pageURL {
						candidates = append(candidates, fullURL)
					}
				}
			}
		}
	})

	// Deduplicate
	seen := make(map[string]bool)
	var unique []string
	for _, c := range candidates {
		if !seen[c] && !isListingPath(c) && isValidSeriesCandidate(c, parsedURL) {
			seen[c] = true
			unique = append(unique, c)
		}
	}

	return unique
}

// resolveURLRelative resolves a potentially relative URL against a base URL.
// Returns the absolute URL, or empty string if invalid.
func resolveURLRelative(href string, base *url.URL) string {
	if base == nil {
		return ""
	}
	href = strings.TrimSpace(href)
	if href == "" {
		return ""
	}
	u, _ := url.Parse(href)
	if u == nil {
		return ""
	}
	if u.Host != "" && u.Scheme != "" {
		// Absolute URL
		return u.String()
	}
	// Relative URL
	resolved, _ := base.Parse(href)
	if resolved == nil {
		return ""
	}
	return resolved.String()
}

// isValidSeriesCandidate checks basic validity: not root, not listing path
func isValidSeriesCandidate(urlStr string, pageURL *url.URL) bool {
	u, _ := url.Parse(urlStr)
	if u == nil {
		return false
	}
	if u.Path == "/" || u.Path == "" {
		return false
	}
	normalizedPageHost := strings.TrimPrefix(pageURL.Host, "www.")
	normalizedURLHost := strings.TrimPrefix(u.Host, "www.")
	if normalizedURLHost != normalizedPageHost {
		return false
	}
	return true
}

// isListingPath checks if a path looks like a series listing page
func isListingPath(urlStr string) bool {
	u, _ := url.Parse(urlStr)
	if u == nil {
		return false
	}
	path := strings.ToLower(strings.TrimSuffix(u.Path, "/"))
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
	if len(parts) == 1 && (parts[0] == "manga" || parts[0] == "series" || parts[0] == "comics") {
		return true
	}
	return false
}

// chapterNumberRe matches a chapter number in a chapter title, accepting
// "Chapter 10", "Ch. 10", "C. 10", "Cap. 10", the Spanish "Capítulo 10", the
// Italian "Capitolo 10" and the French "Chapitre 10" (case insensitive).
var chapterNumberRe = regexp.MustCompile(`(?i)\b(?:chapter|chapitre|cap(?:[ií]tulo|itolo)?|ch|c)\.?\s*(\d+\.?\d*)`)

// volumeNumberRe matches a volume number ("Volume 18", "Vol. 2", the Spanish
// "Volumen 3"), used as a fallback for sites that list volumes instead of
// chapters (e.g. sushiscan).
var volumeNumberRe = regexp.MustCompile(`(?i)\bvol(?:ume|umen)?\.?\s*(\d+\.?\d*)`)

// urlChapterNumberRe is a last-resort fallback for rows whose visible text
// carries no recognizable chapter/volume keyword (e.g. mangahub.io, whose
// latest-chapter row often just repeats the manga name and number, like
// "One Piece 1187" with no "Chapter" word) but whose reader URL always
// contains "chapter-<number>" (or the French "chapitre-<number>").
var urlChapterNumberRe = regexp.MustCompile(`(?i)chap(?:ter|itre)[-_](\d+\.?\d*)`)

// turkishChapterNumberRe matches the Turkish "Bölüm 12" (or ASCII "Bolum 12"),
// and turkishSuffixChapterNumberRe the number-first order some Turkish sites
// use: "12. Bölüm", "375.1. Bölüm" or "#12 .Bölüm".
var turkishChapterNumberRe = regexp.MustCompile(`(?i)b[öo]l[üu]m\.?\s*(\d+\.?\d*)`)
var turkishSuffixChapterNumberRe = regexp.MustCompile(`(?i)#?(\d+\.?\d*)\s*\.?\s*b[öo]l[üu]m`)

// thaiChapterNumberRe matches the Thai "ตอนที่ 12" ("ตอน 12" too).
var thaiChapterNumberRe = regexp.MustCompile(`ตอน(?:ที่)?\s*(\d+\.?\d*)`)

// parseChapterNumber extracts the chapter number from a chapter title, falling
// back to a volume number. Returns false if no number could be found (these
// are usually site announcements rather than actual chapters).
func parseChapterNumber(text string) (float64, bool) {
	match := chapterNumberRe.FindStringSubmatch(text)
	if len(match) == 0 {
		match = turkishChapterNumberRe.FindStringSubmatch(text)
	}
	if len(match) == 0 {
		match = turkishSuffixChapterNumberRe.FindStringSubmatch(text)
	}
	if len(match) == 0 {
		match = thaiChapterNumberRe.FindStringSubmatch(text)
	}
	if len(match) == 0 {
		// only checked as fallback so "Vol.2 Chapter 15" still prefers the chapter
		match = volumeNumberRe.FindStringSubmatch(text)
	}
	if len(match) == 0 {
		return 0, false
	}
	number, err := strconv.ParseFloat(match[1], 64)
	if err != nil {
		return 0, false
	}
	return number, true
}

// FetchChapters returns a slice of chapters, one per release: a row whose
// upload sub-rows are set (Uploads) gives one chapter for each of them
func (m PlainHTML) FetchChapters() (chapters Filterables, errs []error) {
	seriesTitle, _ := m.FetchTitle()
	var strip *regexp.Regexp
	if m.site.ChapterTitleStrip != "" {
		strip = regexp.MustCompile(m.site.ChapterTitleStrip)
	}
	var idRe *regexp.Regexp
	if m.site.IDRe != "" {
		idRe = regexp.MustCompile(m.site.IDRe)
	}
	lockedNumbers := m.islandLockedNumbers()
	lists := []*goquery.Selection{m.rows}
	if m.site.Paginated {
		more, listErrs := m.listingPages()
		lists = append(lists, more...)
		errs = append(errs, listErrs...)
	}
	for _, rows := range lists {
		rows.Each(func(i int, s *goquery.Selection) {
			// an empty Chapter/ChapterTitle selector means the row itself carries
			// the text (i.e. mangapill, where each chapter row is a plain <a>
			// with no dedicated child element for the chapter number)
			chapterText := textWithoutNoise(s)
			if m.site.Chapter != "" {
				chapterText = textWithoutNoise(s.Find(m.site.Chapter))
			}

			number, ok := m.rowChapterNumber(s)
			if !ok {
				return
			}

			title := chapterText
			if m.site.ChapterTitle != "" {
				title = textWithoutNoise(s.Find(m.site.ChapterTitle))
			}
			title = sanitizeTitle(title)
			title = stripSeriesTitle(title, seriesTitle)
			if strip != nil {
				title = sanitizeTitle(strip.ReplaceAllString(title, ""))
			}
			if chapterOnlyTitleRe.MatchString(title) {
				// nothing left but the number, which the label already gives
				title = ""
			}

			locked := lockedNumbers[number] || (m.site.Locked != "" && s.Find(m.site.Locked).Length() > 0)
			for _, upload := range m.rowUploads(s) {
				u := m.resolveURL(upload.href)
				if !strings.HasPrefix(u, "http") {
					u = m.BaseUrl() + u
				}
				// the ID is read before ChapterQuery is appended, so it never
				// carries the query
				id := releaseID(u, idRe)
				if m.site.ChapterQuery != "" {
					sep := "?"
					if strings.Contains(u, "?") {
						sep = "&"
					}
					u += sep + m.site.ChapterQuery
				}
				chapter := &PlainHTMLChapter{
					Chapter{
						Number: number,
						Title:  title,
						ID:     id,
						Locked: locked,
						Label:  Label{Style: LabelNumbered},
					},
					u,
				}
				chapter.SetGroups(upload.groups...)

				chapters = append(chapters, chapter)
			}
		})
	}

	return
}

// maxListingPages caps the ?page=N walk of a Paginated chapter list
const maxListingPages = 100

// listingPages fetches the chapter list's pages after the first, as ?page=N,
// until one has no rows. It returns each page's rows.
func (m PlainHTML) listingPages() (pages []*goquery.Selection, errs []error) {
	base, err := url.Parse(m.URL)
	if err != nil {
		return nil, []error{err}
	}
	for page := 2; page <= maxListingPages; page++ {
		u := *base
		query := u.Query()
		query.Set("page", strconv.Itoa(page))
		u.RawQuery = query.Encode()

		body, err := http.Get(http.RequestParams{URL: u.String()})
		if err != nil {
			return pages, append(errs, err)
		}
		doc, err := goquery.NewDocumentFromReader(body)
		body.Close()
		if err != nil {
			return pages, append(errs, err)
		}
		rows := doc.Find(m.site.Rows)
		if rows.Length() == 0 {
			break
		}
		pages = append(pages, rows)
	}

	return pages, errs
}

// islandLockedNumbers returns the numbers of the chapters LockedIsland marks
// as paid in the page's Astro islands (nil without the hook)
func (m PlainHTML) islandLockedNumbers() map[float64]bool {
	if m.site.LockedIsland == nil || m.doc == nil {
		return nil
	}
	locked := map[float64]bool{}
	m.doc.Find("astro-island[props]").Each(func(_ int, s *goquery.Selection) {
		var props any
		if json.Unmarshal([]byte(s.AttrOr("props", "")), &props) != nil {
			return
		}
		walkAstroProps(astroUnwrap(props), func(obj map[string]any) {
			number, ok := obj["number"].(float64)
			if _, hasSlug := obj["slug"]; ok && hasSlug && m.site.LockedIsland(obj) {
				locked[number] = true
			}
		})
	})
	return locked
}

// astroUnwrap drops Astro's props encoding: every value is a [type, payload]
// pair, with type 0 a plain value and 1 a list of such pairs
func astroUnwrap(v any) any {
	switch t := v.(type) {
	case []any:
		if len(t) == 2 {
			if kind, ok := t[0].(float64); ok {
				if kind == 1 {
					list, _ := t[1].([]any)
					out := make([]any, len(list))
					for i, e := range list {
						out[i] = astroUnwrap(e)
					}
					return out
				}
				return astroUnwrap(t[1])
			}
		}
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, e := range t {
			out[k] = astroUnwrap(e)
		}
		return out
	}
	return v
}

// walkAstroProps calls fn for every object nested in v
func walkAstroProps(v any, fn func(map[string]any)) {
	switch t := v.(type) {
	case map[string]any:
		fn(t)
		for _, e := range t {
			walkAstroProps(e, fn)
		}
	case []any:
		for _, e := range t {
			walkAstroProps(e, fn)
		}
	}
}

// plainHTMLUpload is one release a chapter row lists: the link it's read
// from (as written in the markup) and the groups that made it
type plainHTMLUpload struct {
	href   string
	groups []string
}

// rowUploads returns the releases a chapter row lists. With Uploads set, that's
// one per upload sub-row that has a link. A row with no sub-rows, or none with
// a link, lists its own link instead, as the single release it is.
func (m PlainHTML) rowUploads(s *goquery.Selection) []plainHTMLUpload {
	if m.site.Uploads != "" {
		var uploads []plainHTMLUpload
		s.Find(m.site.Uploads).Each(func(_ int, sub *goquery.Selection) {
			href := sub.Find(m.site.UploadLink).AttrOr("href", "")
			if href == "" {
				return
			}
			uploads = append(uploads, plainHTMLUpload{href, m.uploadGroups(sub)})
		})
		if len(uploads) > 0 {
			return uploads
		}
	}

	href := s.AttrOr("href", "")
	if m.site.Link != "" {
		href = s.Find(m.site.Link).AttrOr("href", "")
	}
	return []plainHTMLUpload{{href, m.uploadGroups(s)}}
}

// uploadGroups returns the group(s) named inside an upload's markup (or a
// row's, when it has no sub-rows), none when it names none
func (m PlainHTML) uploadGroups(s *goquery.Selection) []string {
	if m.site.UploadGroup == "" {
		return nil
	}
	var names []string
	s.Find(m.site.UploadGroup).Each(func(_ int, g *goquery.Selection) {
		if name := sanitizeTitle(g.Text()); name != "" {
			names = append(names, name)
		}
	})
	return names
}

// releaseID returns the first capture of idRe in the URL, else chapterURLID. Pass the
// URL before ChapterQuery is appended, so it never lands in the ID.
func releaseID(u string, idRe *regexp.Regexp) string {
	if idRe != nil {
		if match := idRe.FindStringSubmatch(u); len(match) > 1 {
			return match[1]
		}
	}
	return chapterURLID(u)
}

// chapterOnlyTitleRe matches a chapter title that is just its number ("12",
// "#12", "12.5."), which says nothing once it's a filename
var chapterOnlyTitleRe = regexp.MustCompile(`^#?\s*\d+(\.\d+)?\.?$`)

// stripSeriesTitle drops the series title from the start of a chapter title,
// case insensitively, with the separators that follow it, so "One Piece
// Chapter 5" reads "Chapter 5". The title must end right after the series
// name or at a non-letter, so a series "Dan" doesn't eat "Danger".
func stripSeriesTitle(title, series string) string {
	if series == "" {
		return title
	}
	t, s := []rune(title), []rune(series)
	if len(t) < len(s) || !strings.EqualFold(string(t[:len(s)]), series) {
		return title
	}
	if len(t) > len(s) && unicode.IsLetter(t[len(s)]) {
		return title
	}
	return sanitizeTitle(strings.TrimLeft(string(t[len(s):]), " \t-–—:|·"))
}

// resolveURL turns a possibly-relative href into an absolute URL, resolved
// against the series page URL (m.URL).
func (m PlainHTML) resolveURL(href string) string {
	if strings.HasPrefix(href, "http") {
		return href
	}
	base, _ := url.Parse(m.URL)
	return resolveURLRelative(href, base)
}

// FetchChapter fetches a chapter and its pages
func (m PlainHTML) FetchChapter(f Filterable) (*Chapter, error) {
	mchap := f.(*PlainHTMLChapter)
	body, err := http.Get(http.RequestParams{
		URL: mchap.URL,
	})
	if err != nil {
		return nil, err
	}
	defer body.Close()
	doc, err := goquery.NewDocumentFromReader(body)
	if err != nil {
		return nil, err
	}

	authorizeMadaradex(doc)

	return m.chapterFromDoc(f, doc), nil
}

// chapterFromDoc builds a Chapter from an already parsed reader page
func (m PlainHTML) chapterFromDoc(f Filterable, doc *goquery.Document) *Chapter {
	pimages := getPlainHTMLImageURL(m.site.Image, doc)
	pcount := len(pimages)

	chapter := &Chapter{
		Title:      f.GetTitle(),
		Label:      labelOf(f),
		Number:     f.GetNumber(),
		PagesCount: int64(pcount),
		Language:   "en",
	}
	// the release's own identity comes from the listing, which is where
	// FetchChapters read it; the page doesn't carry it
	if listed, ok := f.(*PlainHTMLChapter); ok {
		chapter.SetGroups(listed.GetGroups()...)
		chapter.ID = listed.ID
	}

	for i, img := range pimages {
		if img == "" {
			// this error is not critical and is not from our side, so just log it out
			notice.Warn("page %d of %s has no URL to fetch from 😕 (will be ignored)", i, ChapterLabel(chapter))
			continue
		}
		if isThemeImage(img) {
			continue
		}
		if !strings.HasPrefix(img, "http") {
			img = m.BaseUrl() + img
		}

		page := Page{
			Number:     int64(i),
			URL:        img,
			RetryFlaky: m.retryFlaky(),
		}
		chapter.Pages = append(chapter.Pages, page)
	}

	return chapter
}

// isSVGPlaceholder reports whether src is a lazy-load spinner (an .svg) with
// the real image in data-src; pages are never SVG.
func isSVGPlaceholder(src string, s *goquery.Selection) bool {
	return strings.HasSuffix(strings.ToLower(src), ".svg") && s.AttrOr("data-src", "") != ""
}

// isThemeImage reports whether u is a promo or "to be continued" image from
// the theme's own folder, which Madara clones pad chapters with.
func isThemeImage(u string) bool {
	return strings.Contains(u, "/wp-content/themes/")
}

// canonicalHost returns the host of the page's canonical link, without the
// www. prefix, or "" when the page has none
func canonicalHost(doc *goquery.Document) string {
	href := doc.Find(`link[rel="canonical"]`).AttrOr("href", "")
	u, err := url.Parse(href)
	if err != nil {
		return ""
	}
	return strings.TrimPrefix(u.Hostname(), "www.")
}

func getPlainHTMLImageURL(selector string, doc *goquery.Document) []string {
	// Madara sites using the chapter images protection plugin ship no <img>
	if urls, ok, err := wpMangaProtectorImages(doc); ok {
		if err != nil {
			notice.Warn("could not decode the protected pages: %s", err)
		}
		return urls
	}

	// Check for chapImages JavaScript variable first

	html, _ := doc.Html()
	re := regexp.MustCompile(`var chapImages = '([^']+)'`)
	matches := re.FindStringSubmatch(html)
	if len(matches) > 1 {
		// Found chapImages variable, split by comma
		return strings.Split(matches[1], ",")
	}
	// some sites store a plain text array with the urls into a hidden layer
	pimages := doc.Find("#arraydata")
	if pimages.Length() == 1 {
		return strings.Split(pimages.Text(), ",")
	}

	// mangastream/themesia based readers (e.g. sushiscan) embed all the pages
	// of the chapter in a ts_reader javascript call
	re = regexp.MustCompile(`(?s)ts_reader\.run\(.*?"images":\s*\[(.*?)\]`)
	matches = re.FindStringSubmatch(html)
	if len(matches) > 1 {
		urls := regexp.MustCompile(`"([^"]+)"`).FindAllStringSubmatch(matches[1], -1)
		imgs := []string{}
		for _, u := range urls {
			imgs = append(imgs, strings.ReplaceAll(u[1], `\/`, `/`))
		}
		return imgs
	}

	// mangakatana.com assigns each page's real URL from an obfuscated JS
	// array (variable name changes, e.g. `thzq`) into data-src once the page
	// loads: `obj.attr('data-src', thzq[i])`; the <img> tags themselves only
	// ever contain a data-src="#" placeholder in the raw HTML. Find the
	// array's name from that assignment, then pull its literal contents.
	re = regexp.MustCompile(`\.attr\('data-src',\s*(\w+)\[i\]\)`)
	matches = re.FindStringSubmatch(html)
	if len(matches) > 1 {
		arrayRe := regexp.MustCompile(`var\s+` + regexp.QuoteMeta(matches[1]) + `\s*=\s*\[(.*?)\];`)
		arrayMatches := arrayRe.FindStringSubmatch(html)
		if len(arrayMatches) > 1 {
			urls := regexp.MustCompile(`'([^']+)'`).FindAllStringSubmatch(arrayMatches[1], -1)
			imgs := []string{}
			for _, u := range urls {
				imgs = append(imgs, u[1])
			}
			return imgs
		}
	}

	// FoOlSlide readers (e.g. deathtollscans.net) embed the full ordered page
	// list as a proper JSON array in a `var pages = [...]` variable, one
	// object per page with a "url" field; only the current page is actually
	// rendered as an <img> in the HTML. Anchoring on the following
	// `var next_chapter` avoids truncating at a stray "]" inside a string.
	re = regexp.MustCompile(`(?s)var pages\s*=\s*(\[.+?\]);\s*var next_chapter`)
	matches = re.FindStringSubmatch(html)
	if len(matches) > 1 {
		type foolslidePage struct {
			URL string `json:"url"`
		}
		var fpages []foolslidePage
		if err := json.Unmarshal([]byte(matches[1]), &fpages); err == nil {
			// scan-vf.net has the same `var pages` shape, but its objects only
			// carry a file name ("page_image"), so no url is found and the
			// <img> data-src fallback below takes over
			imgs := []string{}
			for _, p := range fpages {
				if p.URL != "" {
					imgs = append(imgs, p.URL)
				}
			}
			if len(imgs) > 0 {
				return imgs
			}
		}
	}

	// dynasty-scans.com (Dynasty Reader) embeds the full, ordered page list
	// as a JSON array in a `var pages = [...]` blob; only the first page's
	// <img> actually exists in the DOM, the rest are swapped in by JS on
	// navigation, so the Image selector alone would only find one page.
	re = regexp.MustCompile(`(?s)var pages\s*=\s*(\[.*?\]);`)
	matches = re.FindStringSubmatch(html)
	if len(matches) > 1 {
		var pages []struct {
			Image string `json:"image"`
		}
		if err := json.Unmarshal([]byte(matches[1]), &pages); err == nil {
			// the array's objects name files (scan-vf.net), not images, so
			// without an image field the <img> tags below are used instead
			imgs := []string{}
			for _, p := range pages {
				if p.Image != "" {
					imgs = append(imgs, p.Image)
				}
			}
			if len(imgs) > 0 {
				return imgs
			}
		}
	}

	// silentquill.net (Next.js RSC too) embeds a double-escaped list of
	// objects with a "url" each, relative to the site root (templetoons.com
	// is handled further down, with its own key-agnostic match):
	// `\"pages\":[{\"url\":\"/img/p/1-x\",\"width\":null,...}]`. The caller
	// prefixes the base URL on relative paths. Matched before the flat array
	// below, which would otherwise pick up every key of these objects.
	re = regexp.MustCompile(`\\"pages\\":\[(\{\\"url\\":.*?)\]`)
	matches = re.FindStringSubmatch(html)
	if len(matches) > 1 {
		urls := regexp.MustCompile(`\\"url\\":\\"([^\\"]+)\\"`).FindAllStringSubmatch(matches[1], -1)
		imgs := []string{}
		for _, u := range urls {
			imgs = append(imgs, u[1])
		}
		return imgs
	}

	// templetoons.com's reader (Next.js RSC streaming) embeds the page list
	// as a flat JSON array inside a double-escaped JS string, i.e. the raw
	// HTML literally contains `\"pages\":[\"https://...\",...]`. Extract it
	// and unescape the `\"` delimiters.
	re = regexp.MustCompile(`\\"pages\\":\[([^\]]+)\]`)
	matches = re.FindStringSubmatch(html)
	if len(matches) > 1 {
		urls := regexp.MustCompile(`"([^"\\]+)\\?"`).FindAllStringSubmatch(matches[1], -1)
		imgs := []string{}
		for _, u := range urls {
			imgs = append(imgs, u[1])
		}
		return imgs
	}

	// the same reader's key is obfuscated and changes between builds (it was
	// "pages", now "gzap5a2"), so on templetoons.com alone any key whose array
	// starts with a URL is the page list. Other Next.js sites carry URL arrays
	// too, hence the gate on the canonical host.
	if canonicalHost(doc) == "templetoons.com" {
		re = regexp.MustCompile(`\\"[\w-]+\\":\[(\\"https?://[^\]]+)\]`)
		matches = re.FindStringSubmatch(html)
		if len(matches) > 1 {
			urls := regexp.MustCompile(`"([^"\\]+)\\?"`).FindAllStringSubmatch(matches[1], -1)
			imgs := []string{}
			for _, u := range urls {
				imgs = append(imgs, u[1])
			}
			return imgs
		}
	}

	// ritharscans.com's Alpine.js reader stores every page's path inline in an
	// `x-data` attribute, either as a literal list with paths relative to a
	// `baseLink`:
	// x-data="immersiveReader({ pages: [{"path":"...","width":...},...], baseLink: 'https://.../storage/', ... })"
	// or, since the reader moved to the CDN, as a JS string holding the JSON
	// (quotes as ", absolute paths):
	// x-data="immersiveReader({ pages: JSON.parse('[{"path":"https:\\\/\\\/cdn...",...}]'), baseLink: 'https:\/\/cdn...\/', ... })"
	reader := doc.Find(`[x-data*="immersiveReader("]`)
	if reader.Length() > 0 {
		xdata := reader.First().AttrOr("x-data", "")
		base := ""
		if m := regexp.MustCompile(`baseLink:\s*'((?:[^'\\]|\\.)*)'`).FindStringSubmatch(xdata); len(m) > 1 {
			base = unescapeJSString(m[1])
		}
		if m := regexp.MustCompile(`JSON\.parse\('((?:[^'\\]|\\.)*)'\)`).FindStringSubmatch(xdata); len(m) > 1 {
			xdata = unescapeJSString(m[1])
		}
		paths := regexp.MustCompile(`"path":"([^"]+)"`).FindAllStringSubmatch(xdata, -1)
		imgs := []string{}
		for _, p := range paths {
			path := strings.ReplaceAll(p[1], `\/`, "/")
			if !strings.HasPrefix(path, "http") {
				path = base + path
			}
			imgs = append(imgs, path)
		}
		if len(imgs) > 0 {
			return imgs
		}
	}

	// some sites (e.g. writerscans.com) lazy-load reader images: the <img>
	// only carries a placeholder `src` plus a `uid` attribute, and an inline
	// script builds the real URL from a template literal like
	// `https://cdn.example.com/uploads/${uid}`. If we find that pattern,
	// build each image's URL from its `uid` instead of reading src/data-src.
	re = regexp.MustCompile("`(https?://[^`]+?)\\$\\{uid\\}`")
	matches = re.FindStringSubmatch(html)
	if len(matches) > 1 {
		prefix := matches[1]
		pimages := doc.Find(selector)
		imgs := []string{}
		pimages.Each(func(i int, s *goquery.Selection) {
			uid := s.AttrOr("uid", "")
			imgs = append(imgs, prefix+uid)
		})
		return imgs
	}

	// images are inside picture objects
	pimages = doc.Find(selector)

	imgs := []string{}
	pimages.Each(func(i int, s *goquery.Selection) {
		// asmotoon.com: pages lazy-load via vanilla-lazyload, so src stays a
		// shared placeholder until the browser scrolls the image into view.
		// The real per-page identifier is always present in the uid
		// attribute, so prefer reconstructing the CDN URL from it.
		if uid := s.AttrOr("uid", ""); uid != "" {
			imgs = append(imgs, "https://cdn.meowing.org/uploads/"+uid)
			return
		}
		src := s.AttrOr("src", "")
		if src == "" || strings.HasPrefix(src, "data:image") || isSVGPlaceholder(src, s) {
			src = s.AttrOr("data-src", "")
		}
		if src == "" {
			// maid.my.id keeps the real URL in data-lazy-src only
			src = s.AttrOr("data-lazy-src", "")
		}
		imgs = append(imgs, strings.Trim(src, " \n\r\t")) // trim whitespaces
	})

	return imgs
}

// unescapeJSString undoes the escapes of a JavaScript string literal body
// (`\uXXXX`, and `\c` for any other character c), e.g. the `"` quotes
// inside a `JSON.parse('...')` argument.
func unescapeJSString(s string) string {
	escapes := regexp.MustCompile(`\\u([0-9a-fA-F]{4})|\\(.)`)
	return escapes.ReplaceAllStringFunc(s, func(m string) string {
		sub := escapes.FindStringSubmatch(m)
		if sub[1] != "" {
			r, _ := strconv.ParseUint(sub[1], 16, 32)
			return string(rune(r))
		}
		return sub[2]
	})
}

// sanitizeTitle sanitizes titles, trimming and removing extra spaces from titles
func sanitizeTitle(title string) string {
	spaces := regexp.MustCompile(`\s+`)
	title = spaces.ReplaceAllString(title, " ")
	title = strings.TrimSpace(title)

	return title
}
