package monobank

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/Goodmoder-ux/collect-bot/internal/domain"
)

const (
	defaultBaseURL = "https://api.monobank.ua"
	requestTimeout = 30 * time.Second
	// statementPageSize is the maximum number of items Monobank returns per statement request.
	statementPageSize = 500
)

var ErrRateLimited = errors.New("monobank: rate limited")

type RateLimitError struct {
	RetryAfter time.Duration
}

func (e *RateLimitError) Error() string {
	if e.RetryAfter > 0 {
		return fmt.Sprintf("%v (retry after %s)", ErrRateLimited, e.RetryAfter)
	}
	return ErrRateLimited.Error()
}

func (e *RateLimitError) Unwrap() error { return ErrRateLimited }

// RetryAfterDelay returns the delay requested by Monobank, if any.
func (e *RateLimitError) RetryAfterDelay() time.Duration { return e.RetryAfter }

type Client struct {
	token      string
	httpClient *http.Client
	baseURL    string
}

func NewClient(token string) *Client {
	return &Client{token: token, httpClient: &http.Client{Timeout: requestTimeout}, baseURL: defaultBaseURL}
}

func NewClientWithHTTPClient(token string, httpClient *http.Client, baseURL string) *Client {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &Client{token: token, httpClient: httpClient, baseURL: baseURL}
}

var _ domain.TransactionSource = (*Client)(nil)

type statementItem struct {
	ID          string `json:"id"`
	Time        int64  `json:"time"`
	Amount      int64  `json:"amount"`
	Description string `json:"description"`
	Comment     string `json:"comment"`
}

func (c *Client) GetTransactions(ctx context.Context, accountID string, from, to time.Time) ([]domain.Transaction, error) {
	url := fmt.Sprintf("%s/personal/statement/%s/%s/%s", c.baseURL, accountID,
		strconv.FormatInt(from.Unix(), 10), strconv.FormatInt(to.Unix(), 10))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("monobank: create statement request: %w", err)
	}
	req.Header.Set("X-Token", c.token)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("monobank: request statement: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusTooManyRequests {
		retryAfter := parseRetryAfter(resp.Header.Get("Retry-After"))
		return nil, &RateLimitError{RetryAfter: retryAfter}
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("monobank: statement request returned HTTP %s", resp.Status)
	}
	var items []statementItem
	if err := json.NewDecoder(resp.Body).Decode(&items); err != nil {
		return nil, fmt.Errorf("monobank: decode statement response: %w", err)
	}
	if len(items) >= statementPageSize {
		slog.Warn("monobank statement hit the page limit; older transactions in the range may be missing",
			"items", len(items), "from", from, "to", to)
	}
	transactions := make([]domain.Transaction, 0, len(items))
	for _, item := range items {
		if item.Amount <= 0 {
			continue
		}
		comment := item.Comment
		if comment == "" {
			comment = item.Description
		}
		transactions = append(transactions, domain.Transaction{
			ID: item.ID, Amount: item.Amount, Comment: comment,
			OccurredAt: time.Unix(item.Time, 0).UTC(),
		})
	}
	return transactions, nil
}

func parseRetryAfter(value string) time.Duration {
	if seconds, err := strconv.Atoi(value); err == nil && seconds > 0 {
		return time.Duration(seconds) * time.Second
	}
	return 0
}
