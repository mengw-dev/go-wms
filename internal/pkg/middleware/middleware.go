package middleware

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"runtime/debug"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"gowms/internal/pkg/errcode"
	"gowms/internal/pkg/jwt"
	"gowms/internal/pkg/log"
	"gowms/internal/pkg/response"
	"gowms/internal/pkg/tenant"
)

// context key 与取值辅助。
type ctxKey string

const (
	ctxRequestID ctxKey = "request_id"
	ctxUserID    ctxKey = "user_id"
	ctxUsername  ctxKey = "username"
	ctxTenantID  ctxKey = "tenant_id"
)

func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := normalizeRequestID(c.GetHeader("X-Request-ID"))
		if id == "" {
			id = newRequestID()
		}
		c.Set(string(ctxRequestID), id)
		c.Writer.Header().Set("X-Request-ID", id)
		c.Request = c.Request.WithContext(log.WithRequestID(c.Request.Context(), id))
		c.Next()
	}
}

// UserID returns the authenticated user ID stored by Auth.
func UserID(c *gin.Context) int64 {
	value, _ := c.Get(string(ctxUserID))
	userID, _ := value.(int64)
	return userID
}

// Username returns the authenticated username stored by Auth.
func Username(c *gin.Context) string {
	value, _ := c.Get(string(ctxUsername))
	username, _ := value.(string)
	return username
}

// RequestIDOf returns the request ID injected by RequestID.
func RequestIDOf(c *gin.Context) string {
	value, _ := c.Get(string(ctxRequestID))
	requestID, _ := value.(string)
	return requestID
}

// TenantIDOf 取当前请求的租户 ID（来自 JWT claims；0 表示平台/默认租户）。
func TenantIDOf(c *gin.Context) int64 {
	value, _ := c.Get(string(ctxTenantID))
	tenantID, _ := value.(int64)
	return tenantID
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
		// 签名有效后，数据库复核也必须限定在 Token 声明的租户内。
		c.Request = c.Request.WithContext(tenant.WithTenant(c.Request.Context(), claims.TenantID))
		if validator != nil {
			if err := validator.ValidateToken(c.Request.Context(), claims.UserID, claims.TokenVersion); err != nil {
				response.Fail(c, err)
				c.Abort()
				return
			}
		}
		c.Set(string(ctxUserID), claims.UserID)
		c.Set(string(ctxUsername), claims.Username)
		c.Set(string(ctxTenantID), claims.TenantID)
		// ctx 链式叠加：保留 RequestID，再挂 UserID（日志）与租户 ID（GORM 隔离回调）
		c.Request = c.Request.WithContext(
			tenant.WithTenant(log.WithUserID(c.Request.Context(), claims.UserID), claims.TenantID))
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
					"err", r, "path", c.Request.URL.Path, "stack", string(debug.Stack()))
				response.Fail(c, errcode.Internal)
				c.Abort()
			}
		}()
		c.Next()
	}
}

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

// AccessLog 访问日志：方法/路径/状态/耗时/操作人，>500ms 打 warn。
func AccessLog() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()

		c.Next()

		cost := time.Since(start).Milliseconds()
		attrs := []any{
			"method", c.Request.Method,
			"path", c.Request.URL.Path,
			"status", c.Writer.Status(),
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

// CORS 跨域：只允许配置中的精确 Origin，避免生产环境使用通配符。
func CORS(allowedOrigins []string) gin.HandlerFunc {
	allowed := make(map[string]struct{}, len(allowedOrigins))
	for _, origin := range allowedOrigins {
		allowed[strings.TrimRight(origin, "/")] = struct{}{}
	}
	return func(c *gin.Context) {
		origin := strings.TrimRight(c.GetHeader("Origin"), "/")
		if origin == "" {
			c.Next()
			return
		}
		if _, ok := allowed[origin]; !ok {
			if c.Request.Method == http.MethodOptions {
				c.AbortWithStatus(http.StatusForbidden)
				return
			}
			c.Next()
			return
		}
		c.Header("Access-Control-Allow-Origin", origin)
		c.Header("Access-Control-Allow-Methods", "GET,POST,PUT,DELETE,OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Authorization,Content-Type,X-Request-ID,X-Demo-Session")
		c.Header("Access-Control-Expose-Headers", "X-Request-ID")
		c.Header("Vary", "Origin")
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}

func normalizeRequestID(id string) string {
	id = strings.TrimSpace(id)
	if id == "" || len(id) > 128 {
		return ""
	}
	for _, r := range id {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' || r == '.' || r == ':' {
			continue
		}
		return ""
	}
	return id
}

func newRequestID() string {
	return time.Now().Format("20060102150405") + randHex()
}
