package anylist

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"time"

	pb "github.com/cosgroveb/al/internal/anylistpb"
	"google.golang.org/protobuf/proto"
)

const (
	serviceURL       = "https://www.anylist.com"
	requestTimeout   = 30 * time.Second
	maxResponseBytes = 64 << 20
)

type Options struct {
	Email      string
	Password   string
	ClientID   string
	BaseURL    string
	HTTPClient *http.Client
}

// Client keeps authentication and account data in memory. Use it sequentially.
type Client struct {
	email, password, clientID string
	baseURL                   string
	http                      *http.Client
	token, userID             string
	account                   *pb.PBUserDataResponse
	lists                     map[string]*pb.ShoppingList
	parents                   map[string]string
	groups                    map[string][]*pb.PBListCategoryGroup
	filters                   map[string][]*pb.PBStoreFilter
	settings                  map[string]*pb.PBListSettings
}

func NewClient(options Options) *Client {
	httpClient := http.Client{}
	if options.HTTPClient != nil {
		httpClient = *options.HTTPClient
	}
	// A redirect must not replay a write or send credentials to another endpoint.
	httpClient.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	baseURL := options.BaseURL
	if baseURL == "" {
		baseURL = serviceURL
	}
	return &Client{email: options.Email, password: options.Password, clientID: options.ClientID,
		baseURL: strings.TrimRight(baseURL, "/"), http: &httpClient}
}

// NewID returns AnyList's lowercase UUIDv4 representation without hyphens.
func NewID() string {
	var id [16]byte
	rand.Read(id[:])
	id[6] = (id[6] & 0x0f) | 0x40
	id[8] = (id[8] & 0x3f) | 0x80
	return hex.EncodeToString(id[:])
}

func derivedID(namespace, name string) string {
	// Namespaces below are fixed protocol constants, not user input.
	ns, _ := hex.DecodeString(namespace)
	h := sha1.New()
	h.Write(ns)
	h.Write([]byte(name))
	id := h.Sum(nil)[:16]
	id[6] = (id[6] & 0x0f) | 0x50
	id[8] = (id[8] & 0x3f) | 0x80
	return hex.EncodeToString(id)
}

// Authenticate obtains an in-memory token if this client has none.
func (c *Client) Authenticate(ctx context.Context) error {
	if c.token != "" {
		return nil
	}
	if c.email == "" || c.password == "" {
		return &Error{Code: "configuration", Message: "set ANYLIST_EMAIL and ANYLIST_PASSWORD"}
	}
	if c.clientID == "" {
		c.clientID = NewID()
	}
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	if err := w.WriteField("email", c.email); err != nil {
		return encodingError()
	}
	if err := w.WriteField("password", c.password); err != nil {
		return encodingError()
	}
	if err := w.Close(); err != nil {
		return encodingError()
	}
	data, err := c.request(ctx, "/auth/token", w.FormDataContentType(), &body, false)
	if err != nil {
		return err
	}
	var login struct {
		Token  string `json:"access_token"`
		UserID string `json:"user_id"`
	}
	if json.Unmarshal(data, &login) != nil || login.Token == "" || login.UserID == "" {
		return &Error{Code: "protocol", Message: "AnyList returned an invalid authentication response"}
	}
	c.token, c.userID = login.Token, login.UserID
	return nil
}

func (c *Client) request(ctx context.Context, path, contentType string, body io.Reader, mutation bool) ([]byte, error) {
	if ctx.Err() != nil {
		return nil, &Error{Code: "transport", Message: "AnyList request was canceled before sending"}
	}
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, body)
	if err != nil {
		return nil, &Error{Code: "configuration", Message: "invalid AnyList request configuration"}
	}
	req.Header.Set("X-AnyLeaf-API-Version", "3")
	req.Header.Set("X-AnyLeaf-Client-Identifier", c.clientID)
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		message := "AnyList request failed"
		if errors.Is(err, context.DeadlineExceeded) || ctx.Err() == context.DeadlineExceeded {
			message = "AnyList request timed out"
		} else if errors.Is(err, context.Canceled) {
			message = "AnyList request was canceled"
		}
		return nil, &Error{Code: "transport", Message: message, Unknown: mutation}
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		code, message := "operation_failed", "AnyList rejected the request"
		if resp.StatusCode < 400 || resp.StatusCode >= 500 {
			message = "AnyList returned an unexpected HTTP status"
		}
		switch resp.StatusCode {
		case http.StatusUnauthorized:
			code, message = "authentication", "AnyList rejected authentication"
		case http.StatusForbidden:
			code, message = "permission", "AnyList denied permission"
		}
		return nil, &Error{Code: code, Message: message, HTTPStatus: resp.StatusCode,
			Unknown: mutation && (resp.StatusCode < 400 || resp.StatusCode >= 500)}
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	if err != nil {
		return nil, &Error{Code: "transport", Message: "could not read AnyList response", Unknown: mutation}
	}
	if len(data) > maxResponseBytes {
		return nil, &Error{Code: "protocol", Message: "AnyList response exceeded the size limit", Unknown: mutation}
	}
	return data, nil
}

func (c *Client) send(ctx context.Context, path, part string, message proto.Message) ([]byte, error) {
	data, err := proto.Marshal(message)
	if err != nil {
		return nil, encodingError()
	}
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	field, err := w.CreateFormField(part)
	if err != nil {
		return nil, encodingError()
	}
	if _, err = field.Write(data); err != nil {
		return nil, encodingError()
	}
	if err = w.Close(); err != nil {
		return nil, encodingError()
	}
	return c.request(ctx, path, w.FormDataContentType(), &body, true)
}

func (c *Client) metadata(handler string) *pb.PBOperationMetadata {
	return &pb.PBOperationMetadata{OperationId: proto.String(NewID()), HandlerId: proto.String(handler), UserId: proto.String(c.userID)}
}

func (c *Client) updateList(ctx context.Context, op *pb.PBListOperation) error {
	return c.sendUpdate(ctx, "/data/shopping-lists/update", op.GetMetadata().GetOperationId(), &pb.PBListOperationList{Operations: []*pb.PBListOperation{op}})
}

func (c *Client) sendUpdate(ctx context.Context, path, operationID string, message proto.Message) error {
	if operationID == "" {
		return encodingError()
	}
	data, err := c.send(ctx, path, "operations", message)
	if err != nil {
		return err
	}
	var response pb.PBEditOperationResponse
	if len(data) == 0 || proto.Unmarshal(data, &response) != nil {
		return &Error{Code: "protocol", Message: "AnyList returned an invalid operation acknowledgment", Unknown: true}
	}
	for _, processedID := range response.ProcessedOperations {
		if processedID == operationID {
			return nil
		}
	}
	return &Error{Code: "protocol", Message: "AnyList did not acknowledge the submitted operation", Unknown: true}
}

func encodingError() *Error {
	return &Error{Code: "internal", Message: "could not encode AnyList request"}
}

func failedMutation(result MutationResult, err error, priorWrite bool) (MutationResult, error) {
	var safe *Error
	if !errors.As(err, &safe) {
		safe = &Error{Code: "operation_failed", Message: "AnyList operation failed"}
	}
	copy := *safe
	copy.Unknown = copy.Unknown || priorWrite
	result.Outcome = "failed"
	if copy.Unknown {
		result.Outcome = "unknown"
	}
	return result, &copy
}
