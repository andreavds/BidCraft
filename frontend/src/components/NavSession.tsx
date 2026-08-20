import { useEffect, useState } from 'react';
import { clearSession, fetchCurrentUser, readSession } from '../lib/session';

interface Props {
  apiUrl: string;
}

export default function NavSession({ apiUrl }: Props) {
  const [name, setName] = useState<string | null>(null);
  const [ready, setReady] = useState(false);

  useEffect(() => {
    const session = readSession();

    if (!session) {
      setReady(true);
      return;
    }

    fetchCurrentUser(apiUrl, session.token)
      .then((user) => {
        if (user) {
          setName(user.full_name);
        } else {
          clearSession();
        }
      })
      .catch(() => undefined)
      .finally(() => setReady(true));
  }, [apiUrl]);

  function signOut() {
    clearSession();
    setName(null);
    window.location.href = '/';
  }

  if (!ready) {
    return <span className="nav-slot" />;
  }

  return (
    <div className="nav-session">
      {name ? (
        <>
          <span className="welcome">
            Bienvenido/a, <strong>{name}</strong>
          </span>
          <button type="button" className="signout" onClick={signOut}>
            Cerrar sesión
          </button>
          <a className="btn btn-accent btn-sm" href="/auctions/new">
            Publicar subasta
          </a>
        </>
      ) : (
        <>
          <a className="signin" href="/login">
            Iniciar sesión
          </a>
          <a className="btn btn-accent btn-sm" href="/auctions/new">
            Publicar subasta
          </a>
        </>
      )}

      <style>{`
        .nav-slot { display: inline-block; width: 168px; height: 34px; }
        .nav-session { display: flex; align-items: center; gap: 1rem; }
        .welcome { font-size: 0.9rem; color: rgba(255,255,255,0.62); white-space: nowrap; }
        .welcome strong { color: #fff; font-weight: 600; }
        @media (max-width: 900px) { .welcome { display: none; } }
        .signin, .signout {
          font: inherit; font-size: 0.92rem; color: rgba(255,255,255,0.62);
          background: none; border: 0; padding: 0; cursor: pointer;
          transition: color 0.15s ease; white-space: nowrap;
        }
        .signin:hover, .signout:hover { color: #fff; }
        @media (max-width: 520px) { .signin, .signout { display: none; } }
      `}</style>
    </div>
  );
}
