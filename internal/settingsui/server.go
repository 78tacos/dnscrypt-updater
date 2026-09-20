package settingsui

import (
	"context"
	"crypto/rand"
	"embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/78tacos/dnscrypt-updater/internal/proxyconf"
)

//go:embed web/*
var webFS embed.FS

// Options configure the loopback settings UI.
type Options struct {
	Locate         func() (installDir, binaryPath string, manageService bool)
	Log            *slog.Logger
	NeedsElevation func() bool
	Elevate        func(ctx context.Context, staging string) error
	Check          func(ctx context.Context, bin, configPath string) error
	StopService    func(ctx context.Context, bin string) error
	StartService   func(ctx context.Context, bin string) error
	PendingDir     func() string
	CanWrite       func(path string) bool
	OnPending      func(proxyconf.PendingStatus)
}

// Server is a 127.0.0.1 HTTP UI.
type Server struct {
	opts  Options
	token string
	mu    sync.Mutex
	srv   *http.Server
	url   string
	cat   proxyconf.Catalog
}

func New(opts Options) (*Server, error) {
	cat, err := proxyconf.LoadCatalog()
	if err != nil {
		return nil, err
	}
	tok, err := randomToken()
	if err != nil {
		return nil, err
	}
	if opts.Log == nil {
		opts.Log = slog.Default()
	}
	return &Server{opts: opts, token: tok, cat: cat}, nil
}

func randomToken() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}

func (s *Server) tokenOK(r *http.Request) bool {
	q := strings.TrimSpace(r.URL.Query().Get("token"))
	if q == s.token {
		return true
	}
	return strings.TrimSpace(r.Header.Get("X-Settings-Token")) == s.token
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	sub, err := fs.Sub(webFS, "web")
	if err != nil {
		panic(err)
	}
	file := http.FileServer(http.FS(sub))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			file.ServeHTTP(w, r)
			return
		}
		file.ServeHTTP(w, r)
	})
	mux.HandleFunc("/api/state", s.handleState)
	mux.HandleFunc("/api/apply", s.handleApply)
	mux.HandleFunc("/api/pending.zip", s.handlePendingZip)
	mux.HandleFunc("/api/pending", s.handlePending)
	return mux
}

// Start listens on 127.0.0.1 and returns the URL including the token.
func (s *Server) Start(ctx context.Context) (string, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", err
	}
	s.srv = &http.Server{
		Handler:           s.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		BaseContext:       func(net.Listener) context.Context { return ctx },
	}
	go func() {
		_ = s.srv.Serve(ln)
	}()
	go func() {
		<-ctx.Done()
		c, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = s.srv.Shutdown(c)
	}()
	addr := ln.Addr().String()
	s.url = fmt.Sprintf("http://%s/?token=%s", addr, s.token)
	return s.url, nil
}

func (s *Server) URL() string { return s.url }

func (s *Server) handleState(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}
	if !s.tokenOK(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	st, err := s.snapshot()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, st)
}

type snapshot struct {
	Token          string                     `json:"token"`
	UpstreamTag    string                     `json:"upstream_tag"`
	InstallDir     string                     `json:"install_dir"`
	BinaryPath     string                     `json:"binary_path"`
	TomlPath       string                     `json:"toml_path"`
	TomlExists     bool                       `json:"toml_exists"`
	Writable       bool                       `json:"writable"`
	NeedsElevation bool                       `json:"needs_elevation"`
	Catalog        proxyconf.Catalog          `json:"catalog"`
	Current        map[string]proxyconf.Value `json:"current"`
	Files          []proxyconf.FileChange     `json:"files"`
	Presets        []proxyconf.Preset         `json:"presets"`
	Suggestions    []proxyconf.Suggestion     `json:"suggestions"`
	MissingProxy   bool                       `json:"missing_proxy"`
	Pending        proxyconf.PendingStatus    `json:"pending"`
}

func (s *Server) locate() (installDir, binaryPath string, manageService bool) {
	if s.opts.Locate != nil {
		return s.opts.Locate()
	}
	return "", "", false
}

func (s *Server) snapshot() (snapshot, error) {
	installDir, binaryPath, manageService := s.locate()
	_ = manageService
	out := snapshot{
		Token:       s.token,
		UpstreamTag: s.cat.UpstreamTag,
		InstallDir:  installDir,
		BinaryPath:  binaryPath,
		TomlPath:    filepath.Join(installDir, "dnscrypt-proxy.toml"),
		Catalog:     s.cat,
		Presets:     proxyconf.Presets(),
		Current:     map[string]proxyconf.Value{},
	}
	if installDir == "" {
		out.MissingProxy = true
		return out, nil
	}
	if _, err := os.Stat(out.TomlPath); err == nil {
		out.TomlExists = true
		raw, err := os.ReadFile(out.TomlPath)
		if err != nil {
			return out, err
		}
		out.Current = proxyconf.CurrentValues(string(raw), s.cat)
		out.Suggestions = proxyconf.Suggestions(s.cat, out.Current)
		files, err := proxyconf.LoadListFiles(installDir)
		if err != nil {
			return out, err
		}
		out.Files = files
	} else if !os.IsNotExist(err) {
		return out, err
	}
	if binaryPath == "" {
		out.MissingProxy = true
	} else if _, err := os.Stat(binaryPath); err != nil {
		out.MissingProxy = true
	}
	out.Writable = out.TomlExists && s.canWrite(out.TomlPath)
	if s.opts.NeedsElevation != nil {
		out.NeedsElevation = s.opts.NeedsElevation() && !out.Writable
	}
	out.Pending = proxyconf.ReadPending(s.pendingDir())
	return out, nil
}

func (s *Server) pendingDir() string {
	if s.opts.PendingDir != nil {
		return s.opts.PendingDir()
	}
	return ""
}

func (s *Server) canWrite(path string) bool {
	if s.opts.CanWrite != nil {
		return s.opts.CanWrite(path)
	}
	return proxyconf.CanWrite(path)
}

func (s *Server) notifyPending() {
	if s.opts.OnPending == nil {
		return
	}
	s.opts.OnPending(proxyconf.ReadPending(s.pendingDir()))
}

func (s *Server) handleApply(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}
	if !s.tokenOK(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 4<<20))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	var req proxyconf.ApplyRequest
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, "invalid json: "+err.Error(), http.StatusBadRequest)
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	res, err := s.apply(r.Context(), req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, res)
}

func (s *Server) apply(ctx context.Context, req proxyconf.ApplyRequest) (proxyconf.ApplyResult, error) {
	installDir, binaryPath, manageService := s.locate()
	if installDir == "" {
		return proxyconf.ApplyResult{}, fmt.Errorf("dnscrypt-proxy is not installed; use Install from the tray first")
	}
	toml := filepath.Join(installDir, "dnscrypt-proxy.toml")
	if _, err := os.Stat(toml); err != nil {
		return proxyconf.ApplyResult{}, fmt.Errorf("missing %s", toml)
	}
	staging, err := os.MkdirTemp("", "dnscrypt-settings-*")
	if err != nil {
		return proxyconf.ApplyResult{}, err
	}
	defer func() { _ = os.RemoveAll(staging) }()
	if err := proxyconf.Stage(installDir, staging, req, s.cat); err != nil {
		return proxyconf.ApplyResult{}, err
	}
	env := proxyconf.ApplyEnv{
		InstallDir:    installDir,
		BinaryPath:    binaryPath,
		ManageService: manageService,
		Check:         s.opts.Check,
		StopService:   s.opts.StopService,
		StartService:  s.opts.StartService,
	}
	writable := s.canWrite(toml)
	var elevate func(context.Context, string) error
	if !writable && s.opts.NeedsElevation != nil && s.opts.NeedsElevation() && s.opts.Elevate != nil {
		elevate = s.opts.Elevate
	}
	res, err := proxyconf.TryCommit(ctx, env, staging, s.pendingDir(), writable, elevate)
	if err != nil {
		return res, err
	}
	s.notifyPending()
	return res, nil
}

func (s *Server) handlePendingZip(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}
	if !s.tokenOK(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", `attachment; filename="dnscrypt-proxy-settings.zip"`)
	if err := proxyconf.WritePendingZip(s.pendingDir(), w); err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
}

func (s *Server) handlePending(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}
	if !s.tokenOK(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	if err := proxyconf.ClearPending(s.pendingDir()); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.notifyPending()
	writeJSON(w, proxyconf.ReadPending(s.pendingDir()))
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(true)
	_ = enc.Encode(v)
}
