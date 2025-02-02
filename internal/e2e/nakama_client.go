package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
)

const (
	envNakamaAddress = "NAKAMA_ADDRESS"
)

type nakamaClient struct {
	addr       string
	authHeader string
}

func newClient() *nakamaClient {
	host := os.Getenv(envNakamaAddress)
	if host == "" {
		host = "http://127.0.0.1:7350"
	}
	h := &nakamaClient{
		addr: host,
	}
	return h
}

func (c *nakamaClient) registerDevice(username, deviceID string) error {
	path := "v2/account/authenticate/device"
	options := fmt.Sprintf("create=true&username=%s", username)
	url := fmt.Sprintf("%s/%s?%s", c.addr, path, options)
	body := map[string]any{
		"id": deviceID,
	}
	buf, err := json.Marshal(body)
	if err != nil {
		return err
	}
	reader := bytes.NewReader(buf)
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, url, reader)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	// defaultkey is the default server key. See https://heroiclabs.com/docs/nakama/concepts/authentication/ for more
	// details.
	req.SetBasicAuth("defaultkey", "")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		buf, err = io.ReadAll(resp.Body)
		return fmt.Errorf("request failed with status code %d. body is:\n%v\nerror:%w", resp.StatusCode, string(buf), err)
	}

	if err = json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return fmt.Errorf("failed to decode body: %w", err)
	}
	c.authHeader = fmt.Sprintf("Bearer %s", body["token"].(string))
	return nil
}

func (c *nakamaClient) rpc(path string, body any) (*http.Response, error) {
	buf, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	url := fmt.Sprintf("%s/v2/rpc/%s?unwrap", c.addr, path)
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, url, bytes.NewReader(buf))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", c.authHeader)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	return resp, nil
}
