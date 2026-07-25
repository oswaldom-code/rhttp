package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"
)

type result struct {
	name   string
	ns     []float64
	bytes  int64
	allocs int64
}

var benchLine = regexp.MustCompile(`^(Benchmark\S+?)(?:-\d+)?\s+\d+\s+([\d.]+) ns/op\s+([\d.]+) B/op\s+([\d.]+) allocs/op`)

var labels = map[string]string{
	"Overhead_NetHTTP_Bare":       "net/http (Timeout only, no retry)",
	"Overhead_Rhttp_TimeoutRetry": "rhttp (Timeout+Retry)",
	"Overhead_Rhttp_FullStack":    "rhttp (Timeout+Retry+CircuitBreaker)",
	"Overhead_Resty_Retry":        "Resty (retry)",
	"Overhead_Retryablehttp":      "go-retryablehttp",
	"Overhead_Heimdall_Retry":     "Heimdall (retry)",
	"E2E_NetHTTP_Bare":            "net/http (Timeout only, no retry)",
	"E2E_Rhttp_TimeoutRetry":      "rhttp (Timeout+Retry)",
	"E2E_Rhttp_FullStack":         "rhttp (Timeout+Retry+CircuitBreaker)",
	"E2E_Resty_Retry":             "Resty (retry)",
	"E2E_Retryablehttp":           "go-retryablehttp",
	"E2E_Heimdall_Retry":          "Heimdall (retry)",
}

func cpuModel() string {
	data, err := os.ReadFile("/proc/cpuinfo")
	if err != nil {
		return "unknown"
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "model name") {
			if i := strings.Index(line, ":"); i >= 0 {
				return strings.TrimSpace(line[i+1:])
			}
		}
	}
	return "unknown"
}

func depVersions() []string {
	deps := []string{
		"github.com/go-resty/resty/v2",
		"github.com/hashicorp/go-retryablehttp",
		"github.com/gojek/heimdall/v7",
	}
	args := append([]string{"list", "-m", "-f", "{{.Path}} {{.Version}}"}, deps...)
	out, err := exec.Command("go", args...).Output()
	if err != nil {
		return []string{"unavailable"}
	}
	var versions []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line != "" {
			versions = append(versions, line)
		}
	}
	return versions
}

func runBenchmarks(pattern string, count int) ([]string, error) {
	cmd := exec.Command("go", "test",
		"-bench="+pattern, "-benchmem",
		"-count="+strconv.Itoa(count),
		"-timeout=30m", ".")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	var lines []string
	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		line := scanner.Text()
		fmt.Fprintln(os.Stderr, line)
		if benchLine.MatchString(line) {
			lines = append(lines, line)
		}
	}
	if err := cmd.Wait(); err != nil {
		return nil, err
	}
	return lines, nil
}

func parse(lines []string) map[string]*result {
	results := make(map[string]*result)
	for _, line := range lines {
		m := benchLine.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		name := strings.TrimPrefix(m[1], "Benchmark")
		ns, _ := strconv.ParseFloat(m[2], 64)
		bytes, _ := strconv.ParseFloat(m[3], 64)
		allocs, _ := strconv.ParseFloat(m[4], 64)
		r, ok := results[name]
		if !ok {
			r = &result{name: name}
			results[name] = r
		}
		r.ns = append(r.ns, ns)
		r.bytes = int64(bytes)
		r.allocs = int64(allocs)
	}
	return results
}

func minMean(xs []float64) (float64, float64) {
	minV := xs[0]
	sum := 0.0
	for _, x := range xs {
		if x < minV {
			minV = x
		}
		sum += x
	}
	return minV, sum / float64(len(xs))
}

func writeTable(sb *strings.Builder, prefix string, results map[string]*result) {
	type row struct {
		label         string
		minNs, meanNs float64
		bytes, allocs int64
	}
	var rows []row
	for name, r := range results {
		if !strings.HasPrefix(name, prefix) {
			continue
		}
		label, ok := labels[name]
		if !ok {
			label = name
		}
		minV, mean := minMean(r.ns)
		rows = append(rows, row{label, minV, mean, r.bytes, r.allocs})
	}
	if len(rows) == 0 {
		sb.WriteString("_no results_\n\n")
		return
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].minNs < rows[j].minNs })
	best := rows[0].minNs
	sb.WriteString("| Client | ns/op (min) | ns/op (mean) | B/op | allocs/op | vs best |\n")
	sb.WriteString("|---|---:|---:|---:|---:|---:|\n")
	for _, r := range rows {
		fmt.Fprintf(sb, "| %s | %.0f | %.0f | %d | %d | %.2fx |\n",
			r.label, r.minNs, r.meanNs, r.bytes, r.allocs, r.minNs/best)
	}
	sb.WriteString("\n")
}

func buildReport(results map[string]*result, count int) string {
	var sb strings.Builder
	sb.WriteString("# HTTP client comparison report\n\n")
	fmt.Fprintf(&sb, "Generated: %s\n\n", time.Now().Format("2006-01-02 15:04 MST"))
	sb.WriteString("## Environment\n\n")
	sb.WriteString("| | |\n|---|---|\n")
	fmt.Fprintf(&sb, "| CPU | %s (%d threads) |\n", cpuModel(), runtime.NumCPU())
	fmt.Fprintf(&sb, "| OS/arch | %s/%s |\n", runtime.GOOS, runtime.GOARCH)
	fmt.Fprintf(&sb, "| Go | %s |\n", runtime.Version())
	fmt.Fprintf(&sb, "| Samples per benchmark | %d (min reported as typical cost) |\n\n", count)
	sb.WriteString("## Tool versions\n\n")
	sb.WriteString("- github.com/oswaldom-code/rhttp (local, via replace)\n")
	for _, v := range depVersions() {
		fmt.Fprintf(&sb, "- %s\n", v)
	}
	sb.WriteString("\n## Methodology\n\n")
	sb.WriteString("All clients are configured equivalently: 5s timeout, 3 total attempts, ")
	sb.WriteString("exponential backoff 100ms-2s. Every client fully consumes and closes the ")
	sb.WriteString("response body.\n\n")
	sb.WriteString("- **Overhead**: a no-op transport returns 200 OK without touching the ")
	sb.WriteString("network, isolating client/middleware cost per request.\n")
	sb.WriteString("- **E2E**: a local httptest.Server returns ~1 KB of JSON over loopback, ")
	sb.WriteString("measuring total request cost including a real HTTP round trip.\n\n")
	sb.WriteString("Caveats: net/http does not retry (it is the floor, not a symmetric ")
	sb.WriteString("competitor); Heimdall runs without its Hystrix circuit breaker (retry ")
	sb.WriteString("only, for feature symmetry); Resty buffers the full response body by ")
	sb.WriteString("design; loopback amplifies relative overhead — against a real network ")
	sb.WriteString("(0.5-500 ms) these differences are negligible.\n\n")
	sb.WriteString("## Results: wrapper overhead (no network)\n\n")
	writeTable(&sb, "Overhead_", results)
	sb.WriteString("## Results: end-to-end (loopback, ~1 KB JSON)\n\n")
	writeTable(&sb, "E2E_", results)
	sb.WriteString("## Reproduce\n\n")
	sb.WriteString("```bash\ncd benchmarks\ngo run ./report -count 5\n```\n")
	return sb.String()
}

func main() {
	count := flag.Int("count", 5, "runs per benchmark")
	pattern := flag.String("bench", ".", "benchmark regex")
	out := flag.String("out", "REPORT.md", "output file")
	flag.Parse()

	lines, err := runBenchmarks(*pattern, *count)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	results := parse(lines)
	if len(results) == 0 {
		fmt.Fprintln(os.Stderr, "error: no benchmark results parsed")
		os.Exit(1)
	}
	if err := os.WriteFile(*out, []byte(buildReport(results, *count)), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	fmt.Println("report written to", *out)
}
