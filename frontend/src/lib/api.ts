export type AuctionStatus = 'ACTIVE' | 'FINISHED';

export interface Auction {
  id: number;
  title: string;
  artist: string;
  created_by: number;
  base_price: number;
  current_price: number;
  image_url: string | null;
  minimum_increment: number;
  minimum_bid: number;
  start_at: string;
  end_at: string;
  status: AuctionStatus;
  winner_id: number | null;
  winner_name: string | null;
}

export interface Bid {
  id: number;
  auction_id: number;
  user_id: number;
  user_name: string;
  amount: number;
  created_at: string;
}

/** URL del backend tal como la ve el navegador. Se fija durante el build. */
export const browserApiUrl = (): string =>
  import.meta.env.PUBLIC_API_URL ?? 'http://localhost:8080';

export interface ApiErrorPayload {
  error?: string;
  message?: string;
}

export function apiErrorMessage(payload: ApiErrorPayload, fallback: string): string {
  switch (payload.error) {
    case 'email_taken':
      return 'Este correo ya está registrado.';
    case 'invalid_credentials':
      return 'El correo o la contraseña no son correctos.';
    case 'unauthorized':
      return 'Necesitas iniciar sesión para continuar.';
    case 'auction_not_found':
      return 'No se encontró la subasta.';
    case 'auction_closed':
      return 'La subasta ya no acepta pujas.';
    case 'own_auction':
      return 'No puedes pujar en una subasta que publicaste.';
    case 'bid_too_low': {
      const amount = payload.message?.match(/([0-9]+(?:\.[0-9]{2}))/)?.[1];
      return amount ? `La puja mínima aceptada es $${amount}.` : 'La puja es demasiado baja.';
    }
    case 'validation_error':
      return translateValidationMessage(payload.message) ?? 'Revisa los datos introducidos.';
    case 'internal_error':
      return 'Ha ocurrido un error inesperado. Inténtalo de nuevo.';
    default:
      return fallback;
  }
}

function translateValidationMessage(message?: string): string | null {
  if (!message) return null;
  if (message === 'Request body must be valid JSON')
    return 'El cuerpo de la solicitud no es válido.';
  if (message === 'email is required') return 'El correo es obligatorio.';
  if (message === 'email is not a valid address') return 'Introduce un correo válido.';
  if (message === 'full_name is required') return 'El nombre completo es obligatorio.';
  if (message === 'password is required') return 'La contraseña es obligatoria.';
  if (message === 'amount must be greater than zero, in cents')
    return 'La puja debe ser mayor que cero.';
  if (message === 'title is required') return 'El título es obligatorio.';
  if (message === 'image_url must be an absolute http(s) URL')
    return 'La imagen debe usar una URL http(s) válida.';
  if (message.includes('duration_seconds'))
    return 'La duración debe estar entre 10 segundos y 7 días.';
  if (message.includes('base_price')) return 'El precio de salida no puede ser negativo.';
  if (message.includes('minimum_increment')) return 'El incremento mínimo debe ser mayor que cero.';
  if (message.includes('full_name must be'))
    return 'El nombre completo no puede superar los 120 caracteres.';
  if (message.includes('password must be at least'))
    return 'La contraseña debe tener al menos 8 caracteres.';
  if (message.includes('password must be at most'))
    return 'La contraseña no puede superar los 72 caracteres.';
  if (message.includes('image_url must be at most'))
    return 'La URL de la imagen es demasiado larga.';
  if (message.includes('limit must be') || message.includes('offset must be')) {
    return 'El valor de paginación no es válido.';
  }
  return null;
}

/**
 * URL del backend para las peticiones que hace el servidor de Astro.
 *
 * Dentro de Docker el backend no está en localhost, sino en el servicio
 * `backend`, así que se lee de las variables del proceso en tiempo de ejecución:
 * import.meta.env solo contiene las variables PUBLIC_ del build.
 */
export const serverApiUrl = (): string => {
  const processEnv = (globalThis as { process?: { env?: Record<string, string | undefined> } })
    .process?.env;

  return processEnv?.API_URL ?? browserApiUrl();
};

/** 12345 -> "$123.45" */
export function formatMoney(cents: number): string {
  return `$${(cents / 100).toFixed(2)}`;
}

/** Segundos que faltan para `endAt`, nunca negativos. */
export function secondsLeft(endAt: string): number {
  return Math.max(0, Math.floor((new Date(endAt).getTime() - Date.now()) / 1000));
}

/**
 * Cuenta atrás con segundos visibles: `mm:ss`, `h:mm:ss` o `d hh:mm:ss`.
 *
 * La usan la sala y las tarjetas del catálogo, y también el render en servidor
 * de la tarjeta, para que el primer pintado ya tenga el mismo formato que el
 * primer tic del navegador.
 */
export function formatCountdown(seconds: number): string {
  const pad = (value: number) => String(value).padStart(2, '0');

  const days = Math.floor(seconds / 86400);
  const hours = Math.floor((seconds % 86400) / 3600);
  const minutes = Math.floor((seconds % 3600) / 60);
  const rest = seconds % 60;

  if (days > 0) return `${days} d ${pad(hours)}:${pad(minutes)}:${pad(rest)}`;
  if (hours > 0) return `${hours}:${pad(minutes)}:${pad(rest)}`;

  return `${pad(minutes)}:${pad(rest)}`;
}

/** Texto del tiempo que queda para las tarjetas renderizadas en servidor. */
export function timeLeftLabel(endAt: string): string {
  const seconds = secondsLeft(endAt);

  return seconds === 0 ? 'Cerrando…' : formatCountdown(seconds);
}
