# Preparing a domain for Let's Encrypt

The gateway (Caddy) is an **optional** container: the stack runs HTTP-only
without it (the web service publishes `WEB_PORT` directly). Caddy only runs when
the compose profile `https` is enabled — see "Enable the gateway" below. When
it runs, it obtains the HTTPS certificate directly from Let's Encrypt for the
**public DNS name** you enter in **Admin panel → System → Network**.

## Enable the gateway (only needed for HTTPS served by the app)

To use the app-managed HTTPS (Options 1 below), enable the gateway once:

1. Edit `deploy/.env` and set `COMPOSE_PROFILES=https`.
2. Restart the stack: `sh deploy/start.sh local-build` (or `deploy/start.sh prod`).
3. The gateway publishes `GATEWAY_HTTP_PORT` (HTTP) and `GATEWAY_HTTPS_PORT`
   (HTTPS) on the host; the web service keeps publishing `WEB_PORT`.

If you do not need app-managed HTTPS (Tailscale, tunnel, own proxy), leave
`COMPOSE_PROFILES=` empty and skip the gateway entirely.

## How a certificate is actually issued

Let's Encrypt proves you control a domain by connecting to it from the public
internet:

- **HTTP-01** — it fetches `http://<your-domain>/.well-known/acme-challenge/...`
  on port 80.
- **TLS-ALPN-01** — it connects on port 443 and checks a special TLS extension.

Both require the host to be reachable from the public internet at that name. If
nothing answers, Let's Encrypt issues nothing and the screen keeps showing
**Certificate expires: Not installed yet**.

Names Let's Encrypt will never issue a normal public certificate for:

- `localhost` and private-only names.
- Tailscale `*.ts.net` names. Those are owned by Tailscale's DNS, not you, and
  are not reachable from the public internet, so Let's Encrypt cannot validate
  them. Tailscale provides its own certificates for `*.ts.net` instead.

## Pick the way that fits you

There is no single right answer — choose how *your* domain becomes reachable.
The next four sections are ordered roughly by how much control they need.

### Option 1 — Your own public domain (best for public access)

1. Register a domain (or a subdomain of one you own) with any registrar, e.g.
   `media.example.com`.
2. In the DNS zone create an `A` record `media.example.com → your public IP`
   (and an `AAAA` record if you have IPv6).
3. Forward **TCP 80 and 443** from your router/gateway to this host. UDP 443 is
   optional and enables HTTP/3.
4. Enable the gateway: set `COMPOSE_PROFILES=https` in `deploy/.env` and restart
   the stack (see "Enable the gateway" above).
5. In **Admin panel → System → Network** enable HTTPS, enter
   `media.example.com` and a real contact email, then save.
6. Confirm **Certificate expires** shows a date.

Notes:

- Keep port 80 forwarded even in HTTPS-only mode — Caddy still uses it for the
  HTTP-01 challenge.
- For devices on your LAN, have local DNS resolve the name to your server's LAN
  IP (or enable NAT hairpin), otherwise internal clients may not reach the host
  when the name points at the public IP.
- If your router is behind carrier-grade NAT and has no public IP, port
  forwarding will not work — use Option 3 (tunnel) or run the proxy on a public
  VPS (Option 4).

### Option 2 — Tailscale (private tailnet)

This is how this project is deployed by its developer. Tailscale serves HTTPS
for `*.ts.net` names using **its own certificates**, not Let's Encrypt. Use it
when only you and your devices need access.

1. Install Tailscale on the host and run `tailscale up`.
2. Keep `COMPOSE_PROFILES=` empty (no gateway) — the stack is HTTP-only and the
   web service publishes `WEB_PORT`.
3. In **Admin panel → System → Network** keep HTTP enabled and **leave HTTPS
   disabled**.
4. Expose the app to the tailnet. With the default `WEB_PORT=8080`:

   ```sh
   tailscale serve --https=8443 http://127.0.0.1:8080
   ```

5. Open `https://<machine-name>.ts.net:8443/` from any device in your tailnet.
6. To reach the service from the public internet, use `tailscale funnel`
   instead of `serve` (Tailscale relays it; still Tailscale's certificate).

Limitations to know:

- A `*.ts.net` name can **never** get a Let's Encrypt certificate. If you
  specifically need a Let's Encrypt cert, use Option 1 or Option 3.
- The app's HTTPS mode must stay off; Tailscale handles TLS in front of it.

### Option 3 — HTTP tunnel (Cloudflare Tunnel and similar)

If you want public HTTPS but have no public IP or do not want to open ports, put
a tunnel in front. The tunnel terminates TLS with the provider's edge
certificate, so no Let's Encrypt certificate is obtained on your host.

1. Keep the app in HTTP mode (**HTTP on, HTTPS off**) with no gateway
   (`COMPOSE_PROFILES=` empty).
2. Install `cloudflared`, run `cloudflared tunnel login`, and create a tunnel.
3. Add a public hostname mapping, e.g.:

   ```sh
   cloudflared tunnel route dns media.example.com <tunnel-id>
   cloudflared tunnel run <tunnel-id>
   ```

   with config `service = http://127.0.0.1:8080` (`WEB_PORT`).
4. Cloudflare issues the edge certificate for `media.example.com` itself.

### Option 4 — Your own reverse proxy on the public internet

If you already run nginx, Traefik, Caddy, or similar on a public VPS:

1. Run the app in HTTP mode with no gateway (`WEB_PORT=8080`).
2. Configure your existing proxy to serve `https://media.example.com` with its
   own Let's Encrypt certificate (certbot, `acme.sh`, Traefik's ACME, …) and
   reverse-proxy to `http://127.0.0.1:8080`.
3. Your proxy does the ACME validation; the app stays HTTP-only and no gateway
   container is used.

## Which option should I choose?

| Your situation | Option |
| --- | --- |
| Only you / your devices; you already use Tailscale | 2 |
| Public access; you control DNS and can forward ports | 1 |
| Public access; no public IP / no port forwarding | 3 |
| You already run a public reverse proxy or VPS | 4 |

## Troubleshooting

- **HTTPS is on in Settings but nothing listens on 443** — the gateway
  container is not running. Set `COMPOSE_PROFILES=https` in `deploy/.env`,
  restart the stack, then re-save the network settings.
- **Certificate expires: Not installed yet** — ACME validation has not
  completed. Check the DNS record, that TCP 80 and 443 reach this host from
  outside (`curl -vk https://media.example.com`), and that the DNS name and
  email are spelled correctly.
- **I entered a `*.ts.net` name** — it cannot be validated by Let's Encrypt.
  Switch the app back to HTTP mode and use `tailscale serve`/`funnel`.
- **It worked later, after some minutes** — normal. Caddy retries issuance
  automatically; after a DNS change allow a minute or two.
- **Rate limits** — Let's Encrypt limits how often a certificate can be issued
  for the same name. Do not toggle HTTPS on/off repeatedly while debugging.
