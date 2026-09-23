package main

import (
	"bufio"
	"context"
	"flag"
	"io"
	"log"
	"os"
	"strings"
	"time"

	"coldharbour/internal/detect"
)

func main() {
	log.SetFlags(0)
	if len(os.Args) < 2 || os.Args[1] != "run" {
		log.Fatal("usage: sentinel run --in - --out -")
	}
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	inPath := fs.String("in", "-", "input path, or - for stdin")
	outPath := fs.String("out", "-", "output path, or - for stdout")
	follow := fs.Bool("follow", false, "keep reading after the current end of a file")
	maxInline := fs.Int("max-inline-bytes", 65536, "drop a longer line until handoff exists")
	shadow := fs.Bool("shadow", false, "count detections and write the line unchanged")
	metrics := fs.String("metrics", ":9101", "Prometheus listen address")
	control := fs.String("control-plane", "", "Cold Harbour base URL")
	stateDir := fs.String("state-dir", "", "directory for the cached policy")
	policyFile := fs.String("policy", "", "policy file used when the control plane is unreachable")
	interval := fs.Duration("policy-interval", 60*time.Second, "how often to fetch the policy")
	if err := fs.Parse(os.Args[2:]); err != nil {
		log.Fatal("sentinel: bad flags")
	}
	if *follow && *inPath == "-" {
		log.Fatal("sentinel: --follow needs a file")
	}
	if err := serveMetrics(*metrics); err != nil {
		log.Fatalf("sentinel: metrics: %v", err)
	}
	active.Store(maskSettings{maxInline: *maxInline, kinds: append([]detect.Kind(nil), maskKinds...), mode: detect.ModeRedact})
	if *control != "" || *policyFile != "" {
		doc, etag, err := loadInitial(*control, *stateDir, *policyFile, os.Getenv("SENTINEL_API_KEY"))
		if err != nil {
			log.Fatalf("sentinel: %v", err)
		}
		active.Store(settingsFrom(doc, etag))
		if *control != "" {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			go poll(ctx, *control, *stateDir, os.Getenv("SENTINEL_API_KEY"), *interval)
		}
	}
	in, err := openIn(*inPath)
	if err != nil {
		log.Fatalf("sentinel: input: %v", err)
	}
	defer in.Close()
	out, err := openOut(*outPath)
	if err != nil {
		log.Fatalf("sentinel: output: %v", err)
	}
	defer out.Close()
	if err := readLines(in, *follow, func(line string) error {
		start := time.Now()
		st := active.Load().(maskSettings)
		result := maskWith(line, st.maxInline, *shadow, st.kinds, st.mode)
		if _, err := out.WriteString(result.line + "\n"); err != nil {
			return err
		}
		if err := out.Flush(); err != nil {
			return err
		}
		linesTotal.Inc()
		if result.dropped {
			droppedTotal.Inc()
		}
		for kind, n := range result.counts {
			detections.WithLabelValues(string(kind)).Add(float64(n))
		}
		lineSeconds.Observe(time.Since(start).Seconds())
		return nil
	}); err != nil {
		log.Fatalf("sentinel: %v", err)
	}
}

type flushWriter struct {
	w *bufio.Writer
	c io.Closer
}

func (f flushWriter) WriteString(s string) (int, error) { return f.w.WriteString(s) }
func (f flushWriter) Flush() error                      { return f.w.Flush() }
func (f flushWriter) Close() error {
	err := f.w.Flush()
	if f.c != nil {
		if cerr := f.c.Close(); err == nil {
			err = cerr
		}
	}
	return err
}

func openIn(path string) (io.ReadCloser, error) {
	if path == "-" {
		return io.NopCloser(os.Stdin), nil
	}
	return os.Open(path) //#nosec G304 -- operator input path
}

func openOut(path string) (flushWriter, error) {
	if path == "-" {
		return flushWriter{w: bufio.NewWriter(os.Stdout)}, nil
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600) //#nosec G304 -- operator output path
	if err != nil {
		return flushWriter{}, err
	}
	return flushWriter{w: bufio.NewWriter(f), c: f}, nil
}

func readLines(r io.Reader, follow bool, fn func(string) error) error {
	br := bufio.NewReader(r)
	var pending string
	for {
		chunk, err := br.ReadString('\n')
		if len(chunk) > 0 {
			pending += chunk
		}
		if strings.HasSuffix(pending, "\n") {
			line := strings.TrimRight(pending, "\r\n")
			pending = ""
			if err2 := fn(line); err2 != nil {
				return err2
			}
		}
		if err == io.EOF {
			if !follow {
				if pending != "" {
					return fn(pending)
				}
				return nil
			}
			time.Sleep(30 * time.Millisecond)
			continue
		}
		if err != nil {
			return err
		}
	}
}
