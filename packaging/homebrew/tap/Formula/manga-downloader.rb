class MangaDownloader < Formula
  desc "Download manga chapters from supported sites into CBZ files"
  homepage "https://elboletaire.github.io/manga-downloader/"
  version "1.11.1"
  license "AGPL-3.0-only"

  livecheck do
    url :stable
    strategy :github_latest
  end

  on_macos do
    on_arm do
      url "https://github.com/elboletaire/manga-downloader/releases/download/v#{version}/manga-downloader-v#{version}-darwin-arm64.tar.gz"
      sha256 "08dfc0ec75bcb2e6de67f75d0a0cc7ede4a2c1266146fbe34e04c941d94f128d"
    end
    on_intel do
      url "https://github.com/elboletaire/manga-downloader/releases/download/v#{version}/manga-downloader-v#{version}-darwin-amd64.tar.gz"
      sha256 "1af333016f968d8b858075d6d3d2fdb313e431123a3d5b79643a70664fbf6e76"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/elboletaire/manga-downloader/releases/download/v#{version}/manga-downloader-v#{version}-linux-arm64.tar.gz"
      sha256 "ffee16f342898bdb25bfc9c7ba31654d66716d8ed35c136c76525fd4d0aa180b"
    end
    on_intel do
      url "https://github.com/elboletaire/manga-downloader/releases/download/v#{version}/manga-downloader-v#{version}-linux-amd64.tar.gz"
      sha256 "1aa3a781be61a1883c007f9a322dcbf42edb94229ccde2b938456abe1111c3d2"
    end
  end

  def install
    bin.install "manga-downloader"
  end

  test do
    assert_match "Usage:", shell_output("#{bin}/manga-downloader --help")
  end
end
