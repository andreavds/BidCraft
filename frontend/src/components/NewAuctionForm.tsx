import { useEffect, useState, type ChangeEvent, type FormEvent } from 'react';
import { apiErrorMessage } from '../lib/api';
import { readSession, type Session } from '../lib/session';

interface Props {
  apiUrl: string;
}

export default function NewAuctionForm({ apiUrl }: Props) {
  const [session, setSession] = useState<Session | null>(null);
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);
  const [preview, setPreview] = useState('');
  const [fileName, setFileName] = useState('');

  useEffect(() => {
    const stored = readSession();
    if (!stored) {
      window.location.replace('/login?redirect=/auctions/new');
      return;
    }
    setSession(stored);
  }, []);

  function pickFile(changeEvent: ChangeEvent<HTMLInputElement>) {
    const file = changeEvent.target.files?.[0];
    setFileName(file ? file.name : '');
    setPreview(file ? URL.createObjectURL(file) : '');
  }

  async function uploadImage(file: File): Promise<string> {
    const body = new FormData();
    body.append('file', file);

    const response = await fetch(`${apiUrl}/api/v1/uploads`, {
      method: 'POST',
      headers: { Authorization: `Bearer ${session?.token}` },
      body,
    });

    const payload = await response.json();

    if (!response.ok) {
      throw new Error(apiErrorMessage(payload, 'No se pudo subir la imagen.'));
    }

    return `${apiUrl}${payload.path}`;
  }

  async function submit(formEvent: FormEvent<HTMLFormElement>) {
    formEvent.preventDefault();
    setError('');
    setBusy(true);

    const form = new FormData(formEvent.currentTarget);
    const file = form.get('image_file');
    const typedURL = String(form.get('image_url') ?? '').trim();

    try {
      let imageURL = typedURL;
      if (file instanceof File && file.size > 0) {
        imageURL = await uploadImage(file);
      }

      const response = await fetch(`${apiUrl}/api/v1/auctions`, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          Authorization: `Bearer ${session?.token}`,
        },
        body: JSON.stringify({
          title: form.get('title'),
          base_price: Math.round(Number(form.get('base_price')) * 100),
          minimum_increment: Math.round(Number(form.get('minimum_increment')) * 100),
          duration_seconds: Number(form.get('duration_seconds')),
          image_url: imageURL === '' ? null : imageURL,
        }),
      });

      const payload = await response.json();

      if (!response.ok) {
        setError(apiErrorMessage(payload, 'No se pudo crear la subasta.'));
        return;
      }

      window.location.href = `/auctions/${payload.id}`;
    } catch (failure) {
      setError(
        failure instanceof Error ? failure.message : 'No se pudo contactar con el servidor.',
      );
    } finally {
      setBusy(false);
    }
  }

  if (!session) {
    return <p className="muted">Redirigiendo al inicio de sesión…</p>;
  }

  return (
    <form className="publish" onSubmit={submit}>
      <label className="field">
        <span>Título de la pieza</span>
        <input name="title" type="text" placeholder="Neon Dreams" required disabled={busy} />
      </label>

      <div className="field">
        <span>Imagen (opcional)</span>

        <div className="image-picker">
          <label className="dropzone">
            <input name="image_file" type="file" accept="image/*" onChange={pickFile} disabled={busy} />
            {preview ? (
              <img src={preview} alt="Vista previa" />
            ) : (
              <span className="dropzone-text">
                <strong>Sube una imagen</strong>
                JPG, PNG, WEBP o GIF · hasta 5 MB
              </span>
            )}
          </label>

          <div className="or">
            <span>o pega una dirección</span>
            <input
              name="image_url"
              type="url"
              placeholder="https://picsum.photos/seed/neon/800/600"
              disabled={busy || Boolean(fileName)}
            />
            {fileName && <p className="picked muted">Se usará el archivo: {fileName}</p>}
          </div>
        </div>
      </div>

      <div className="row">
        <label className="field">
          <span>Precio de salida</span>
          <input
            name="base_price"
            type="number"
            step="0.01"
            min="0"
            defaultValue="100"
            required
            disabled={busy}
          />
        </label>

        <label className="field">
          <span>Incremento mínimo</span>
          <input
            name="minimum_increment"
            type="number"
            step="0.01"
            min="0.01"
            defaultValue="10"
            required
            disabled={busy}
          />
        </label>

        <label className="field">
          <span>Duración (segundos)</span>
          <input
            name="duration_seconds"
            type="number"
            min="10"
            defaultValue="300"
            required
            disabled={busy}
          />
        </label>
      </div>

      <p className="hint muted">
        Con 60 segundos verás el cierre automático sin tener que esperar.
      </p>

      <button className="btn btn-accent btn-lg" type="submit" disabled={busy}>
        {busy ? 'Publicando…' : 'Publicar subasta →'}
      </button>

      {error && <p className="form-error">{error}</p>}

      <style>{`
        .publish { display: flex; flex-direction: column; gap: 1.1rem; }
        .publish .row { display: grid; grid-template-columns: repeat(auto-fit, minmax(150px, 1fr)); gap: 0.85rem; }
        .publish .btn { align-self: flex-start; }
        .image-picker { display: grid; grid-template-columns: 200px minmax(0, 1fr); gap: 0.85rem; align-items: stretch; }
        .dropzone {
          position: relative; display: grid; place-items: center; overflow: hidden;
          border: 1px dashed var(--line-strong); border-radius: var(--r-sm);
          background: var(--panel); cursor: pointer; min-height: 130px;
          transition: border-color 0.15s ease, background 0.15s ease;
        }
        .dropzone:hover { border-color: var(--accent); background: var(--accent-soft); }
        .dropzone input { position: absolute; inset: 0; opacity: 0; cursor: pointer; }
        .dropzone img { width: 100%; height: 100%; object-fit: cover; }
        .dropzone-text {
          display: flex; flex-direction: column; gap: 0.15rem; text-align: center;
          padding: 0 1rem; font-size: 0.82rem; color: var(--text-2);
        }
        .or { display: flex; flex-direction: column; gap: 0.4rem; justify-content: center; }
        .or > span { font-size: 0.82rem; color: var(--text-3); }
        .picked { font-size: 0.8rem; }
        @media (max-width: 620px) { .image-picker { grid-template-columns: 1fr; } }
        .hint { font-size: 0.88rem; margin-top: -0.35rem; }
      `}</style>
    </form>
  );
}
