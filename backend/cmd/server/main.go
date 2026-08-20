package main

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"bidcraft/internal/auctions"
	"bidcraft/internal/auth"
	"bidcraft/internal/bids"
	"bidcraft/internal/database"
	appmiddleware "bidcraft/internal/middleware"
	"bidcraft/internal/uploads"
	"bidcraft/internal/websocket"
	"bidcraft/migrations"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

func main() {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		log.Fatal("DATABASE_URL is required")
	}

	jwtSecret := os.Getenv("JWT_SECRET")
	if jwtSecret == "" {
		log.Fatal("JWT_SECRET is required")
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := database.Connect(ctx, databaseURL)
	if err != nil {
		log.Fatalf("database connection failed: %v", err)
	}
	defer pool.Close()
	log.Println("connected to postgres")

	if err := database.Migrate(databaseURL, migrations.FS); err != nil {
		log.Fatalf("migrations failed: %v", err)
	}
	log.Println("migrations applied")

	authHandler := auth.NewHandler(auth.NewService(auth.NewPostgresUserRepository(pool), jwtSecret))

	// El gestor de locks se crea una sola vez y se comparte: es el que serializa
	// las operaciones sobre una misma subasta dentro de este proceso, tanto las
	// pujas como el cierre automático.
	auctionLocks := auctions.NewLocks()
	auctionRepo := auctions.NewPostgresRepository(pool)
	bidRepo := bids.NewPostgresRepository(pool)

	// Una sala WebSocket por subasta, compartida por el motor de pujas (bid_placed,
	// outbid) y por el cierre automático (auction_finished).
	hub := websocket.NewHub()

	// Los timers viven en memoria, así que al arrancar se reprograman los de las
	// subastas activas; las que vencieron con el proceso apagado se cierran de
	// inmediato.
	scheduler := auctions.NewScheduler(auctions.NewCloser(pool, auctionLocks, hub), auctionRepo)
	if err := scheduler.RecoverActive(ctx); err != nil {
		log.Fatalf("auction scheduler failed to start: %v", err)
	}
	defer scheduler.Stop()

	uploadDir := os.Getenv("UPLOAD_DIR")
	if uploadDir == "" {
		uploadDir = "uploads"
	}
	uploadHandler, err := uploads.NewHandler(uploadDir)
	if err != nil {
		log.Fatalf("uploads unavailable: %v", err)
	}

	auctionHandler := auctions.NewHandler(auctions.NewService(auctionRepo, scheduler))
	bidHandler := bids.NewHandler(bids.NewService(bidRepo, auctionLocks, hub))
	wsHandler := websocket.NewHandler(hub, bidRepo.AuctionExists, jwtSecret)

	router := chi.NewRouter()
	router.Use(middleware.RequestID, middleware.Logger, middleware.Recoverer, appmiddleware.CORS())
	router.Get("/health", health)
	router.Handle("/uploads/*", http.StripPrefix("/uploads/", uploadHandler.FileServer()))

	router.Route("/api/v1", func(r chi.Router) {
		r.Route("/auth", func(r chi.Router) {
			r.Post("/register", authHandler.Register)
			r.Post("/login", authHandler.Login)

			r.Group(func(r chi.Router) {
				r.Use(appmiddleware.RequireAuth(jwtSecret))
				r.Get("/me", authHandler.Me)
			})
		})

		r.Group(func(r chi.Router) {
			r.Use(appmiddleware.RequireAuth(jwtSecret))
			r.Post("/uploads", uploadHandler.Upload)
		})

		r.Route("/auctions", func(r chi.Router) {
			// Consultar el catálogo es público; crear requiere JWT.
			r.Get("/", auctionHandler.List)
			r.Get("/{id}", auctionHandler.Get)
			r.Get("/{id}/bids", bidHandler.List)
			// La sala en vivo es pública; el token opcional en query solo sirve
			// para dirigir el evento outbid.
			r.Get("/{id}/ws", wsHandler.Serve)

			r.Group(func(r chi.Router) {
				r.Use(appmiddleware.RequireAuth(jwtSecret))
				r.Post("/", auctionHandler.Create)
				r.Post("/{id}/bids", bidHandler.Place)
			})
		})
	})

	server := &http.Server{
		Addr:              ":" + port,
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
	}

	serverErr := make(chan error, 1)
	go func() {
		log.Printf("server listening on :%s", port)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
	}()

	// Either a shutdown signal or a fatal listener error ends the wait, and both
	// paths fall through to the graceful shutdown below so the pool always closes.
	select {
	case err := <-serverErr:
		log.Printf("server failed: %v", err)
	case <-ctx.Done():
		log.Println("shutdown signal received")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("shutdown error: %v", err)
	}
	log.Println("server stopped")
}

func health(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}
