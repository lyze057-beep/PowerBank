package main

import (
	"bytes"
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
	concurrency = 2 // 对 AI 接口，2 并发已经很大了
	testCount   = 4 // 总共测 4 次对话
)

func main() {
	fmt.Printf("🤖 开始 AI 客服专项压测...\n基础地址: %s, 并发数: %d\n", baseURL, concurrency)

	var wg sync.WaitGroup
	start := time.Now()

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			client := &http.Client{Timeout: 60 * time.Second}
			for j := 0; j < testCount/concurrency; j++ {
				reqStart := time.Now()
				payload := map[string]string{
					"session_id": "stress_session_1",
					"content":    "帮我看看账户余额还有多少？",
					"client_req_id": fmt.Sprintf("ai_stress_%d_%d", id, j),
				}
				b, _ := json.Marshal(payload)
				
				req, _ := http.NewRequest("POST", baseURL+"/v1/support/chat:send", bytes.NewBuffer(b))
				req.Header.Set("x-debug-uid", testUID)
				req.Header.Set("Content-Type", "application/json")

				fmt.Printf("[协程 %d] 正在发起第 %d 轮 AI 提问...\n", id, j+1)
				resp, err := client.Do(req)
				
				if err != nil {
					fmt.Printf("[协程 %d] 出错: %v\n", id, err)
					continue
				}

				body, _ := io.ReadAll(resp.Body)
				resp.Body.Close()

				fmt.Printf("[协程 %d] AI 回复耗时: %v, 状态: %d\n", id, time.Since(reqStart), resp.StatusCode)
				if resp.StatusCode == 200 {
					var result map[string]interface{}
					json.Unmarshal(body, &result)
					fmt.Printf("[协程 %d] AI 回复内容概要: %s\n", id, result["reply"])
				}
			}
		}(i)
	}

	wg.Wait()
	fmt.Printf("\n--- AI 专项压测完成 ---\n总耗时: %v, 平均每轮耗时: %v\n", time.Since(start), time.Since(start)/time.Duration(testCount))
}
