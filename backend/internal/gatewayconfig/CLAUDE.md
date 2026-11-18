# Gateway config (`internal/gatewayconfig`)

- Generates the Caddyfile into `/runtime/app-config` from Settings → Network
  access (HTTP/HTTPS, domain). TLS terminates at Caddy 2.10 via Let's Encrypt.
- The deploy compose runs a small watcher that reloads Caddy when the generated
  file's checksum changes — never require a container restart.
- The `gateway` service only runs under the `https` compose profile; api/web stay
  private on the compose network.
- See `docs/https-domain-setup.md` for operator steps.
