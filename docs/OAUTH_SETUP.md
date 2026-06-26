# OAuth Setup Guide for Free Developer Plan

This guide will help you get a refresh token for the Enphase API v4 (free developer plan).

> **📖 Learning Path**: This guide explains both the "how" and "why" of OAuth 2.0. If you just want to get set up quickly, follow the [Step-by-Step Instructions](#step-by-step-instructions). If you want to understand how OAuth works, read the [Understanding OAuth 2.0](#understanding-oauth-20-how-and-why) section first.

## Table of Contents

- [Prerequisites](#prerequisites)
- [Understanding OAuth 2.0: How and Why](#understanding-oauth-20-how-and-why)
  - [What is OAuth 2.0?](#what-is-oauth-20)
  - [The OAuth Components](#the-oauth-components-what-they-are-and-why-they-matter)
  - [How Authentication Works](#how-authentication-works-what-the-api-server-expects)
  - [How Authorization Works](#how-authorization-works-the-complete-flow)
- [Step-by-Step Instructions](#step-by-step-instructions)
- [Finding Your System IDs](#finding-your-system-ids)
- [How Your Application Uses Tokens](#how-your-application-uses-tokens-after-setup)
- [Troubleshooting](#troubleshooting)

## Prerequisites

1. You have an Enphase Developer account at https://developer-v4.enphase.com/
2. You have created an application and have:
   - API Key
   - Client ID
   - Client Secret
3. You have configured a redirect URI in your application settings (e.g., `http://localhost:8080/callback`)

## Understanding OAuth 2.0: How and Why

> **💡 Tip**: If you are in a hurry, you can skip to [Step-by-Step Instructions](#step-by-step-instructions) and come back to this section later. However, understanding these concepts will help you troubleshoot issues and understand what is happening behind the scenes.

Before diving into the setup steps, you should understand **what OAuth 2.0 is**, **why each component exists**, and **how the Enphase API server authenticates and authorizes your application**.

### What is OAuth 2.0?

OAuth 2.0 is an industry-standard protocol that allows your application to access a user's data on their behalf **without storing their password**. Instead of asking users for their Enlighten password (which would be insecure), OAuth uses tokens that represent permission to access specific resources.

### The OAuth Components: What They Are and Why They Matter

#### 1. **API Key** (`key`)
- **What it is**: A unique identifier for your application, provided by Enphase
- **Why it exists**: Identifies which application is making the request. Think of it as your application's "name tag"
- **Where it is used**: 
  - As a query parameter (`?key=YOUR_API_KEY`) on every API request
  - As a header (`key: YOUR_API_KEY`) when exchanging authorization codes for tokens
- **Security**: Not secret by itself, but required for all API calls

#### 2. **Client ID** (`client_id`)
- **What it is**: A public identifier for your OAuth application
- **Why it exists**: Identifies your application during the OAuth flow. It is like a username - public but unique
- **Where it is used**:
  - In the authorization URL to tell Enphase which app is requesting access
  - In HTTP Basic Authentication when exchanging codes for tokens
- **Security**: Can be public (it is in URLs), but must match your registered application

#### API Key vs Client ID: Why Both?

These two seem similar—both identify your application and both are public. However, they serve different purposes in Enphase's architecture:

| Aspect | API Key | Client ID |
|--------|---------|-----------|
| **Purpose** | Track and control API usage | Prove app identity during OAuth |
| **Used for** | API Budget enforcement, analytics | Authorization flow, token exchange |
| **Where** | Query param on every API request | Authorization URLs, HTTP Basic Auth |
| **Think of it as** | A metered access pass | A username (paired with `client_secret`) |

**Why Enphase uses both:**

```
┌─────────────────────────────────────────────────────────────────┐
│  OAuth Layer (Who are you?)                                     │
│  └─► client_id + client_secret = "I am App X, and I can prove   │
│       it"                                                       │
│  └─► Results in: access_token (user authorization)              │
└─────────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│  API Layer (What are you allowed to do?)                        │
│  └─► api_key = "Track this request against App X's quota"       │
│  └─► access_token = "User Y authorized this"                    │
│  └─► Results in: data returned (if both valid)                  │
└─────────────────────────────────────────────────────────────────┘
```

This separation allows Enphase to:
- **Revoke OAuth access** without affecting your API key (e.g., user removes app permission)
- **Enforce API Budget per API key** independently of OAuth tokens
- **Track usage per app** even when multiple users authorize the same app
- **Change OAuth credentials** without reissuing API keys (or vice versa)

#### 3. **Client Secret** (`client_secret`)
- **What it is**: A private credential that proves your application's identity
- **Why it exists**: Proves you are the legitimate owner of the Client ID. Only you (and Enphase) know this secret
- **Where it is used**: 
  - In HTTP Basic Authentication (`client_id:client_secret`) when exchanging authorization codes for tokens
  - In HTTP Basic Authentication when refreshing access tokens
- **Security**: **MUST BE KEPT SECRET** - never commit to version control or share publicly

#### 4. **Redirect URI** (`redirect_uri`)
- **What it is**: The URL where Enphase sends the user after they authorize your application
- **Why it exists**: Security measure - ensures authorization codes are only sent to URLs you control. Prevents attackers from intercepting codes
- **Where it is used**: 
  - In the authorization URL (tells Enphase where to redirect)
  - When exchanging the authorization code (must match exactly)
- **Security**: Must match exactly what you registered in the Enphase Developer Portal

#### 5. **Authorization Code** (`code`)
- **What it is**: A temporary, single-use code that represents the user's consent
- **Why it exists**: Short-lived (expires in minutes) and can only be used once. This prevents replay attacks
- **Where it is used**: 
  - Received in the redirect URL after user authorization
  - Exchanged for access and refresh tokens
- **Security**: Expires quickly and is single-use for security

#### 6. **Access Token** (`access_token`)
- **What it is**: A short-lived token (typically 1 hour) that grants access to the API
- **Why it exists**: Represents the user's authorization to access their data. Short-lived for security - if stolen, it expires quickly
- **Where it is used**: 
  - In the `Authorization: Bearer <token>` header on every API request
  - Automatically refreshed by the application when it expires
- **Security**: Short-lived, scoped to specific permissions

#### 7. **Refresh Token** (`refresh_token`)
- **What it is**: A long-lived token that can obtain new access tokens
- **Why it exists**: Allows your application to get new access tokens without asking the user to authorize again. This is what you store in your config
- **Where it is used**: 
  - Stored in your `credentials.yaml` (this is the one-time setup step)
  - Used to get new access tokens when they expire
- **Security**: Long-lived but should be kept secure. If compromised, regenerate it

### How Authentication Works: What the API Server Expects

The Enphase API server uses **two-factor authentication** for every API request:

#### Factor 1: API Key (Application Identity)
Every API request must include the API key as a query parameter:
```
GET /api/v4/systems/12345/telemetry/production_meter?key=YOUR_API_KEY&start_at=...
```

**Why**: Identifies which application is making the request. The API server uses this to:
- Track usage and enforce the API Budget
- Apply application-specific settings
- Log requests for debugging

#### Factor 2: Access Token (User Authorization)
Every API request must include the OAuth access token in the Authorization header:
```
Authorization: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...
```

**Why**: Proves that:
1. A user has authorized your application to access their data
2. The authorization is still valid (token has not expired)
3. The token has the correct permissions (scopes)

### How Authorization Works: The Complete Flow

Here is what happens behind the scenes:

#### Phase 1: Initial Authorization (One-Time Setup)

```
┌─────────┐         ┌──────────────┐         ┌─────────────┐
│  You    │         │   Enphase    │         │   Enphase   │
│ (User)  │         │  Auth Server │         │  API Server │
└────┬────┘         └──────┬───────┘         └──────┬──────┘
     │                     │                        │
     │ 1. Visit auth URL   │                        │
     │────────────────────>│                        │
     │                     │                        │
     │ 2. Login & Authorize│                        │
     │<────────────────────│                        │
     │                     │                        │
     │ 3. Redirect with    │                        │
     │    authorization    │                        │
     │    code             │                        │
     │<────────────────────│                        │
     │                     │                        │
     │ 4. Exchange code    │                        │
     │    for tokens       │                        │
     │────────────────────>│                        │
     │                     │                        │
     │ 5. Receive tokens   │                        │
     │<────────────────────│                        │
     │                     │                        │
     │                     │                        │
```

**Step-by-step breakdown:**

1. **Authorization Request**: Your application redirects the user to Enphase's authorization server with:
   - `client_id`: Identifies your app
   - `redirect_uri`: Where to send the user back
   - `response_type=code`: Requesting an authorization code
   - `scope=read+write`: What permissions you are requesting

2. **User Authorization**: The user logs in and clicks "Authorize". Enphase's server:
   - Verifies the user's identity
   - Checks that the redirect_uri matches what you registered
   - Creates a temporary authorization code

3. **Authorization Code Return**: Enphase redirects the user back to your redirect_uri with the code:
   ```
   http://localhost:8080/callback?code=ABC123XYZ...
   ```

4. **Token Exchange**: Your application exchanges the code for tokens by sending:
   - **HTTP Basic Auth**: `client_id:client_secret` (proves you own the app)
   - **Headers**: 
     - `key: YOUR_API_KEY` (identifies your application)
     - `Content-Type: application/x-www-form-urlencoded`
   - **Body**:
     - `grant_type=authorization_code`
     - `code=ABC123XYZ...` (the authorization code)
     - `redirect_uri=http://localhost:8080/callback` (must match exactly)

5. **Token Response**: Enphase's server validates everything and returns:
   - `access_token`: Short-lived token for API access
   - `refresh_token`: Long-lived token for getting new access tokens
   - `expires_in`: How long the access token is valid (typically 3600 seconds)

#### Phase 2: Making API Requests (Ongoing)

Once you have a refresh token, your application can make API requests:

```
┌─────────────┐         ┌──────────────┐         ┌─────────────┐
│  Your App   │         │   Enphase    │         │   Enphase   │
│             │         │  Auth Server │         │  API Server │
└──────┬──────┘         └──────┬───────┘         └──────┬──────┘
       │                       │                        │
       │ 1. Need access token? │                        │
       │    Check cache        │                        │
       │                       │                        │
       │ 2. Token expired?     │                        │
       │    Refresh it         │                        │
       │──────────────────────>│                        │
       │    (Basic Auth:       │                        │
       │     client_id:secret) │                        │
       │    (Header: key)      │                        │
       │    (Body:             │                        │
       │     grant_type=       │                        │
       │     refresh_token)    │                        │
       │                       │                        │
       │ 3. New access token   │                        │
       │<──────────────────────│                        │
       │                       │                        │
       │ 4. Make API request   │                        │
       │    (Query: ?key=...)  │                        │
       │    (Header:           │                        │
       │     Bearer token)     │                        │
       │───────────────────────────────────────────────>│
       │                       │                        │
       │ 5. API response       │                        │
       │<───────────────────────────────────────────────│
       │                       │                        │
```

**What happens in your application:**

1. **Token Check**: Before making an API request, the application checks if it has a valid access token in cache
   - If cached and not expired → use it
   - If expired or missing → refresh it

2. **Token Refresh**: To get a new access token, your application sends:
   - **HTTP Basic Auth**: `client_id:client_secret`
   - **Headers**: 
     - `key: YOUR_API_KEY`
     - `Content-Type: application/x-www-form-urlencoded`
   - **Body**:
     - `grant_type=refresh_token`
     - `refresh_token=YOUR_REFRESH_TOKEN`

3. **API Request**: With a valid access token, your application makes requests like:
   ```
   GET /api/v4/systems/12345/telemetry/production_meter?key=YOUR_API_KEY&start_at=...
   Headers:
     Authorization: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...
   ```

4. **API Response**: The server validates:
   - API key is valid and matches your application
   - Access token is valid and not expired
   - Access token has permission to access the requested system
   - If all checks pass → returns the data

### Why This Two-factor Approach?

1. **API Key**: Identifies **which application** is making the request
   - Enables API Budget enforcement per application
   - Allows Enphase to track usage
   - Applies application-specific settings

2. **Access Token**: Proves **which user** authorized the request and **what permissions** they granted
   - Ensures you can only access systems the user owns
   - Enforces scope restrictions (read vs. write)
   - Provides audit trail of who accessed what

Together, they ensure that:
- Only registered applications can use the API (API key)
- Only authorized users' data is accessible (access token)
- Applications cannot access data without user consent (OAuth flow)

## Step-by-Step Instructions

### Step 1: Add Redirect URI to Your Credentials

First, add the `redirect_uri` to the shared `api:` block in `config.yaml`. This must match **exactly** what you configured in the Enphase Developer Portal, and is shared by every credential set:

```yaml
# config.yaml
api:
  authorization_url: https://api.enphaseenergy.com/oauth/token
  redirect_uri: http://localhost:8080/callback  # Must match your app settings
```

Your secrets stay in `credentials.yaml`:

```yaml
# credentials.yaml
credentials:
  - name: enphase-monitor-01
    key: YOUR_API_KEY
    client_id: YOUR_CLIENT_ID
    client_secret: YOUR_CLIENT_SECRET
```

**Why this matters**: The redirect URI is a security feature. Enphase will only send authorization codes to URLs you have pre-registered. This prevents attackers from intercepting codes by using a different redirect URI. The redirect_uri must match exactly (including protocol, host, port, and path) between:
- Your config file
- The authorization URL
- The token exchange request
- Your Enphase Developer Portal settings

### Step 2: Generate Authorization URL

This URL initiates the OAuth flow by redirecting the user to Enphase's authorization server.

**Run the setup wizard:**
```bash
./enphase-monitor --update-refresh-tokens
# With more than one credential set, name the one to set up:
./enphase-monitor --update-refresh-tokens enphase-monitor-02
# Or re-authorize every credential in turn (e.g. after they all expired):
./enphase-monitor --update-refresh-tokens --all
```

The wizard launches a headed Chrome window (via chromedp), drives the Enphase
authorization/consent screen to completion — logging in and approving on your
behalf where it can — captures the authorization code, exchanges it for tokens,
and saves the refresh token to `credentials.yaml`. It performs the whole flow end
to end; the manual URL construction and code-exchange steps below are retained only
to explain what the wizard does under the hood (the older hand-paste / loopback-listener
flow has been removed).

**Or manually construct the authorization URL:**
```
https://api.enphaseenergy.com/oauth/authorize?response_type=code&client_id=YOUR_CLIENT_ID&redirect_uri=http://localhost:8080/callback&scope=read+write
```

**URL Components Explained:**
- `https://api.enphaseenergy.com/oauth/authorize`: Enphase's authorization endpoint
- `response_type=code`: Requests an authorization code (not a token directly - this is the Authorization Code flow)
- `client_id=YOUR_CLIENT_ID`: Identifies your application to Enphase
- `redirect_uri=...`: Where Enphase will send the user after authorization (must match registered URI)
- `scope=read+write`: Permissions you are requesting (read data and write data)

Replace:
- `YOUR_CLIENT_ID` with your actual client ID
- `http://localhost:8080/callback` with your redirect URI (must match your app settings)

### Step 3: Authorize in Browser

This step is where the user grants permission to your application.

1. **Open the authorization URL** in your browser
   - This takes you to Enphase's authorization server
   - The server shows what permissions you are requesting

2. **Log in with your Enlighten account**
   - Enphase verifies your identity
   - This ensures only you can authorize access to your systems

3. **Click "Authorize"** to grant access
   - You are granting permission for your application to access your Enphase data
   - This creates an authorization code on Enphase's server

4. **You will be redirected** to your redirect URI with a `code` parameter:
   ```
   http://localhost:8080/callback?code=ABC123XYZ...
   ```

**What just happened**: Enphase created a temporary authorization code and sent it to your redirect URI. This code:
- Is single-use (can only be exchanged once)
- Expires quickly (typically within 10 minutes)
- Represents your consent to grant access

**Security note**: The code is sent via URL parameter, which is why the redirect_uri must be pre-registered. Only your application should receive this code.

### Step 4: Exchange Code for Tokens

This step converts the temporary authorization code into long-lived tokens that your application can use.

**What the API server expects:**
When you exchange the code, the Enphase token endpoint requires:

1. **HTTP Basic Authentication** with `client_id:client_secret`
   - This proves you own the application registered with that client_id
   - The server validates this against their database

2. **API Key header** (`key: YOUR_API_KEY`)
   - Identifies which application is making the request
   - Used for API Budget enforcement and usage tracking

3. **Request body** with:
   - `grant_type=authorization_code`: Tells the server you are exchanging a code
   - `code=YOUR_CODE`: The authorization code from Step 3
   - `redirect_uri=...`: Must match exactly what was used in Step 2

4. **Content-Type header**: `application/x-www-form-urlencoded`
   - Required format for OAuth token requests

**Copy the `code` value from the redirect URL**, then use one of these methods:

#### Option A: Use the Setup Wizard

The setup wizard will prompt you for the code and exchange it automatically. It handles all the authentication details for you.

#### Option B: Manual Exchange (using curl)

This shows exactly what the API server expects:

```bash
curl -X POST https://api.enphaseenergy.com/oauth/token \
  -u "YOUR_CLIENT_ID:YOUR_CLIENT_SECRET" \
  -H "key: YOUR_API_KEY" \
  -H "Content-Type: application/x-www-form-urlencoded" \
  -d "grant_type=authorization_code&code=YOUR_CODE&redirect_uri=http://localhost:8080/callback"
```

**Breaking down the request:**
- `-u "CLIENT_ID:CLIENT_SECRET"`: HTTP Basic Auth (proves app ownership)
- `-H "key: YOUR_API_KEY"`: Application identifier header
- `-H "Content-Type: application/x-www-form-urlencoded"`: Required content type
- `-d "grant_type=..."`: Request body with grant type, code, and redirect_uri

Replace:
- `YOUR_CLIENT_ID` and `YOUR_CLIENT_SECRET` with your credentials
- `YOUR_API_KEY` with your API key
- `YOUR_CODE` with the code from the redirect URL
- `http://localhost:8080/callback` with your redirect URI (must match exactly)

**The response will look like:**
```json
{
  "access_token": "eyJ...",
  "refresh_token": "def...",
  "token_type": "Bearer",
  "expires_in": 3600
}
```

**What you get:**
- `access_token`: Short-lived token (1 hour) for making API requests
- `refresh_token`: Long-lived token (does not expire) for getting new access tokens
- `token_type`: Always "Bearer" (indicates how to use the token)
- `expires_in`: Seconds until access_token expires (3600 = 1 hour)

### Step 5: Add Refresh Token to Credentials

> If you used the `--update-refresh-tokens` wizard, this is done for you — it writes the
> `refresh_token` straight into the matching credential entry in `credentials.yaml`.
> The steps below apply only if you ran the OAuth flow manually (e.g. with `curl`).

Add the `refresh_token` from the response to the matching credential entry in your `credentials.yaml` (the shared `authorization_url` / `redirect_uri` stay in `config.yaml`):

```yaml
credentials:
  - name: enphase-monitor-01
    key: YOUR_API_KEY
    client_id: YOUR_CLIENT_ID
    client_secret: YOUR_CLIENT_SECRET
    refresh_token: YOUR_REFRESH_TOKEN  # Add this!
```

**Why this is the only token you store:**
- The `access_token` expires in 1 hour, so storing it does not help
- The `refresh_token` is long-lived and can get new access tokens whenever needed
- Your application will automatically use the refresh_token to get new access tokens when needed
- This is a **one-time setup** - you will not need to do this again unless the refresh token is revoked

**Security note**: The refresh_token is sensitive. Keep your `credentials.yaml` secure and never commit it to version control (it is gitignored by default).

### Step 6: Update Systems to Use Cloud API

Configure your systems with their system IDs:

```yaml
systems:
  - name: "Left Subpanel"
    id: "YOUR_SYSTEM_ID"  # Get from Enlighten URL
  
  - name: "Right Subpanel"
    id: "YOUR_OTHER_SYSTEM_ID"
```

**Why system IDs are needed**: The API needs to know which specific Enphase system to query. Each system has a unique ID that identifies it in Enphase's database.

## Finding Your System IDs

For detailed instructions on finding your System IDs, see [Finding Your System IDs](README.md#finding-your-system-ids) in the README.

## How Your Application Uses Tokens (After Setup)

Once you have completed the setup, here is what happens automatically when your application makes API requests:

### Automatic Token Management

1. **Before each API request**, the application checks if it has a valid access token:
   - Checks in-memory cache for a non-expired access token
   - If found and still valid → uses it directly

2. **If token is expired or missing**, the application automatically refreshes it:
   - Makes a POST request to `https://api.enphaseenergy.com/oauth/token`
   - Uses HTTP Basic Auth with `client_id:client_secret`
   - Sends `grant_type=refresh_token` and your `refresh_token` from config
   - Receives a new `access_token` and caches it

3. **Makes the API request** with both authentication factors:
   - Query parameter: `?key=YOUR_API_KEY`
   - Header: `Authorization: Bearer <access_token>`

### Example: What Happens When You Run the Application

```
1. Application starts
   └─> Loads credentials.yaml (reads refresh_token)

2. First API request needed
   └─> No cached access token
   └─> POST /oauth/token with refresh_token
   └─> Receives new access_token
   └─> Caches it in memory

3. Makes API request
   └─> GET /api/v4/systems/12345/telemetry/production_meter?key=...
   └─> Header: Authorization: Bearer <access_token>
   └─> Server validates both API key and access token
   └─> Returns data

4. Subsequent requests (within 1 hour)
   └─> Uses cached access_token (no refresh needed)

5. After 1 hour (token expired)
   └─> Automatically refreshes using refresh_token
   └─> Gets new access_token
   └─> Continues making requests
```

**You do not need to do anything** - the application handles all token management automatically!

## Troubleshooting

### "redirect_uri mismatch"

**Error**: The redirect URI in your request does not match what is registered in the Enphase Developer Portal.

**Why this happens**: Enphase validates that authorization codes are only sent to pre-registered URLs. This is a security feature.

**Solution**: 
- Check your `credentials.yaml` - the `redirect_uri` must match exactly
- Check the Enphase Developer Portal - ensure the redirect URI is registered there
- Check the authorization URL - the `redirect_uri` parameter must match
- Check the token exchange request - the `redirect_uri` in the body must match
- **Exact match required**: Protocol (`http` vs `https`), host, port, and path must all match

### "invalid_client"

**Error**: The client_id or client_secret is incorrect.

**Why this happens**: The API server validates your credentials using HTTP Basic Authentication. If the client_id or client_secret does not match what is registered, authentication fails.

**Solution**:
- Verify `client_id` in your config matches what is in the Enphase Developer Portal
- Verify `client_secret` in your config matches what is in the Enphase Developer Portal
- Check for typos or extra whitespace (especially when copy/pasting)
- Ensure you are using the credentials for the correct application (if you have multiple apps)

### "invalid_grant"

**Error**: The authorization code is invalid, expired, or already used.

**Why this happens**: Authorization codes are:
- **Single-use**: Once exchanged, they cannot be used again
- **Short-lived**: Expire within 10 minutes typically
- **Tied to redirect_uri**: Must be used with the same redirect_uri that created them

**Solution**:
- Get a fresh authorization code by going through Step 3 again
- Use the code immediately after receiving it (do not wait)
- Ensure the `redirect_uri` in the token exchange matches exactly what was used in the authorization URL
- Do not try to reuse codes from previous attempts

### "unauthorized" or 401 errors when making API requests

**Error**: The access token is invalid or expired.

**Why this happens**: Access tokens expire after 1 hour. If the application cannot refresh the token (e.g., refresh_token is invalid), you will get this error.

**Solution**:
- Check that your `refresh_token` in credentials.yaml is correct
- If the refresh_token was revoked or is invalid, you will need to go through the setup process again (Steps 2-5)
- Run `./enphase-monitor --update-refresh-tokens` to regenerate your refresh token

### Token refresh fails

**Error**: Cannot get a new access token using the refresh token.

**Why this happens**: Refresh tokens can be revoked if:
- The user revokes access in their Enphase account
- The application credentials change
- The token is very old and has been rotated

**Solution**:
- Go through the OAuth setup process again (Steps 2-5) to get a new refresh_token
- Ensure your `client_id` and `client_secret` are still valid
- Check that your Enphase Developer account is still active
