package auctions

import (
	"context"
	"log"
	"sync"
	"time"
)

const (
	// closeTimeout acota cada cierre disparado por un timer.
	closeTimeout = 15 * time.Second

	// clockGrace es el retraso que se añade al timer. El plazo (end_at) se fija
	// con el reloj del proceso Go, pero quien decide si la subasta venció es
	// PostgreSQL con el suyo, y ambos derivan unos milisegundos. Sin este margen
	// el timer se adelanta y el cierre no llega a hacerse.
	clockGrace = time.Second
)

// Scheduler programa el cierre de cada subasta con un time.AfterFunc.
//
// Un timer por subasta: el heap de timers del runtime de Go ya hace de
// planificador, así que no hace falta ni una goroutine dormida por subasta ni un
// scheduler propio.
type Scheduler struct {
	closer *Closer
	repo   *PostgresRepository

	mu      sync.Mutex
	timers  map[int64]*time.Timer
	stopped bool
}

func NewScheduler(closer *Closer, repo *PostgresRepository) *Scheduler {
	return &Scheduler{closer: closer, repo: repo, timers: make(map[int64]*time.Timer)}
}

// Schedule programa el cierre de una subasta. Si el plazo ya venció, se dispara
// de inmediato.
func (s *Scheduler) Schedule(auctionID int64, endAt time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.stopped {
		return
	}
	if _, exists := s.timers[auctionID]; exists {
		return
	}

	// El margen cubre la deriva de unos pocos milisegundos entre el reloj del
	// proceso y el de PostgreSQL, que es quien decide si la subasta ya venció.
	delay := time.Until(endAt) + clockGrace
	if delay < 0 {
		delay = 0
	}

	s.timers[auctionID] = time.AfterFunc(delay, func() { s.fire(auctionID) })
}

// RecoverActive reprograma los timers al arrancar, porque viven en memoria y se
// pierden al reiniciar el proceso. Las subastas cuyo plazo venció mientras el
// servidor estaba apagado se cierran de inmediato, porque Schedule las dispara
// con retardo cero.
func (s *Scheduler) RecoverActive(ctx context.Context) error {
	active, err := s.repo.ListActive(ctx)
	if err != nil {
		return err
	}

	for _, auction := range active {
		s.Schedule(auction.ID, auction.EndAt)
	}
	log.Printf("auction scheduler: %d active auctions scheduled", len(active))

	return nil
}

// Stop cancela los timers pendientes al apagar el servidor.
func (s *Scheduler) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.stopped = true
	for _, timer := range s.timers {
		timer.Stop()
	}
	s.timers = make(map[int64]*time.Timer)
}

func (s *Scheduler) fire(auctionID int64) {
	s.mu.Lock()
	delete(s.timers, auctionID)
	s.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), closeTimeout)
	defer cancel()

	closed, retryAt, err := s.closer.Close(ctx, auctionID)
	if err != nil {
		log.Printf("auction scheduler: could not close auction %d: %v", auctionID, err)
		return
	}

	// El timer se adelantó respecto al reloj de PostgreSQL: se reprograma en
	// lugar de dejar la subasta abierta para siempre.
	if !closed && !retryAt.IsZero() {
		log.Printf("auction scheduler: auction %d not expired yet, rescheduling", auctionID)
		s.Schedule(auctionID, retryAt)
	}
}
