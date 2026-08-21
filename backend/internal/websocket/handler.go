package websocket

import (
	"context"
	"log"
	"net/http"
	"strconv"

	"bidcraft/internal/auth"
	"bidcraft/internal/httpx"

	"github.com/go-chi/chi/v5"
	"github.com/gorilla/websocket"
)

// AuctionExistsFunc comprueba que la subasta existe antes de abrir la conexión.
type AuctionExistsFunc func(ctx context.Context, auctionID int64) (bool, error)

type Handler struct {
	hub       *Hub
	exists    AuctionExistsFunc
	jwtSecret string
	upgrader  websocket.Upgrader
}

func NewHandler(hub *Hub, exists AuctionExistsFunc, jwtSecret string) *Handler {
	return &Handler{
		hub:       hub,
		exists:    exists,
		jwtSecret: jwtSecret,
		upgrader: websocket.Upgrader{
			// El frontend Astro corre en otro puerto que la API, así que la
			// conexión siempre es de otro origen. En una prueba local basta.
			CheckOrigin: func(*http.Request) bool { return true },
		},
	}
}

// Serve atiende GET /api/v1/auctions/{id}/ws.
//
// La conexión es pública: cualquiera puede seguir una subasta. El token es
// opcional y solo sirve para saber a quién dirigir el evento outbid; se pasa por
// query porque el navegador no permite cabeceras propias al abrir un WebSocket.
func (h *Handler) Serve(w http.ResponseWriter, r *http.Request) {
	auctionID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || auctionID <= 0 {
		httpx.Error(w, http.StatusBadRequest, "validation_error", "Auction id must be a positive integer")
		return
	}

	exists, err := h.exists(r.Context(), auctionID)
	if err != nil {
		log.Printf("websocket: could not check auction %d: %v", auctionID, err)
		httpx.Error(w, http.StatusInternalServerError, "internal_error", "Unexpected server error")
		return
	}
	if !exists {
		httpx.Error(w, http.StatusNotFound, "auction_not_found", "Auction not found")
		return
	}

	var userID int64
	if token := r.URL.Query().Get("token"); token != "" {
		userID, err = auth.ParseToken(h.jwtSecret, token)
		if err != nil {
			httpx.Error(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
			return
		}
	}

	h.serveRoom(w, r, auctionID, userID)
}

// ServeCatalog atiende GET /api/v1/auctions/ws, la sala del catálogo.
//
// Es pública y sin token: solo difunde altas de subastas, que ya son públicas en
// GET /api/v1/auctions. Sirve para que la lista se actualice sola cuando otro
// usuario publica una subasta, sin recargar la página.
func (h *Handler) ServeCatalog(w http.ResponseWriter, r *http.Request) {
	h.serveRoom(w, r, catalogRoom, 0)
}

// serveRoom sube la conexión a WebSocket y la mantiene dada de alta en una sala
// hasta que el cliente se va.
func (h *Handler) serveRoom(w http.ResponseWriter, r *http.Request, room, userID int64) {
	conn, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("websocket: upgrade failed for room %d: %v", room, err)
		return
	}

	c := &client{conn: conn, userID: userID, send: make(chan []byte, sendBuffer)}
	h.hub.register(room, c)
	log.Printf("websocket connected: room=%d user=%d clients=%d", room, userID, h.hub.clientCount(room))

	// Una goroutine escribe y la actual lee: así el hub nunca escribe en la
	// conexión y una conexión rota no afecta a los demás clientes.
	go c.writePump()
	c.readPump()

	h.hub.unregister(room, c)
	conn.Close()
	log.Printf("websocket disconnected: room=%d user=%d clients=%d", room, userID, h.hub.clientCount(room))
}

// readPump descarta lo que envíe el cliente —la sala es de solo lectura— y
// existe para detectar la desconexión, que llega como error de lectura.
func (c *client) readPump() {
	for {
		if _, _, err := c.conn.ReadMessage(); err != nil {
			return
		}
	}
}

// writePump serializa todas las escrituras de una conexión en una sola goroutine:
// gorilla/websocket no admite escrituras concurrentes.
func (c *client) writePump() {
	for payload := range c.send {
		if err := c.conn.WriteMessage(websocket.TextMessage, payload); err != nil {
			// El cliente se fue; readPump lo detectará y lo dará de baja.
			c.conn.Close()
			return
		}
	}

	// El canal se cerró en unregister: despedida ordenada.
	_ = c.conn.WriteMessage(websocket.CloseMessage,
		websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
}
