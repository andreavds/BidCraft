// Package httpx contiene los helpers de respuesta HTTP compartidos por los handlers.
package httpx

import (
	"encoding/json"
	"log"
	"net/http"
)

// ErrorResponse es el formato único de error de la API.
type ErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message"`
}

// JSON escribe una respuesta con el status indicado.
func JSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if payload == nil {
		return
	}
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		// El status ya se envió; solo queda dejar constancia en el log del servidor.
		log.Printf("write json response: %v", err)
	}
}

// Error responde con el formato {"error": code, "message": message}.
// El mensaje es siempre apto para el cliente: nunca errores SQL ni internos.
func Error(w http.ResponseWriter, status int, code, message string) {
	JSON(w, status, ErrorResponse{Error: code, Message: message})
}
