package main

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/choiceoh/phaeton/backend/internal/ai"
	"github.com/choiceoh/phaeton/backend/internal/config"
	"github.com/choiceoh/phaeton/backend/internal/db"
	"github.com/choiceoh/phaeton/backend/internal/events"
	"github.com/choiceoh/phaeton/backend/internal/handler"
	"github.com/choiceoh/phaeton/backend/internal/infra/lifecycle"
	"github.com/choiceoh/phaeton/backend/internal/infra/logging"
	"github.com/choiceoh/phaeton/backend/internal/infra/metrics"
	"github.com/choiceoh/phaeton/backend/internal/infra/workerpool"
	"github.com/choiceoh/phaeton/backend/internal/middleware"
	"github.com/choiceoh/phaeton/backend/internal/migration"
	"github.com/choiceoh/phaeton/backend/internal/notify"
	"github.com/choiceoh/phaeton/backend/internal/samlsp"
	"github.com/choiceoh/phaeton/backend/internal/schema"
	"github.com/choiceoh/phaeton/backend/internal/sync"
	"github.com/choiceoh/phaeton/backend/internal/sync/amaranth"
)

//go:embed static/*
var staticFS embed.FS

const version = "0.2.0"

func main() {
	os.Exit(run())
}

// run initializes and starts the HTTP server. Initialization order:
//  1. Logging (JSON in prod, colored console in dev)
//  2. JWT_SECRET validation (required in prod)
//  3. Database pool (pgxpool with configurable limits)
//  4. Bootstrap (idempotent schema creation: auth, _meta, data schemas)
//  5. Seed director (creates initial admin user if none exist)
//  6. Schema layer (Store -> Cache.Load -> MigrationEngine)
//  7. Event bus (in-process pub/sub)
//  8. HTTP handlers (Schema, Dynamic, View, AI, etc.)
//  9. Rate limiters (API: 60 req/s per user, Login: 5 failures -> 30min lockout)
// 10. Optional integrations (SAML SP, Amaranth sync, email notifier)
// 11. Automation (engine + scheduler subscribe to event bus)
// 12. Router (chi with global + per-route middleware)
// 13. HTTP server with graceful shutdown (lifecycle.RunWithSignals)
func run() int {
	// Load all configuration from environment (validates production requirements).
	appCfg, err := config.Load()
	if err != nil {
		// Cannot use structured logger yet — it depends on config.
		fmt.Fprintf(os.Stderr, "config: %v\n", err)
		return 1
	}

	// Logging: JSON in production, console otherwise.
	var logHandler slog.Handler
	if appCfg.IsProd {
		logHandler = slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo})
	} else {
		logHandler = logging.NewConsoleHandler(os.Stderr, slog.LevelInfo, !appCfg.NoColor)
	}
	logger := slog.New(logHandler)
	slog.SetDefault(logger)

	// Database pool.
	pool, err := db.NewPool(context.Background(), appCfg.DB, appCfg.IsProd)
	if err != nil {
		logger.Error("failed to connect to database", "error", err)
		return 1
	}
	defer pool.Close()

	// Bootstrap all schemas + tables (idempotent).
	if err := db.Bootstrap(context.Background(), pool); err != nil {
		logger.Error("bootstrap failed", "error", err)
		return 1
	}
	logger.Info("database bootstrap complete")

	// Seed the initial director user if none exist.
	if err := handler.SeedDirector(context.Background(), pool); err != nil {
		logger.Warn("seed director failed", "error", err)
	}

	// Schema engine (store + cache + migration engine + data engine).
	store := schema.NewStore(pool)
	cache := schema.NewCache(store)
	if err := cache.Load(context.Background()); err != nil {
		logger.Error("cache load failed", "error", err)
		return 1
	}
	logger.Info("schema cache loaded", "collections", len(cache.Collections()))

	migEngine := migration.NewEngine(pool, store, cache)

	// Event bus.
	bus := events.NewBus()

	// Schema & dynamic handlers (PR #53 — built-in dynamic handler).
	schemaHandler := handler.NewSchemaHandler(pool, store, cache, migEngine)
	dynHandler := handler.NewDynHandler(pool, cache, bus)
	viewHandler := handler.NewViewHandler(store)
	savedViewHandler := handler.NewSavedViewHandler(store)
	historyHandler := handler.NewHistoryHandler(pool, cache)
	memberHandler := handler.NewMemberHandler(pool)

	// AI client (local vLLM).
	aiClient := ai.NewClient(appCfg.AI)
	aiHandler := handler.NewAIHandler(aiClient, store, pool, cache)

	// SSE real-time events.
	sseBroker := events.NewBroker()
	sseHandler := handler.NewSSEHandler(sseBroker)
	// Forward bus events to SSE broker.
	bus.Subscribe(func(_ context.Context, ev events.Event) {
		sseBroker.Broadcast(events.SSEMessage{
			Type:           string(ev.Type),
			CollectionID:   ev.CollectionID,
			CollectionSlug: ev.CollectionSlug,
			RecordID:       ev.RecordID,
			ActorUserID:    ev.ActorUserID,
			ActorName:      ev.ActorName,
		})
	})

	// API rate limiter: 60 req/s per user, burst of 120.
	apiLimiter := middleware.NewAPILimiter(60, 120)
	defer apiLimiter.Close()

	// Notification subscriber.
	commentHandler := handler.NewCommentHandler(pool, cache, bus)
	notifHandler := handler.NewNotificationHandler(pool)
	handler.SubscribeNotifications(pool, bus, cache)

	// Worker pool retained for future background tasks (bid scheduler, etc.).
	wp := workerpool.New(0)
	_ = wp

	// Login rate limiter: 5 failures / 15 minutes → 30 minute lockout.
	loginLimiter := middleware.NewRateLimiter(5, 15*60*1000, 30*60*1000)
	defer loginLimiter.Close()

	// SAML SP (optional).
	var samlMiddleware *samlsp.Middleware
	if appCfg.SAML != nil {
		sp, err := samlsp.New(&samlsp.Config{
			EntityID:       appCfg.SAML.EntityID,
			RootURL:        appCfg.SAML.RootURL,
			CertPath:       appCfg.SAML.CertPath,
			KeyPath:        appCfg.SAML.KeyPath,
			IdPMetadataURL: appCfg.SAML.IdPMetadataURL,
		})
		if err != nil {
			logger.Error("SAML SP init failed", "error", err)
			return 1
		}
		samlMiddleware = sp
	}

	// Sync runner (Amaranth HR integration).
	syncRunner := sync.NewRunner(6*time.Hour, logger)
	if appCfg.Amaranth != nil {
		src := amaranth.NewSource(pool, &amaranth.Config{
			BaseURL: appCfg.Amaranth.BaseURL,
			APIKey:  appCfg.Amaranth.APIKey,
		}, logger)
		syncRunner.Register(src)
		logger.Info("amaranth sync registered")
	}

	// Email + Report handler (optional).
	var reportHandler *handler.ReportHandler
	if appCfg.SMTP != nil {
		smtpCfg := notify.SMTPConfig{
			Host:     appCfg.SMTP.Host,
			Port:     appCfg.SMTP.Port,
			Username: appCfg.SMTP.Username,
			Password: appCfg.SMTP.Password,
			From:     appCfg.SMTP.From,
		}
		emailNotifier := notify.NewEmailNotifier(smtpCfg, func(ctx context.Context, userID string) (string, error) {
			var email string
			err := pool.QueryRow(ctx, "SELECT email FROM auth.users WHERE id = $1", userID).Scan(&email)
			return email, err
		})
		reportHandler = handler.NewReportHandler(dynHandler, emailNotifier)
		logger.Info("SMTP email configured", "host", smtpCfg.Host)
	} else {
		reportHandler = handler.NewReportHandler(dynHandler, nil)
	}

	// Router.
	r := buildRouter(routerConfig{
		pool:          pool,
		cache:         cache,
		schemaH:       schemaHandler,
		dynH:          dynHandler,
		viewH:         viewHandler,
		savedViewH:    savedViewHandler,
		histH:         historyHandler,
		memberH:       memberHandler,
		commentH:      commentHandler,
		notifH:        notifHandler,
		aiH:           aiHandler,
		sseH:          sseHandler,
		reportH:       reportHandler,
		logger:        logger,
		loginLimiter:  loginLimiter,
		apiLimiter:    apiLimiter,
		samlMW:        samlMiddleware,
		jwtSecret:     appCfg.JWTSecret,
		authDisabled:  appCfg.AuthDisabled,
		corsOrigin:    appCfg.CORSOrigin,
		isProd:        appCfg.IsProd,
		webhookSecret: appCfg.WebhookSecret,
	})

	addr := appCfg.Addr
	srv := &http.Server{
		Addr:              addr,
		Handler:           r,
		ReadHeaderTimeout: 10 * time.Second,
		WriteTimeout:      150 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	startedAt := time.Now()

	return lifecycle.RunWithSignals(func(ctx context.Context) error {
		ln, err := net.Listen("tcp", addr)
		if err != nil {
			return fmt.Errorf("listen: %w", err)
		}

		// Start sync runner in background.
		go syncRunner.Start(ctx)

		logging.PrintBanner(os.Stderr, logging.BannerInfo{
			Version: version,
			Addr:    ln.Addr().String(),
			DB:      "connected",
		}, !appCfg.IsProd)

		if appCfg.AuthDisabled {
			slog.Warn("AUTH_DISABLED=true — authentication is bypassed, all requests use dev user (director)")
		}

		go func() {
			<-ctx.Done()
			logging.PrintShutdown(os.Stderr, time.Since(startedAt), !appCfg.IsProd)

			// Stop background workers before draining HTTP connections.
			// syncRunner stops automatically via ctx cancellation.
			sseBroker.Close()
			wp.Wait()
			logger.Info("background workers stopped")

			shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			if err := srv.Shutdown(shutdownCtx); err != nil {
				logger.Error("shutdown failed", "error", err)
			}
		}()

		if err := srv.Serve(ln); err != http.ErrServerClosed {
			return err
		}
		return nil
	}, logger)
}

type routerConfig struct {
	pool          *pgxpool.Pool
	cache         *schema.Cache
	schemaH       *handler.SchemaHandler
	dynH          *handler.DynHandler
	viewH         *handler.ViewHandler
	savedViewH    *handler.SavedViewHandler
	histH         *handler.HistoryHandler
	memberH       *handler.MemberHandler
	commentH      *handler.CommentHandler
	notifH        *handler.NotificationHandler
	aiH           *handler.AIHandler
	sseH          *handler.SSEHandler
	reportH       *handler.ReportHandler
	logger        *slog.Logger
	loginLimiter  *middleware.RateLimiter
	apiLimiter    *middleware.APILimiter
	samlMW        *samlsp.Middleware
	jwtSecret     string
	authDisabled  bool
	corsOrigin    string
	isProd        bool
	webhookSecret string
}

// buildRouter assembles the chi router with the following route groups:
//   - /api/health, /metrics: public health check and Prometheus metrics
//   - /api/auth/*: public login/logout + SAML endpoints
//   - /api/hooks/*: public webhook receiver (HMAC-verified)
//   - Protected group (RequireAuth + API rate limiter):
//   - /api/auth/me: current user profile
//   - /api/users, /api/subsidiaries, /api/departments: org management
//   - /api/schema/*: collection/field/view/automation CRUD (role-gated)
//   - /api/data/*: dynamic record CRUD with collection-level access control
//   - /api/ai/*: AI-powered features (build, chat, formula, filter, etc.)
//   - /api/notifications, /api/webhooks, /api/events: notifications + SSE
//   - /*: SPA catch-all serving embedded static files with index.html fallback
func buildRouter(cfg routerConfig) *chi.Mux {
	r := chi.NewRouter()

	// Global middleware.
	r.Use(chimw.RequestID)
	r.Use(chimw.RealIP)
	r.Use(middleware.Logger(cfg.logger))
	r.Use(middleware.CORS(cfg.corsOrigin, cfg.isProd))
	r.Use(chimw.Recoverer)

	// Health — includes DB connectivity check.
	r.Get("/api/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := cfg.pool.Ping(r.Context()); err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			w.Write([]byte(`{"status":"unhealthy","db":"unreachable"}`))
			return
		}
		w.Write([]byte(`{"status":"ok","db":"connected"}`))
	})

	// Metrics — Prometheus-compatible text format.
	r.Get("/metrics", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		metrics.WriteMetrics(w)
	})

	// Auth (public).
	r.Post("/api/auth/login", handler.Login(cfg.pool, cfg.loginLimiter, cfg.jwtSecret))
	r.Post("/api/auth/logout", handler.Logout())

	// Webhooks (public — HMAC-verified via WEBHOOK_SECRET).
	webhookH := handler.NewWebhookHandler(cfg.pool, cfg.webhookSecret)
	r.Post("/api/hooks/{topic}", webhookH.Receive)

	// SAML SP endpoints (metadata + ACS).
	if cfg.samlMW != nil {
		r.Handle("/saml/*", cfg.samlMW.Handler())
	}

	// Protected routes.
	r.Group(func(r chi.Router) {
		r.Use(middleware.RequireAuth(cfg.jwtSecret, cfg.authDisabled))
		r.Use(cfg.apiLimiter.Middleware())

		// Current user.
		r.Get("/api/auth/me", handler.Me(cfg.pool, cfg.authDisabled))
		r.Patch("/api/auth/me", handler.UpdateMe(cfg.pool))
		r.Post("/api/auth/password", handler.ChangePassword(cfg.pool))

		// Users (list: all, write: director only).
		r.Get("/api/users", handler.ListUsers(cfg.pool))
		r.Get("/api/users/{id}", handler.GetUser(cfg.pool))
		r.Post("/api/users", handler.CreateUser(cfg.pool))
		r.Patch("/api/users/{id}", handler.UpdateUser(cfg.pool))

		// Subsidiaries.
		r.Get("/api/subsidiaries", handler.ListSubsidiaries(cfg.pool))
		r.Get("/api/subsidiaries/{id}", handler.GetSubsidiary(cfg.pool))
		r.Group(func(r chi.Router) {
			r.Use(middleware.RequireRole("director"))
			r.Post("/api/subsidiaries", handler.CreateSubsidiary(cfg.pool))
			r.Patch("/api/subsidiaries/{id}", handler.UpdateSubsidiary(cfg.pool))
			r.Delete("/api/subsidiaries/{id}", handler.DeleteSubsidiary(cfg.pool))
		})

		// Departments.
		r.Get("/api/departments", handler.ListDepartments(cfg.pool))
		r.Get("/api/departments/{id}", handler.GetDepartment(cfg.pool))
		r.Group(func(r chi.Router) {
			r.Use(middleware.RequireRole("director"))
			r.Post("/api/departments", handler.CreateDepartment(cfg.pool))
			r.Patch("/api/departments/{id}", handler.UpdateDepartment(cfg.pool))
			r.Delete("/api/departments/{id}", handler.DeleteDepartment(cfg.pool))
		})

		// My tasks (cross-collection, process-based).
		r.Get("/api/my-tasks", cfg.schemaH.MyTasks)

		// Global calendar (cross-collection).
		r.Get("/api/calendar/events", cfg.schemaH.GlobalCalendarEvents)

		// Schema API — collection/field/migration management.
		r.Route("/api/schema", func(r chi.Router) {
			// Read-only: all authenticated users.
			r.Get("/collections", cfg.schemaH.ListCollections)
			r.Get("/collections/counts", cfg.schemaH.CollectionCounts)
			r.Get("/collections/{id}", cfg.schemaH.GetCollection)
			r.Get("/migrations/history", cfg.schemaH.MigrationHistory)
			r.Get("/relationship-graph", cfg.schemaH.RelationshipGraph)
			r.Get("/collections/{id}/process/transitions", cfg.schemaH.AvailableTransitions)

			// Create collection: all authenticated users.
			r.Post("/collections", cfg.schemaH.CreateCollection)

			// Modify collection: director/pm OR the collection creator.
			r.Group(func(r chi.Router) {
				r.Use(middleware.RequireCollectionOwnerOrRole(cfg.cache, "director", "pm"))
				r.Patch("/collections/{id}", cfg.schemaH.UpdateCollection)
				r.Delete("/collections/{id}", cfg.schemaH.DeleteCollection)

				r.Get("/collections/{id}/process", cfg.schemaH.GetProcess)
				r.Put("/collections/{id}/process", cfg.schemaH.SaveProcess)

				r.Post("/collections/{id}/fields", cfg.schemaH.AddField)
			})

			// Field update/delete: director/pm OR the field's collection creator.
			r.Group(func(r chi.Router) {
				r.Use(middleware.RequireFieldOwnerOrRole(cfg.cache, "director", "pm"))
				r.Patch("/fields/{fieldId}", cfg.schemaH.UpdateField)
				r.Delete("/fields/{fieldId}", cfg.schemaH.DeleteField)
			})

			// Migration rollback: director/pm only.
			r.Group(func(r chi.Router) {
				r.Use(middleware.RequireRole("director", "pm"))
				r.Post("/migrations/rollback/{migrationId}", cfg.schemaH.RollbackMigration)
			})

			// Collection members
			r.Get("/collections/{id}/members", cfg.memberH.List)
			r.Post("/collections/{id}/members", cfg.memberH.Add)
			r.Patch("/collections/{id}/members/{userId}", cfg.memberH.Update)
			r.Delete("/collections/{id}/members/{userId}", cfg.memberH.Remove)

			// Views: read/write for all authenticated users.
			r.Get("/collections/{id}/views", cfg.viewH.ListViews)
			r.Post("/collections/{id}/views", cfg.viewH.CreateView)
			r.Patch("/views/{viewId}", cfg.viewH.UpdateView)
			r.Delete("/views/{viewId}", cfg.viewH.DeleteView)

			// Saved views (filter/sort persistence).
			r.Get("/collections/{id}/saved-views", cfg.savedViewH.ListSavedViews)
			r.Post("/collections/{id}/saved-views", cfg.savedViewH.CreateSavedView)
			r.Patch("/saved-views/{savedViewId}", cfg.savedViewH.UpdateSavedView)
			r.Delete("/saved-views/{savedViewId}", cfg.savedViewH.DeleteSavedView)

		})

		// Dynamic API — auto-generated CRUD for data tables.
		r.Route("/api/data", func(r chi.Router) {
			r.Use(middleware.CollectionAccess(cfg.pool))
			r.Get("/{slug}", cfg.dynH.List)
			r.Get("/{slug}/totals", cfg.dynH.Totals)
			r.Get("/{slug}/defaults", cfg.dynH.GetDefaults)
			r.Get("/{slug}/similar", cfg.dynH.SimilarRecords)
			r.Get("/{slug}/aggregate", cfg.dynH.Aggregate)
			r.Post("/{slug}/aggregate/batch", cfg.dynH.BatchAggregate)
			r.Get("/{slug}/calendar", cfg.dynH.CalendarView)
			r.Get("/{slug}/gantt", cfg.dynH.GanttView)
			r.Get("/{slug}/kanban", cfg.dynH.KanbanView)
			r.Get("/{slug}/export.csv", cfg.dynH.ExportCSV)
			r.Get("/{slug}/export.pdf", cfg.dynH.ExportPDF)
			r.Post("/{slug}/email-report", cfg.reportH.EmailReport)
			r.Get("/{slug}/{id}", cfg.dynH.Get)

			// Write: director, pm, engineer.
			r.Group(func(r chi.Router) {
				r.Use(middleware.RequireRole("director", "pm", "engineer"))
				r.Post("/{slug}", cfg.dynH.Create)
				r.Post("/{slug}/bulk", cfg.dynH.BulkCreate)
				r.Post("/{slug}/formula-preview", cfg.dynH.FormulaPreview)
				r.Patch("/{slug}/batch", cfg.dynH.BatchUpdate)
				r.Delete("/{slug}/bulk", cfg.dynH.BulkDelete)
				r.Post("/{slug}/import", cfg.dynH.ImportCSV)
				r.Patch("/{slug}/{id}", cfg.dynH.Update)
				r.Delete("/{slug}/{id}", cfg.dynH.Delete)
			})

			// Record history
			r.Get("/{slug}/{id}/history", cfg.histH.ListRecordHistory)

			// Comments
			r.Get("/{slug}/{id}/comments", cfg.commentH.List)
			r.Post("/{slug}/{id}/comments", cfg.commentH.Create)
			r.Patch("/{slug}/{id}/comments/{commentId}", cfg.commentH.Update)
			r.Delete("/{slug}/{id}/comments/{commentId}", cfg.commentH.Delete)
		})

		// File upload & download (authenticated).
		r.Post("/api/upload", handler.Upload)
		r.Get("/api/uploads/{filename}", handler.ServeUpload)

		// AI endpoints — longer write deadline for LLM inference.
		// Only site-usage chat remains after the Topbids refactor.
		r.Route("/api/ai", func(ai chi.Router) {
			ai.Get("/health", cfg.aiH.HealthCheck)
			ai.Post("/chat", cfg.aiH.Chat)
		})

		// Notifications
		r.Get("/api/notifications", cfg.notifH.List)
		r.Get("/api/notifications/unread-count", cfg.notifH.UnreadCount)
		r.Patch("/api/notifications/{id}/read", cfg.notifH.MarkRead)
		r.Post("/api/notifications/read-all", cfg.notifH.MarkAllRead)

		// Webhook events (read/delete: director only).
		r.Group(func(r chi.Router) {
			r.Use(middleware.RequireRole("director"))
			r.Get("/api/webhooks", webhookH.List)
			r.Get("/api/webhooks/{id}", webhookH.Get)
			r.Delete("/api/webhooks/{id}", webhookH.Delete)
		})

		// SSE real-time events
		r.Get("/api/events", cfg.sseH.Stream)
	})

	// SPA static files — catch-all for non-API routes.
	serveSPA(r)

	return r
}

func serveSPA(r *chi.Mux) {
	sub, err := fs.Sub(staticFS, "static")
	if err != nil {
		slog.Warn("no embedded static files, SPA serving disabled")
		return
	}

	fileServer := http.FileServer(http.FS(sub))

	indexHTML, err := fs.ReadFile(sub, "index.html")
	if err != nil {
		slog.Warn("index.html not found in embedded static", "error", err)
		indexHTML = []byte("<!DOCTYPE html><html><body>phaeton</body></html>")
	}

	r.Get("/*", func(w http.ResponseWriter, req *http.Request) {
		path := strings.TrimPrefix(req.URL.Path, "/")
		if path == "" {
			writeIndex(w, indexHTML)
			return
		}

		info, err := fs.Stat(sub, path)
		if err != nil || info.IsDir() {
			writeIndex(w, indexHTML)
			return
		}
		fileServer.ServeHTTP(w, req)
	})
}

func writeIndex(w http.ResponseWriter, indexHTML []byte) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Write(indexHTML)
}

