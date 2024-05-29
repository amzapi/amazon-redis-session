package amazonsession

import (
	"context"
	"net/http"
	"testing"
	"time"
)

func TestNewAmazonSession(t *testing.T) {
	type args struct {
		cfg *Config
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{
			name: "Valid configuration",
			args: args{
				cfg: &Config{
					Addr:     "127.0.0.1:6379",
					Password: "123456",
					Db:       0,
				},
			},
			wantErr: false,
		},
		{
			name: "Invalid address",
			args: args{
				cfg: &Config{
					Addr:     "invalid_address",
					Password: "",
					Db:       0,
				},
			},
			wantErr: true,
		},
		{
			name: "Invalid password",
			args: args{
				cfg: &Config{
					Addr:     "127.0.0.1:6379",
					Password: "wrong_password",
					Db:       0,
				},
			},
			wantErr: true,
		},
		{
			name: "Invalid database number",
			args: args{
				cfg: &Config{
					Addr:     "127.0.0.1:6379",
					Password: "",
					Db:       -1,
				},
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rdb, err := NewAmazonSession(tt.args.cfg)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewAmazonSession() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && rdb == nil {
				t.Errorf("NewAmazonSession() returned nil, expected non-nil")
			}
		})
	}
}

func createTestSession(country string, sessionID string) *Session {
	cookies := []*http.Cookie{
		{
			Name: "session-id", Value: sessionID, Path: "/", Domain: ".amazon.com", Expires: time.Now().Add(24 * time.Hour),
		},
	}
	return &Session{
		Cookies: cookies,
		Country: country,
	}
}

func TestAmazonSessionOperations(t *testing.T) {
	ctx := context.Background()
	cfg := &Config{
		Addr:     "127.0.0.1:6379",
		Password: "123456",
		Db:       10,
	}

	sessionManager, err := NewAmazonSession(cfg)
	if err != nil {
		t.Fatalf("无法连接到 Redis: %v", err)
	}

	err = sessionManager.ClearAllCookies(ctx)
	if err != nil {
		t.Fatalf("ClearAllCookies failed: %v", err)
	}

	country := "US"

	// Push a session
	session1 := createTestSession(country, "session1")
	if err := sessionManager.PushSession(ctx, session1); err != nil {
		t.Fatalf("PushSession failed: %v", err)
	}

	// Pop the session
	poppedSession, err := sessionManager.PopSession(ctx, country)
	if err != nil {
		t.Fatalf("PopSession failed: %v", err)
	}
	if poppedSession.SessionID != "session1" {
		t.Fatalf("Expected session1, got %v", poppedSession.SessionID)
	}

	// Push another session
	session2 := createTestSession(country, "session2")
	if err := sessionManager.PushSession(ctx, session2); err != nil {
		t.Fatalf("PushSession failed: %v", err)
	}

	// Push the first session again
	if err := sessionManager.PushSession(ctx, session1); err != nil {
		t.Fatalf("PushSession failed: %v", err)
	}

	// Get a random session
	randomSession, err := sessionManager.GetRandomSession(ctx, country)
	if err != nil {
		t.Fatalf("GetRandomSession failed: %v", err)
	}
	if randomSession.SessionID != "session1" && randomSession.SessionID != "session2" {
		t.Fatalf("Unexpected session ID: %v", randomSession.SessionID)
	}

	err = sessionManager.ClearAllCookies(ctx)
	if err != nil {
		t.Fatalf("ClearAllCookies failed: %v", err)
	}
}
