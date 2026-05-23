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

# Create your configuration file
make setup
# OR manually:
# cp config.yaml.example config.yaml
```

## Step 3: Configure Your Systems

Edit `config.yaml` with your favorite text editor:

```yaml
api:
  key: "YOUR_API_KEY"              # From Developer Portal
  client_id: "YOUR_CLIENT_ID"       # From Developer Portal
  client_secret: "YOUR_CLIENT_SECRET"  # From Developer Portal
  authorization_url: "https://api.enphaseenergy.com/oauth/token"
  redirect_uri: "http://localhost:8080/callback"
  refresh_token: ""  # Will be filled in Step 4

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
./enphase-monitor --oauth-setup
```

This interactive wizard will:
1. Generate an authorization URL
2. Guide you to authorize the application in your browser
3. Help you exchange the authorization code for tokens
4. Show you the `refresh_token` to add to your config

**After the wizard completes:**
1. Copy the `refresh_token` from the output
2. Add it to your `config.yaml`:
   ```yaml
   api:
     refresh_token: "YOUR_REFRESH_TOKEN"  # Paste here
   ```

> **📖 Want to understand OAuth better?** See **[OAUTH_SETUP.md](docs/OAUTH_SETUP.md)** for a detailed explanation of how OAuth works, what each component does, and how authentication is performed.

## Step 5: Build and Install

```bash
# Install dependencies
go mod download

# Build the application
go build -o enphase-monitor
```

## Step 6: Test It!

Run a single query to make sure everything works:

```bash
./enphase-monitor
```

You should see output showing your combined system metrics!

## Step 7: Start Monitoring

Start continuous monitoring (refreshes every hour by default):

```bash
make run
```

Press `Ctrl+C` to stop.

## Common First-Time Issues

### "api configuration required"
→ Make sure you have filled in `api.key`, `api.client_id`, and `api.client_secret` in `config.yaml`

### "API request failed with status 401"
→ Your refresh token might be missing or expired. Complete OAuth setup: `./enphase-monitor --oauth-setup`

### "system must have id"
→ You need to replace the example system IDs with your actual ones from Enlighten (see Step 3)

### "redirect_uri mismatch"
→ The redirect URI in your config must match exactly what you registered in the Enphase Developer Portal

## Next Steps

Once it is working:

- Query a Past Period: `./enphase-monitor --date 2026-01-15`
- View your True-Up Period balance: `./enphase-monitor --true-up 2025-01-15` (use the True-Up Start Date from your utility account)
- Run on startup: Add to cron or systemd
- Build a dashboard: Parse the output or extend the code
- Set up alerts: Monitor grid dependence and trigger notifications

## Need More Help?

- **[README.md](README.md)** - Complete documentation with all features and options
- **[OAUTH_SETUP.md](docs/OAUTH_SETUP.md)** - Detailed OAuth guide with troubleshooting
- **[ARCHITECTURE.md](docs/ARCHITECTURE.md)** - Understand how the codebase works
- **Troubleshooting section** in README.md

Happy monitoring! ☀️🔋
