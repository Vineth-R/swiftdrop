package servicehttp

import (
	"log/slog"

	"github.com/gin-gonic/gin"

	"github.com/Vineth-R/swiftdrop/packages/go-common/httpx"
)

func New(logger *slog.Logger, configurations ...Options) (*gin.Engine, error) {
	router := httpx.NewRouter(logger)
	httpx.RegisterHealth(router)
	options := Options{}
	if len(configurations) > 0 {
		options = configurations[0]
	}
	if err := router.SetTrustedProxies(options.TrustedProxies); err != nil {
		return nil, err
	}
	router.RemoteIPHeaders = []string{"X-Real-IP"}
	registerTracking(router, logger, options)
	if err := registerReports(router, logger, options); err != nil {
		return nil, err
	}
	if err := registerDispatch(router, logger, options); err != nil {
		return nil, err
	}
	return router, nil
}
