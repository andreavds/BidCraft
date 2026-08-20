package auth

import "context"

// ctxKey es un tipo privado del paquete: nadie fuera de aquí puede fabricar esta
// clave, así que el user_id del contexto solo puede haberlo puesto ContextWithUserID.
type ctxKey int

const userIDKey ctxKey = iota

// ContextWithUserID guarda la identidad autenticada en el contexto.
// Solo debe llamarla el middleware de autenticación, después de validar el JWT.
func ContextWithUserID(ctx context.Context, userID int64) context.Context {
	return context.WithValue(ctx, userIDKey, userID)
}

// UserIDFromContext es la única forma que tienen los handlers de conocer al
// usuario autenticado. Nunca se debe leer un user_id del body o de la query.
func UserIDFromContext(ctx context.Context) (int64, bool) {
	userID, ok := ctx.Value(userIDKey).(int64)
	return userID, ok
}
