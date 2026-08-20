package middleware

import "net/http"

// CORS permite que el frontend, servido en otro puerto, llame a la API desde el
// navegador. Se refleja el origen de la petición en vez de fijar uno concreto
// para que funcione igual en local, en Docker o desde otra máquina de la red.
//
// No se usan cookies —la sesión viaja en la cabecera Authorization— así que no
// hace falta habilitar credenciales.
func CORS() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if origin := r.Header.Get("Origin"); origin != "" {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
				w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
				w.Header().Set("Access-Control-Max-Age", "600")
				w.Header().Add("Vary", "Origin")
			}

			// El preflight se responde aquí: no hay ninguna ruta OPTIONS.
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
