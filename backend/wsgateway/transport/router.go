package transport

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"

	"github.com/imtaco/audio-rtc-exp/internal/jwt"
	"github.com/imtaco/audio-rtc-exp/internal/log"
)

type Router struct {
	jwtAuth *jwt.Auth
	engine  *gin.Engine
	logger  *log.Logger
}

func NewRouter(jwtAuth *jwt.Auth, logger *log.Logger) *Router {
	gin.SetMode(gin.ReleaseMode)
	engine := gin.New()
	engine.Use(gin.Recovery())

	// Add OpenTelemetry middleware for automatic HTTP tracing
	engine.Use(otelgin.Middleware("wsgateway"))

	r := &Router{
		jwtAuth: jwtAuth,
		engine:  engine,
		logger:  logger,
	}

	r.setupRoutes()
	return r
}

func (r *Router) setupRoutes() {
	// Health check
	r.engine.GET("/health", r.healthCheck)
}

func (r *Router) healthCheck(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status":    "ok",
		"timestamp": time.Now().Unix(),
	})
}

func (r *Router) Run(addr string) error {
	return r.engine.Run(addr)
}
