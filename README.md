# hankoid

An admin web UI for [Hanko](https://github.com/teamhanko/hanko), generated from Hanko's admin OpenAPI spec. See [NOTES.md](./NOTES.md) for how the spec becomes UI.

## Layout

- `webui/`: the UI pages, `admin.html` (the admin UI) and `signin.html` (Hanko sign-in).
  - Normal builds embed them.
  - `-tags dev` builds read them from disk and add a live-reload endpoint.
- `frontend/`: the frontend server component. Add new frontend endpoints in `frontend.Handler`. Routes:
  - `/signin?next=…`: Hanko sign-in (hanko-elements). Already-signed-in visitors are redirected to `next`, which must be a same-origin path.
  - `/admin/`: the admin UI. Anonymous visitors are redirected to `/signin`; signed-in users not on the allowlist get a 403 page.
  - `/admin/api/*`: proxied to the Hanko admin API with the same protection. Mutations also need the `X-Admin-UI: 1` header.
  - `/admin/session`: the signed-in admin as JSON.
  - Everything else goes to the Hanko public API, so hanko-elements can call it same-origin (login flow, `/logout`, `/sessions/validate`, …).
- `cmd/hankoid`: the Hanko CLI (v3.1.0) rebuilt around Hanko's own command packages, with additions:
  - `serve frontend`: serves the frontend against a running admin/public server. It reads only the config's `server` section, so no database or secrets are needed.
  - `serve all`: starts public, admin, management and frontend. `--no-frontend` skips the UI.
  - `license`: prints the full AGPLv3 license text. The top-level help and `version` also mention the license and source URL.
  - `version`: prints the hankoid and Hanko versions. It replaces upstream's, which depends on a file missing from the published module.
  - `dev`: only in `-tags dev` builds.

## Configuration

The frontend reads its settings from the Hanko config (`--config`, default `./config/config.yaml`). `server.frontend` is our addition; Hanko ignores it.

```yaml
server:
  public:
    address: ":8000"            # Hanko default
  admin:
    address: ":8001"            # Hanko default
  frontend:
    address: "127.0.0.1:8080"   # default when unset
    admins:                     # who may use /admin; empty denies everyone
      - admin@example.com       # a verified email,
      - alice                   # a username,
      - 3c2ad943-586a-4adc-b633-29a00c8bdb56   # or a user ID
cors:
  allow_origins:
    - http://localhost:8080     # the frontend's origin, for hanko-elements
```

## Admin access

- `/admin` is protected by Hanko itself.
  - The frontend forwards the visitor's session cookie to the public API's `/sessions/validate`, which checks signature, expiry, revocation and idle timeout.
  - It then matches the user against `server.frontend.admins` by user ID, username or **verified** email, case-insensitive.
  - `SERVER_FRONTEND_ADMINS` (comma-separated) overrides the list.
- Admins are ordinary Hanko users, so the first admin has to register through `/signin` before being added to the list. Registration must be enabled in Hanko.
- The browser never sees the admin API token. Admin API calls carry `HANKO_ADMIN_TOKEN` from the server, and cookies are stripped before forwarding.

- The UI proxies to the public and admin addresses. A blank or wildcard host (`:8001`, `0.0.0.0:8001`) is dialed as `127.0.0.1`.
- The `SERVER_PUBLIC_ADDRESS`, `SERVER_ADMIN_ADDRESS` and `SERVER_FRONTEND_ADDRESS` environment variables override the file.
- The `--frontend-listen` / `--frontend-admin` / `--frontend-public` flags override both. The admin/public flags take full URLs, which may include a path prefix.

## Usage

```sh
go build -o hankoid ./cmd/hankoid
./hankoid migrate up --config hanko.yaml
./hankoid serve all --config hanko.yaml       # admin UI at http://127.0.0.1:8080/admin/
./hankoid serve frontend --config hanko.yaml  # UI only, against an already-running Hanko

go build -tags dev -o hankoid-dev ./cmd/hankoid
./hankoid-dev dev
```

`.local/dev` (not committed) starts the compose Postgres and Mailpit, and runs migrations. It then runs a `-tags dev` build of `serve all` on the host, with live reload.

## Docker

```sh
ADMINS=you@example.com docker compose up --build
```

- Services:
  - `postgres` (17), published on `127.0.0.1:5432`.
  - `mailpit`, which catches Hanko's emails (registration passcodes and so on). Inbox at http://localhost:8025, SMTP on `127.0.0.1:1025`.
  - `migrate`, which runs `migrate up` once.
  - `hankoid` (`serve all`): public `:8000`, admin `:8001`, frontend `:8080`, all bound to localhost.
- The image uses `deploy/config.yaml`; per-environment values come from env vars set in `compose.yaml`.
- Override with:
  - `ADMINS`: comma-separated admin allowlist.
  - `SECRETS_KEYS`: change it outside local use.
  - `POSTGRES_PASSWORD`
  - `ORIGIN` / `RP_ID`: the frontend's public URL and WebAuthn relying party ID.
  - `COOKIE_SECURE=true`: when served over TLS.
  - `SMTP_HOST` / `SMTP_PORT`: send email through a real SMTP server instead of Mailpit.
  - `PLATFORM`: container platform, default `linux/arm64`. Use `linux/amd64` on x86 hosts. `.local/dev` sets it from `uname -m`.
- Register at http://localhost:8080/signin (the passcode arrives in Mailpit), then open http://localhost:8080/admin/.

Environment:
- `HANKO_ADMIN_TOKEN`: bearer token sent to the admin API.

## Adding commands

- Add `cobra` commands in `cmd/hankoid`.
- Dev-only commands go in a `//go:build dev` file that appends to `registrars` in an `init` function (see `cmd/hankoid/dev.go`).
- To change an upstream command, register your own in place of the upstream `RegisterCommands` call in `newRootCmd`. `registerServe` does this for `serve`.

## License

AGPLv3. See [LICENSE](./LICENSE). This project builds on Hanko, whose backend is AGPLv3.
