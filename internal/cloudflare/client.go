package cloudflare

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	cloudflarego "github.com/cloudflare/cloudflare-go/v2"
	"github.com/cloudflare/cloudflare-go/v2/option"
	"github.com/cloudflare/cloudflare-go/v2/zones"
	"github.com/sparklyi/tunnelbox/internal/provision"
)

type Config struct {
	Token      string
	AccountID  string
	ZoneID     string
	BaseURL    string
	HTTPClient *http.Client
}

type Client struct {
	api       *cloudflarego.Client
	accountID string
	zoneID    string
}

type Error struct {
	Code      string
	Temporary bool
	Cause     error
}

func (e *Error) Error() string {
	if e == nil || e.Code == "" {
		return "cloudflare request failed"
	}
	return e.Code
}

func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

func (e *Error) FailureCode() string {
	if e == nil {
		return ""
	}
	return e.Code
}

func (e *Error) RemoteStateUnknown() bool {
	return e != nil && e.Temporary
}

func New(cfg Config) (*Client, error) {
	if strings.TrimSpace(cfg.Token) == "" {
		return nil, errors.New("cloudflare token is required")
	}
	if strings.TrimSpace(cfg.AccountID) == "" {
		return nil, errors.New("cloudflare account id is required")
	}
	options := []option.RequestOption{
		option.WithAPIToken(strings.TrimSpace(cfg.Token)),
		option.WithMaxRetries(0),
	}
	if cfg.HTTPClient != nil {
		options = append(options, option.WithHTTPClient(cfg.HTTPClient))
	}
	if cfg.BaseURL != "" {
		baseURL := cfg.BaseURL
		if !strings.HasSuffix(baseURL, "/") {
			baseURL += "/"
		}
		options = append(options, option.WithBaseURL(baseURL))
	}
	return &Client{api: cloudflarego.NewClient(options...), accountID: cfg.AccountID, zoneID: cfg.ZoneID}, nil
}

type TokenStatus struct {
	ID     string
	Status string
}

func (c *Client) VerifyToken(ctx context.Context) (TokenStatus, error) {
	result, err := c.api.User.Tokens.Verify(ctx)
	if err != nil {
		return TokenStatus{}, normalizeError(err)
	}
	return TokenStatus{ID: result.ID, Status: string(result.Status)}, nil
}

type Zone = provision.Zone

func (c *Client) Zones(ctx context.Context) ([]Zone, error) {
	params := zones.ZoneListParams{PerPage: cloudflarego.F(float64(100))}
	if c.accountID != "" {
		params.Account = cloudflarego.F(zones.ZoneListParamsAccount{ID: cloudflarego.F(c.accountID)})
	}
	page, err := c.api.Zones.List(ctx, params)
	if err != nil {
		return nil, normalizeError(err)
	}
	result := make([]Zone, 0, len(page.Result))
	for _, item := range page.Result {
		result = append(result, Zone{ID: item.ID, Name: item.Name})
	}
	return result, nil
}

func (c *Client) EnsureTunnel(ctx context.Context, spec provision.TunnelSpec) (provision.RemoteTunnel, error) {
	if spec.ID == "" && strings.TrimSpace(spec.Name) == "" {
		return provision.RemoteTunnel{}, errors.New("tunnel id or name is required")
	}
	var result tunnelResult
	if spec.ID != "" {
		if err := c.call(ctx, http.MethodGet, c.accountPath("cfd_tunnel", spec.ID), nil, &result); err != nil {
			return provision.RemoteTunnel{}, err
		}
	} else {
		var existing []tunnelResult
		path := c.accountPath("cfd_tunnel") + "?name=" + url.QueryEscape(spec.Name) + "&is_deleted=false&per_page=100"
		if err := c.call(ctx, http.MethodGet, path, nil, &existing); err != nil {
			return provision.RemoteTunnel{}, err
		}
		switch len(existing) {
		case 1:
			result = existing[0]
		case 0:
			secret, err := tunnelSecret()
			if err != nil {
				return provision.RemoteTunnel{}, err
			}
			if err := c.call(ctx, http.MethodPost, c.accountPath("cfd_tunnel"), map[string]string{
				"name": spec.Name, "tunnel_secret": secret, "config_src": "cloudflare",
			}, &result); err != nil {
				return provision.RemoteTunnel{}, err
			}
		default:
			return provision.RemoteTunnel{}, &Error{Code: "tunnel_name_conflict"}
		}
	}
	if result.ID == "" {
		return provision.RemoteTunnel{}, &Error{Code: "cloudflare_invalid_tunnel_response"}
	}
	var tokenResult json.RawMessage
	if err := c.call(ctx, http.MethodGet, c.accountPath("cfd_tunnel", result.ID, "token"), nil, &tokenResult); err != nil {
		return provision.RemoteTunnel{}, err
	}
	token, err := decodeToken(tokenResult)
	if err != nil {
		return provision.RemoteTunnel{}, err
	}
	return provision.RemoteTunnel{ID: result.ID, Name: firstNonEmpty(result.Name, spec.Name), ConnectorToken: token}, nil
}

func (c *Client) ApplyWebRoute(ctx context.Context, spec provision.RouteSpec) error {
	if spec.TunnelID == "" || spec.Hostname == "" || spec.OriginURL == "" {
		return errors.New("tunnel id, hostname and origin url are required")
	}
	body := map[string]any{"config": map[string]any{"ingress": []any{
		map[string]string{"hostname": spec.Hostname, "service": spec.OriginURL},
		map[string]string{"service": "http_status:404"},
	}}}
	return c.call(ctx, http.MethodPut, c.accountPath("cfd_tunnel", spec.TunnelID, "configurations"), body, nil)
}

func (c *Client) EnsureApplication(ctx context.Context, spec provision.AccessApplicationSpec) (provision.RemoteRef, error) {
	if spec.Name == "" || spec.Domain == "" {
		return provision.RemoteRef{}, errors.New("application name and domain are required")
	}
	body := map[string]any{
		"name": spec.Name, "domain": spec.Domain, "type": "self_hosted",
		"destinations":               []map[string]string{{"type": "public", "uri": spec.Domain}},
		"session_duration":           "24h",
		"enable_binding_cookie":      true,
		"http_only_cookie_attribute": true,
		"app_launcher_visible":       false,
	}
	if spec.PolicyID != "" {
		body["policies"] = []map[string]any{{"id": spec.PolicyID, "precedence": 1}}
	}
	path := c.accountPath("access", "apps")
	method := http.MethodPost
	if spec.ID != "" {
		method = http.MethodPut
		path = c.accountPath("access", "apps", spec.ID)
	} else {
		var existing []accessApplicationResult
		lookupPath := path + "?domain=" + url.QueryEscape(spec.Domain) + "&exact=true&per_page=100"
		if err := c.call(ctx, http.MethodGet, lookupPath, nil, &existing); err != nil {
			return provision.RemoteRef{}, err
		}
		matches := make([]accessApplicationResult, 0, len(existing))
		for _, item := range existing {
			if strings.EqualFold(item.Domain, spec.Domain) && item.Name == spec.Name {
				matches = append(matches, item)
			}
		}
		switch len(matches) {
		case 1:
			spec.ID = matches[0].ID
			method = http.MethodPut
			path = c.accountPath("access", "apps", spec.ID)
		case 0:
			if len(existing) > 0 {
				return provision.RemoteRef{}, &Error{Code: "access_application_conflict"}
			}
		default:
			return provision.RemoteRef{}, &Error{Code: "access_application_conflict"}
		}
	}
	var result accessApplicationResult
	if err := c.call(ctx, method, path, body, &result); err != nil {
		return provision.RemoteRef{}, err
	}
	if result.ID == "" {
		result.ID = spec.ID
	}
	if result.ID == "" {
		return provision.RemoteRef{}, &Error{Code: "cloudflare_invalid_application_response"}
	}
	return provision.RemoteRef{ID: result.ID}, nil
}

func (c *Client) EnsurePolicy(ctx context.Context, spec provision.AccessPolicySpec) (provision.RemoteRef, error) {
	if spec.ApplicationID == "" || spec.Name == "" || spec.AllowValue == "" {
		return provision.RemoteRef{}, errors.New("application id, policy name and allow value are required")
	}
	var include map[string]any
	switch spec.AllowType {
	case "email":
		include = map[string]any{"email": map[string]string{"email": spec.AllowValue}}
	case "email_domain":
		include = map[string]any{"email_domain": map[string]string{"domain": spec.AllowValue}}
	default:
		return provision.RemoteRef{}, errors.New("unsupported policy allow type")
	}
	body := map[string]any{"name": spec.Name, "decision": "allow", "include": []any{include}, "session_duration": "24h"}
	path := c.accountPath("access", "apps", spec.ApplicationID, "policies")
	method := http.MethodPost
	if spec.ID != "" {
		method = http.MethodPut
		path = c.accountPath("access", "apps", spec.ApplicationID, "policies", spec.ID)
	} else {
		var existing []accessPolicyResult
		if err := c.call(ctx, http.MethodGet, path+"?per_page=1000", nil, &existing); err != nil {
			return provision.RemoteRef{}, err
		}
		matches := make([]accessPolicyResult, 0, len(existing))
		for _, item := range existing {
			if item.Name == spec.Name {
				matches = append(matches, item)
			}
		}
		switch len(matches) {
		case 1:
			spec.ID = matches[0].ID
			method = http.MethodPut
			path = c.accountPath("access", "apps", spec.ApplicationID, "policies", spec.ID)
		case 0:
		default:
			return provision.RemoteRef{}, &Error{Code: "access_policy_conflict"}
		}
	}
	var result accessPolicyResult
	if err := c.call(ctx, method, path, body, &result); err != nil {
		return provision.RemoteRef{}, err
	}
	if result.ID == "" {
		result.ID = spec.ID
	}
	if result.ID == "" {
		return provision.RemoteRef{}, &Error{Code: "cloudflare_invalid_policy_response"}
	}
	return provision.RemoteRef{ID: result.ID}, nil
}

func (c *Client) EnsureCNAME(ctx context.Context, spec provision.CNAMESpec) (provision.RemoteRef, error) {
	if c.zoneID == "" {
		return provision.RemoteRef{}, errors.New("cloudflare zone id is required")
	}
	if spec.Name == "" || spec.Target == "" {
		return provision.RemoteRef{}, errors.New("cname name and target are required")
	}
	body := map[string]any{"type": "CNAME", "name": spec.Name, "content": spec.Target, "proxied": spec.Proxied, "ttl": 1}
	if spec.ID != "" {
		var result dnsRecordResult
		if err := c.call(ctx, http.MethodPut, c.zonePath("dns_records", spec.ID), body, &result); err != nil {
			return provision.RemoteRef{}, err
		}
		return provision.RemoteRef{ID: firstNonEmpty(result.ID, spec.ID)}, nil
	}
	var records []dnsRecordResult
	if err := c.call(ctx, http.MethodGet, c.zonePath("dns_records")+"?type=CNAME&name="+url.QueryEscape(spec.Name), nil, &records); err != nil {
		return provision.RemoteRef{}, err
	}
	if len(records) > 0 {
		if len(records) == 1 && strings.EqualFold(records[0].Content, spec.Target) {
			return provision.RemoteRef{ID: records[0].ID}, nil
		}
		return provision.RemoteRef{}, &Error{Code: "dns_record_conflict"}
	}
	var result dnsRecordResult
	if err := c.call(ctx, http.MethodPost, c.zonePath("dns_records"), body, &result); err != nil {
		return provision.RemoteRef{}, err
	}
	if result.ID == "" {
		return provision.RemoteRef{}, &Error{Code: "cloudflare_invalid_dns_response"}
	}
	return provision.RemoteRef{ID: result.ID}, nil
}

type apiEnvelope struct {
	Success bool            `json:"success"`
	Errors  []apiMessage    `json:"errors"`
	Result  json.RawMessage `json:"result"`
}

type apiMessage struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type tunnelResult struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type accessApplicationResult struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Domain string `json:"domain"`
}

type accessPolicyResult struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type dnsRecordResult struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Content string `json:"content"`
	Type    string `json:"type"`
}

func (c *Client) call(ctx context.Context, method, path string, body any, result any) error {
	var envelope apiEnvelope
	if err := c.api.Execute(ctx, method, path, body, &envelope); err != nil {
		return normalizeError(err)
	}
	if !envelope.Success || len(envelope.Errors) > 0 {
		code := "cloudflare_api_error"
		if len(envelope.Errors) > 0 && envelope.Errors[0].Code != 0 {
			code = fmt.Sprintf("cloudflare_%d", envelope.Errors[0].Code)
		}
		return &Error{Code: code}
	}
	if result == nil || len(envelope.Result) == 0 || string(envelope.Result) == "null" {
		return nil
	}
	if err := json.Unmarshal(envelope.Result, result); err != nil {
		return &Error{Code: "cloudflare_invalid_response", Cause: err}
	}
	return nil
}

func normalizeError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.Canceled) {
		return err
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return &Error{Code: "cloudflare_timeout", Temporary: true, Cause: err}
	}
	var apiErr *cloudflarego.Error
	if errors.As(err, &apiErr) {
		return &Error{Code: "cloudflare_http_error", Temporary: apiErr.StatusCode >= 500 || apiErr.StatusCode == http.StatusTooManyRequests, Cause: err}
	}
	return &Error{Code: "cloudflare_unavailable", Temporary: true, Cause: err}
}

func (c *Client) accountPath(parts ...string) string {
	values := append([]string{"accounts", c.accountID}, parts...)
	return escapedPath(values...)
}

func (c *Client) zonePath(parts ...string) string {
	values := append([]string{"zones", c.zoneID}, parts...)
	return escapedPath(values...)
}

func escapedPath(parts ...string) string {
	encoded := make([]string, 0, len(parts))
	for _, part := range parts {
		encoded = append(encoded, url.PathEscape(part))
	}
	return strings.Join(encoded, "/")
}

func tunnelSecret() (string, error) {
	var value [32]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", fmt.Errorf("generate tunnel secret: %w", err)
	}
	return base64.StdEncoding.EncodeToString(value[:]), nil
}

func decodeToken(raw json.RawMessage) (string, error) {
	var token string
	if json.Unmarshal(raw, &token) == nil && strings.TrimSpace(token) != "" {
		return strings.TrimSpace(token), nil
	}
	var object struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(raw, &object); err == nil && strings.TrimSpace(object.Token) != "" {
		return strings.TrimSpace(object.Token), nil
	}
	return "", &Error{Code: "cloudflare_invalid_tunnel_token"}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

var _ provision.TunnelPort = (*Client)(nil)
var _ provision.AccessPort = (*Client)(nil)
var _ provision.DNSPort = (*Client)(nil)
