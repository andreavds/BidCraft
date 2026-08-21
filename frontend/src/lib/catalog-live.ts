/**
 * Mantiene al día una lista de subastas renderizada en el servidor.
 *
 * Las listas (el catálogo y las subastas abiertas de la landing) las pinta
 * Astro en el servidor. Este módulo escucha la sala del catálogo y, cuando
 * llega auction_created o auction_finished, vuelve a pedir la misma página y
 * sustituye solo el contenedor marcado con data-catalog. Así el filtro, el
 * orden y el límite los sigue decidiendo el backend, y no hace falta
 * reconstruir las tarjetas en el navegador.
 *
 * La página solo tiene que marcar su contenedor:
 *
 *   <section data-catalog data-api={browserApiUrl()}> … </section>
 *
 * y opcionalmente un [data-count] con el recuento a refrescar.
 */
export function connectCatalog(): void {
  const catalog = document.querySelector<HTMLElement>('[data-catalog]');
  if (!catalog) return;

  const base = (catalog.dataset.api ?? '').replace(/^http/, 'ws');
  const socket = new WebSocket(`${base}/api/v1/auctions/ws`);

  let refreshing = false;
  let queued = false;

  async function refresh(container: HTMLElement): Promise<void> {
    // Si llega otro evento mientras se está refrescando, se repite una sola vez
    // al terminar: así ninguna subasta se queda fuera de la lista.
    if (refreshing) {
      queued = true;
      return;
    }
    refreshing = true;

    try {
      const response = await fetch(location.href, {
        headers: { accept: 'text/html' },
      });
      if (!response.ok) return;

      const fresh = new DOMParser().parseFromString(await response.text(), 'text/html');

      const list = fresh.querySelector('[data-catalog]');
      if (list) container.innerHTML = list.innerHTML;

      const count = fresh.querySelector('[data-count]');
      const current = document.querySelector('[data-count]');
      if (count && current) current.textContent = count.textContent;
    } finally {
      refreshing = false;

      if (queued) {
        queued = false;
        refresh(container);
      }
    }
  }

  socket.onmessage = (message) => {
    const { type } = JSON.parse(message.data);
    if (type === 'auction_created' || type === 'auction_finished') refresh(catalog);
  };

  // Red de seguridad: una tarjeta cuya cuenta atrás llegó a cero avisa por si el
  // evento de cierre no llegó.
  document.addEventListener('auction:expired', () => refresh(catalog));

  window.addEventListener('beforeunload', () => socket.close());
}
