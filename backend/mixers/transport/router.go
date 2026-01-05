package transport

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"

	"github.com/imtaco/audio-rtc-exp/internal/log"
)

type Router struct {
	mixerID string
	engine  *gin.Engine
	logger  *log.Logger
}

func NewRouter(mixerID string, logger *log.Logger) *Router {
	gin.SetMode(gin.ReleaseMode)
	engine := gin.New()
	engine.Use(gin.Recovery())

	// Add OpenTelemetry middleware for automatic HTTP tracing
	engine.Use(otelgin.Middleware("mixer-service"))

	r := &Router{
		mixerID: mixerID,
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
	// Health check
	r.engine.GET("/health", r.healthCheck)
}

func (r *Router) healthCheck(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status":    "ok",
		"mixer_id":  r.mixerID,
		"service":   "mixer-service",
		"timestamp": time.Now(),
	})
}
