cask "manga-downloader" do
  version "1.11.1"
  sha256 "1cf62b8b1bf2ec1887f044087d4ba69a394194b56abc10670de3604a965afa6d"

  url "https://github.com/elboletaire/manga-downloader/releases/download/v#{version}/manga-downloader-gui-v#{version}-darwin-universal.zip"
  name "Manga Downloader"
  desc "Download manga chapters to CBZ files"
  homepage "https://elboletaire.github.io/manga-downloader/"

  livecheck do
    url :url
    strategy :github_latest
  end

  # The app and the command line tool go together: `manga-downloader ui` is the
  # CLI's, and the formula shares this cask's name, so it's easy to miss.
  depends_on formula: "elboletaire/tap/manga-downloader"

  app "Manga Downloader.app"
  # `manga-downloader ui` looks for manga-downloader-gui next to the CLI or on
  # PATH, so link the app's executable there.
  binary "#{appdir}/Manga Downloader.app/Contents/MacOS/manga-downloader-gui", target: "manga-downloader-gui"

  # ~/Library/Application Support/manga-downloader is shared with the CLI (the
  # Manga Plus secret lives there), so only the app's own settings file goes.
  zap trash: [
    "~/Library/Application Support/manga-downloader/gui.json",
    "~/Library/Caches/com.github.elboletaire.mangadownloader",
    "~/Library/HTTPStorages/com.github.elboletaire.mangadownloader",
    "~/Library/Preferences/com.github.elboletaire.mangadownloader.plist",
    "~/Library/Saved Application State/com.github.elboletaire.mangadownloader.savedState",
    "~/Library/WebKit/com.github.elboletaire.mangadownloader",
  ]

  caveats <<~EOS
    Manga Downloader is not notarized yet, so macOS blocks it the first time it opens.
    Go to System Settings > Privacy & Security and click "Open Anyway".
  EOS
end
