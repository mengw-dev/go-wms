package middleware

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"gowms/internal/pkg/errcode"
	"gowms/internal/pkg/response"
)

type bodyWriter struct {
	gin.ResponseWriter
	body    *bytes.Buffer
	omitted bool
}

func (w *bodyWriter) Write(chunk []byte) (int, error) {
	const maxAuditBodyBytes = 64 << 10
	if !w.omitted && w.body.Len()+len(chunk) <= maxAuditBodyBytes {
		w.body.Write(chunk)
	} else {
		w.omitted = true
		w.body.Reset()
	}
	return w.ResponseWriter.Write(chunk)
}

func (w *bodyWriter) WriteString(text string) (int, error) { return w.Write([]byte(text)) }

// OperLogRecord 操作日志记录内容。
type OperLogRecord struct {
	UserID   int64
	Username string
	Path     string
	Method   string
	Params   string
	IP       string
	CostMs   int64
	Status   int
	Result   string
}

// OperLogRecorder 由 system 模块实现，异步写库。
type OperLogRecorder interface {
	Record(ctx context.Context, r OperLogRecord)
}

// BodyLimit 限制请求体大小，防止超大 JSON 或上传请求耗尽内存。
func BodyLimit(limitMB int64) gin.HandlerFunc {
	if limitMB <= 0 {
		return func(c *gin.Context) { c.Next() }
	}
	maxBytes := limitMB << 20
	return func(c *gin.Context) {
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxBytes)
		c.Next()
	}
}

// OperLog 操作日志中间件：捕获请求参数与响应结果，异步落库。
func OperLog(rec OperLogRecorder) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.Method == http.MethodGet || c.Request.URL.Path == "/healthz" {
			c.Next()
			return
		}
		start := time.Now()
		bw := &bodyWriter{ResponseWriter: c.Writer, body: bytes.NewBuffer(nil)}
		c.Writer = bw
		params := sanitizeOperLogParams("application/x-www-form-urlencoded", c.Request.URL.RawQuery)
		// 读取请求体用于审计后必须回填，否则 Handler 的 ShouldBindJSON 拿不到参数；
		// multipart（文件上传）不读取，避免破坏请求流。
		if c.Request.Body != nil && !strings.HasPrefix(c.Request.Header.Get("Content-Type"), "multipart/form-data") {
			requestBody, err := io.ReadAll(c.Request.Body)
			_ = c.Request.Body.Close()
			if err != nil {
				// 不能把超限请求的截断内容回填后继续执行写操作。
				var limitErr *http.MaxBytesError
				if errors.As(err, &limitErr) {
					response.Fail(c, errcode.PayloadTooLarge)
					c.Abort()
				} else {
					response.Fail(c, errcode.ParamError)
					c.Abort()
				}
				return
			}
			if len(requestBody) > 0 {
				params = sanitizeOperLogParams(c.GetHeader("Content-Type"), string(requestBody))
			}
			c.Request.Body = io.NopCloser(bytes.NewBuffer(requestBody))
		}

		c.Next()

		if rec != nil {
			responseBody := "[LARGE RESPONSE OMITTED]"
			if !bw.omitted {
				responseBody = sanitizeOperLogParams(c.Writer.Header().Get("Content-Type"), bw.body.String())
			}
			record := OperLogRecord{
				UserID:   UserID(c),
				Username: Username(c),
				Path:     c.Request.URL.Path,
				Method:   c.Request.Method,
				Params:   params,
				IP:       c.ClientIP(),
				CostMs:   time.Since(start).Milliseconds(),
				Status:   bw.Status(),
				Result:   responseBody,
			}
			rec.Record(c.Request.Context(), record)
		}
	}
}
