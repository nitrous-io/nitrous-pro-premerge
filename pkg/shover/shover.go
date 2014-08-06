package shover

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

type Client struct {
	publishURL   string
	broadcastURL string
	signer       MessageSigner
	httpclient   *http.Client
}

type MessageSigner interface {
	Generate(data ...interface{}) (string, error)
}

var (
	ErrInvalidPayload = errors.New("shover: invalid payload")
)

func (c *Client) Publish(userID int, event string, data map[string]interface{}) error {
	data["_t"] = time.Now().Unix()
	payload, err := c.signer.Generate(userID, event, data)
	if err != nil {
		return err
	}
	return c.post(c.publishURL, payload)
}

func (c *Client) Broadcast(event string, data map[string]interface{}) error {
	data["_t"] = time.Now().Unix()
	payload, err := c.signer.Generate(event, data)
	if err != nil {
		return err
	}
	return c.post(c.broadcastURL, payload)
}

func (c *Client) post(u, payload string) error {
	resp, err := c.httpclient.PostForm(u, url.Values{"p": {payload}})
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
		return nil
	case http.StatusBadRequest:
		return ErrInvalidPayload
	default:
		return fmt.Errorf("shover: unexpected response from Shover server: %v", resp.Status)
	}
}

func NewClient(baseURL string, signer MessageSigner, httpclient *http.Client) (*Client, error) {
	if signer == nil {
		return nil, errors.New("a MessageSigner is required")
	}
	if httpclient == nil {
		httpclient = http.DefaultClient
	}
	return &Client{
		publishURL:   fmt.Sprintf("%s/publish", baseURL),
		broadcastURL: fmt.Sprintf("%s/broadcast", baseURL),
		signer:       signer,
		httpclient:   httpclient,
	}, nil
}
