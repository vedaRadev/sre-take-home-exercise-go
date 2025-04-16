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

var stats = make(map[string]*DomainStats)
var statsMutex sync.Mutex

// TODO Maybe name this something better? It's specifically for managing the concurrency of endpoint
// check requests but the name itself doesn't reflect that.
type ConcurrencyControl struct {
	wg *sync.WaitGroup
	// Used to limit the number of in-flight requests at any given time
	semaphore chan struct{}
}

func checkHealth(endpoint Endpoint, isTimeoutDisabled bool, concurrencyControl *ConcurrencyControl) {
	concurrencyControl.semaphore <- struct{}{}
	defer func(){
		<- concurrencyControl.semaphore
		concurrencyControl.wg.Done()
	}()

	var client = &http.Client{}

	reqCtx := context.Background()
	if !isTimeoutDisabled {
		timeoutCtx, cancel := context.WithTimeout(context.Background(), 500 * time.Millisecond)
		defer cancel()
		reqCtx = timeoutCtx
	}

	reqBody := bytes.NewReader([]byte(endpoint.Body))
	req, err := http.NewRequestWithContext(reqCtx, endpoint.Method, endpoint.URL, reqBody)
	if err != nil {
		log.Printf("%v, %v to %v, error creating request: %v\n", endpoint.Name, endpoint.Method, endpoint.URL, err)
		return
	}

	for key, value := range endpoint.Headers {
		req.Header.Set(key, value)
	}

	sentTime := time.Now()
	resp, err := client.Do(req)
	receivedTime := time.Now()
	reqTime := receivedTime.Sub(sentTime)
	log.Printf("%v, %v to %v responded or was aborted in %v\n", endpoint.Name, endpoint.Method, endpoint.URL, reqTime)

	domain := extractDomain(endpoint.URL)

	statsMutex.Lock()
	// NOTE(RA): Explicitly NOT deferring mutex.Lock() here in favor of manuallying unlocking later
	// because I don't want the mutex to be locked while doing extraneous things like reading body
	// bytes on non-200-range responses, which could cause the  mutex to be locked for way longer
	// than it should be.
	stats[domain].Total++
	if err == nil && resp.StatusCode >= 200 && resp.StatusCode < 300 {
		log.Printf("%v, %v to %v: success\n", endpoint.Name, endpoint.Method, endpoint.URL)
		stats[domain].Success++
		statsMutex.Unlock()
	} else {
		statsMutex.Unlock()
		if err != nil {
			log.Printf("%v, %v to %v failed: %v\n", endpoint.Name, endpoint.Method, endpoint.URL, err)
			return
		}

		log.Printf("%v, %v to %v, response %v. Reading body...\n", endpoint.Name, endpoint.Method, endpoint.URL, resp.StatusCode)
		bodyBytes, err := io.ReadAll(resp.Body)
		if err != nil {
			log.Printf("%v, %v to %v: failed to read body: %v\n", endpoint.Name, endpoint.Method, endpoint.URL, err)
			return
		}
		log.Printf("%v, %v to %v, body: %v\n", endpoint.Name, endpoint.Method, endpoint.URL, string(bodyBytes))
	}
}

func extractDomain(url string) string {
	urlSplit := strings.Split(url, "//")
	domain := strings.Split(urlSplit[len(urlSplit)-1], "/")[0]
	return domain
}

func monitorEndpoints(endpoints []Endpoint, isTimeoutDisabled bool) {
	for _, endpoint := range endpoints {
		domain := extractDomain(endpoint.URL)
		if stats[domain] == nil {
			stats[domain] = &DomainStats{}
		}
	}

	var wg sync.WaitGroup
	// TODO default to 10 or so but allow the the number of goroutines for requests to be passed in via command line
	goroutineLimiter := make(chan struct{}, 10)
	concurrencyControl := ConcurrencyControl {
		wg: &wg,
		semaphore: goroutineLimiter,
	}
	for {
		// TODO need to ensure that checks and logs run every 15 seconds regardless of the previous
		// cycle completion. Right now we're doing our checks then waiting 15 seconds to try again
		// regardless of the time the checks themselves took.
		for _, endpoint := range endpoints {
			wg.Add(1)
			go checkHealth(endpoint, isTimeoutDisabled, &concurrencyControl)
		}
		wg.Wait()
		logResults()
		time.Sleep(15 * time.Second)
	}
}

func logResults() {
	statsMutex.Lock()
	defer statsMutex.Unlock()
	for domain, stat := range stats {
		percentage := int(math.Round(100 * float64(stat.Success) / float64(stat.Total)))
		fmt.Printf("%s has %d%% availability\n", domain, percentage)
	}
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
