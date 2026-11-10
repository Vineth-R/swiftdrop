package servicehttp

import (
	"log/slog"
	"time"

	"github.com/Vineth-R/swiftdrop/packages/go-common/auth"

	"github.com/gin-gonic/gin"

	"github.com/Vineth-R/swiftdrop/apps/services/merchant/internal/repository"
	"github.com/Vineth-R/swiftdrop/packages/go-common/httpx"
)

func New(logger *slog.Logger, options ...any) *gin.Engine {
	router := httpx.NewRouter(logger)
	httpx.RegisterHealth(router)
	var repo *repository.Repository
	secret, cookie := "", "sd_access"
	for _, option := range options {
		switch value := option.(type) {
		case *repository.Repository:
			repo = value
		case string:
			if secret == "" {
				secret = value
			} else {
				cookie = value
			}
		}
	}
	if repo != nil {
		registerMerchantRoutes(router, repo, logger)
		if secret != "" {
			manager, err := auth.NewTokenManager(secret, time.Minute)
			if err == nil {
				_ = manager
			}
		}
		// Route handlers validate identity injected by the auth middleware. The
		// merchant service receives claims through this shared middleware.
		if secret != "" {
			manager, _ := auth.NewTokenManager(secret, time.Minute)
			group := router.Group("/merchants")
			group.Use(auth.Authenticate(manager, cookie), auth.RequireRoles(auth.RoleMerchant))
			group.Use(func(c *gin.Context) {
				claims, _ := auth.ClaimsFrom(c)
				merchant, err := repo.MerchantByOwnerUserID(c.Request.Context(), claims.UserID)
				if err != nil {
					if err == repository.ErrNotFound {
						httpx.Error(c, 404, "NOT_FOUND", "merchant not found")
					} else {
						httpx.Error(c, 500, "INTERNAL_ERROR", "merchant lookup failed")
					}
					c.Abort()
					return
				}
				WithIdentity(c, Identity{UserID: claims.UserID, MerchantID: merchant.ID, Role: string(claims.Role)})
				c.Next()
			})
			group.GET("/me", (&merchantHandler{repository: repo, logger: logger}).profile)
			group.PATCH("/me", (&merchantHandler{repository: repo, logger: logger}).updateProfile)
			group.GET("/me/keys", (&merchantHandler{repository: repo, logger: logger}).listKeys)
			group.POST("/me/keys", (&merchantHandler{repository: repo, logger: logger}).createKey)
			group.DELETE("/me/keys/:keyId", (&merchantHandler{repository: repo, logger: logger}).revokeKey)
		}
	}
	return router
}
