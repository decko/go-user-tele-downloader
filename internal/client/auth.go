package client

import (
	"bufio"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/auth"
	"github.com/gotd/td/tg"
	"golang.org/x/crypto/pbkdf2"

	"github.com/decko/go-user-tele-downloader/internal/config"
)

// Authenticator handles MTProto authentication flow
type Authenticator struct {
	config *config.Config
	logger interface {
		Info(msg string, args ...any)
		Error(msg string, args ...any)
	}
}

// NewAuthenticator creates a new authenticator
func NewAuthenticator(cfg *config.Config, logger interface {
	Info(msg string, args ...any)
	Error(msg string, args ...any)
}) *Authenticator {
	return &Authenticator{
		config: cfg,
		logger: logger,
	}
}

// Authenticate performs the authentication flow
func (a *Authenticator) Authenticate(ctx context.Context, client *telegram.Client) error {
	// Check if we have a valid session
	status, err := client.Auth().Status(ctx)
	if err != nil {
		return fmt.Errorf("failed to get auth status: %w", err)
	}

	if status.Authorized {
		a.logger.Info("already authenticated", "user", status.User.FirstName)
		return nil
	}

	a.logger.Info("starting authentication flow", "phone", a.config.Phone)

	// Create authentication flow
	flow := auth.NewFlow(
		auth.Constant(a.config.Phone, a.config.Password, a),
		auth.SendCodeOptions{},
	)

	// Run authentication with retry
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			a.logger.Info("retrying authentication", "attempt", attempt+1)
			time.Sleep(time.Duration(attempt) * 2 * time.Second)
		}

		if err := flow.Run(ctx, client.Auth()); err != nil {
			lastErr = err
			a.logger.Error("authentication failed", "error", err, "attempt", attempt+1)
			continue
		}

		// Success
		a.logger.Info("authentication successful")
		return nil
	}

	return fmt.Errorf("authentication failed after 3 attempts: %w", lastErr)
}

// Code implements auth.CodeAuthenticator
func (a *Authenticator) Code(ctx context.Context, sentCode *tg.AuthSentCode) (string, error) {
	// Try environment variable first (headless mode)
	if code := os.Getenv("TELEGRAM_VERIFICATION_CODE"); code != "" {
		a.logger.Info("using verification code from environment")
		return code, nil
	}

	// Interactive mode
	fmt.Printf("Enter verification code sent to %s: ", a.config.Phone)
	reader := bufio.NewReader(os.Stdin)
	code, err := reader.ReadString('\n')
	if err != nil {
		return "", fmt.Errorf("failed to read code: %w", err)
	}
	return strings.TrimSpace(code), nil
}

// Password implements auth.PasswordAuthenticator
func (a *Authenticator) Password(ctx context.Context) (string, error) {
	if a.config.Password != "" {
		return a.config.Password, nil
	}

	// Interactive mode
	fmt.Print("Enter 2FA password: ")
	reader := bufio.NewReader(os.Stdin)
	password, err := reader.ReadString('\n')
	if err != nil {
		return "", fmt.Errorf("failed to read password: %w", err)
	}
	return strings.TrimSpace(password), nil
}

// AcceptTermsOfService implements auth.TermsOfServiceAuthenticator
func (a *Authenticator) AcceptTermsOfService(ctx context.Context, tos tg.HelpTermsOfService) error {
	return nil
}

// SignUp implements auth.SignUpAuthenticator
func (a *Authenticator) SignUp(ctx context.Context) (auth.UserInfo, error) {
	return auth.UserInfo{}, fmt.Errorf("sign up not supported, please register via Telegram app first")
}

// SessionManager handles encrypted session storage
type SessionManager struct {
	sessionPath string
	password    string
}

// NewSessionManager creates a new session manager
func NewSessionManager(sessionPath, password string) *SessionManager {
	return &SessionManager{
		sessionPath: sessionPath,
		password:    password,
	}
}

// LoadSession loads and decrypts a session from disk.
// Implements session.Storage interface.
func (m *SessionManager) LoadSession(ctx context.Context) ([]byte, error) {
	encrypted, err := os.ReadFile(m.sessionPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // No session yet
		}
		return nil, fmt.Errorf("failed to read session file: %w", err)
	}

	// Decrypt session
	decrypted, err := m.decrypt(encrypted)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt session: %w", err)
	}

	return decrypted, nil
}

// StoreSession encrypts and saves a session to disk.
// Implements session.Storage interface.
func (m *SessionManager) StoreSession(ctx context.Context, data []byte) error {
	// Encrypt session
	encrypted, err := m.encrypt(data)
	if err != nil {
		return fmt.Errorf("failed to encrypt session: %w", err)
	}

	// Write to file with restrictive permissions
	if err := os.WriteFile(m.sessionPath, encrypted, 0600); err != nil {
		return fmt.Errorf("failed to write session file: %w", err)
	}

	return nil
}

// encrypt encrypts data using AES-256-GCM
func (m *SessionManager) encrypt(plaintext []byte) ([]byte, error) {
	// Derive key from password using PBKDF2
	key := pbkdf2.Key([]byte(m.password), []byte("go-user-tele-downloader"), 100000, 32, sha256.New)

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}

	return gcm.Seal(nonce, nonce, plaintext, nil), nil
}

// decrypt decrypts data using AES-256-GCM
func (m *SessionManager) decrypt(ciphertext []byte) ([]byte, error) {
	// Derive key from password using PBKDF2
	key := pbkdf2.Key([]byte(m.password), []byte("go-user-tele-downloader"), 100000, 32, sha256.New)

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonceSize := gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return nil, fmt.Errorf("ciphertext too short")
	}

	nonce, ciphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]
	return gcm.Open(nil, nonce, ciphertext, nil)
}
