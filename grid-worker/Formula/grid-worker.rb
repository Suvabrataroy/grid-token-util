# Homebrew formula for grid-worker
# Place this in your tap: homebrew-tap/Formula/grid-worker.rb
# Install: brew tap grid-computing/tap && brew install grid-worker

class GridWorker < Formula
  desc "Distributed AI coding grid worker daemon"
  homepage "https://github.com/grid-computing/grid-worker"
  version "0.1.0"
  license "MIT"

  on_macos do
    on_arm do
      url "https://github.com/grid-computing/grid-worker/releases/download/v#{version}/grid-worker_#{version}_darwin_arm64.tar.gz"
      sha256 "PLACEHOLDER_SHA256_DARWIN_ARM64"
    end
    on_intel do
      url "https://github.com/grid-computing/grid-worker/releases/download/v#{version}/grid-worker_#{version}_darwin_amd64.tar.gz"
      sha256 "PLACEHOLDER_SHA256_DARWIN_AMD64"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/grid-computing/grid-worker/releases/download/v#{version}/grid-worker_#{version}_linux_arm64.tar.gz"
      sha256 "PLACEHOLDER_SHA256_LINUX_ARM64"
    end
    on_intel do
      url "https://github.com/grid-computing/grid-worker/releases/download/v#{version}/grid-worker_#{version}_linux_amd64.tar.gz"
      sha256 "PLACEHOLDER_SHA256_LINUX_AMD64"
    end
  end

  def install
    bin.install "grid-worker"
  end

  service do
    run [opt_bin/"grid-worker", "daemon", "--config", "#{Dir.home}/.grid-worker/config.yaml"]
    keep_alive true
    log_path var/"log/grid-worker.log"
    error_log_path var/"log/grid-worker.log"
    process_type :background
  end

  def caveats
    <<~EOS
      To get started:
        1. Configure your API key:
             grid-worker set-key <your-api-key>

        2. Run preflight checks:
             grid-worker preflight

        3. Start the daemon:
             brew services start grid-worker

        Config file: ~/.grid-worker/config.yaml
    EOS
  end

  test do
    system "#{bin}/grid-worker", "version"
  end
end
