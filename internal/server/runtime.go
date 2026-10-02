package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net"
	"strings"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/komari-monitor/komari/database"
	"github.com/komari-monitor/komari/database/dbcore"
	"github.com/komari-monitor/komari/internal/sharing"
	"github.com/komari-monitor/komari/web/share"
	"github.com/komari-monitor/komari/database/accounts"
	"github.com/komari-monitor/komari/database/auditlog"
	"github.com/komari-monitor/komari/database/tasks"
	"github.com/komari-monitor/komari/internal/config"
	"github.com/komari-monitor/komari/internal/lifecycle"
	"github.com/komari-monitor/komari/internal/metricstore"
	"github.com/komari-monitor/komari/internal/scheduler"
	"github.com/komari-monitor/komari/utils/geoip"
	"github.com/komari-monitor/komari/utils"
	logger "github.com/komari-monitor/komari/utils/log"
	"github.com/komari-monitor/komari/web/api"
	"github.com/komari-monitor/komari/web/oauth"
	recoveryweb "github.com/komari-monitor/komari/web/recovery"
	"github.com/komari-monitor/komari/web/router"
	"github.com/komari-monitor/komari/web/security"
)

// ErrRestartRequested is returned after a clean shutdown when a configuration
// change requires the next startup to enter a restricted guide.
var ErrRestartRequested = errors.New("server restart requested")

const (
	// Give in-flight HTTP requests time to finish before the listener closes.
	httpShutdownTimeout = 10 * time.Second
	// Keep an independent budget for report flushing and store teardown. Reusing
	// the HTTP deadline here can skip queued metric writes after a slow request.
	resourceCleanupTimeout = 30 * time.Second
)

// StartBackground starts scheduled work after all stores are ready.
func (a *App) StartBackground() error {
	registerScheduledWork()
	a.addCleanup("scheduler", func(context.Context) error {
		scheduler.StopAll()
		return nil
	})
	return nil
}

func (a *App) registerReloadHandlers(cors *security.CorsController) {
	a.reload.Register("oauth-provider", func(event config.ConfigEvent) {
		if ok, providerName := config.IsChangedT[string](event, config.OAuthProviderKey); ok {
			if providerName == "" || providerName == "none" {
				providerName = "github"
			}
			oidcProvider, err := database.GetOidcConfigByName(providerName)
			if err != nil {
				logger.Errorf("server", "Failed to get OIDC provider config: %v", err)
				return
			}
			logger.Infof("server", "Using %s as OIDC provider", oidcProvider.Name)
			if err := oauth.LoadProvider(oidcProvider.Name, oidcProvider.Addition); err != nil {
				auditlog.EventLog("error", fmt.Sprintf("Failed to load OIDC provider: %v", err))
			}
		}
	})
	a.reload.Register("geoip-provider", func(event config.ConfigEvent) {
		if event.IsChanged(config.GeoIpProviderKey) {
			go geoip.InitGeoIp()
		}
	})
	a.reload.Register("cors", func(event config.ConfigEvent) { cors.Update(event) })
}

// BuildRouter constructs the normal application router and starts reloads.
func (a *App) BuildRouter() error {
	_, err := sharing.ValidateConfig(a.listenAddr, a.shareListen, "")
	if err != nil { return err }
	sharing.Configure("")
	if a.shareListen != "" {
		var proxies []string
		if a.shareTrustedProxy != "" { for _, p := range strings.Split(a.shareTrustedProxy, ",") { proxies = append(proxies, strings.TrimSpace(p)) } }
		a.shareEngine, err = share.New(sharing.New(dbcore.GetDBInstance()), proxies)
		if err != nil { return err }
		sharing.ConfigureSource(func() string {
			base, err := config.GetAs[string](config.SharePublicBaseURLKey)
			if err != nil { return "" }; return base
		})
	}
	r := gin.New()
	if err := utils.ConfigureTrustedProxies(r); err != nil { return fmt.Errorf("trusted proxies: %w", err) }
	r.Use(logger.GinLogger(), logger.GinRecovery())
	cors := security.NewCorsController(a.settings.CorsOriginCheckEnabled, a.settings.CorsAllowedOrigins)
	r.Use(api.RequestLimits(), cors.Middleware(), security.CookieMutationProtection(), api.IdentityMiddleware(), api.PrivateSiteMiddleware(), noStoreAPIResponses())

	// The recovery UI belongs only to its temporary restricted listener.
	r.GET(recoveryweb.PagePath, func(c *gin.Context) {
		c.Redirect(http.StatusTemporaryRedirect, "/")
	})
	router.Register(r)

	a.registerReloadHandlers(cors)
	a.reload.Start()
	a.engine = r
	return nil
}

// Run starts the normal HTTP server and blocks until shutdown or fatal error.
func (a *App) Run() error {
	a.server = &http.Server{Addr: a.listenAddr, Handler: a.engine, ReadHeaderTimeout: 5*time.Second, ReadTimeout: 45*time.Second, IdleTimeout: 60*time.Second}
	serverErr := make(chan error, 2)
	// Bind both ports before serving either; a conflict rolls back atomically.
	mainListener, err := net.Listen("tcp", a.listenAddr)
	if err != nil { _ = a.Shutdown(); return fmt.Errorf("main listener: %w", err) }
	var shareListener net.Listener
	if a.shareEngine != nil {
		shareListener, err = net.Listen("tcp", a.shareListen)
		if err != nil { _ = mainListener.Close(); _ = a.Shutdown(); return fmt.Errorf("share listener: %w", err) }
		a.shareServer = &http.Server{Addr:a.shareListen, Handler:a.shareEngine, ReadHeaderTimeout:5*time.Second, ReadTimeout:15*time.Second, WriteTimeout:15*time.Second, IdleTimeout:60*time.Second}
		logger.Infof("server", "Starting share server on %s", a.shareListen)
		go func() { if err := a.shareServer.Serve(shareListener); err != nil && err != http.ErrServerClosed { serverErr <- err } }()
	}
	logger.Infof("server", "Starting server on %s ...", a.listenAddr)
	go func() {
		if err := a.server.Serve(mainListener); err != nil && err != http.ErrServerClosed {
			serverErr <- err
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(quit)
	select {
	case err := <-serverErr:
		_ = a.Shutdown()
		return fmt.Errorf("listen: %w", err)
	case reason := <-lifecycle.RestartRequests():
		logger.Infof("server", "Restarting service for %s", reason)
		if err := a.Shutdown(); err != nil {
			logger.Errorf("server", "Cleanup before restart failed: %v", err)
		}
		return fmt.Errorf("%w: %s", ErrRestartRequested, reason)
	case <-quit:
		return a.Shutdown()
	}
}

// Shutdown stops HTTP first, then releases registered resources in LIFO order.
func (a *App) Shutdown() error {
	if a.dbReady {
		auditlog.Log("", "", "server is shutting down", "info")
	}
	httpCtx, cancelHTTP := context.WithTimeout(context.Background(), httpShutdownTimeout)
	defer cancelHTTP()
	sharing.Configure("")
	if a.shareServer != nil { if err := a.shareServer.Shutdown(httpCtx); err != nil { _ = a.shareServer.Close() } }
	if a.server != nil {
		if err := a.server.Shutdown(httpCtx); err != nil {
			_ = a.server.Close()
			logger.Infof("server", "HTTP server forced to shutdown: %v", err)
		}
	}

	cleanupCtx, cancelCleanup := context.WithTimeout(context.Background(), resourceCleanupTimeout)
	defer cancelCleanup()
	return a.runCleanups(cleanupCtx)
}

func (a *App) onFatal(err error) {
	if a.dbReady {
		auditlog.Log("", "", "server encountered a fatal error: "+err.Error(), "error")
	}
	ctx, cancel := context.WithTimeout(context.Background(), resourceCleanupTimeout)
	defer cancel()
	if cleanupErr := a.runCleanups(ctx); cleanupErr != nil {
		logger.Errorf("server", "Cleanup after fatal server error failed: %v", cleanupErr)
	}
}

func (a *App) runCleanups(ctx context.Context) error {
	var cleanupErrors []error
	for i := len(a.cleanups) - 1; i >= 0; i-- {
		cleanup := a.cleanups[i]
		if err := cleanup.fn(ctx); err != nil {
			logger.Errorf("server", "cleanup %q failed: %v", cleanup.name, err)
			cleanupErrors = append(cleanupErrors, fmt.Errorf("cleanup %q: %w", cleanup.name, err))
		}
	}
	return errors.Join(cleanupErrors...)
}

func registerScheduledWork() {
	if err := tasks.ReloadPingSchedule(); err != nil {
		logger.ErrorArgs("server", "Failed to reload ping schedule:", err)
	}
	if err := scheduler.AddFunc("records:cleanup", "@every 30m", cleanupScheduledData); err != nil {
		logger.ErrorArgs("server", "Failed to add cleanup scheduled task:", err)
	}
	if err := scheduler.AddContextFunc("metrics:compact", "@every 5m", true, compactMetricStore); err != nil {
		logger.ErrorArgs("server", "Failed to add metric compact scheduled task:", err)
	}
	if err := scheduler.AddContextFunc("metrics:retention", "@every 1h", true, cleanupMetricStore); err != nil {
		logger.ErrorArgs("server", "Failed to add metric retention scheduled task:", err)
	}
}

func cleanupScheduledData() {
	auditlog.RemoveOldLogs()
	accounts.RemoveExpiredSessions()
	if err := sharing.New(dbcore.GetDBInstance()).CleanupSessions(); err != nil { logger.Errorf("server", "Share session cleanup failed") }
}

func compactMetricStore(ctx context.Context) {
	compactCtx, cancel := context.WithTimeout(ctx, 15*time.Minute)
	defer cancel()
	written, err := metricstore.Compact(compactCtx, time.Now().UTC())
	if errors.Is(err, metricstore.ErrCompactInProgress) {
		return
	}
	if err != nil {
		logger.Errorf("server", "Failed to compact metric store after writing %d rollup buckets: %v", written, err)
		return
	}
	if written > 0 {
		logger.Infof("server", "Metric store compacted %d rollup buckets", written)
	}
}

func cleanupMetricStore(ctx context.Context) {
	cleanupCtx, cancel := context.WithTimeout(ctx, 15*time.Minute)
	defer cancel()
	deleted, err := metricstore.CleanupExpired(cleanupCtx, time.Now().UTC())
	if errors.Is(err, metricstore.ErrCompactInProgress) {
		return
	}
	if err != nil {
		logger.Errorf("server", "Failed to clean expired metric data after deleting %d rows: %v", deleted, err)
		return
	}
	if deleted > 0 {
		logger.Infof("server", "Metric retention cleanup deleted %d rows", deleted)
	}
}
