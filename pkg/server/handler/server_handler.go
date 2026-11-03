package server_handler

import (
	"runtime/debug"
	"time"

	"github.com/gin-gonic/gin"
	"go.mau.fi/whatsmeow/store"
)

const whatsmeowModule = "go.mau.fi/whatsmeow"

type ServerInfo struct {
	Version   string
	StartedAt time.Time
}

type ServerHandler interface {
	ServerOk(ctx *gin.Context)
	ServerInfo(ctx *gin.Context)
}

type serverHandler struct {
	info ServerInfo
}

// ServerOk implements ServerHandler.
func (s *serverHandler) ServerOk(ctx *gin.Context) {
	ctx.JSON(200, gin.H{
		"status": "ok",
	})
}

// Get server info
// @Summary Get server info
// @Description Get the server version, whatsmeow version, WhatsApp Web version and start time
// @Tags Instance
// @Produce json
// @Success 200 {object} gin.H "Server info"
// @Router /instance/server-info [get]
func (s *serverHandler) ServerInfo(ctx *gin.Context) {
	ctx.JSON(200, gin.H{
		"version":   s.info.Version,
		"whatsmeow": whatsmeowVersion(),
		"waVersion": store.GetWAVersion().String(),
		"startedAt": s.info.StartedAt.Format(time.RFC3339),
	})
}

func whatsmeowVersion() string {
	buildInfo, ok := debug.ReadBuildInfo()
	if !ok {
		return ""
	}
	for _, dep := range buildInfo.Deps {
		if dep.Path == whatsmeowModule {
			return dep.Version
		}
	}
	return ""
}

func NewServerHandler(info ServerInfo) ServerHandler {
	return &serverHandler{info: info}
}
