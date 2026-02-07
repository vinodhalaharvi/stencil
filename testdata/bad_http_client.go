package client

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
)

type UserService struct {
	baseURL string
	client  *http.Client
}

// GetUser makes an HTTP call with NO context timeout - BAD!
func (s *UserService) GetUser(id string) (*User, error) {
	resp, err := s.client.Get(s.baseURL + "/users/" + id)
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

// CreateUser makes an HTTP POST with NO context timeout - BAD!
func (s *UserService) CreateUser(user *User) error {
	resp, err := s.client.Post(s.baseURL+"/users", "application/json", nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}
	return nil
}

// FetchAll uses http.Get directly - also BAD!
func FetchAll(url string) ([]byte, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// ... read body
	return nil, nil
}

// DialBackend makes a raw network call with NO timeout - BAD!
func DialBackend(addr string) error {
	conn, err := net.Dial("tcp", addr)
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
