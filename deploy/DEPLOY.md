# SV Gateway — Production Deployment Runbook (EC2 + Supabase)

This document covers standing up the gateway on an AWS EC2 instance with Supabase as the database and Caddy for automatic HTTPS.

---

## 1. Supabase Setup

### Main database

1. Create a new Supabase project at [supabase.com](https://supabase.com).
2. In **Project Settings → Database → Connection String**, select the **Session pooler** tab (port **5432**, IPv4-compatible).
3. Copy the connection string and paste it into `SQL_DSN` in `.env.prod`. Always append `?sslmode=require`:
   ```
   SQL_DSN=postgresql://postgres.<ref>:<password>@<region>.pooler.supabase.com:5432/postgres?sslmode=require
   ```
   - Use the **Session pooler** (port 5432), NOT the transaction pooler (port 6543). GORM uses prepared statements which are incompatible with the transaction pooler.
4. new-api **auto-migrates** all tables on first boot — no manual SQL required.

> **Free tier note:** Supabase free tier provides 500 MB. The `request_logs` table (Plan H) stores full prompt/response bodies and can grow quickly under production load.

### Log database (recommended)

Create a second Supabase project (or any separate Postgres instance) dedicated to heavy request/usage logs. Copy its Session-pooler connection string into `LOG_SQL_DSN`. This keeps analytics writes off the main OLTP database.

If `LOG_SQL_DSN` is left empty, logs fall back to the main database.

---

## 2. EC2 Setup

1. **Launch an EC2 instance** (Amazon Linux 2023 or Ubuntu 22.04 recommended, t3.small or larger).

2. **Install Docker + Compose plugin:**
   ```bash
   # Amazon Linux 2023
   sudo dnf install -y docker
   sudo systemctl enable --now docker
   sudo usermod -aG docker ec2-user
   # Log out and back in, then:
   sudo mkdir -p /usr/local/lib/docker/cli-plugins
   sudo curl -SL https://github.com/docker/compose/releases/latest/download/docker-compose-linux-x86_64 \
     -o /usr/local/lib/docker/cli-plugins/docker-compose
   sudo chmod +x /usr/local/lib/docker/cli-plugins/docker-compose

   # Ubuntu
   # sudo apt-get update && sudo apt-get install -y docker.io docker-compose-plugin
   ```

3. **Security group rules:** Open inbound ports **80** (HTTP) and **443** (HTTPS) only.
   - Do NOT expose port 3000 (gateway), 5432 (DB), or 6379 (Redis) to the internet.

4. **DNS:** Point an A record for `GATEWAY_DOMAIN` (e.g. `gateway.storyverseai.art`) to the EC2 instance's public IP. Caddy will use this for automatic TLS certificate provisioning via ACME (Let's Encrypt).

---

## 3. Configure

Clone the repository and check out the `sv/deploy` branch:
```bash
git clone https://github.com/your-org/new-api.git && cd new-api
git checkout sv/deploy
```

Copy the example env file and fill in all values:
```bash
cp deploy/.env.prod.example deploy/.env.prod
```

Generate secure random secrets:
```bash
openssl rand -hex 32   # paste into SESSION_SECRET
openssl rand -hex 32   # paste into CRYPTO_SECRET
openssl rand -hex 32   # paste into REDIS_PASSWORD (also update REDIS_CONN_STRING)
```

Paste the Supabase connection strings into `SQL_DSN` and (recommended) `LOG_SQL_DSN`.

Fill in all upstream API keys and endpoint IDs in the `UPSTREAM KEYS` section. These are consumed by `seed_channels.sh` and are not needed by the running container.

For fal support, set `FAL_KEY` (or export `FAL_API_KEY` before running the seed script). The seed script uses this key to create the `fal-media` channel after the updated gateway image is deployed.

---

## 4. Boot

This deployment supports two TLS/proxy modes. **Production runs in host-caddy mode.** Choose the matching command below and use the *same* invocation for every subsequent `up`, rebuild, or single-service recreate.

### Host-caddy mode (production)

Caddy runs as a **host process** (systemd, config at `/etc/caddy/Caddyfile`) that reverse-proxies `gateway.<domain>` → `127.0.0.1:3000`. The `docker-compose.host-caddy.yml` override publishes the gateway port to `127.0.0.1:3000` so the host caddy can reach it, and the in-container caddy service is left unstarted (avoiding a clash over 80/443).

```bash
docker compose -f deploy/docker-compose.prod.yml -f deploy/docker-compose.host-caddy.yml \
  --env-file deploy/.env.prod up -d --build
```

> ⚠️ **Always pass BOTH `-f` files in host-caddy mode** — including for a single-service recreate such as `... up -d --build sv-gateway`. If you omit `docker-compose.host-caddy.yml`, the container is recreated **without** the `127.0.0.1:3000:3000` publish, the host caddy can no longer reach it, and every public request returns **502** (even though the gateway itself is healthy). Verify after any recreate:
> ```bash
> docker port sv-gateway   # must print: 3000/tcp -> 127.0.0.1:3000
> ```

### All-in-docker mode (self-contained alternative)

An in-container caddy terminates TLS and proxies to sv-gateway over the docker network (port 3000 is only `expose`d). Use this **only if you are not running a host caddy**:

```bash
docker compose -f deploy/docker-compose.prod.yml --env-file deploy/.env.prod up -d --build
```

### Check health

```bash
docker compose -f deploy/docker-compose.prod.yml -f deploy/docker-compose.host-caddy.yml ps
docker compose -f deploy/docker-compose.prod.yml logs sv-gateway --tail=50
```

> **Startup window:** on every boot the gateway runs GORM AutoMigrate against the (remote) database *before* it binds `:3000`. Against Supabase this can take several minutes, during which the host caddy returns **502**. This is expected — confirm readiness from inside the container before testing the public URL:
> ```bash
> docker exec sv-gateway wget -qO- http://localhost:3000/api/status
> ```

Caddy provisions/serves a TLS certificate for `GATEWAY_DOMAIN`. This requires that the DNS A record is already pointing at the EC2 IP and that ports 80 and 443 are open.

---

## 5. Bootstrap Admin + Seed Channels

### First boot: create the root admin

On first startup, navigate to `https://GATEWAY_DOMAIN` in a browser and complete the initial setup (create root user). Alternatively, call the setup API:
```bash
curl -X POST https://GATEWAY_DOMAIN/api/setup \
  -H "Content-Type: application/json" \
  -d '{"username":"root","password":"<your-root-password>","display_name":"Root"}'
```

Add the root credentials to `deploy/.env.prod`:
```
GATEWAY_ROOT_PASSWORD=<your-root-password>
# OR generate/retrieve an access token from the admin panel and use:
GATEWAY_ROOT_ACCESS_TOKEN=<token>
```

### Run the seed script

```bash
bash deploy/seed_channels.sh
```

This script is idempotent — it is safe to run multiple times. It will:
- Set `SelfUseModeEnabled=true` (required for model routing to work without per-model pricing)
- Register groups `sv-monorepo` and `bragi-canvas` with ratio 1
- Create access tokens `sv-monorepo-token` and `bragi-canvas-token` (unlimited, no expiry)
- Create all 5 upstream channels (tokenrouter, byteplus seedream/seedance, apimart, fal)
- Skip any items that already exist

After deploying a gateway image that adds a new provider, run `bash deploy/seed_channels.sh` again so the new provider's channel is created in the production database. The script is creation-idempotent: existing tokens and channels are skipped by name, and system options are re-applied with the same values. It does **not** overwrite an existing channel's key, base URL, model list, or model mapping.

To **add models to an existing channel** (e.g. new fal video models on the already-seeded `fal-media` channel), seed will skip it — instead patch it in place:
```bash
set -a; source deploy/.env.prod; set +a   # provides GATEWAY_URL + root creds
bash deploy/update_fal_channel.sh           # idempotently merges new models + mappings
```
Or edit the channel's **Models** and **Model Redirect (mapping)** fields manually in the admin console. Either way, wait ~30s for the channel cache to sync (see below).

> **Note:** After channel creation, the in-memory channel cache syncs every 30 seconds (`CHANNEL_UPDATE_FREQUENCY`). Wait ~30s before testing.

---

## 6. Verify

Retrieve a token key from the admin panel (`https://GATEWAY_DOMAIN` → Tokens → copy key), then:
```bash
# Text generation
curl -s https://GATEWAY_DOMAIN/v1/chat/completions \
  -H "Authorization: Bearer sk-..." \
  -H "Content-Type: application/json" \
  -d '{"model":"sv-text-pro","messages":[{"role":"user","content":"ping"}]}'

# Image generation
curl -s https://GATEWAY_DOMAIN/v1/images/generations \
  -H "Authorization: Bearer sk-..." \
  -H "Content-Type: application/json" \
  -d '{"model":"sv-image-gpt","prompt":"a red apple","n":1,"size":"1024x1024"}'
```

---

## 7. Backups

- **Main DB (Supabase):** Supabase manages automated daily backups. Point-in-time recovery is available on paid plans.
- **Log DB (self-hosted Postgres):** Add a `pg_dump` cron job on the log DB host, e.g.:
  ```bash
  # /etc/cron.d/sv-log-backup
  0 3 * * * postgres pg_dump -Fc sv_logs > /backups/sv-logs-$(date +%F).dump
  ```
- **Redis:** Redis is used for caching only. Data loss on restart is acceptable; the gateway will resync from the DB.

---

## 8. Gotchas

| Issue | Detail |
|-------|--------|
| **Session pooler vs transaction pooler** | Always use port 5432 (Session pooler). GORM's prepared-statement cache is incompatible with the transaction pooler (port 6543). |
| **`sslmode=require` in DSN** | Supabase requires TLS. Omitting this will result in connection refused. |
| **Never change `CRYPTO_SECRET`** | This key encrypts channel API keys stored in the DB. Changing it after channels exist will make all stored keys unreadable, breaking all upstream calls. |
| **`SelfUseModeEnabled`** | Currently enabled (`true`) which bypasses per-model billing checks. This is intentional for internal use. If you later enable multi-product billing and user quotas, disable this and configure model prices. |
| **Channel cache lag** | After `seed_channels.sh` creates channels, the in-memory cache takes up to 30s to sync. Test after waiting. |
| **502 after rebuild (host-caddy mode)** | Production proxies via a host caddy → `127.0.0.1:3000`. Recreating sv-gateway **without** `-f deploy/docker-compose.host-caddy.yml` drops the `127.0.0.1:3000:3000` publish, so caddy can't reach it → 502. Always pass both `-f` files; verify with `docker port sv-gateway`. See §4. |
| **502 during startup** | On every boot the gateway runs AutoMigrate before binding `:3000`; against Supabase this takes several minutes and caddy returns 502 until it finishes. Check `docker exec sv-gateway wget -qO- http://localhost:3000/api/status`. |
| **BytePlus base URL** | Use `https://ark.ap-southeast.bytepluses.com` (overseas). The Beijing URL `https://ark.cn-beijing.volces.com` returns 401 for overseas accounts. |
| **Caddy TLS provisioning** | DNS A record must be live before first boot. Caddy will fail to get a cert if the domain doesn't resolve to the server's public IP. |
| **Free-tier Supabase pause** | Supabase free-tier projects pause after 7 days of inactivity. Upgrade to a paid plan or use the Pro tier for production. |
