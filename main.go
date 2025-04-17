package main

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"strings"
	"time"
	"context"
	"sync"
	"os"
	"strconv"

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
	syncedPrinter.DebugLogf("%v, %v to %v responded or was aborted in %v\n", endpoint.Name, endpoint.Method, endpoint.URL, reqTime)

	domain := extractDomain(endpoint.URL)

	statsMutex.Lock()
	// NOTE(RA): Explicitly NOT deferring mutex.Lock() here in favor of manuallying unlocking later
	// because I don't want the mutex to be locked while doing extraneous things like reading body
	// bytes on non-200-range responses, which could cause the  mutex to be locked for way longer
	// than it should be.
	stats[domain].Total++
	if err == nil && resp.StatusCode >= 200 && resp.StatusCode < 300 {
		syncedPrinter.DebugLogf("%v, %v to %v: success\n", endpoint.Name, endpoint.Method, endpoint.URL)
		stats[domain].Success++
		statsMutex.Unlock()
	} else {
		statsMutex.Unlock()
		if err != nil {
			syncedPrinter.DebugLogf("%v, %v to %v failed: %v\n", endpoint.Name, endpoint.Method, endpoint.URL, err)
			return
		}

		// TODO maybe debugEnabled should just be a global value, or maybe passed in? Seems weird to
		// be checking if the syncedPrinter has _it's_ debug stuff enabled here...
		// Should be okay for now. Will change if we keep doing this in the future.
		if syncedPrinter.debugEnabled {
			// Don't need to use Debug functions here because we already checked above
			syncedPrinter.Logf("%v, %v to %v, response %v. Reading body...\n", endpoint.Name, endpoint.Method, endpoint.URL, resp.StatusCode)
			bodyBytes, err := io.ReadAll(resp.Body)
			if err != nil {
				syncedPrinter.Logf("%v, %v to %v: failed to read body: %v\n", endpoint.Name, endpoint.Method, endpoint.URL, err)
				return
			}
			syncedPrinter.Logf("%v, %v to %v, body: %v\n", endpoint.Name, endpoint.Method, endpoint.URL, string(bodyBytes))
		}
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
func monitorEndpoints(endpoints []Endpoint, isTimeoutDisabled bool, maxConcurrencyPerDomain int) {
	var domainConcurrencyControl = make(map[string]Semaphore)
	for _, endpoint := range endpoints {
		domain := extractDomain(endpoint.URL)
		if stats[domain] == nil {
			stats[domain] = &DomainStats{}
		}
		if domainConcurrencyControl[domain] == nil {
			semaphore, err := NewSemaphore(maxConcurrencyPerDomain)
			if err != nil { log.Fatalf("failed to create semaphore for domain %v: %v", domain, err) }
			domainConcurrencyControl[domain] = semaphore
		}
	}

	for {
		go func() {
			cycleCount += 1
			thisCycleCount := cycleCount
			var wg sync.WaitGroup
			syncedPrinter.DebugLock()
			syncedPrinter.DebugPrintlnBypassLock("==============================")
			syncedPrinter.DebugLogfBypassLock("CHECK CYCLE %v BEGIN\n", thisCycleCount)
			syncedPrinter.DebugPrintlnBypassLock("==============================")
			syncedPrinter.DebugUnlock()

			for _, endpoint := range endpoints {
				wg.Add(1)
				domain := extractDomain(endpoint.URL)
				go checkHealth(endpoint, isTimeoutDisabled, &wg, domainConcurrencyControl[domain])
			}
			wg.Wait()

			syncedPrinter.DebugLock()
			syncedPrinter.DebugPrintlnBypassLock("==============================")
			syncedPrinter.DebugLogfBypassLock("CHECK CYCLE %v END\n", thisCycleCount)
			syncedPrinter.DebugPrintlnBypassLock("==============================")
			syncedPrinter.DebugUnlock()

			logResults()
		}()

		time.Sleep(15 * time.Second)
	}
}

func logResults() {
	statsMutex.Lock()
	syncedPrinter.Lock()
	defer statsMutex.Unlock()
	defer syncedPrinter.Unlock()
	syncedPrinter.PrintlnBypassLock("==============================")
	syncedPrinter.LogfBypassLock("AVAILABILITY REPORT")
	for domain, stat := range stats {
		percentage := int(math.Round(100 * float64(stat.Success) / float64(stat.Total)))
		syncedPrinter.PrintfBypassLock("%s has %d%% availability\n", domain, percentage)
	}
	syncedPrinter.PrintlnBypassLock("==============================")
}

func main() {
	var filePath string
	var isTimeoutDisabled bool
	var isDebugEnabled bool
	maxConcurrencyPerDomain := 10

	filePathParsed := false
	args := os.Args[1:]
	for argIdx := 0; argIdx < len(args); argIdx++ {
		arg := args[argIdx]
		if strings.HasPrefix(arg, "--") {
			switch arg[2:] {

			case "no-req-timeout":
				isTimeoutDisabled = true

			case "debug-logs":
				isDebugEnabled = true

			case "max-domain-concurrency":
				argIdx++
				if argIdx < len(args) {
					val, err := strconv.Atoi(args[argIdx])
					if err != nil {
						log.Fatalf("invalid max-domain-concurrency value, expected integer but got %v", args[argIdx])
					}
					if val <= 0 {
						log.Fatal("max-domain-concurrency must be greater than 0")
					}
					maxConcurrencyPerDomain = val
				} else {
					log.Fatal("max-domain-concurrency requires an integer value")
				}

			default:
				log.Fatal("unrecognized flag: ", arg)

			}
		} else if !filePathParsed {
			filePathParsed = true
			filePath = arg
		} else {
			log.Fatal("multiple config files is not supported")
		}
	}

	if filePath == "" {
		log.Fatal("Usage: go run main.go <config_file> [--no-req-timeout] [--debug-logs] [--max-domain-concurrency <int>]")
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		log.Fatal("Error reading file:", err)
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

	syncedPrinter.debugEnabled = isDebugEnabled
	monitorEndpoints(endpoints, isTimeoutDisabled, maxConcurrencyPerDomain)
}
