package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"coldharbour/internal/detect"
)

type ScanReport struct {
	Target       string             `json:"target"`
	FilesScanned int                `json:"filesScanned"`
	LinesScanned int                `json:"linesScanned"`
	Findings     map[string]int     `json:"findings"`
	TotalPII     int                `json:"totalPII"`
	RiskScore    string             `json:"riskScore"`
	FileDetails  []FileFinding      `json:"fileDetails,omitempty"`
}

type FileFinding struct {
	File     string         `json:"file"`
	Findings map[string]int `json:"findings"`
}

func runScan(args []string) {
	var flagArgs, posArgs []string
	for _, arg := range args {
		if strings.HasPrefix(arg, "-") {
			flagArgs = append(flagArgs, arg)
		} else {
			posArgs = append(posArgs, arg)
		}
	}
	fsCmd := flag.NewFlagSet("scan", flag.ContinueOnError)
	jsonOutput := fsCmd.Bool("json", false, "output report as JSON")
	if err := fsCmd.Parse(flagArgs); err != nil {
		os.Exit(2)
	}

	target := "."
	if len(posArgs) > 0 {
		target = posArgs[0]
	}

	info, err := os.Stat(target)
	if err != nil {
		fmt.Fprintf(os.Stderr, "sentinel scan: %v\n", err)
		os.Exit(2)
	}

	kinds := []detect.Kind{
		detect.KindAadhaar,
		detect.KindPAN,
		detect.KindCreditCard,
		detect.KindEmail,
		detect.KindPhoneIN,
		detect.KindPhoneUS,
		detect.KindSSN,
		detect.KindIP,
	}

	report := ScanReport{
		Target:   target,
		Findings: make(map[string]int),
	}

	walkFn := func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			name := d.Name()
			if strings.HasPrefix(name, ".") || name == "node_modules" || name == "vendor" {
				return filepath.SkipDir
			}
			return nil
		}
		// Skip binary files or archives
		ext := strings.ToLower(filepath.Ext(path))
		if ext == ".exe" || ext == ".dll" || ext == ".so" || ext == ".zip" || ext == ".gz" || ext == ".tar" || ext == ".png" || ext == ".jpg" {
			return nil
		}

		fileFindings, lines, err := scanFile(path, kinds)
		if err != nil {
			return nil
		}
		report.FilesScanned++
		report.LinesScanned += lines

		if len(fileFindings) > 0 {
			report.FileDetails = append(report.FileDetails, FileFinding{
				File:     path,
				Findings: fileFindings,
			})
			for k, count := range fileFindings {
				report.Findings[k] += count
				report.TotalPII += count
			}
		}
		return nil
	}

	if info.IsDir() {
		_ = filepath.WalkDir(target, walkFn)
	} else {
		fileFindings, lines, err := scanFile(target, kinds)
		if err == nil {
			report.FilesScanned = 1
			report.LinesScanned = lines
			if len(fileFindings) > 0 {
				report.FileDetails = append(report.FileDetails, FileFinding{
					File:     target,
					Findings: fileFindings,
				})
				for k, count := range fileFindings {
					report.Findings[k] += count
					report.TotalPII += count
				}
			}
		}
	}

	if report.TotalPII == 0 {
		report.RiskScore = "CLEAN"
	} else if report.TotalPII < 5 {
		report.RiskScore = "LOW"
	} else if report.TotalPII < 20 {
		report.RiskScore = "MEDIUM"
	} else {
		report.RiskScore = "HIGH"
	}

	if *jsonOutput {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(report)
	} else {
		fmt.Println("==================================================")
		fmt.Println("   PROJECT SENTINEL - DPDP VULNERABILITY REPORT   ")
		fmt.Println("==================================================")
		fmt.Printf(" Target:         %s\n", report.Target)
		fmt.Printf(" Files Scanned:  %d\n", report.FilesScanned)
		fmt.Printf(" Lines Scanned:  %d\n", report.LinesScanned)
		fmt.Printf(" Total PII Spans: %d\n", report.TotalPII)
		fmt.Printf(" Risk Posture:   %s\n", report.RiskScore)
		fmt.Println("--------------------------------------------------")
		if len(report.Findings) == 0 {
			fmt.Println(" No unmasked PII detected.")
		} else {
			fmt.Println(" Findings by Category:")
			for kind, count := range report.Findings {
				fmt.Printf("   • %-14s : %d\n", kind, count)
			}
		}
		fmt.Println("==================================================")
	}

	if report.TotalPII > 0 {
		os.Exit(1)
	}
}

func scanFile(path string, kinds []detect.Kind) (map[string]int, int, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, 0, err
	}
	defer f.Close()

	counts := make(map[string]int)
	scanner := bufio.NewScanner(f)
	// Support up to 1MB per line
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	lines := 0

	for scanner.Scan() {
		lines++
		line := scanner.Text()
		spans := detect.Scan(line, kinds)
		for _, sp := range spans {
			counts[string(sp.Kind)]++
		}
	}
	return counts, lines, scanner.Err()
}
