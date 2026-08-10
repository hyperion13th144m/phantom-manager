// Command phantom-manager is a local web UI for running phantom-release on
// Windows + WSL2. It runs inside WSL and drives the environment directly,
// unlike its WinForms predecessor which drove WSL from the outside via wsl.exe.
package main

import (
	"embed"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/hyperion13th144m/phantom-manager/internal/jobs"
	"github.com/hyperion13th144m/phantom-manager/internal/paths"
	"github.com/hyperion13th144m/phantom-manager/internal/server"
	"github.com/hyperion13th144m/phantom-manager/internal/wslenv"
)

//go:embed all:web
var webFS embed.FS

// Version is stamped at build time with -ldflags "-X main.Version=...".
var Version = "dev"

// portAttempts is how many consecutive ports to try before giving up. A stale
// manager left running should not stop a new one from starting.
const portAttempts = 10

func main() {
	port := flag.Int("port", 7777, "listen port on 127.0.0.1")
	releaseDir := flag.String("release", paths.DefaultReleaseDir(), "phantom-release checkout directory")
	noOpen := flag.Bool("no-open", false, "do not open the browser on startup")
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Println(Version)
		return
	}

	if !wslenv.IsWSL() {
		fmt.Fprintln(os.Stderr, "警告: WSL 環境ではないようです。Windows 連携の機能は動作しません。")
	}

	web, err := fs.Sub(webFS, "web")
	if err != nil {
		fatal(err)
	}

	ln, actualPort, err := listen(*port)
	if err != nil {
		fatal(err)
	}

	mgr := jobs.New()
	srv := server.New(server.Config{
		Version:    Version,
		Port:       actualPort,
		ReleaseDir: paths.Expand(*releaseDir),
	}, web, mgr)

	url := fmt.Sprintf("http://localhost:%d", actualPort)
	httpSrv := &http.Server{
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		// No WriteTimeout: /api/events is a long-lived stream.
	}

	fmt.Printf("phantom-manager %s\n", Version)
	fmt.Printf("  %s をブラウザで開いてください\n", url)
	mgr.Announce(fmt.Sprintf("phantom-manager %s を起動しました (%s)", Version, url))

	if !*noOpen {
		if err := wslenv.Open(url); err != nil {
			fmt.Fprintf(os.Stderr, "  ブラウザの自動起動に失敗しました (%v)。上の URL を手動で開いてください。\n", err)
		}
	}

	go func() {
		if err := httpSrv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			fatal(err)
		}
	}()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	<-sig
	fmt.Println("\n終了します。")
	_ = httpSrv.Close()
}

// listen binds 127.0.0.1, moving to the next port if one is already taken.
func listen(start int) (net.Listener, int, error) {
	var lastErr error
	for p := start; p < start+portAttempts; p++ {
		ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", p))
		if err == nil {
			if p != start {
				fmt.Printf("ポート %d は使用中のため %d で起動します。\n", start, p)
			}
			return ln, p, nil
		}
		lastErr = err
	}
	return nil, 0, fmt.Errorf("127.0.0.1 の %d〜%d を listen できませんでした: %w", start, start+portAttempts-1, lastErr)
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "エラー:", err)
	os.Exit(1)
}
