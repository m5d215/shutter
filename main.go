package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

type config struct {
	listenAddr     string
	ffmpegBin      string
	device         string
	videoSize      string
	framerate      string
	captureTimeout time.Duration
}

var captureMu sync.Mutex

func main() {
	cfg, err := loadConfig()
	if err != nil {
		log.Fatal(err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/capture", func(w http.ResponseWriter, r *http.Request) {
		handleCapture(w, r, cfg)
	})
	srv := &http.Server{
		Addr:              cfg.listenAddr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	log.Printf("shutter listening on %s (device=%q size=%s fps=%s)",
		cfg.listenAddr, cfg.device, cfg.videoSize, cfg.framerate)
	if err := srv.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}

func handleCapture(w http.ResponseWriter, r *http.Request, cfg config) {
	start := time.Now()
	log.Printf("req %s %s from %s", r.Method, r.URL.Path, r.RemoteAddr)

	captureMu.Lock()
	defer captureMu.Unlock()

	jpg, err := capture(cfg)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, context.DeadlineExceeded) {
			status = http.StatusGatewayTimeout
		}
		log.Printf("capture failed status=%d dur=%v err=%v", status, time.Since(start), err)
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(status)
		fmt.Fprintln(w, err.Error())
		return
	}

	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(jpg)))
	w.WriteHeader(http.StatusOK)
	w.Write(jpg)
	log.Printf("capture ok bytes=%d dur=%v", len(jpg), time.Since(start))
}

func capture(cfg config) ([]byte, error) {
	tmp, err := os.CreateTemp("", "shutter-*.jpg")
	if err != nil {
		return nil, fmt.Errorf("mktemp: %w", err)
	}
	tmpPath := tmp.Name()
	tmp.Close()
	defer os.Remove(tmpPath)

	ctx, cancel := context.WithTimeout(context.Background(), cfg.captureTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, cfg.ffmpegBin,
		"-f", "avfoundation",
		"-video_size", cfg.videoSize,
		"-framerate", cfg.framerate,
		"-i", cfg.device,
		"-frames:v", "1",
		"-y",
		tmpPath,
	)
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return nil, fmt.Errorf("ffmpeg timed out after %s: %w", cfg.captureTimeout, context.DeadlineExceeded)
		}
		return nil, fmt.Errorf("ffmpeg: %w", err)
	}

	data, err := os.ReadFile(tmpPath)
	if err != nil {
		return nil, fmt.Errorf("read: %w", err)
	}
	return data, nil
}

func loadConfig() (config, error) {
	if path := os.Getenv("SHUTTER_CONFIG"); path != "" {
		if err := loadConfigFile(path); err != nil {
			return config{}, fmt.Errorf("load %s: %w", path, err)
		}
	}

	cfg := config{
		listenAddr: envOr("SHUTTER_LISTEN_ADDR", "127.0.0.1:9998"),
		ffmpegBin:  envOr("SHUTTER_FFMPEG", "/opt/homebrew/bin/ffmpeg"),
		device:     os.Getenv("SHUTTER_DEVICE"),
		videoSize:  envOr("SHUTTER_VIDEO_SIZE", "3840x2160"),
		framerate:  envOr("SHUTTER_FRAMERATE", "30"),
	}

	if cfg.device == "" {
		return config{}, errors.New("SHUTTER_DEVICE is required (set in SHUTTER_CONFIG file or environment)")
	}

	timeoutStr := envOr("SHUTTER_CAPTURE_TIMEOUT", "10s")
	d, err := time.ParseDuration(timeoutStr)
	if err != nil {
		return config{}, fmt.Errorf("SHUTTER_CAPTURE_TIMEOUT %q: %w", timeoutStr, err)
	}
	cfg.captureTimeout = d

	return cfg, nil
}

func loadConfigFile(path string) error {
	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		k = strings.TrimSpace(k)
		v = strings.TrimSpace(v)
		if _, exists := os.LookupEnv(k); exists {
			continue
		}
		if err := os.Setenv(k, v); err != nil {
			return err
		}
	}
	return sc.Err()
}

func envOr(key, def string) string {
	if v, ok := os.LookupEnv(key); ok {
		return v
	}
	return def
}
