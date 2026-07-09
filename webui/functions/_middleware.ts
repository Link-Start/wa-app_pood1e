/**
 * Pages Function middleware: same-origin API proxy.
 *
 * The dashboard SPA calls RELATIVE paths (`/api/wa/*`, `/login`, `/logout`,
 * `/healthz`) with `credentials: 'same-origin'`, so the backend must appear on
 * the SAME origin as the site. This middleware runs on the Cloudflare edge and
 * transparently reverse-proxies those paths to the VPS backend, forwarding the
 * method, headers (incl. `Cookie`) and body upstream, and relaying the upstream
 * response verbatim — including `Set-Cookie`, so the auth cookie is issued for
 * this origin. Plain HTTP to the VPS is fine because the request runs
 * server-side (no browser mixed-content). All other paths fall through to the
 * static asset server via `next()`.
 */

// Not a secret: the public VPS origin serving the dashboard BFF.
//
// The Workers/Pages runtime rejects `fetch()` to a raw IP literal (Cloudflare
// error 1003, "Direct IP Access Not Allowed"), so the origin is reached via a
// hostname. `api.pood1e.space` is a self-owned DNS-only A record pointing at the
// VPS IP, letting the edge connect directly to the VPS on its HTTP port.
const UPSTREAM_ORIGIN = 'http://api.pood1e.space:8091';

function shouldProxy(pathname: string): boolean {
  return (
    pathname.startsWith('/api/') ||
    pathname === '/login' ||
    pathname === '/logout' ||
    pathname === '/healthz'
  );
}

export const onRequest: PagesFunction = async (context) => {
  const { request, next } = context;
  const url = new URL(request.url);

  if (!shouldProxy(url.pathname)) {
    return next();
  }

  const upstreamUrl = `${UPSTREAM_ORIGIN}${url.pathname}${url.search}`;

  // Cloning the incoming request carries method, headers (Cookie included) and
  // the streaming body faithfully; `redirect: manual` relays upstream 3xx
  // (e.g. the /login redirect) straight to the browser instead of following it.
  const upstreamRequest = new Request(upstreamUrl, request);
  const upstreamResponse = await fetch(upstreamRequest, { redirect: 'manual' });

  // Reconstruct so every upstream header — notably Set-Cookie — passes through
  // unchanged while keeping the body streamable.
  return new Response(upstreamResponse.body, upstreamResponse);
};
