package transport

import (
	"net/http"
	"time"

	"github.com/imtaco/audio-rtc-exp/internal/jwt"
	"github.com/imtaco/audio-rtc-exp/internal/log"

	"github.com/gin-gonic/gin"
)

type Router struct {
	jwtAuth *jwt.JWTAuth
	engine  *gin.Engine
	logger  *log.Logger
}

func NewRouter(jwtAuth *jwt.JWTAuth, logger *log.Logger) *Router {
	gin.SetMode(gin.ReleaseMode)
	engine := gin.New()
	engine.Use(gin.Recovery())

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
