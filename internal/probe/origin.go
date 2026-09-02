package probe

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

var ErrUnreachable = errors.New("origin cannot be reached")

type Origin struct {
	Client *http.Client
}

func NewOrigin(client *http.Client) *Origin {
	if client == nil {
		client = &http.Client{Timeout: 8 * time.Second}
	}
	return &Origin{Client: client}
}

func (p *Origin) Check(ctx context.Context, rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil || u.Hostname() == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return fmt.Errorf("%w: invalid origin URL", ErrUnreachable)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return fmt.Errorf("%w: build request", ErrUnreachable)
	}
	request.Header.Set("Range", "bytes=0-0")
	response, err := p.Client.Do(request)
	if err != nil {
		return fmt.Errorf("%w: origin request failed", ErrUnreachable)
	}
	defer response.Body.Close()
	_, _ = io.CopyN(io.Discard, response.Body, 1<<20)
	if response.StatusCode >= http.StatusInternalServerError {
		return fmt.Errorf("%w: origin returned %s", ErrUnreachable, strings.ToLower(http.StatusText(response.StatusCode)))
	}
	return nil
}
