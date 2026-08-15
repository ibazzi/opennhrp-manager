package api

import (
	"log"

	"github.com/gin-gonic/gin"

	"opennhrp-manager/internal/service"
)

type LogWSHandler struct {
	logHub *service.LogHub
}

func NewLogWSHandler(logHub *service.LogHub) *LogWSHandler {
	return &LogWSHandler{logHub: logHub}
}

func (h *LogWSHandler) HandleWS(c *gin.Context) {
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Printf("[LogWS] Upgrade failed: %v", err)
		return
	}
	defer conn.Close()

	h.logHub.Register(conn)
	defer h.logHub.Unregister(conn)

	// Keep reading to detect client disconnection
	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			break
		}
	}
}
