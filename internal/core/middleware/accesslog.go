package middleware

import (
	"fmt"
	"regexp"

	"github.com/gin-gonic/gin"
)

var publicTokenPath = regexp.MustCompile(`(/public/(?:quotations|invoices)/)[^/?]+`)

// MaskPublicToken menyembunyikan token link publik dari path sebelum dicatat.
func MaskPublicToken(path string) string {
	return publicTokenPath.ReplaceAllString(path, "${1}***")
}

// AccessLogger adalah gin.Logger() dengan token link publik di-mask: token
// adalah kredensial akses dokumen dan tidak boleh tersimpan di log.
func AccessLogger() gin.HandlerFunc {
	return gin.LoggerWithConfig(gin.LoggerConfig{
		Formatter: func(p gin.LogFormatterParams) string {
			return fmt.Sprintf("[GIN] %s | %3d | %13v | %15s | %-7s %q\n%s",
				p.TimeStamp.Format("2006/01/02 - 15:04:05"), p.StatusCode, p.Latency, p.ClientIP,
				p.Method, MaskPublicToken(p.Path), p.ErrorMessage)
		},
	})
}
