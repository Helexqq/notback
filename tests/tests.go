package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

const (
	baseURL          = "http://localhost:8080"
	checkoutEndpoint = "/checkout"
	purchaseEndpoint = "/purchase"
	maxItems         = 10000
	maxItemsPerUser  = 10
	numUsers         = 1000
)

var (
	mode   = flag.String("mode", "full", "Test mode: full, sim")
	client = &http.Client{Timeout: 5 * time.Second}
)

var (
	totalRequests       uint64
	failureRequests     uint64
	totalLatencyNano    uint64
	successfulCheckouts uint64
	successfulPurchases uint64

	allLatenciesMutex sync.Mutex
	allLatencies      []time.Duration
)

func main() {
	flag.Parse()

	switch *mode {
	case "full":
		runFullPurchase()
	case "sim":
		runSimulation()
	default:
		log.Fatalf("Unknown mode: %s", *mode)
	}
}

func recordMetrics(latency time.Duration, success bool) {
	atomic.AddUint64(&totalRequests, 1)
	atomic.AddUint64(&totalLatencyNano, uint64(latency.Nanoseconds()))
	if !success {
		atomic.AddUint64(&failureRequests, 1)
	}

	allLatenciesMutex.Lock()
	allLatencies = append(allLatencies, latency)
	allLatenciesMutex.Unlock()
}

func printMetrics(totalElapsed time.Duration) {
	total := atomic.LoadUint64(&totalRequests)
	failure := atomic.LoadUint64(&failureRequests)
	latencyNano := atomic.LoadUint64(&totalLatencyNano)
	successCheckouts := atomic.LoadUint64(&successfulCheckouts)
	successPurchases := atomic.LoadUint64(&successfulPurchases)

	avgLatency := time.Duration(0)
	if total > 0 {
		avgLatency = time.Duration(latencyNano / total)
	}

	rps := float64(0)
	if totalElapsed.Seconds() > 0 {
		rps = float64(total) / totalElapsed.Seconds()
	}

	errorRate := float64(0)
	if total > 0 {
		errorRate = (float64(failure) / float64(total)) * 100
	}

	// Calculate percentiles
	p95 := time.Duration(0)
	p99 := time.Duration(0)

	allLatenciesMutex.Lock()
	latenciesCopy := make([]time.Duration, len(allLatencies))
	copy(latenciesCopy, allLatencies)
	allLatenciesMutex.Unlock()

	if len(latenciesCopy) > 0 {
		sort.Slice(latenciesCopy, func(i, j int) bool {
			return latenciesCopy[i] < latenciesCopy[j]
		})

		p95Index := int(float64(len(latenciesCopy)) * 0.95)
		p99Index := int(float64(len(latenciesCopy)) * 0.99)

		if p95Index >= len(latenciesCopy) {
			p95Index = len(latenciesCopy) - 1
		}
		if p99Index >= len(latenciesCopy) {
			p99Index = len(latenciesCopy) - 1
		}

		p95 = latenciesCopy[p95Index]
		p99 = latenciesCopy[p99Index]
	}

	fmt.Printf("Total Requests: %d\n", total)
	fmt.Printf("Successful Checkouts: %d\n", successCheckouts)
	fmt.Printf("Successful Purchases: %d\n", successPurchases)
	fmt.Printf("Failed Requests: %d\n", failure)
	fmt.Printf("Requests Per Second (RPS): %.2f\n", rps)
	fmt.Printf("Error Rate: %.2f%%\n", errorRate)
	fmt.Printf("Average Latency: %v\n", avgLatency)
	fmt.Printf("P95 Latency: %v\n", p95)
	fmt.Printf("P99 Latency: %v\n", p99)
}

func runFullPurchase() {
	fmt.Println("Starting Full Purchase Test")

	startTotal := time.Now()

	for itemID := 1; itemID <= maxItems; itemID++ {
		userID := fmt.Sprintf("user_%d", itemID)

		code := checkoutAndGetCode(userID, itemID)

		if code != "-1" {
			atomic.AddUint64(&successfulCheckouts, 1)
		} else {
			continue
		}

		success := purchaseWithCode(code)
		if success {
			atomic.AddUint64(&successfulPurchases, 1)
		}

		if itemID%1000 == 0 {
			fmt.Printf("Processed %d items\n", itemID)
		}
	}

	fmt.Println("\nAttempting to checkout and purchase one more item beyond maxItems...")
	userID := fmt.Sprintf("user_%d", maxItems+1)
	code := checkoutAndGetCode(userID, maxItems+1)
	if code != "-1" {
		fmt.Printf("ERROR: Should not be able to checkout after all items are sold. Code: %s\n", code)
		if purchaseWithCode(code) {
			fmt.Printf("ERROR: Should not be able to purchase after all items are sold.\n")
		}
	} else {
		fmt.Printf("SUCCESS: Correctly prevented checkout after all items are sold.\n")
	}

	totalElapsed := time.Since(startTotal)
	printMetrics(totalElapsed)
	fmt.Println("Full Purchase test completed.")
}

func checkoutAndGetCode(user string, item int) string {
	url := fmt.Sprintf("%s%s?user_id=%s&id=item_%d", baseURL, checkoutEndpoint, user, item)
	fmt.Printf("Sending checkout request to URL: %s\n", url)

	start := time.Now()

	emptyBody := bytes.NewBufferString("{}")
	resp, err := client.Post(url, "application/json", emptyBody)

	latency := time.Since(start)
	recordMetrics(latency, err == nil && resp.StatusCode == http.StatusOK)

	if err != nil {
		fmt.Printf("Checkout request FAILED due to network/client error: %v\n", err)
		return "-1"
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Printf("Error reading checkout response body: %v\n", err)
		return "-1"
	}

	responseBodyStr := string(body)
	if resp.StatusCode != http.StatusOK {
		fmt.Printf("Checkout FAILED: status %d, response body: %s\n", resp.StatusCode, responseBodyStr)
		return "-1"
	}

	var res struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal(body, &res); err != nil {
		fmt.Printf("Failed to unmarshal checkout JSON response: %v, body: %s\n", err, responseBodyStr)
		return "-1"
	}

	fmt.Printf("Checkout SUCCESSFUL. Generated code: %s\n", res.Code)
	return res.Code
}

func purchaseWithCode(code string) bool {
	url := fmt.Sprintf("%s%s?code=%s", baseURL, purchaseEndpoint, code)
	fmt.Printf("Attempting purchase for code: %s, URL: %s\n", code, url)

	start := time.Now()

	emptyBody := bytes.NewBufferString("{}")
	resp, err := client.Post(url, "application/json", emptyBody)

	latency := time.Since(start)
	recordMetrics(latency, err == nil && resp.StatusCode == http.StatusOK)

	if err != nil {
		fmt.Printf("Purchase request FAILED due to network/client error for code %s: %v\n", code, err)
		return false
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Printf("Error reading purchase response body for code %s: %v\n", code, err)
		return false
	}

	responseBodyStr := string(body)
	if resp.StatusCode != http.StatusOK {
		fmt.Printf("Purchase FAILED for code %s: status %d, response body: %s\n", code, resp.StatusCode, responseBodyStr)
		return false
	}

	fmt.Printf("Purchase SUCCESSFUL for code %s. Response status: %d, body: %s\n", code, resp.StatusCode, responseBodyStr)
	return true
}

func runSimulation() {
	fmt.Println("Starting User Simulation test.")
	var wg sync.WaitGroup
	var counter int32 = 0

	startTotal := time.Now()

	for u := 0; u < numUsers; u++ {
		wg.Add(1)
		go func(uid int) {
			defer wg.Done()
			userID := fmt.Sprintf("user_%d", uid)

			for i := 0; i < maxItemsPerUser; i++ {
				itemNum := atomic.AddInt32(&counter, 1)
				itemID := int(itemNum % maxItems)

				code := checkoutAndGetCode(userID, itemID)

				if code != "-1" {
					atomic.AddUint64(&successfulCheckouts, 1)
					success := purchaseWithCode(code)
					if success {
						atomic.AddUint64(&successfulPurchases, 1)
					}
				}

				time.Sleep(time.Millisecond * time.Duration(10))
			}
		}(u)
	}
	wg.Wait()

	totalElapsed := time.Since(startTotal)
	printMetrics(totalElapsed)
	fmt.Println("User Simulation test completed")
}
