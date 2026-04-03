package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

const (
	baseURL     = "http://127.0.0.1:8000"
	testUID     = "1001"
	concurrency = 20 // 模拟并发数
	duration    = 10 * time.Second
)

type Result struct {
	Duration time.Duration
	Err      error
	Status   int
}

func main() {
	fmt.Printf("🚀 开始压测...\n基础地址: %s, 并发数: %d, 持续时间: %v\n", baseURL, concurrency, duration)

	// 测试路径
	endpoints := []struct {
		name   string
		path   string
		method string
		body   func() []byte
	}{
		{
			name:   "查余额 (Read-Heavy)",
			path:   "/v1/wallet/balance",
			method: "GET",
			body:   func() []byte { return nil },
		},
		{
			name:   "支付宝通知 (Write-Heavy)",
			path:   "/v1/payments/alipay/notify",
			method: "POST",
			body: func() []byte {
				// 模拟不同的 notify_id 以触发并发事务
				payload := map[string]string{
					"signature": "MOCK_SIGN",
					"notify_id": fmt.Sprintf("stress_%d", time.Now().UnixNano()),
					"body":      fmt.Sprintf("{\"notify_id\":\"ALI_%d\",\"out_trade_no\":\"STRESS_OTN\",\"trade_status\":\"TRADE_SUCCESS\"}", time.Now().UnixNano()),
				}
				b, _ := json.Marshal(payload)
				return b
			},
		},
	}

	for _, ep := range endpoints {
		runTest(ep.name, ep.path, ep.method, ep.body)
	}
}

func runTest(name, path, method string, bodyFunc func() []byte) {
	fmt.Printf("\n--- 正在测试: %s ---\n", name)
	results := make(chan Result, 100000)
	ctx, cancel := context.WithTimeout(context.Background(), duration)
	defer cancel()

	var wg sync.WaitGroup
	start := time.Now()

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			client := &http.Client{Timeout: 5 * time.Second}
			for {
				select {
				case <-ctx.Done():
					return
				default:
					reqStart := time.Now()
					req, _ := http.NewRequest(method, baseURL+path, bytes.NewBuffer(bodyFunc()))
					// 使用我们的亮点功能：Debug UID 绕过登录
					req.Header.Set("x-debug-uid", testUID)
					req.Header.Set("Content-Type", "application/json")

					resp, err := client.Do(req)
					status := 0
					if err == nil {
						status = resp.StatusCode
						_, _ = io.Copy(io.Discard, resp.Body)
						resp.Body.Close()
					}
					results <- Result{
						Duration: time.Since(reqStart),
						Err:      err,
						Status:   status,
					}
				}
			}
		}()
	}

	go func() {
		wg.Wait()
		close(results)
	}()

	var count, success int
	var totalDur time.Duration
	for res := range results {
		count++
		if res.Err == nil && res.Status == 200 {
			success++
		}
		totalDur += res.Duration
	}

	elapsed := time.Since(start)
	qps := float64(count) / elapsed.Seconds()
	avgLat := totalDur.Seconds() / float64(count) * 1000

	fmt.Printf("执行结果:\n")
	fmt.Printf("  总请求数: %d\n", count)
	fmt.Printf("  成功请求: %d (成功率 %.2f%%)\n", success, float64(success)/float64(count)*100)
	fmt.Printf("  平均 QPS: %.2f\n", qps)
	fmt.Printf("  平均延迟: %.2f ms\n", avgLat)
}
