import { useState, type FormEvent } from 'react';
import { authenticate } from '../lib/session';

interface Props {
  apiUrl: string;
  /** A dónde volver después de entrar; por defecto, al catálogo. */
  redirectTo?: string;
  /** Pantalla inicial: iniciar sesión o crear cuenta. */
  initialMode?: 'login' | 'register';
}

export default function LoginForm({ apiUrl, redirectTo = '/auctions', initialMode = 'login' }: Props) {
  const [mode, setMode] = useState<'login' | 'register'>(initialMode);
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);

  const registering = mode === 'register';

  async function submit(formEvent: FormEvent<HTMLFormElement>) {
    formEvent.preventDefault();
    setError('');
    setBusy(true);

    const form = new FormData(formEvent.currentTarget);

    try {
      await authenticate(
        apiUrl,
        mode,
        String(form.get('email')),
        String(form.get('password')),
        String(form.get('full_name') ?? ''),
      );
      window.location.href = redirectTo;
    } catch (failure) {
      setError(failure instanceof Error ? failure.message : 'No se pudo continuar.');
      setBusy(false);
    }
  }

  function switchMode() {
    setMode(registering ? 'login' : 'register');
    setError('');
  }

  return (
    <form className="login" onSubmit={submit}>
      <div className="login-head">
        <h1>{registering ? 'Crea tu cuenta' : 'Inicia sesión'}</h1>
        <p className="muted">
          {registering
            ? 'Solo necesitas un email y una contraseña para empezar a pujar o publicar.'
            : 'Entra para pujar en las salas en vivo y publicar tus piezas.'}
        </p>
      </div>

      {registering && (
        <label className="field">
          <span>Nombre completo</span>
          <input name="full_name" type="text" placeholder="John Smith" required disabled={busy} />
        </label>
      )}

      <label className="field">
        <span>Email</span>
        <input name="email" type="email" placeholder="tu@email.com" required disabled={busy} />
      </label>

      <label className="field">
        <span>Contraseña</span>
        <input
          name="password"
          type="password"
          placeholder="mínimo 8 caracteres"
          minLength={8}
          required
          disabled={busy}
        />
      </label>

      {error && <p className="form-error">{error}</p>}

      <button className="btn btn-accent btn-lg" type="submit" disabled={busy}>
        {busy ? 'Un momento…' : registering ? 'Crear cuenta' : 'Entrar'}
      </button>

      <p className="switch muted">
        {registering ? '¿Ya tienes cuenta?' : '¿Todavía no tienes cuenta?'}{' '}
        <button type="button" className="link" onClick={switchMode} disabled={busy}>
          {registering ? 'Inicia sesión' : 'Créala aquí'}
        </button>
      </p>

      <style>{`
        .login { display: flex; flex-direction: column; gap: 1rem; text-align: center; }
        .login-head { display: flex; flex-direction: column; gap: 0.4rem; margin-bottom: 0.5rem; }
        .login-head h1 { font-size: 1.6rem; }
        .login-head p { font-size: 0.93rem; }
        .login .field { text-align: left; }
        .login .btn { width: 100%; margin-top: 0.35rem; }
        .switch { font-size: 0.9rem; margin-top: 0.25rem; }
        .link {
          font: inherit; font-weight: 600; color: var(--accent-ink);
          background: none; border: 0; padding: 0; cursor: pointer;
        }
        .link:hover { text-decoration: underline; }
      `}</style>
    </form>
  );
}
