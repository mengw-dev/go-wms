package middleware

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"gowms/internal/pkg/errcode"
	"gowms/internal/pkg/jwt"
	"gowms/internal/pkg/log"
	"gowms/internal/pkg/response"
)

// context key 与取值辅助。
type ctxKey string

const (
	ctxRequestID ctxKey = "request_id"
	ctxUserID    ctxKey = "user_id"
	ctxUsername  ctxKey = "username"
)

func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.GetHeader("X-Request-ID")
		if id == "" {
			id = newRequestID()
		}
		c.Set(string(ctxRequestID), id)
		c.Writer.Header().Set("X-Request-ID", id)
		c.Request = c.Request.WithContext(log.WithRequestID(c.Request.Context(), id))
		c.Next()
	}
}

func UserID(c *gin.Context) int64 {
	v, _ := c.Get(string(ctxUserID))
	id, _ := v.(int64)
	return id
}

func Username(c *gin.Context) string {
	v, _ := c.Get(string(ctxUsername))
	s, _ := v.(string)
	return s
}

func RequestIDOf(c *gin.Context) string {
	v, _ := c.Get(string(ctxRequestID))
	s, _ := v.(string)
	return s
}

// AuthValidator 在签名校验后复核用户状态和 Token 版本，使禁用用户/改密后的旧 Token 立即失效。
type AuthValidator interface {
	ValidateToken(ctx context.Context, userID int64, tokenVersion int) error
}

// Auth JWT 鉴权：解析 Bearer Token，复核用户状态，注入 userID/username。
func Auth(secret string, validator AuthValidator) gin.HandlerFunc {
	return func(c *gin.Context) {
		token := c.GetHeader("Authorization")
		if len(token) > 7 && token[:7] == "Bearer " {
			token = token[7:]
		} else {
			response.Fail(c, errcode.Unauthorized)
			c.Abort()
			return
		}
		claims, err := jwt.Parse(secret, token)
		if err != nil {
			response.Fail(c, errcode.Unauthorized)
			c.Abort()
			return
		}
		if validator != nil {
			if err := validator.ValidateToken(c.Request.Context(), claims.UserID, claims.TokenVersion); err != nil {
				response.Fail(c, err)
				c.Abort()
				return
			}
		}
		c.Set(string(ctxUserID), claims.UserID)
		c.Set(string(ctxUsername), claims.Username)
		c.Request = c.Request.WithContext(log.WithUserID(c.Request.Context(), claims.UserID))
		c.Next()
	}
}

// PermsChecker 由 system 模块实现，供 Permission 中间件校验权限。
type PermsChecker interface {
	HasPerm(ctx context.Context, userID int64, perm string) bool
}

// Permission 权限校验中间件。
func Permission(checker PermsChecker, perm string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !checker.HasPerm(c.Request.Context(), UserID(c), perm) {
			response.Fail(c, errcode.Forbidden)
			c.Abort()
			return
		}
		c.Next()
	}
}

// Recovery panic 兜底，防止进程退出。
func Recovery() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if r := recover(); r != nil {
				log.WithContext(c.Request.Context()).Error("gin panic recovered",
					"err", r, "path", c.Request.URL.Path)
				response.Fail(c, errcode.Internal)
				c.Abort()
			}
		}()
		c.Next()
	}
}

type bodyWriter struct {
	gin.ResponseWriter
	body *bytes.Buffer
}

func (w *bodyWriter) Write(b []byte) (int, error) {
	w.body.Write(b)
	return w.ResponseWriter.Write(b)
}

// AccessLog 访问日志：方法/路径/状态/耗时/操作人，>500ms 打 warn。
func AccessLog() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		bw := &bodyWriter{ResponseWriter: c.Writer, body: bytes.NewBuffer(nil)}
		c.Writer = bw

		c.Next()

		cost := time.Since(start).Milliseconds()
		attrs := []any{
			"method", c.Request.Method,
			"path", c.Request.URL.Path,
			"status", bw.Status(),
			"cost_ms", cost,
			"ip", c.ClientIP(),
			"request_id", RequestIDOf(c),
			"user_id", UserID(c),
		}
		l := log.WithContext(c.Request.Context())
		if cost > 500 {
			l.Warn("slow request", attrs...)
		} else {
			l.Info("access", attrs...)
		}
	}
}

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
		params := c.Request.URL.RawQuery
		// 读取请求体用于审计后必须回填，否则 Handler 的 ShouldBindJSON 拿不到参数；
		// multipart（文件上传）不读取，避免破坏请求流。
		if c.Request.Body != nil && !strings.HasPrefix(c.Request.Header.Get("Content-Type"), "multipart/form-data") {
			data, _ := io.ReadAll(c.Request.Body)
			_ = c.Request.Body.Close()
			if len(data) > 0 {
				params = string(data)
				if len(params) > 2048 { // 仅截断日志，回填用完整 data
					params = params[:2048]
				}
			}
			c.Request.Body = io.NopCloser(bytes.NewBuffer(data))
		}

		c.Next()

		if rec != nil {
			r := OperLogRecord{
				UserID:   UserID(c),
				Username: Username(c),
				Path:     c.Request.URL.Path,
				Method:   c.Request.Method,
				Params:   params,
				IP:       c.ClientIP(),
				CostMs:   time.Since(start).Milliseconds(),
				Status:   bw.Status(),
				Result:   bw.body.String(),
			}
			if len(r.Result) > 1024 {
				r.Result = r.Result[:1024]
			}
			go rec.Record(context.Background(), r)
		}
	}
}

// CORS 跨域。
func CORS() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET,POST,PUT,DELETE,OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Authorization,Content-Type,X-Request-ID")
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}

func newRequestID() string {
	return time.Now().Format("20060102150405") + randHex()
}
