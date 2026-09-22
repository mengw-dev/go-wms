// Package dto 定义 AI 问答 HTTP 接口的请求和响应结构。
package dto

// ChatReq 提问请求；question 非空且不超过 200 字符。
type ChatReq struct {
	Question string `json:"question" binding:"required,max=200"`
}

// ChatResp AI 回答（纯文本，前端按纯文本渲染防 XSS）。
type ChatResp struct {
	Answer string `json:"answer"`
}
