package auctions

import "sync"

// Locks da un mutex propio a cada subasta.
//
// Un mutex global serializaría las operaciones de subastas independientes; con
// este mapa, dos usuarios pujando en subastas distintas no se estorban, mientras
// que dos operaciones sobre la misma subasta se serializan.
//
// Es una optimización de contención dentro del proceso: mantiene la cola de
// espera fuera de PostgreSQL en lugar de acumular conexiones bloqueadas sobre la
// misma fila. La garantía de corrección es SELECT ... FOR UPDATE, que sigue
// aplicando aunque haya varias instancias del backend y estos mutexes no se vean
// entre sí.
//
// Lo comparten el motor de pujas y el cierre automático: ambas operaciones
// modifican el estado de una subasta y deben serializarse entre sí.
type Locks struct {
	// auctionID int64 -> *sync.Mutex
	locks sync.Map
}

func NewLocks() *Locks {
	return &Locks{}
}

// Get devuelve el mutex de una subasta, creándolo la primera vez.
//
// LoadOrStore es atómico: si dos goroutines llegan a la vez a una subasta nueva,
// ambas reciben el mismo mutex y una de las dos descarta el que había creado.
//
// Las entradas nunca se eliminan. Borrar un mutex del mapa sería una carrera en
// sí misma —otra goroutine podría estar a punto de bloquearse sobre él— y
// hacerlo con seguridad exigiría contar referencias. El coste de no borrarlas es
// un mutex por subasta que haya tenido actividad.
func (l *Locks) Get(auctionID int64) *sync.Mutex {
	lock, _ := l.locks.LoadOrStore(auctionID, &sync.Mutex{})
	return lock.(*sync.Mutex)
}
