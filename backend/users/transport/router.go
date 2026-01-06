package transport

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"

	"github.com/imtaco/audio-rtc-exp/internal/jwt"
	"github.com/imtaco/audio-rtc-exp/internal/log"
	"github.com/imtaco/audio-rtc-exp/internal/validation"
	"github.com/imtaco/audio-rtc-exp/users"
)

type Router struct {
	userService users.UserService
	jwtAuth     jwt.Auth
	engine      *gin.Engine
	logger      *log.Logger
}

func NewRouter(userService users.UserService, jwtAuth jwt.Auth, logger *log.Logger) *Router {
	gin.SetMode(gin.ReleaseMode)
	engine := gin.New()
	engine.Use(gin.Recovery())

	// Add OpenTelemetry middleware for automatic HTTP tracing
	engine.Use(otelgin.Middleware("user-service"))

	r := &Router{
		userService: userService,
		jwtAuth:     jwtAuth,
		engine:      engine,
		logger:      logger,
	}

	r.setupRoutes()
	return r
}

func (r *Router) Handler() http.Handler {
	return r.engine
}

func (r *Router) setupRoutes() {
	// User management routes
	r.engine.POST("/api/rooms/:roomId/users", r.createUser)
	r.engine.DELETE("/api/rooms/:roomId/users/:userId", r.deleteUser)

	// Health check
	r.engine.GET("/health", r.healthCheck)
}

func (r *Router) createUser(c *gin.Context) {
	var uriParams CreateUserURI
	var bodyParams CreateUserBody

	// Bind URI params
	if err := c.ShouldBindUri(&uriParams); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Validation failed",
			"details": validation.FormatValidationError(err),
		})
		return
	}

	// Bind JSON body
	if err := c.ShouldBindJSON(&bodyParams); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Validation failed",
			"details": validation.FormatValidationError(err),
		})
		return
	}

	// Generate unique user ID
	userID := uuid.New().String()
	ctx := c.Request.Context()

	// Create user
	_, token, err := r.userService.CreateUser(ctx, uriParams.RoomID, userID, bodyParams.Role)
	if err != nil {
		r.logger.Error("Failed to create user", log.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	r.logger.Info("User created",
		log.String("roomId", uriParams.RoomID),
		log.String("userID", userID),
		log.String("role", bodyParams.Role),
	)

	c.JSON(http.StatusOK, gin.H{
		"userID": userID,
		"token":  token,
	})
}

func (r *Router) deleteUser(c *gin.Context) {
	ctx := c.Request.Context()

	// Bind URI params
	var req DeleteUserURI
	if err := c.ShouldBindUri(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Validation failed",
			"details": validation.FormatValidationError(err),
		})
		return
	}

	if err := r.userService.DeleteUser(ctx, req.RoomID, req.UserID); err != nil {
		r.logger.Error("Failed to delete user", log.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": err.Error(),
		})
		return
	}

	r.logger.Info("User deleted", log.String("userID", req.UserID))

	c.JSON(http.StatusOK, gin.H{})
}

func (r *Router) healthCheck(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status":    "ok",
		"timestamp": time.Now().Unix(),
	})
}
