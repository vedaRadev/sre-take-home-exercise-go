package main

import (
	"bytes"
	"fmt"
	// TODO remove io/ioutil in favor of just io
	"io/ioutil"
	"io"
	"log"
	"math"
	"net/http"
	"os"
	"strings"
	"time"
	"context"
	"sync"

	"gopkg.in/yaml.v3"
)

type Endpoint struct {
	Name    string            `yaml:"name"`
	URL     string            `yaml:"url"`
	Method  string            `yaml:"method"`
	Headers map[string]string `yaml:"headers"`
	Body    string            `yaml:"body"`
}

type DomainStats struct {
	Success int
	Total   int
}

type Semaphore chan struct{}
func (s Semaphore) Down() { s <- struct{}{} }
func (s Semaphore) Up() { <- s }
func NewSemaphore(capacity int) (Semaphore, error) {
	if capacity <= 0 { return nil, fmt.Errorf("semaphore capacity must be greater than 0") }
	return make(Semaphore, capacity), nil
}

type SyncedPrinter struct  { mu sync.Mutex }

func (tsl *SyncedPrinter) Logf(format string, params ...any) {
	tsl.mu.Lock()
	defer tsl.mu.Unlock()
	log.Printf(format, params...)
}

func (tsl *SyncedPrinter) LogfBypassLock(format string, params ...any) {
	log.Printf(format, params...)
}

func (tsl *SyncedPrinter) LogLine(line string) {
	tsl.mu.Lock()
	defer tsl.mu.Unlock()
	log.Println(line)
}

func (tsl *SyncedPrinter) LogLineBypassLock(line string) {
	log.Println(line)
}

func (tsl *SyncedPrinter) Printf(format string, params ...any) {
	tsl.mu.Lock()
	defer tsl.mu.Unlock()
	fmt.Printf(format, params...)
}

func (tsl *SyncedPrinter) PrintfBypassLock(format string, params ...any) {
	fmt.Printf(format, params...)
}

func (tsl *SyncedPrinter) Println(line string) {
	tsl.mu.Lock()
	defer tsl.mu.Unlock()
	fmt.Println(line)
}

func (tsl *SyncedPrinter) PrintlnBypassLock(line string) {
	fmt.Println(line)
}


// TODO default to 10 or so but allow the the number of goroutines per domain for requests to be passed in via command line
const MAX_CONCURRENCY_PER_DOMAIN int = 10
const REQUEST_TIMEOUT_DURATION time.Duration = 500 * time.Millisecond
var stats = make(map[string]*DomainStats)
var statsMutex sync.Mutex
var syncedPrinter SyncedPrinter

func checkHealth(endpoint Endpoint, isTimeoutDisabled bool, wg *sync.WaitGroup, semaphore Semaphore) {
	semaphore.Down()
	defer semaphore.Up()
	defer wg.Done()

	var client = &http.Client{}

	reqCtx := context.Background()
	if !isTimeoutDisabled {
		timeoutCtx, cancel := context.WithTimeout(context.Background(), REQUEST_TIMEOUT_DURATION)
		defer cancel()
		reqCtx = timeoutCtx
	}

	reqBody := bytes.NewReader([]byte(endpoint.Body))
	req, err := http.NewRequestWithContext(reqCtx, endpoint.Method, endpoint.URL, reqBody)
	if err != nil {
		syncedPrinter.Logf("%v, %v to %v, error creating request: %v\n", endpoint.Name, endpoint.Method, endpoint.URL, err)
		return
	}

	for key, value := range endpoint.Headers {
		req.Header.Set(key, value)
	}

	sentTime := time.Now()
	resp, err := client.Do(req)
	receivedTime := time.Now()
	reqTime := receivedTime.Sub(sentTime)
	syncedPrinter.Logf("%v, %v to %v responded or was aborted in %v\n", endpoint.Name, endpoint.Method, endpoint.URL, reqTime)

	domain := extractDomain(endpoint.URL)

	statsMutex.Lock()
	// NOTE(RA): Explicitly NOT deferring mutex.Lock() here in favor of manuallying unlocking later
	// because I don't want the mutex to be locked while doing extraneous things like reading body
	// bytes on non-200-range responses, which could cause the  mutex to be locked for way longer
	// than it should be.
	stats[domain].Total++
	if err == nil && resp.StatusCode >= 200 && resp.StatusCode < 300 {
		syncedPrinter.Logf("%v, %v to %v: success\n", endpoint.Name, endpoint.Method, endpoint.URL)
		stats[domain].Success++
		statsMutex.Unlock()
	} else {
		statsMutex.Unlock()
		if err != nil {
			syncedPrinter.Logf("%v, %v to %v failed: %v\n", endpoint.Name, endpoint.Method, endpoint.URL, err)
			return
		}

		syncedPrinter.Logf("%v, %v to %v, response %v. Reading body...\n", endpoint.Name, endpoint.Method, endpoint.URL, resp.StatusCode)
		bodyBytes, err := io.ReadAll(resp.Body)
		if err != nil {
			syncedPrinter.Logf("%v, %v to %v: failed to read body: %v\n", endpoint.Name, endpoint.Method, endpoint.URL, err)
			return
		}
		syncedPrinter.Logf("%v, %v to %v, body: %v\n", endpoint.Name, endpoint.Method, endpoint.URL, string(bodyBytes))
	}
}

func extractDomain(url string) string {
	urlSplit := strings.Split(url, "//")
	domain := strings.Split(urlSplit[len(urlSplit)-1], "/")[0]
	// domain names can't contain colons, so assume that everything after a colon
	// is a port number and just get rid of it
	colonIndex := strings.IndexByte(domain, ':')
	if colonIndex != -1 { domain = domain[:colonIndex] }
	return domain
}

var cycleCount int
func monitorEndpoints(endpoints []Endpoint, isTimeoutDisabled bool) {
	var domainConcurrencyControl = make(map[string]Semaphore)
	for _, endpoint := range endpoints {
		domain := extractDomain(endpoint.URL)
		if stats[domain] == nil {
			stats[domain] = &DomainStats{}
		}
		if domainConcurrencyControl[domain] == nil {
			semaphore, err := NewSemaphore(MAX_CONCURRENCY_PER_DOMAIN)
			if err != nil { log.Fatalf("failed to create semaphore for domain %v: %v", domain, err) }
			domainConcurrencyControl[domain] = semaphore
		}
	}

	for {
		go func() {
			cycleCount += 1
			thisCycleCount := cycleCount
			var wg sync.WaitGroup
			syncedPrinter.mu.Lock()
			syncedPrinter.PrintlnBypassLock("==============================")
			syncedPrinter.LogfBypassLock("CHECK CYCLE %v BEGIN\n", thisCycleCount)
			syncedPrinter.PrintlnBypassLock("==============================")
			syncedPrinter.mu.Unlock()
			for _, endpoint := range endpoints {
				wg.Add(1)
				domain := extractDomain(endpoint.URL)
				go checkHealth(endpoint, isTimeoutDisabled, &wg, domainConcurrencyControl[domain])
			}
			wg.Wait()
			syncedPrinter.mu.Lock()
			syncedPrinter.PrintlnBypassLock("==============================")
			syncedPrinter.LogfBypassLock("CHECK CYCLE %v END\n", thisCycleCount)
			syncedPrinter.PrintlnBypassLock("==============================")
			syncedPrinter.mu.Unlock()
			logResults()
		}()

		time.Sleep(15 * time.Second)
	}
}

func logResults() {
	statsMutex.Lock()
	syncedPrinter.mu.Lock()
	defer statsMutex.Unlock()
	defer syncedPrinter.mu.Unlock()
	syncedPrinter.PrintlnBypassLock("==============================")
	syncedPrinter.LogfBypassLock("AVAILABILITY REPORT")
	for domain, stat := range stats {
		percentage := int(math.Round(100 * float64(stat.Success) / float64(stat.Total)))
		syncedPrinter.PrintfBypassLock("%s has %d%% availability\n", domain, percentage)
	}
	syncedPrinter.PrintlnBypassLock("==============================")
}

func main() {
	if len(os.Args) < 2 {
		log.Fatal("Usage: go run main.go <config_file> [--no-req-timeout]")
	}

	filePath := os.Args[1]
	// TODO ioutil is deprecated as of Go 1.16. Use os.ReadFile instead.
	data, err := ioutil.ReadFile(filePath)
	if err != nil {
		log.Fatal("Error reading file:", err)
	}

	// TODO If I end up adding more options, do more advanced arg parsing
	isTimeoutDisabled := false
	if len(os.Args) == 3 {
		if os.Args[2] != "--no-req-timeout" {
			log.Fatal("Error, unrecognized option:", os.Args[3])
		}
		isTimeoutDisabled = true
	}

	var endpoints []Endpoint
	if err := yaml.Unmarshal(data, &endpoints); err != nil {
		log.Fatal("Error parsing YAML:", err)
	}

	// Default values as required
	for i := range endpoints {
		endpoint := &endpoints[i]
		if endpoint.Method == "" { endpoint.Method = "GET" }
	}

	monitorEndpoints(endpoints, isTimeoutDisabled)
}
