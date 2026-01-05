package transport

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"

	"github.com/imtaco/audio-rtc-exp/internal/log"
)

type Router struct {
	janusID string
	engine  *gin.Engine
	logger  *log.Logger
}

func NewRouter(janusID string, logger *log.Logger) *Router {
	gin.SetMode(gin.ReleaseMode)
	engine := gin.New()
	engine.Use(gin.Recovery())

	r := &Router{
		janusID: janusID,
		engine:  engine,
		logger:  logger,
	}

	r.setupRoutes()
	return r
}

func (r *Router) Handler() http.Handler {
	return r.engine
}

func (r *Router) setupRoutes() {
	r.engine.Use(otelgin.Middleware("janus-service"))

	// Health check
	r.engine.GET("/health", r.healthCheck)
}

func (r *Router) healthCheck(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status":    "ok",
		"janus_id":  r.janusID,
		"service":   "janus-service",
		"timestamp": time.Now(),
	})
}
