# Quick Start Guide

Get up and running with Enphase Monitor in 5 minutes!

This guide will help you:
1. Set up your configuration file
2. Complete OAuth authentication
3. Configure your systems
4. Run your first query

## Step 1: Prerequisites

Make sure you have:
- [ ] Go 1.21+ installed (`go version` to check)
- [ ] Enphase Developer Portal account at https://developer-v4.enphase.com/
- [ ] API credentials from the Developer Portal:
  - API Key
  - Client ID
  - Client Secret
- [ ] System IDs for your Enphase Systems (see Step 3)

## Step 2: Initial Setup

```bash
# Navigate to the project directory
cd enphase-monitor

# Create your configuration files
make setup
# OR manually:
# cp config.yaml.example config.yaml
# cp credentials.yaml.example credentials.yaml
```

Configuration is split across two files:
- **`config.yaml`** — non-secret settings (systems, refresh interval, colors). Safe to share/commit.
- **`credentials.yaml`** — your API key and OAuth secrets. Kept local (gitignored); never commit it.

## Step 3: Configure Your Credentials and Systems

Edit `credentials.yaml` with your API credentials from the Developer Portal.
`credentials:` is a list — one entry is enough to start; add more keys later to
spread the rate limit across systems.

```yaml
credentials:
  - name: enphase-monitor-001                       # Unique label for this credential set
    key: "YOUR_API_KEY"              # From Developer Portal
    client_id: "YOUR_CLIENT_ID"       # From Developer Portal
    client_secret: "YOUR_CLIENT_SECRET"  # From Developer Portal
    refresh_token: ""  # Will be filled in Step 4
```

The shared, non-secret `authorization_url` and `redirect_uri` go in `config.yaml`
(not per credential). Then edit `config.yaml` with those and your systems:

```yaml
api:
  authorization_url: "https://api.enphaseenergy.com/oauth/token"
  redirect_uri: "http://localhost:8080/callback"  # Must match your Developer Portal app settings

systems:
  - name: "Left Subpanel"      # Give it a friendly name
    id: "1234567"               # Your first system ID (see below)
    
  - name: "Right Subpanel"     # Name your second system
    id: "7654321"               # Your second system ID

refresh_interval: 3600         # Query every hour (recommended)
```

**Finding Your System IDs:** See [Finding Your System IDs](README.md#finding-your-system-ids) in the README for detailed instructions.

## Step 4: Complete OAuth Setup

**⚠️ This step is required before you can use the application!**

The application uses OAuth 2.0 for authentication. You need to complete a one-time OAuth setup:

```bash
./enphase-monitor --update-refresh-tokens
```

This interactive wizard will:
1. Open your browser to the authorization page
2. Wait while you log in and authorize the application
3. Capture the authorization code automatically (via a local listener on your `redirect_uri`) and exchange it for tokens
4. Write the `refresh_token` straight into the matching credential entry in your `credentials.yaml`

No copy-paste needed — just authorize in the browser and the credential is ready to use. (If your `redirect_uri` isn't a localhost address, the wizard falls back to asking you to paste the redirect URL.)

> With more than one credential set, name the one to set up: `./enphase-monitor --update-refresh-tokens enphase-monitor-002`.

> **📖 Want to understand OAuth better?** See **[OAUTH_SETUP.md](docs/OAUTH_SETUP.md)** for a detailed explanation of how OAuth works, what each component does, and how authentication is performed.

## Step 5: Build and Install

```bash
# Install dependencies
go mod download

# Build the application
go build -o enphase-monitor
```

## Step 6: Initialize (one-time)

Resolve and cache your systems' location for weather reporting. **This is required before any report will run:**

```bash
./enphase-monitor --init
```

Run it once. Re-run it only if you clear the cache (add `--force` to re-resolve from the API).

## Step 7: Test It!

Run a single query to make sure everything works:

```bash
./enphase-monitor
```

You should see output showing your combined system metrics!

## Step 8: Start Monitoring

Start continuous monitoring (refreshes every hour by default):

```bash
make run
```

Press `Ctrl+C` to stop.

## Common First-Time Issues

### "not initialized — run `enphase-monitor --init` first"
→ You skipped Step 6. Run `./enphase-monitor --init` once to cache your systems' location, then retry.

### "no credentials configured"
→ Make sure each entry under `credentials:` has a unique `name` plus `key`, `client_id`, and `client_secret` in `credentials.yaml`

### "API request failed with status 401"
→ Your refresh token might be missing or expired. Complete OAuth setup: `./enphase-monitor --update-refresh-tokens`

### "system must have id"
→ You need to replace the example system IDs with your actual ones from Enlighten (see Step 3)

### "redirect_uri mismatch"
→ The redirect URI in your `credentials.yaml` must match exactly what you registered in the Enphase Developer Portal

## Next Steps

Once it is working:

- Query a Past Period: `./enphase-monitor --date 2026-01-15`
- View your True-Up Period balance: `./enphase-monitor --true-up 2025-01-15` (use the True-Up Start Date from your utility account)
- Build a historical dataset: `./enphase-monitor --start-date 2025-06-19` (writes one JSON record per day into `history/`)
- Run on startup: Add to cron or systemd
- Build a dashboard: Parse the output or extend the code
- Set up alerts: Monitor grid dependence and trigger notifications

## Need More Help?

- **[README.md](README.md)** - Complete documentation with all features and options
- **[OAUTH_SETUP.md](docs/OAUTH_SETUP.md)** - Detailed OAuth guide with troubleshooting
- **[ARCHITECTURE.md](docs/ARCHITECTURE.md)** - Understand how the codebase works
- **Troubleshooting section** in README.md

Happy monitoring! ☀️🔋
