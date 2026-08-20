import { apiErrorMessage } from './api';

const TOKEN_KEY = 'bidcraft_token';
const USER_KEY = 'bidcraft_user_id';

export interface Session {
  token: string;
  userId: number;
}

export function readSession(): Session | null {
  const token = localStorage.getItem(TOKEN_KEY);
  const userId = localStorage.getItem(USER_KEY);

  return token && userId ? { token, userId: Number(userId) } : null;
}

export function saveSession(session: Session): void {
  localStorage.setItem(TOKEN_KEY, session.token);
  localStorage.setItem(USER_KEY, String(session.userId));
}

export function clearSession(): void {
  localStorage.removeItem(TOKEN_KEY);
  localStorage.removeItem(USER_KEY);
}

export async function fetchCurrentUser(
  apiUrl: string,
  token: string,
): Promise<{ id: number; full_name: string } | null> {
  const response = await fetch(`${apiUrl}/api/v1/auth/me`, {
    headers: { Authorization: `Bearer ${token}` },
  });

  return response.ok ? response.json() : null;
}

export async function authenticate(
  apiUrl: string,
  mode: 'login' | 'register',
  email: string,
  password: string,
  fullName?: string,
): Promise<Session> {
  const response = await fetch(`${apiUrl}/api/v1/auth/${mode}`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(
      mode === 'register' ? { full_name: fullName, email, password } : { email, password },
    ),
  });

  const payload = await response.json();

  if (!response.ok) {
    throw new Error(apiErrorMessage(payload, 'No se pudo iniciar sesión.'));
  }

  const session = { token: payload.token, userId: payload.user.id };
  saveSession(session);

  return session;
}
