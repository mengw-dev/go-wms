// Package client 封装兼容 OpenAI 协议的智谱 Chat API 调用。
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Message OpenAI 兼容消息结构。
type Message struct {
	Role    string `json:"role"` // system / user / assistant
	Content string `json:"content"`
}

// APIError 智谱接口返回的业务错误（HTTP 非 2xx 时携带错误码，如 1305 拥堵 / 429 限流）。
type APIError struct {
	HTTPStatus int
	Code       string
	Message    string
}

func (e *APIError) Error() string {
	if e.Code != "" {
		return fmt.Sprintf("zhipu api error: http=%d code=%s message=%s", e.HTTPStatus, e.Code, e.Message)
	}
	return fmt.Sprintf("zhipu api error: http=%d message=%s", e.HTTPStatus, e.Message)
}

// Client 智谱 BigModel Chat 客户端（OpenAI 兼容 /chat/completions 接口）。
type Client struct {
	httpClient *http.Client
	baseURL    string
	apiKey     string
}

// New 创建客户端；timeoutSeconds 为单次调用超时。
func New(apiKey, baseURL string, timeoutSeconds int) *Client {
	if timeoutSeconds <= 0 {
		timeoutSeconds = 30
	}
	return &Client{
		httpClient: &http.Client{Timeout: time.Duration(timeoutSeconds) * time.Second},
		baseURL:    strings.TrimRight(baseURL, "/"),
		apiKey:     apiKey,
	}
}

type chatRequest struct {
	Model       string          `json:"model"`
	Messages    []Message       `json:"messages"`
	Temperature float64         `json:"temperature"`
	MaxTokens   int             `json:"max_tokens"`
	Stream      bool            `json:"stream"`
	Thinking    *thinkingConfig `json:"thinking,omitempty"` // GLM-4.5+/4.7 混合推理模型的思维链开关
}

// thinkingConfig 智谱 thinking 参数。库存问答是"基于给定数据的事实型问答"，
// 无需思维链：不禁用时 glm-4.7-flash 会把 max_tokens 耗在推理上，返回空 content。
// 旧模型（如 glm-4-flash）不识别该参数，实测传 disabled 亦兼容（忽略）。
type thinkingConfig struct {
	Type string `json:"type"` // enabled / disabled
}

type chatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Error *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

// Chat 调用指定模型生成回复，返回 choices[0].message.content。
// 主备模型降级由上层 service 控制，这里只负责单次调用。
func (c *Client) Chat(ctx context.Context, model string, messages []Message, temperature float64, maxTokens int) (string, error) {
	reqBody, err := json.Marshal(chatRequest{
		Model: model, Messages: messages,
		Temperature: temperature, MaxTokens: maxTokens, Stream: false,
		Thinking: &thinkingConfig{Type: "disabled"},
	})
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/chat/completions", bytes.NewReader(reqBody))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("zhipu api request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	responseBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", fmt.Errorf("zhipu api read body: %w", err)
	}

	var response chatResponse
	if err := json.Unmarshal(responseBody, &response); err != nil {
		return "", &APIError{HTTPStatus: resp.StatusCode, Message: strings.TrimSpace(string(responseBody))}
	}
	if resp.StatusCode != http.StatusOK {
		apiErr := &APIError{HTTPStatus: resp.StatusCode}
		if response.Error != nil {
			apiErr.Code = response.Error.Code
			apiErr.Message = response.Error.Message
		}
		return "", apiErr
	}
	if response.Error != nil {
		// 个别错误在 HTTP 200 下仍返回 error 字段
		return "", &APIError{HTTPStatus: resp.StatusCode, Code: response.Error.Code, Message: response.Error.Message}
	}
	if len(response.Choices) == 0 {
		return "", fmt.Errorf("zhipu api empty choices")
	}
	content := strings.TrimSpace(response.Choices[0].Message.Content)
	if content == "" {
		return "", fmt.Errorf("zhipu api empty content")
	}
	return content, nil
}
