# Kue - Kuetix Engine CLI

class Kue < Formula
  desc "CLI tool for Kuetix Engine workflow management"
  homepage "https://github.com/kuetix/engine"
  url "https://github.com/kuetix/engine/archive/refs/tags/v0.1.0.tar.gz"
  sha256 "PUT_SHA256_HERE"
  license "SEE LICENSE FILE"
  head "https://github.com/kuetix/engine.git", branch: "main"

  depends_on "go" => :build

  def install
    system "go", "build", *std_go_args(ldflags: "-s -w -X main.Version=#{version} -X main.BuildTime=#{Time.now.utc.iso8601}"), "./cmd/kue"
  end

  test do
    assert_match "Kuetix Engine CLI - Kue", shell_output("#{bin}/kue --help")
    assert_match version.to_s, shell_output("#{bin}/kue version")
  end
end
