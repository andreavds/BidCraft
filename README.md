# BidCraft
Plataforma de subastas de arte digital en tiempo real, desarrollada con Go, PostgreSQL, Astro + React y Docker.

## Levantar el entorno
1. Tener Docker Desktop instalado.
2. Abrir una terminal y entrar en la carpeta del proyecto:
`cd bidcraft`

3. Copiar la configuración:
`cp .env.example .env`

- En PowerShell:
`copy .env.example .env`

4. Levantar el entorno:
`docker compose up --build`

5. Abrir en el navegador: http://localhost:4321

- Para detenerlo:
Ctrl+C

- Para eliminar también los datos:
`docker compose down -v`

## Condiciones de carrera

El motor de pujas utiliza un mutex por subasta para serializar las pujas de una misma subasta sin bloquear las demás. Cada puja se ejecuta dentro de una transacción PostgreSQL con bloqueo de fila (`FOR UPDATE`). Esto garantiza que dos pujas simultáneas no puedan leer y actualizar el mismo precio al mismo tiempo.

El proceso es atómico:

1. Bloquear la subasta.
2. Leer el precio actual.
3. Validar el incremento mínimo.
4. Guardar la puja.
5. Actualizar el precio.
6. Confirmar la transacción.

Ante pujas simultáneas por la misma cantidad, solo una es aceptada y las demás se validan contra el nuevo precio.

El mismo mecanismo se utiliza al cerrar una subasta para evitar conflictos entre una puja y la expiración del temporizador.

## Prueba de concurrencia

Con el entorno levantado y Go instalado:

`go run scripts/concurrent-bids.go`

El script crea una subasta y envía 50 pujas simultáneas por la misma cantidad.

Resultado esperado:
```bash
Concurrent bids: 50
Bid amount: $110.00

Accepted: 1
Rejected: 49
Final price: $110.00

CONCURRENCY TEST PASSED
```

La prueba confirma que solo una puja concurrente es aceptada y que el precio final es consistente.