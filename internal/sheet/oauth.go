package sheet

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/option"
	"google.golang.org/api/sheets/v4"
)

// GoogleAuth picks user OAuth when a token file is present, otherwise the service account JSON.
type GoogleAuth struct {
	CredsFile       string // GOOGLE_APPLICATION_CREDENTIALS (service account)
	OAuthClientFile string // desktop OAuth client secret JSON
	OAuthTokenFile  string // saved user token JSON
}

var sheetsScopes = []string{
	sheets.SpreadsheetsScope,
	"https://www.googleapis.com/auth/drive.file",
}

// OpenGoogle prefers a user OAuth token (no per-sheet service-account sharing),
// then falls back to the service account JSON.
func OpenGoogle(ctx context.Context, auth GoogleAuth) (*Google, error) {
	if auth.OAuthTokenFile != "" {
		if _, err := os.Stat(auth.OAuthTokenFile); err == nil {
			svc, err := sheetsFromOAuth(ctx, auth.OAuthClientFile, auth.OAuthTokenFile)
			if err != nil {
				return nil, err
			}
			return &Google{svc: svc}, nil
		}
	}
	if auth.CredsFile == "" {
		return nil, fmt.Errorf("缺少 Google 凭证：设置 GOOGLE_OAUTH_TOKEN（用户登录）或 GOOGLE_APPLICATION_CREDENTIALS（服务账号）")
	}
	return NewGoogle(ctx, auth.CredsFile)
}

func sheetsFromOAuth(ctx context.Context, clientFile, tokenFile string) (*sheets.Service, error) {
	ts, err := tokenSource(ctx, clientFile, tokenFile)
	if err != nil {
		return nil, err
	}
	return sheets.NewService(ctx, option.WithTokenSource(ts), option.WithScopes(sheetsScopes...))
}

func tokenSource(ctx context.Context, clientFile, tokenFile string) (oauth2.TokenSource, error) {
	cfg, err := oauthConfig(clientFile)
	if err != nil {
		return nil, err
	}
	tok, err := loadToken(tokenFile)
	if err != nil {
		return nil, err
	}
	return cfg.TokenSource(ctx, tok), nil
}

func oauthConfig(clientFile string) (*oauth2.Config, error) {
	if clientFile == "" {
		return nil, fmt.Errorf("GOOGLE_OAUTH_CLIENT 为空：需要桌面应用 OAuth 客户端 JSON 才能刷新用户令牌")
	}
	b, err := os.ReadFile(clientFile)
	if err != nil {
		return nil, err
	}
	cfg, err := google.ConfigFromJSON(b, sheetsScopes...)
	if err != nil {
		return nil, fmt.Errorf("解析 OAuth 客户端 JSON：%w", err)
	}
	return cfg, nil
}

func loadToken(path string) (*oauth2.Token, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var tok oauth2.Token
	if err := json.Unmarshal(b, &tok); err != nil {
		return nil, fmt.Errorf("解析 OAuth token：%w", err)
	}
	if tok.AccessToken == "" && tok.RefreshToken == "" {
		return nil, fmt.Errorf("OAuth token 文件无效：%s", path)
	}
	return &tok, nil
}

func saveToken(path string, tok *oauth2.Token) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(tok, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o600)
}

// LoginOAuth runs a one-shot local browser consent and writes the token file.
// clientFile is a Google Cloud "Desktop app" OAuth client secret JSON.
func LoginOAuth(ctx context.Context, clientFile, tokenFile string) error {
	cfg, err := oauthConfig(clientFile)
	if err != nil {
		return err
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return err
	}
	defer ln.Close()
	port := ln.Addr().(*net.TCPAddr).Port
	cfg.RedirectURL = fmt.Sprintf("http://127.0.0.1:%d/oauth2/callback", port)

	codeCh := make(chan string, 1)
	errCh := make(chan error, 1)
	mux := http.NewServeMux()
	mux.HandleFunc("/oauth2/callback", func(w http.ResponseWriter, r *http.Request) {
		if errMsg := r.URL.Query().Get("error"); errMsg != "" {
			errCh <- fmt.Errorf("oauth error: %s", errMsg)
			fmt.Fprint(w, "授权失败，可以关闭此窗口。")
			return
		}
		code := r.URL.Query().Get("code")
		if code == "" {
			errCh <- fmt.Errorf("oauth callback missing code")
			fmt.Fprint(w, "授权失败：没有 code。")
			return
		}
		codeCh <- code
		fmt.Fprint(w, "授权成功，可以关闭此窗口并回到终端。")
	})
	srv := &http.Server{Handler: mux}
	go func() { _ = srv.Serve(ln) }()
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()

	authURL := cfg.AuthCodeURL("earnings-sync", oauth2.AccessTypeOffline, oauth2.ApprovalForce)
	fmt.Println("请在浏览器打开并登录 Google，允许访问表格：")
	fmt.Println(authURL)

	var code string
	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-errCh:
		return err
	case code = <-codeCh:
	}

	tok, err := cfg.Exchange(ctx, code)
	if err != nil {
		return fmt.Errorf("换取 token 失败：%w", err)
	}
	if tokenFile == "" {
		tokenFile = defaultOAuthTokenPath()
	}
	if err := saveToken(tokenFile, tok); err != nil {
		return err
	}
	fmt.Println("已写入", tokenFile)
	return nil
}

func defaultOAuthTokenPath() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return "data/google-oauth-token.json"
	}
	return filepath.Join(home, ".config", "earnings-sync", "google-oauth-token.json")
}
