package transport

import (
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"

	"github.com/imtaco/audio-rtc-exp/internal/constants"
	"github.com/imtaco/audio-rtc-exp/internal/log"
	"github.com/imtaco/audio-rtc-exp/internal/validation"
	"github.com/imtaco/audio-rtc-exp/rooms"
	utils "github.com/imtaco/audio-rtc-exp/rooms/utils"
)

const (
	defaultMaxAnchors = 3
)

type Router struct {
	roomService rooms.RoomService
	roomStore   rooms.RoomStore
	engine      *gin.Engine
	logger      *log.Logger
}

func NewRouter(roomService rooms.RoomService, roomStore rooms.RoomStore, logger *log.Logger) *Router {
	gin.SetMode(gin.ReleaseMode)
	engine := gin.New()
	engine.Use(gin.Recovery())

	// Add OpenTelemetry middleware for automatic HTTP tracing
	engine.Use(otelgin.Middleware("room-service"))

	r := &Router{
		roomService: roomService,
		roomStore:   roomStore,
		engine:      engine,
		logger:      logger,
	}

	// Request logging middleware
	r.engine.Use(func(c *gin.Context) {
		r.logger.Info("Incoming request",
			log.String("method", c.Request.Method),
			log.String("url", c.Request.URL.String()))
		c.Next()
	})

	r.setupRoutes()
	return r
}

func (r *Router) Handler() http.Handler {
	return r.engine
}

func (r *Router) setupRoutes() {
	r.engine.Use(otelgin.Middleware("room-service"))

	// Room management routes
	r.engine.POST("/api/rooms", r.createRoom)
	r.engine.GET("/api/rooms/:roomId", r.getRoom)
	r.engine.GET("/api/rooms", r.listRooms)
	r.engine.DELETE("/api/rooms/:roomId", r.deleteRoom)

	// Module mark management routes
	r.engine.PUT("/api/modules/:moduleType/:moduleId/mark", r.setModuleMark)
	r.engine.DELETE("/api/modules/:moduleType/:moduleId/mark", r.deleteModuleMark)

	// Stats
	r.engine.GET("/api/stats", r.getStats)

	// Health check
	r.engine.GET("/health", r.healthCheck)
}

func (r *Router) createRoom(c *gin.Context) {
	var req CreateRoomRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Validation failed",
			"details": validation.FormatValidationError(err),
		})
		return
	}

	// Generate room ID if not provided
	roomID := req.RoomID
	if roomID == "" {
		var err error
		roomID, err = utils.GenerateRandomHex(10)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"error":   "Failed to generate room ID",
			})
			return
		}
	}

	// Generate PIN if not provided
	pin := req.Pin
	if pin == "" {
		var err error
		pin, err = utils.GenerateRandomHex(3)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"error":   "Failed to generate PIN",
			})
			return
		}
	}

	maxAnchors := req.MaxAnchors
	if maxAnchors == 0 {
		maxAnchors = defaultMaxAnchors
	}

	ctx := c.Request.Context()
	room, err := r.roomService.CreateRoom(ctx, roomID, pin, maxAnchors)
	if err != nil {
		var roomExistsErr *rooms.RoomExistsError
		if errors.As(err, &roomExistsErr) {
			c.JSON(http.StatusConflict, gin.H{
				"success": false,
				"error":   err.Error(),
			})
			return
		}
		r.logger.Error("Failed to create room", log.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to create room",
		})
		return
	}

	// TODO: separate start live API ?!
	if err := r.roomService.StartLive(ctx, roomID); err != nil {
		r.logger.Error("Failed to start live", log.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to start live",
		})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"success": true,
		"room":    room,
	})
}

func (r *Router) getRoom(c *gin.Context) {
	// Validate room ID using manual validation
	var req GetRoomRequest
	if err := c.ShouldBindUri(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Validation failed",
			"details": validation.FormatValidationError(err),
		})
		return
	}

	roomID := req.RoomID
	ctx := c.Request.Context()

	room, err := r.roomService.GetRoom(ctx, roomID)
	if err != nil {
		var roomNotFoundErr *rooms.RoomNotFoundError
		if errors.As(err, &roomNotFoundErr) {
			c.JSON(http.StatusNotFound, gin.H{
				"success": false,
				"error":   err.Error(),
			})
			return
		}
		r.logger.Error("Failed to get room", log.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to get room",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"room":    room,
	})
}

func (r *Router) listRooms(c *gin.Context) {
	ctx := c.Request.Context()

	result, err := r.roomService.ListRooms(ctx)
	if err != nil {
		r.logger.Error("Failed to list rooms", log.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to list rooms",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"count":   result.Count,
		"rooms":   result.Rooms,
	})
}

func (r *Router) deleteRoom(c *gin.Context) {
	var req DeleteRoomRequest
	if err := c.ShouldBindUri(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Validation failed",
			"details": validation.FormatValidationError(err),
		})
		return
	}

	roomID := req.RoomID
	ctx := c.Request.Context()

	result, err := r.roomService.DeleteRoom(ctx, roomID)
	if err != nil {
		var roomNotFoundErr *rooms.RoomNotFoundError
		if errors.As(err, &roomNotFoundErr) {
			c.JSON(http.StatusNotFound, gin.H{
				"success": false,
				"error":   err.Error(),
			})
			return
		}
		r.logger.Error("Failed to delete room", log.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to delete room",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": result.Message,
	})
}

func (r *Router) getStats(c *gin.Context) {
	ctx := c.Request.Context()

	stats, err := r.roomService.GetStats(ctx)
	if err != nil {
		r.logger.Error("Failed to get stats", log.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to get stats",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"stats":   stats,
	})
}

func (r *Router) healthCheck(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status":    "ok",
		"service":   "rooms",
		"timestamp": time.Now().Unix(),
	})
}

func (r *Router) setModuleMark(c *gin.Context) {
	var uriParams ModuleMarkURI
	var bodyParams SetModuleMarkBody

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

	ctx := c.Request.Context()

	// Convert label string to MarkLabel type
	var markLabel constants.MarkLabel
	switch bodyParams.Label {
	case "ready":
		markLabel = constants.MarkLabelReady
	case "cordon":
		markLabel = constants.MarkLabelCordon
	case "draining":
		markLabel = constants.MarkLabelDraining
	case "drained":
		markLabel = constants.MarkLabelDrained
	case "unready":
		markLabel = constants.MarkLabelUnready
	default:
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Invalid label value",
		})
		return
	}

	// Set the module mark
	if err := r.roomStore.SetModuleMark(ctx, uriParams.ModuleType, uriParams.ModuleID, markLabel, bodyParams.TTL); err != nil {
		r.logger.Error("Failed to set module mark", log.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to set module mark",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Module mark set successfully",
		"module": gin.H{
			"type":  uriParams.ModuleType,
			"id":    uriParams.ModuleID,
			"label": bodyParams.Label,
			"ttl":   bodyParams.TTL,
		},
	})
}

func (r *Router) deleteModuleMark(c *gin.Context) {
	var req ModuleMarkURI

	// Validate the request
	if err := c.ShouldBindUri(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Validation failed",
			"details": validation.FormatValidationError(err),
		})
		return
	}

	ctx := c.Request.Context()

	// Delete the module mark
	if err := r.roomStore.DeleteModuleMark(ctx, req.ModuleType, req.ModuleID); err != nil {
		r.logger.Error("Failed to delete module mark", log.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to delete module mark",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Module mark deleted successfully",
		"module": gin.H{
			"type": req.ModuleType,
			"id":   req.ModuleID,
		},
	})
}
