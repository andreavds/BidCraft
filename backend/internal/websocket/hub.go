// Package websocket difunde los eventos de una subasta a los clientes
// conectados a su sala.
package websocket

import (
	"encoding/json"
	"log"
	"sync"

	"github.com/gorilla/websocket"
)

// sendBuffer es cuántos eventos se le toleran a un cliente antes de darlo por
// perdido. Sin este margen, un cliente lento bloquearía la difusión al resto.
const sendBuffer = 16

// catalogRoom es la sala del catálogo: la de quien mira la lista de subastas en
// lugar de una sala concreta. Reutiliza el mismo mapa de salas con un id que
// ninguna subasta puede tener, así que sus eventos no se cruzan con los de una
// subasta ni al revés.
const catalogRoom int64 = 0

// Event es el formato de todos los mensajes que salen hacia el navegador.
type Event struct {
	Type string `json:"type"`
	Data any    `json:"data"`
}

// client es una conexión abierta. userID es 0 cuando el cliente se conectó sin
// token: puede mirar la sala, pero no recibe eventos dirigidos como outbid.
type client struct {
	conn   *websocket.Conn
	userID int64
	send   chan []byte
}

// close corta la conexión. Cerrarla hace que readPump falle y dé de baja al
// cliente. conn es nil en los tests del hub, que solo comprueban el reparto.
func (c *client) close() {
	if c.conn != nil {
		c.conn.Close()
	}
}

// Hub agrupa las conexiones por subasta. Cada subasta es una sala y los eventos
// de una nunca llegan a los clientes de otra.
type Hub struct {
	mu    sync.RWMutex
	rooms map[int64]map[*client]bool
}

func NewHub() *Hub {
	return &Hub{rooms: make(map[int64]map[*client]bool)}
}

// Broadcast envía un evento a todos los clientes de una subasta.
func (h *Hub) Broadcast(auctionID int64, event Event) {
	h.deliver(auctionID, event, func(*client) bool { return true })
}

// BroadcastCatalog envía un evento a los clientes conectados al catálogo, no a
// los de una subasta concreta. Lo usa la creación de subastas, que cambia la
// lista pero no ninguna sala.
func (h *Hub) BroadcastCatalog(event Event) {
	h.Broadcast(catalogRoom, event)
}

// SendToUser envía un evento solo a las conexiones de un usuario concreto dentro
// de una subasta. Lo usa outbid, que va dirigido al postor superado.
func (h *Hub) SendToUser(auctionID, userID int64, event Event) {
	h.deliver(auctionID, event, func(c *client) bool { return c.userID == userID })
}

func (h *Hub) deliver(auctionID int64, event Event, match func(*client) bool) {
	payload, err := json.Marshal(event)
	if err != nil {
		log.Printf("websocket: could not encode %s event: %v", event.Type, err)
		return
	}

	// Con RLock, varias difusiones simultáneas no se estorban; solo alta y baja
	// de clientes toman el lock exclusivo.
	h.mu.RLock()
	defer h.mu.RUnlock()

	for c := range h.rooms[auctionID] {
		if !match(c) {
			continue
		}

		select {
		case c.send <- payload:
		default:
			// Cliente saturado: se cierra su conexión y el resto sigue. La
			// escritura nunca bloquea a quien difunde el evento.
			log.Printf("websocket: dropping a slow client from auction %d", auctionID)
			c.close()
		}
	}
}

func (h *Hub) register(auctionID int64, c *client) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.rooms[auctionID] == nil {
		h.rooms[auctionID] = make(map[*client]bool)
	}
	h.rooms[auctionID][c] = true
}

func (h *Hub) unregister(auctionID int64, c *client) {
	h.mu.Lock()
	defer h.mu.Unlock()

	room := h.rooms[auctionID]
	if room == nil {
		return
	}

	if _, ok := room[c]; ok {
		delete(room, c)
		close(c.send)
	}
	if len(room) == 0 {
		delete(h.rooms, auctionID)
	}
}

// clientCount es el número de conexiones de una subasta. Solo para tests.
func (h *Hub) clientCount(auctionID int64) int {
	h.mu.RLock()
	defer h.mu.RUnlock()

	return len(h.rooms[auctionID])
}
