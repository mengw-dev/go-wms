package response

import (
	"mime"
	"net/http"

	"github.com/gin-gonic/gin"
)

// Excel sends a complete workbook only after successful generation; failures use Fail before this call.
func Excel(c *gin.Context, name string, data []byte) {
	c.Header("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": name}))
	c.Header("Cache-Control", "private, no-store")
	c.Header("X-Content-Type-Options", "nosniff")
	c.Data(http.StatusOK, "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", data)
}
