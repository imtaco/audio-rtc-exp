package transport

import (
	"crypto/subtle"
	"net/http"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	lru "github.com/hashicorp/golang-lru/v2"

	"github.com/imtaco/audio-rtc-exp/hlsserver"
	commoncrypto "github.com/imtaco/audio-rtc-exp/internal/crypto"
	"github.com/imtaco/audio-rtc-exp/internal/jwt"
	"github.com/imtaco/audio-rtc-exp/internal/log"
	"github.com/imtaco/audio-rtc-exp/internal/validation"
)

var (
	keyCache *lru.Cache[string, []byte]
)

func initKeyCache() {
	var err error
	keyCache, err = lru.New[string, []byte](100)
	if err != nil {
		panic(err)
	}
}

// TokenRouter handles token generation endpoints
type TokenRouter struct {
	roomWatcher hlsserver.RoomWatcher
	jwtAuth     jwt.JWTAuth
	engine      *gin.Engine
	logger      *log.Logger
}

func NewTokenRouter(roomWatcher hlsserver.RoomWatcher, jwtAuth jwt.JWTAuth, logger *log.Logger) *TokenRouter {
	gin.SetMode(gin.ReleaseMode)
	engine := gin.New()
	engine.Use(gin.Recovery())

	r := &TokenRouter{
		roomWatcher: roomWatcher,
		jwtAuth:     jwtAuth,
		engine:      engine,
		logger:      logger,
	}

	r.setupRoutes()
	return r
}

func (r *TokenRouter) Handler() http.Handler {
	return r.engine
}

func (r *TokenRouter) setupRoutes() {
	r.engine.POST("/api/token", r.generateToken)
	r.engine.GET("/health", r.healthCheck)
}

func (r *TokenRouter) generateToken(c *gin.Context) {
	var req GenerateTokenRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Validation failed",
			"details": validation.FormatValidationError(err),
		})
		return
	}

	userID := uuid.New().String()
	token, err := r.jwtAuth.Sign(userID, req.RoomID)
	if err != nil {
		r.logger.Error("Failed to sign token",
			log.String("userId", userID),
			log.String("roomId", req.RoomID),
			log.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to generate token",
		})
		return
	}

	r.logger.Info("Token generated",
		log.String("userId", userID),
		log.String("roomId", req.RoomID))

	c.JSON(http.StatusOK, gin.H{
		"token": token,
	})
}

func (r *TokenRouter) healthCheck(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status": "ok",
	})
}

// KeyRouter handles encryption key serving endpoints
type KeyRouter struct {
	roomWatcher hlsserver.RoomWatcher
	jwtAuth     jwt.JWTAuth
	engine      *gin.Engine
	logger      *log.Logger
}

func NewKeyRouter(roomWatcher hlsserver.RoomWatcher, jwtAuth jwt.JWTAuth, logger *log.Logger) *KeyRouter {
	initKeyCache()

	gin.SetMode(gin.ReleaseMode)
	engine := gin.New()
	engine.Use(gin.Recovery())

	// Configure CORS
	engine.Use(cors.New(cors.Config{
		AllowOrigins:     []string{"*"},
		AllowMethods:     []string{"GET", "POST", "OPTIONS"},
		AllowHeaders:     []string{"Authorization", "Content-Type"},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: false,
	}))

	r := &KeyRouter{
		roomWatcher: roomWatcher,
		jwtAuth:     jwtAuth,
		engine:      engine,
		logger:      logger,
	}

	r.setupRoutes()
	return r
}

func (r *KeyRouter) Handler() http.Handler {
	return r.engine
}

func (r *KeyRouter) setupRoutes() {
	r.engine.GET("/hls/rooms/:roomId/enc.key", r.getEncryptionKey)
	r.engine.GET("/health", r.healthCheck)
}

func (r *KeyRouter) getEncryptionKey(c *gin.Context) {
	// roomID := c.Param("roomId")
	var req GetEncryptionKeyRequest
	if err := c.ShouldBindUri(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Validation failed",
			"details": validation.FormatValidationError(err),
		})
		return
	}

	roomID := req.RoomID
	authHeader := c.GetHeader("Authorization")

	if authHeader == "" {
		r.logger.Warn("Missing authorization header",
			log.String("url", c.Request.URL.String()))
		c.String(http.StatusUnauthorized, "Authorization header required")
		return
	}

	token := authHeader
	if len(authHeader) > 7 && authHeader[:7] == "Bearer " {
		token = authHeader[7:]
	}

	payload, err := r.jwtAuth.Verify(token)
	if err != nil {
		r.logger.Warn("Invalid token",
			log.String("url", c.Request.URL.String()),
			log.Error(err))
		c.String(http.StatusForbidden, "Access denied 1")
		return
	}

	if subtle.ConstantTimeCompare([]byte(roomID), []byte(payload.RoomID)) != 1 {
		r.logger.Warn("RoomId mismatch",
			log.String("roomId", roomID),
			log.String("tokenRoomId", payload.RoomID))
		c.String(http.StatusForbidden, "Access denied 2")
		return
	}

	keyData, ok := keyCache.Get(roomID)
	if ok {
		r.logger.Debug("Key served from cache",
			log.String("roomId", roomID),
			log.String("userId", payload.UserID))
	} else {
		livemeta := r.roomWatcher.GetActiveLiveMeta(roomID)
		if livemeta == nil {
			r.logger.Warn("Room not found or not active",
				log.String("roomId", roomID))
			c.String(http.StatusForbidden, "Access denied 3")
			return
		}

		keyData = commoncrypto.GenerateAESKey(roomID, livemeta.Nonce)
		keyCache.Add(roomID, keyData)

		r.logger.Debug("Key generated and cached",
			log.String("roomId", roomID),
			log.String("userId", payload.UserID),
			log.Int("cacheSize", keyCache.Len()))
	}

	c.Header("Content-Type", "application/octet-stream")
	c.Header("Cache-Control", "no-cache, no-store, must-revalidate")
	c.Header("Pragma", "no-cache")
	c.Header("Expires", "0")

	c.Data(http.StatusOK, "application/octet-stream", keyData)
}

func (r *KeyRouter) healthCheck(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status": "ok",
	})
}
