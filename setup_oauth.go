package main

import (
	"bufio"
	"context"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

// openBrowser opens the specified URL in the default browser
func openBrowser(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "linux":
		cmd = exec.Command("xdg-open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		return fmt.Errorf("unsupported platform")
	}
	return cmd.Start()
}

// SetupOAuth helps users complete the one-time OAuth authorization flow
func SetupOAuth(config *Config) error {
	if config.API == nil {
		return fmt.Errorf("API configuration is required")
	}

	if config.API.ClientID == "" {
		return fmt.Errorf("client_id is required in API configuration")
	}

	// Get redirect URI from config or prompt
	redirectURI := config.API.RedirectURI
	if redirectURI == "" {
		fmt.Print("Enter your redirect URI (e.g., http://localhost:8080/callback): ")
		reader := bufio.NewReader(os.Stdin)
		input, err := reader.ReadString('\n')
		if err != nil {
			return fmt.Errorf("failed to read redirect URI from stdin: %w", err)
		}
		redirectURI = strings.TrimSpace(input)
		if redirectURI == "" {
			redirectURI = "http://localhost:8080/callback"
			fmt.Printf("Using default: %s\n", redirectURI)
		}
		// Update config with the redirect URI (needed for GetAuthorizationURL)
		config.API.RedirectURI = redirectURI
	}

	// Generate authorization URL
	authURL, err := GetAuthorizationURL(config.API)
	if err != nil {
		return fmt.Errorf("failed to generate authorization URL: %w", err)
	}

	fmt.Println("\n==========================================")
	fmt.Println("Enphase OAuth Refresh Token Setup")
	fmt.Println("==========================================")
	fmt.Println()
	fmt.Println("Loaded configuration:")
	clientIDPreview := config.API.ClientID
	if len(clientIDPreview) > 20 {
		clientIDPreview = clientIDPreview[:20] + "..."
	}
	apiKeyPreview := config.API.Key
	if len(apiKeyPreview) > 20 {
		apiKeyPreview = apiKeyPreview[:20] + "..."
	}
	fmt.Printf("  Client ID: %s\n", clientIDPreview)
	fmt.Printf("  API Key: %s\n", apiKeyPreview)
	fmt.Printf("  Redirect URI: %s\n", redirectURI)
	fmt.Println()

	// Try to open the browser automatically
	fmt.Println("STEP 1: Opening your browser to the authorization page...")
	fmt.Println()
	if err := openBrowser(authURL); err != nil {
		fmt.Println("Could not open browser automatically.")
		fmt.Println("Please copy and paste this URL into your browser:")
		fmt.Println()
		fmt.Println(authURL)
	} else {
		fmt.Println("Browser opened! If it did not open, copy and paste this URL:")
		fmt.Println()
		fmt.Println(authURL)
	}
	fmt.Println()
	fmt.Println("STEP 2: Log in with your Enlighten account and authorize the application")
	fmt.Println()
	fmt.Println("STEP 3: After authorization, you will be redirected to:")
	fmt.Printf("  %s?code=ABC123...\n", redirectURI)
	fmt.Println()
	fmt.Println("STEP 4: IMPORTANT - The browser will show 'Site not found' - THIS IS NORMAL!")
	fmt.Println("        Just look at the address bar and copy the ENTIRE URL")
	fmt.Println("        The URL will contain either 'code=' (success) or 'error=' (failure)")
	fmt.Println()
	fmt.Print("Paste the ENTIRE redirect URL from your browser address bar: ")

	reader := bufio.NewReader(os.Stdin)
	code, err := reader.ReadString('\n')
	if err != nil {
		return fmt.Errorf("failed to read authorization URL from stdin: %w", err)
	}
	code = strings.TrimSpace(code)

	if code == "" {
		return fmt.Errorf("authorization code is required")
	}

	// Check if user pasted a URL with an error
	if strings.Contains(code, "error=") {
		fmt.Println()
		fmt.Println("ERROR: Authorization failed. The redirect URL contains an error.")
		fmt.Printf("URL: %s\n", code)
		fmt.Println()
		fmt.Println("Common issues:")
		fmt.Println("1. The redirect_uri does not match your app settings in Developer Portal")
		fmt.Println("2. You used the wrong authorization URL (should have 'read write' scope)")
		fmt.Println()
		return fmt.Errorf("authorization failed - please check the error in the URL and try again")
	}

	// Extract code from URL if user pasted full URL
	if strings.Contains(code, "code=") {
		parsedURL, err := url.Parse(code)
		if err == nil {
			code = parsedURL.Query().Get("code")
		}
	}

	if code == "" {
		fmt.Println()
		fmt.Println("Could not find 'code=' or 'error=' in the URL you provided.")
		fmt.Println("Please paste the complete URL from your browser address bar.")
		return fmt.Errorf("invalid redirect URL - no authorization code found")
	}

	// Exchange code for tokens
	fmt.Println("\nExchanging authorization code for tokens...")
	tokenResp, err := ExchangeAuthorizationCode(context.Background(), config.API, code)
	if err != nil {
		return fmt.Errorf("failed to exchange authorization code: %w", err)
	}

	if tokenResp.RefreshToken == "" {
		return fmt.Errorf("no refresh token in response - authorization may have failed")
	}

	fmt.Println("\n✓ Success! Tokens received:")
	fmt.Println()

	// Display truncated tokens for verification
	accessTokenPreview := tokenResp.AccessToken
	if len(accessTokenPreview) > 20 {
		accessTokenPreview = accessTokenPreview[:20] + "..."
	}
	refreshTokenPreview := tokenResp.RefreshToken
	if len(refreshTokenPreview) > 20 {
		refreshTokenPreview = refreshTokenPreview[:20] + "..."
	}

	fmt.Printf("Access Token: %s\n", accessTokenPreview)
	fmt.Printf("Refresh Token: %s\n", refreshTokenPreview)
	fmt.Printf("Expires In: %d seconds\n", tokenResp.ExpiresIn)

	// Output the complete api section for easy copy-paste into config.yaml
	fmt.Println()
	fmt.Println("==================================================")
	fmt.Println("Replace the entire api section of the config.yaml:")
	fmt.Println("==================================================")
	fmt.Println()
	configLines := []string{
		"api:",
		fmt.Sprintf("  key: %s", config.API.Key),
		fmt.Sprintf("  client_id: %s", config.API.ClientID),
		fmt.Sprintf("  client_secret: %s", config.API.ClientSecret),
		"  authorization_url: https://api.enphaseenergy.com/oauth/token",
		fmt.Sprintf("  redirect_uri: %s", redirectURI),
		fmt.Sprintf("  refresh_token: %s", tokenResp.RefreshToken),
	}
	for _, line := range configLines {
		fmt.Println(line)
	}
	fmt.Println()

	// Wait for user to confirm, then wipe the credentials from the terminal
	fmt.Print("Press Enter after you have copied the above into config.yaml...")
	bufio.NewReader(os.Stdin).ReadString('\n')

	// Erase the config block + surrounding lines from the terminal.
	// Each \033[A moves the cursor up one line, \033[2K erases that line.
	// From bottom to top: newline after Enter, prompt, blank, [configLines],
	// blank, border, header, border, blank = configLines + 10
	linesToErase := len(configLines) + 10
	for i := 0; i < linesToErase; i++ {
		fmt.Print("\033[A\033[2K")
	}
	fmt.Println("[credentials cleared from terminal]")

	return nil
}
