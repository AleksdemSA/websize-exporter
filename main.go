package main

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	pageSizeGauge = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "website_page_size_bytes",
			Help: "Size of the website page in bytes",
		},
		[]string{"url"},
	)
)

func init() {
	prometheus.MustRegister(pageSizeGauge)
}

func readSites(filename string) ([]string, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to open file %s: %w", filename, err)
	}
	defer file.Close()

	return parseSites(file), nil
}

func parseSites(r io.Reader) []string {
	var sites []string
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" && !strings.HasPrefix(line, "#") {
			sites = append(sites, line)
		}
	}
	return sites
}

func checkPageSize(url string, client *http.Client) {
	resp, err := client.Get(url)
	if err != nil {
		log.Printf("Error fetching URL %s: %v\n", url, err)
		pageSizeGauge.WithLabelValues(url).Set(0)
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("Error reading response body from URL %s: %v\n", url, err)
		pageSizeGauge.WithLabelValues(url).Set(0)
		return
	}

	pageSize := len(body)
	log.Printf("Fetched URL %s, size: %d bytes\n", url, pageSize)
	pageSizeGauge.WithLabelValues(url).Set(float64(pageSize))
}

func monitorPages(urls []string, interval time.Duration) {
	client := &http.Client{Timeout: 10 * time.Second}
	for {
		var wg sync.WaitGroup
		for _, url := range urls {
			wg.Add(1)
			go func(url string) {
				defer wg.Done()
				checkPageSize(url, client)
			}(url)
		}
		wg.Wait()
		time.Sleep(interval)
	}
}

func main() {
	const sitesFile = "sites.txt"
	urls, err := readSites(sitesFile)
	if err != nil {
		log.Fatalf("Failed to load sites: %v\n", err)
	}
	if len(urls) == 0 {
		log.Fatalf("No URLs found in %s\n", sitesFile)
	}

	const checkInterval = 30 * time.Second

	registry := prometheus.NewRegistry()

	registry.MustRegister(pageSizeGauge)

	http.Handle("/metrics", promhttp.HandlerFor(registry, promhttp.HandlerOpts{}))

	go monitorPages(urls, checkInterval)

	const port = 9222
	fmt.Printf("Starting exporter on :%d\n", port)
	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", port), nil))
}
