package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type UserService struct {
	baseURL string
	client  *http.Client
}

// GetUser makes an HTTP call WITH context timeout - GOOD!
func (s *UserService) GetUser(ctx context.Context, id string) (*User, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", s.baseURL+"/users/"+id, nil)
	if err != nil {
		return nil, err
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var user User
	if err := json.NewDecoder(resp.Body).Decode(&user); err != nil {
		return nil, err
	}
	return &user, nil
}

// CreateUser makes an HTTP POST WITH context timeout - GOOD!
func (s *UserService) CreateUser(ctx context.Context, user *User) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "POST", s.baseURL+"/users", nil)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}
	return nil
}

// FetchAll uses context - GOOD!
func FetchAll(ctx context.Context, url string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// ... read body
	return nil, nil
}

// DialBackend uses context with timeout - GOOD!
func DialBackend(ctx context.Context, addr string) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	var d net.Dialer
	conn, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return err
	}
	defer conn.Close()
	return nil
}

type User struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}
