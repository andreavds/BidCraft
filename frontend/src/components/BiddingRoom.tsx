import { useEffect, useState, type FormEvent } from 'react';
import { apiErrorMessage, formatMoney, type Auction, type Bid } from '../lib/api';
import { readSession, type Session } from '../lib/session';

interface Props {
  apiUrl: string;
  auction: Auction;
  initialBids: Bid[];
}

function secondsLeft(endAt: string): number {
  return Math.max(0, Math.floor((new Date(endAt).getTime() - Date.now()) / 1000));
}

function formatCountdown(seconds: number): string {
  const hours = Math.floor(seconds / 3600);
  const minutes = Math.floor((seconds % 3600) / 60);
  const rest = seconds % 60;

  const pad = (value: number) => String(value).padStart(2, '0');

  return hours > 0
    ? `${hours}:${pad(minutes)}:${pad(rest)}`
    : `${pad(minutes)}:${pad(rest)}`;
}

export default function BiddingRoom({ apiUrl, auction, initialBids }: Props) {
  const [status, setStatus] = useState(auction.status);
  const [currentPrice, setCurrentPrice] = useState(auction.current_price);
  const [minimumBid, setMinimumBid] = useState(auction.minimum_bid);
  const [winnerId, setWinnerId] = useState<number | null>(auction.winner_id);
  const [winnerName, setWinnerName] = useState<string | null>(auction.winner_name);
  const [bids, setBids] = useState<Bid[]>(initialBids);

  const [remaining, setRemaining] = useState(() => secondsLeft(auction.end_at));
  const [amount, setAmount] = useState('');
  const [error, setError] = useState('');
  const [notice, setNotice] = useState('');
  const [sending, setSending] = useState(false);
  const [session, setSession] = useState<Session | null>(null);

  useEffect(() => setSession(readSession()), []);

  useEffect(() => {
    if (status === 'FINISHED') {
      setRemaining(0);
      return;
    }

    const timer = setInterval(() => setRemaining(secondsLeft(auction.end_at)), 1000);
    return () => clearInterval(timer);
  }, [auction.end_at, status]);

  useEffect(() => {
    if (status === 'FINISHED') {
      return;
    }

    const base = apiUrl.replace(/^http/, 'ws');
    const query = session ? `?token=${encodeURIComponent(session.token)}` : '';
    const socket = new WebSocket(`${base}/api/v1/auctions/${auction.id}/ws${query}`);

    socket.onmessage = (message) => {
      const event = JSON.parse(message.data);

      if (event.type === 'bid_placed') {
        setCurrentPrice(event.data.current_price);
        setMinimumBid(event.data.minimum_bid);
        addBid({
          id: event.data.bid_id,
          auction_id: event.data.auction_id,
          user_id: event.data.user_id,
          user_name: event.data.user_name,
          amount: event.data.amount,
          created_at: event.data.created_at,
        });
        return;
      }

      if (event.type === 'outbid') {
        setNotice(`Tu puja fue superada por ${formatMoney(event.data.new_amount)}.`);
        return;
      }

      if (event.type === 'auction_finished') {
        setStatus('FINISHED');
        setWinnerId(event.data.winner_id);
        setWinnerName(event.data.winner_name ?? null);
        setCurrentPrice(event.data.final_price);
        setNotice('');
        socket.close();
      }
    };

    return () => socket.close();
  }, [apiUrl, auction.id, session, status]);

  function addBid(bid: Bid) {
    setBids((previous) =>
      previous.some((existing) => existing.id === bid.id) ? previous : [bid, ...previous],
    );
  }

  async function submitBid(formEvent: FormEvent<HTMLFormElement>) {
    formEvent.preventDefault();
    setError('');
    setNotice('');
    setSending(true);

    try {
      const response = await fetch(`${apiUrl}/api/v1/auctions/${auction.id}/bids`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${session?.token}` },
        body: JSON.stringify({ amount: Math.round(Number(amount) * 100) }),
      });

      const payload = await response.json();

      if (!response.ok) {
        setError(apiErrorMessage(payload, 'No se pudo registrar la puja.'));
        return;
      }

      setCurrentPrice(payload.auction.current_price);
      setMinimumBid(payload.auction.minimum_bid);
      addBid(payload);
      setAmount('');
    } catch {
      setError('No se pudo contactar con el servidor.');
    } finally {
      setSending(false);
    }
  }

  const finished = status === 'FINISHED';
  const expired = remaining === 0;
  const isOwner = session?.userId === auction.created_by;
  const canBid = Boolean(session) && !isOwner && !finished && !expired;
  const currentBidder = bids.length > 0 ? bids[0] : null;
  const urgent = !finished && remaining > 0 && remaining <= 30;

  return (
    <section className="room">
      <div className={finished ? 'board board-finished' : 'board'}>
        <div className="board-main">
          <span className="board-k">{finished ? 'Precio final' : 'Precio actual'}</span>
          <strong className="board-price">{formatMoney(currentPrice)}</strong>
          <span className="board-sub">
            {currentBidder ? `Va ganando ${currentBidder.user_name}` : 'Todavía sin pujas'}
            {currentBidder?.user_id === session?.userId && ' · eres tú'}
          </span>
        </div>

        <div className={urgent ? 'board-clock urgent' : 'board-clock'}>
          <span className="board-k">{finished ? 'Estado' : 'Tiempo restante'}</span>
          <strong className="board-time">
            {finished ? 'Cerrada' : formatCountdown(remaining)}
          </strong>
          {!finished && <span className="board-sub">Puja mínima {formatMoney(minimumBid)}</span>}
        </div>
      </div>

      {finished && (
        <div className="result">
          <h2>Subasta finalizada</h2>
          {winnerId ? (
            <p>
              <strong>{winnerName ?? 'El ganador'}</strong> ganó la subasta
              {session?.userId === winnerId && ' — ¡eres tú!'} por{' '}
              <strong>{formatMoney(currentPrice)}</strong>.
            </p>
          ) : (
            <p>Nadie pujó, así que la subasta se cerró sin ganador.</p>
          )}
        </div>
      )}

      {!finished && expired && (
        <p className="form-note">El tiempo expiró. Cerrando la subasta…</p>
      )}

      {notice && <p className="form-note">{notice}</p>}

      {!finished && session && isOwner && (
        <p className="form-note">Publicaste esta pieza, así que no puedes pujar por ella.</p>
      )}

      {!finished &&
        !isOwner &&
        (session ? (
          <form className="bid-form card card-pad" onSubmit={submitBid}>
            <label className="field">
              <span>Tu puja</span>
              <div className="bid-input">
                <span className="currency">$</span>
                <input
                  type="number"
                  step="0.01"
                  min="0"
                  placeholder={(minimumBid / 100).toFixed(2)}
                  value={amount}
                  onChange={(changeEvent) => setAmount(changeEvent.target.value)}
                  disabled={!canBid || sending}
                  required
                />
              </div>
            </label>

            <button className="btn btn-accent btn-lg" type="submit" disabled={!canBid || sending}>
              {sending ? 'Enviando…' : 'Pujar'}
            </button>
          </form>
        ) : (
          <div className="card card-pad signin-card">
            <div>
              <strong>Inicia sesión para pujar</strong>
              <p className="muted">Necesitas una cuenta para participar en esta sala.</p>
            </div>
            <a className="btn btn-accent" href={`/login?redirect=/auctions/${auction.id}`}>
              Iniciar sesión
            </a>
          </div>
        ))}

      {error && <p className="form-error">{error}</p>}

      <div className="feed card">
        <div className="feed-head">
          <h2>Historial de pujas</h2>
          <span className="muted">{bids.length}</span>
        </div>

        {bids.length === 0 ? (
          <p className="feed-empty muted">Todavía no hay pujas. Puedes ser el primero.</p>
        ) : (
          <ul>
            {bids.map((bid, index) => (
              <li key={bid.id} className={index === 0 ? 'top' : ''}>
                <span className="who">
                  {bid.user_name}
                  {bid.user_id === session?.userId && <span className="you">tú</span>}
                </span>
                <strong>{formatMoney(bid.amount)}</strong>
              </li>
            ))}
          </ul>
        )}
      </div>

      <style>{`
        .room { display: flex; flex-direction: column; gap: 1rem; }

        .board {
          display: flex; flex-wrap: wrap; gap: 1.5rem; justify-content: space-between;
          padding: 1.75rem; border-radius: var(--r-lg); color: #fff;
          background: linear-gradient(100deg, #35103f 0%, #1b0f24 45%, var(--ink) 100%);
        }
        .board-finished { background: var(--ink); }
        .board-main, .board-clock { display: flex; flex-direction: column; gap: 0.15rem; }
        .board-clock { align-items: flex-end; text-align: right; }
        .board-k {
          font-size: 0.72rem; text-transform: uppercase; letter-spacing: 0.12em;
          color: rgba(255,255,255,0.5);
        }
        .board-price {
          font-size: clamp(2.2rem, 6vw, 3rem); line-height: 1.05; letter-spacing: -0.03em;
          font-variant-numeric: tabular-nums;
        }
        .board-time {
          font-size: clamp(1.6rem, 4.5vw, 2.1rem); letter-spacing: -0.02em;
          font-variant-numeric: tabular-nums;
        }
        .board-sub { font-size: 0.88rem; color: rgba(255,255,255,0.62); }
        .board-clock.urgent .board-time { color: #ff8ba0; }

        .result {
          padding: 1.25rem 1.5rem; border-radius: var(--r-lg);
          background: var(--accent-soft); border: 1px solid #f7cdea;
        }
        .result h2 { font-size: 1.05rem; margin-bottom: 0.25rem; }
        .result p { font-size: 0.95rem; color: var(--text-2); }
        .result strong { color: var(--text); }

        .signin-card {
          display: flex; align-items: center; justify-content: space-between;
          gap: 1rem; flex-wrap: wrap;
        }
        .signin-card p { font-size: 0.92rem; }
        .bid-form { display: flex; gap: 0.75rem; align-items: flex-end; flex-wrap: wrap; }
        .bid-form .field { flex: 1 1 180px; }
        .bid-input { position: relative; }
        .bid-input .currency {
          position: absolute; left: 0.9rem; top: 50%; transform: translateY(-50%);
          color: var(--text-3); font-size: 0.95rem;
        }
        .bid-input input { padding-left: 1.9rem; font-size: 1.05rem; font-weight: 600; }

        .feed { padding: 1.25rem 1.5rem 0.5rem; }
        .feed-head {
          display: flex; align-items: center; justify-content: space-between;
          padding-bottom: 0.75rem; border-bottom: 1px solid var(--line);
        }
        .feed-head h2 { font-size: 1rem; }
        .feed ul { list-style: none; margin: 0; padding: 0; max-height: 320px; overflow-y: auto; }
        .feed li {
          display: flex; align-items: center; justify-content: space-between; gap: 1rem;
          padding: 0.85rem 0; border-bottom: 1px solid var(--line); font-variant-numeric: tabular-nums;
        }
        .feed li:last-child { border-bottom: 0; }
        .feed li.top strong { color: var(--accent-ink); }
        .who { display: inline-flex; align-items: center; gap: 0.5rem; color: var(--text-2); font-size: 0.92rem; }
        .you {
          font-size: 0.7rem; font-weight: 600; text-transform: uppercase; letter-spacing: 0.06em;
          padding: 0.1rem 0.45rem; border-radius: var(--r-pill);
          background: var(--accent-soft); color: var(--accent-ink);
        }
        .feed-empty { padding: 1.5rem 0; text-align: center; font-size: 0.92rem; }

        @media (max-width: 520px) {
          .board { flex-direction: column; gap: 1.25rem; }
          .board-clock { align-items: flex-start; text-align: left; }
        }
      `}</style>
    </section>
  );
}
