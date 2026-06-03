package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// BenchmarkResult represents parsed benchmark data
type BenchmarkResult struct {
	Name       string
	RequestsPS float64
	AvgLatency float64
	P99Latency float64
	ErrorRate  float64
}

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: go run parse_results.go <results_dir>")
		os.Exit(1)
	}

	resultsDir := os.Args[1]
	files, err := filepath.Glob(filepath.Join(resultsDir, "*.txt"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading results: %v\n", err)
		os.Exit(1)
	}

	for _, file := range files {
		result, err := parseWrkResult(file)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error parsing %s: %v\n", filepath.Base(file), err)
			continue
		}

		printResult(result)
	}
}

func parseWrkResult(filename string) (*BenchmarkResult, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	result := &BenchmarkResult{
		Name: filepath.Base(filename),
	}

	scanner := bufio.NewScanner(file)

	// Regex patterns for wrk output
	reqPerSecPattern := regexp.MustCompile(`Requests/sec:\s+([\d.]+)`)
	p99Pattern := regexp.MustCompile(`99%\s+([\d.]+)(ms|s)`)
	avgPattern := regexp.MustCompile(`Avg\s+([\d.]+)(ms|s|us)`)

	for scanner.Scan() {
		line := scanner.Text()

		// Parse Requests/sec
		if matches := reqPerSecPattern.FindStringSubmatch(line); matches != nil {
			result.RequestsPS, _ = strconv.ParseFloat(matches[1], 64)
		}

		// Parse P99 latency
		if matches := p99Pattern.FindStringSubmatch(line); matches != nil {
			val, _ := strconv.ParseFloat(matches[1], 64)
			unit := matches[2]
			result.P99Latency = convertToMs(val, unit)
		}

		// Parse average latency
		if matches := avgPattern.FindStringSubmatch(line); matches != nil {
			val, _ := strconv.ParseFloat(matches[1], 64)
			unit := matches[2]
			result.AvgLatency = convertToMs(val, unit)
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return result, nil
}

func convertToMs(val float64, unit string) float64 {
	switch unit {
	case "us":
		return val / 1000
	case "ms":
		return val
	case "s":
		return val * 1000
	default:
		return val
	}
}

func printResult(r *BenchmarkResult) {
	fmt.Println(strings.Repeat("=", 60))
	fmt.Printf("File: %s\n", r.Name)
	fmt.Println(strings.Repeat("-", 60))
	fmt.Printf("Requests/sec: %.2f\n", r.RequestsPS)
	fmt.Printf("Avg Latency:  %.2f ms\n", r.AvgLatency)
	fmt.Printf("P99 Latency:  %.2f ms\n", r.P99Latency)

	// Evaluation
	var status string
	if strings.Contains(r.Name, "health") {
		status = evaluateHealth(r.RequestsPS, r.P99Latency)
	} else if strings.Contains(r.Name, "products") {
		status = evaluateProducts(r.RequestsPS, r.P99Latency)
	} else if strings.Contains(r.Name, "orders") {
		status = evaluateOrders(r.RequestsPS, r.P99Latency)
	}

	if status != "" {
		fmt.Printf("Status:       %s\n", status)
	}

	fmt.Println(strings.Repeat("=", 60))
	fmt.Println()
}

func evaluateHealth(qps, p99 float64) string {
	if qps >= 10000 && p99 < 1 {
		return "✅ EXCELLENT (target exceeded)"
	} else if qps >= 8000 && p99 < 2 {
		return "✓ GOOD (above 80% of target)"
	} else if qps >= 5000 && p99 < 5 {
		return "⚠ ACCEPTABLE (needs optimization)"
	}
	return "❌ BELOW TARGET"
}

func evaluateProducts(qps, p99 float64) string {
	if qps >= 5000 && p99 < 5 {
		return "✅ EXCELLENT (target met)"
	} else if qps >= 4000 && p99 < 10 {
		return "✓ GOOD (above 80% of target)"
	} else if qps >= 2000 && p99 < 20 {
		return "⚠ ACCEPTABLE (needs optimization)"
	}
	return "❌ BELOW TARGET"
}

func evaluateOrders(qps, p99 float64) string {
	if qps >= 1000 && p99 < 20 {
		return "✅ EXCELLENT (target met)"
	} else if qps >= 800 && p99 < 30 {
		return "✓ GOOD (above 80% of target)"
	} else if qps >= 500 && p99 < 50 {
		return "⚠ ACCEPTABLE (needs optimization)"
	}
	return "❌ BELOW TARGET"
}
