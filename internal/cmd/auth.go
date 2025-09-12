package cmd

import (
	"bufio"
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/crush/internal/auth"
	"github.com/charmbracelet/crush/internal/config"
	"github.com/spf13/cobra"
)

var authCmd = &cobra.Command{
	Use:   "auth",
	Short: "Manage authentication for AI providers",
	Long: `Manage authentication credentials for AI providers.
Supports OAuth authentication for Claude Pro/Max and GitHub Copilot subscriptions and API key management.`,
}

var authLoginCmd = &cobra.Command{
	Use:   "login",
	Short: "Authenticate with an AI provider",
	Long: `Start the authentication process with an AI provider.
Currently supports Claude Pro/Max and GitHub Copilot OAuth authentication for subscription access.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runAuthLogin(cmd.Context())
	},
}

var authLogoutCmd = &cobra.Command{
	Use:   "logout",
	Short: "Remove stored authentication credentials",
	Long:  `Remove stored authentication credentials for AI providers.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runAuthLogout(cmd.Context())
	},
}

var authStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show authentication status",
	Long:  `Display the current authentication status for all configured providers.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runAuthStatus(cmd.Context())
	},
}

func init() {
	authCmd.AddCommand(authLoginCmd)
	authCmd.AddCommand(authLogoutCmd)
	authCmd.AddCommand(authStatusCmd)

	rootCmd.AddCommand(authCmd)
}

// runAuthLogin handles the authentication login process.
func runAuthLogin(ctx context.Context) error {
    if _, err := config.Load("", "", false); err != nil {
        return fmt.Errorf("failed to load configuration: %w", err)
    }
    authManager := auth.NewAuthManager(config.GlobalDataDir())

	// Show current status but do not early-return; allow logging into another provider.
	fmt.Println("🔐 Subscription Authentication")
	fmt.Println("📝 Authenticate with your subscription provider")
	fmt.Println("")
	if authManager.HasClaudeSubAuth() {
		if valid, _ := authManager.IsClaudeSubTokenValid(); valid {
			fmt.Println("• Claude Pro/Max: already authenticated")
		} else {
			fmt.Println("• Claude Pro/Max: needs refresh (will auto-refresh)")
		}
	} else {
		fmt.Println("• Claude Pro/Max: not authenticated")
	}
	if authManager.HasGithubCopilotAuth() {
		if valid, _ := authManager.IsGithubCopilotTokenValid(); valid {
			fmt.Println("• GitHub Copilot: already authenticated")
		} else {
			fmt.Println("• GitHub Copilot: needs new token (will mint on use)")
		}
	} else {
		fmt.Println("• GitHub Copilot: not authenticated")
	}
	fmt.Println("")

	// Show provider selection menu.
	fmt.Println("Available authentication methods:")
	fmt.Println("  1. Claude Pro/Max (OAuth via claude.ai) [RECOMMENDED]")
	fmt.Println("  2. GitHub Copilot (Device OAuth via github.com)")
	fmt.Println("")

	fmt.Print("Select an option [1-2]: ")
	reader := bufio.NewReader(os.Stdin)
	choice, _ := reader.ReadString('\n')
	choice = strings.TrimSpace(choice)
	if choice == "2" {
		fmt.Printf("\n🚀 Starting OAuth login with GitHub Copilot...\n\n")
		return authenticateGithubCopilot(ctx, authManager)
	}

	fmt.Printf("\n🚀 Starting OAuth authentication with Claude Pro/Max...\n\n")
	return authenticateClaudeSub(ctx, authManager)
}

func authenticateClaudeSub(ctx context.Context, authManager *auth.AuthManager) error {
	oauthFlow := auth.NewOAuthFlow()

	// Generate authorization URL.
	authURL, err := oauthFlow.GenerateAuthURL()
	if err != nil {
		return fmt.Errorf("failed to generate authorization URL: %w", err)
	}

	fmt.Println("📱 Opening your browser for authentication...")
	fmt.Printf("🌐 Auth URL: %s\n\n", authURL)

	// Try to open browser.
	if err := oauthFlow.OpenBrowser(authURL); err != nil {
		slog.Debug("Failed to open browser", "error", err)
		fmt.Println("⚠️  Could not open browser automatically")
		fmt.Printf("🔗 Please manually open this URL in your browser:\n%s\n\n", authURL)
	} else {
		fmt.Println("✅ Opened browser for authentication")
		fmt.Println("")
	}

	// Instructions for user.
	fmt.Println("📋 Instructions:")
	fmt.Println("   1. Complete the OAuth authorization in your browser")
	fmt.Println("   2. You'll be redirected to a callback page")
	fmt.Println("   3. Copy the authorization code from the callback page")
	fmt.Println("   4. The code will be in format: code#state")
	fmt.Println("")

	// Prompt for authorization code.
	fmt.Print("📝 Paste the authorization code here: ")
	reader := bufio.NewReader(os.Stdin)
	codeInput, err := reader.ReadString('\n')
	if err != nil {
		return fmt.Errorf("failed to read authorization code: %w", err)
	}

	codeInput = strings.TrimSpace(codeInput)
	if codeInput == "" {
		return fmt.Errorf("no authorization code provided")
	}

	fmt.Println("\n🔄 Exchanging authorization code for tokens...")

	// Exchange code for tokens.
	tokenResp, err := oauthFlow.ExchangeCodeForTokens(ctx, codeInput)
	if err != nil {
		return fmt.Errorf("token exchange failed: %w", err)
	}

	fmt.Println("💾 Storing authentication credentials...")

	// Store credentials.
	if err := authManager.StoreClaudeSubCredentials(tokenResp); err != nil {
		return fmt.Errorf("failed to store credentials: %w", err)
	}

	fmt.Println("✅ Successfully authenticated with Claude Pro/Max!")
	fmt.Printf("⏰ Token expires: %s\n", time.Now().Add(time.Duration(tokenResp.ExpiresIn)*time.Second).Format("2006-01-02 15:04:05"))
	fmt.Println("🎉 You can now use the claudesub provider with your subscription")
	fmt.Println("")
	fmt.Println("💡 Configure your models to use the 'claudesub' provider:")
	fmt.Printf("   crush config set models.large.provider claudesub\n")

	return nil
}

func authenticateGithubCopilot(ctx context.Context, authManager *auth.AuthManager) error {
	// Start device flow.
	dev, err := auth.StartGithubDeviceAuth(ctx)
	if err != nil {
		return fmt.Errorf("failed to start GitHub device authorization: %w", err)
	}

	fmt.Println("📱 Open your browser and authorize GitHub")
	fmt.Printf("🔗 Visit: %s\n", dev.VerificationURI)
	fmt.Printf("🔑 Code:  %s\n\n", dev.UserCode)

	// Try to open the browser automatically.
	if err := auth.NewOAuthFlow().OpenBrowser(dev.VerificationURI); err == nil {
		fmt.Println("✅ Opened browser for GitHub authorization")
	} else {
		fmt.Println("⚠️  Could not open browser automatically; please open the link above.")
	}

	fmt.Println("⏳ Waiting for authorization to complete...")
	ghToken, err := auth.PollGithubDeviceAuth(ctx, dev.DeviceCode, dev.Interval)
	if err != nil {
		return fmt.Errorf("GitHub authorization failed: %w", err)
	}

	// Store the GitHub OAuth token as refresh.
	if err := authManager.StoreGithubCopilotRefresh(ghToken); err != nil {
		return fmt.Errorf("failed to store GitHub token: %w", err)
	}

	fmt.Println("🔄 Exchanging for Copilot access token...")
	if err := authManager.UpdateGithubCopilotAccess(ctx); err != nil {
		return fmt.Errorf("failed to mint Copilot token: %w", err)
	}

	fmt.Println("✅ Successfully authenticated with GitHub Copilot!")
	creds, _ := authManager.GetGithubCopilotCredentials()
	if creds != nil && creds.Expires > 0 {
		fmt.Printf("⏰ Token expires: %s\n", time.UnixMilli(creds.Expires).Format("2006-01-02 15:04:05"))
	}
	fmt.Println("🎉 You can now use your Copilot subscription where supported")
	fmt.Println("")
	fmt.Println("💡 To use Copilot in Crush, set your model provider:")
	fmt.Println("   crush config set models.large.provider github-copilot")
	fmt.Println("   crush config set models.large.model gpt-4o")
	return nil
}

// runAuthLogout handles the authentication logout process.
func runAuthLogout(ctx context.Context) error {
    if _, err := config.Load("", "", false); err != nil {
        return fmt.Errorf("failed to load configuration: %w", err)
    }
    authManager := auth.NewAuthManager(config.GlobalDataDir())

	// Offer to log out from either provider if present.
	hadAny := false
	if authManager.HasClaudeSubAuth() {
		hadAny = true
		fmt.Println("🔓 Signing out of Claude Pro/Max subscription...")
		if err := authManager.ClearClaudeSubCredentials(); err != nil {
			return fmt.Errorf("failed to clear claudesub credentials: %w", err)
		}
		fmt.Println("✅ Signed out of Claude Pro/Max")
	}
	if authManager.HasGithubCopilotAuth() {
		hadAny = true
		fmt.Println("🔓 Signing out of GitHub Copilot subscription...")
		if err := authManager.ClearGithubCopilotCredentials(); err != nil {
			return fmt.Errorf("failed to clear github-copilot credentials: %w", err)
		}
		fmt.Println("✅ Signed out of GitHub Copilot")
	}
	if !hadAny {
		fmt.Println("ℹ️  No subscription authentication found")
		return nil
	}
	fmt.Println("💡 Use 'crush auth login' to authenticate again")
	return nil
}

// runAuthStatus displays the current authentication status.
func runAuthStatus(ctx context.Context) error {
    if _, err := config.Load("", "", false); err != nil {
        return fmt.Errorf("failed to load configuration: %w", err)
    }
    authManager := auth.NewAuthManager(config.GlobalDataDir())

	fmt.Println("🔐 Authentication Status")
	fmt.Println("========================")
	fmt.Println("")

	// Check Claude Pro/Max status.
	if authManager.HasClaudeSubAuth() {
		creds, err := authManager.GetClaudeSubCredentials()
		if err != nil {
			fmt.Printf("❌ Claude Pro/Max: Error reading credentials (%v)\n", err)
		} else {
			valid, err := authManager.IsClaudeSubTokenValid()
			if err != nil {
				fmt.Printf("⚠️  Claude Pro/Max: Cannot verify token validity (%v)\n", err)
			} else if valid {
				expiresAt := time.UnixMilli(creds.Expires)
				fmt.Printf("✅ Claude Pro/Max: Authenticated (expires %s)\n", expiresAt.Format("2006-01-02 15:04:05"))
			} else {
				fmt.Println("⚠️  Claude Pro/Max: Token expired (will auto-refresh on next use)")
			}
		}
	} else {
		fmt.Println("❌ Claude Pro/Max: Not authenticated")
	}

	// Check GitHub Copilot status.
	if authManager.HasGithubCopilotAuth() {
		creds, err := authManager.GetGithubCopilotCredentials()
		if err != nil {
			fmt.Printf("❌ GitHub Copilot: Error reading credentials (%v)\n", err)
		} else {
			valid, err := authManager.IsGithubCopilotTokenValid()
			if err != nil {
				fmt.Printf("⚠️  GitHub Copilot: Cannot verify token validity (%v)\n", err)
			} else if valid {
				expiresAt := time.UnixMilli(creds.Expires)
				fmt.Printf("✅ GitHub Copilot: Authenticated (expires %s)\n", expiresAt.Format("2006-01-02 15:04:05"))
			} else {
				fmt.Println("⚠️  GitHub Copilot: Token expired (will mint a new token on next use)")
			}
		}
	} else {
		fmt.Println("❌ GitHub Copilot: Not authenticated")
	}

	fmt.Println("")
	fmt.Println("💡 Use 'crush auth login' to authenticate")
	fmt.Println("💡 Use 'crush auth logout' to sign out")

	return nil
}
