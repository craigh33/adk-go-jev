package typesafe

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"
)

//nolint:gocognit // Each table case verifies the complete observable result.
func TestRetryResponses(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name       string
		statuses   []int
		retry      *RetryPolicy
		wantCalls  int
		wantStatus int
	}{
		{"disabled", []int{429}, nil, 1, 429},
		{"success after overload", []int{429, 529, 200}, &RetryPolicy{MaxAttempts: 3, InitialBackoff: time.Nanosecond}, 3, 0},
		{"bounded", []int{529, 529, 529}, &RetryPolicy{MaxAttempts: 2, InitialBackoff: time.Nanosecond}, 2, 529},
		{"bad request", []int{400}, &RetryPolicy{}, 1, 400},
		{"server failure", []int{500}, &RetryPolicy{}, 1, 500},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var bodies []string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, _ := io.ReadAll(r.Body)
				bodies = append(bodies, string(body))
				if r.Header.Get("Authorization") != "Bearer test-key" {
					t.Error("retry lost authentication")
				}
				status := tc.statuses[min(len(bodies)-1, len(tc.statuses)-1)]
				w.WriteHeader(status)
				if status == http.StatusOK {
					fmt.Fprint(w, mixedResponse)
				}
			}))
			defer server.Close()
			client, err := New(&Options{APIKey: "test-key", BaseURL: server.URL, Retry: tc.retry})
			if err != nil {
				t.Fatal(err)
			}
			_, err = client.Evaluate(t.Context(), mixedRequest())
			var apiErr *APIError
			if tc.wantStatus == 0 && err != nil {
				t.Fatal(err)
			}
			if tc.wantStatus != 0 && (!errors.As(err, &apiErr) || apiErr.StatusCode != tc.wantStatus) {
				t.Fatalf("error = %v", err)
			}
			if len(bodies) != tc.wantCalls {
				t.Fatalf("calls = %d", len(bodies))
			}
			for _, body := range bodies {
				if body != bodies[0] {
					t.Fatal("retry changed request body")
				}
			}
		})
	}
}

func TestRetryCancellation(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	calls := 0
	client, err := New(
		&Options{
			APIKey: "test-key",
			Retry:  &RetryPolicy{},
			HTTPClient: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
				calls++
				return &http.Response{
					StatusCode: http.StatusTooManyRequests,
					Header:     http.Header{"Retry-After": []string{"2"}},
					Body:       &cancelOnClose{Reader: strings.NewReader("overloaded"), cancel: cancel},
				}, nil
			})},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Evaluate(ctx, mixedRequest())
	if !errors.Is(err, context.Canceled) || calls != 1 {
		t.Fatalf("calls = %d, error = %v", calls, err)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) { return f(request) }

type cancelOnClose struct {
	io.Reader

	cancel context.CancelFunc
}

func (r *cancelOnClose) Close() error { r.cancel(); return nil }

func TestRetryDoesNotReplayTransportFailures(t *testing.T) {
	t.Parallel()
	failure := errors.New("connection reset")
	calls := 0
	client, err := New(
		&Options{
			APIKey: "test-key",
			Retry:  &RetryPolicy{},
			HTTPClient: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
				calls++
				return nil, failure
			})},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.Evaluate(t.Context(), mixedRequest()); !errors.Is(err, failure) || calls != 1 {
		t.Fatalf("calls=%d err=%v", calls, err)
	}
}

func TestRetryAfterExceedsBound(t *testing.T) {
	t.Parallel()
	calls := 0
	client := testClient(t, func(w http.ResponseWriter, _ *http.Request) {
		calls++
		w.Header().Set("Retry-After", "60")
		w.WriteHeader(http.StatusTooManyRequests)
	})
	client.retry, _ = retryPolicy(&RetryPolicy{})
	_, err := client.Evaluate(t.Context(), mixedRequest())
	var apiErr *APIError
	if calls != 1 || !errors.As(err, &apiErr) || apiErr.RetryAfter != "60" {
		t.Fatalf("calls = %d, error = %v", calls, err)
	}
}

func TestRetryDelays(t *testing.T) {
	t.Parallel()
	p := RetryPolicy{InitialBackoff: time.Second, MaxBackoff: 4 * time.Second}
	for attempt, limit := range []time.Duration{time.Second, 2 * time.Second, 4 * time.Second, 4 * time.Second} {
		for range 20 {
			delay, ok := p.delay(attempt+1, "")
			if !ok || delay < limit/2 || delay > limit {
				t.Fatalf("attempt %d delay = %v", attempt+1, delay)
			}
		}
	}
	if delay, ok := p.delay(1, "3"); !ok || delay != 3*time.Second {
		t.Fatalf("Retry-After not respected: %v, %v", delay, ok)
	}
	for _, value := range []string{"5", "9999999999999999999999999"} {
		if _, ok := p.delay(1, value); ok {
			t.Fatalf("accepted excessive Retry-After %q", value)
		}
	}
	date := time.Now().Add(3 * time.Second).UTC().Format(http.TimeFormat)
	if delay, ok := retryAfterDelay(date); !ok || delay < time.Second || delay > 3*time.Second {
		t.Fatalf("HTTP date = %v, %v", delay, ok)
	}
	for _, value := range []string{"-1", "bad"} {
		if _, ok := retryAfterDelay(value); ok {
			t.Fatalf("accepted %q", value)
		}
	}
}

func TestRetryPolicyValidation(t *testing.T) {
	t.Parallel()
	for _, p := range []RetryPolicy{{MaxAttempts: -1}, {InitialBackoff: -1}, {MaxBackoff: -1}, {InitialBackoff: time.Second, MaxBackoff: time.Millisecond}} {
		if _, err := New(&Options{APIKey: "test-key", Retry: &p}); err == nil {
			t.Errorf("accepted %#v", p)
		}
	}
	p := &RetryPolicy{}
	client, err := New(&Options{APIKey: "test-key", Retry: p})
	if err != nil {
		t.Fatal(err)
	}
	want := RetryPolicy{MaxAttempts: 3, InitialBackoff: 250 * time.Millisecond, MaxBackoff: 5 * time.Second}
	if !reflect.DeepEqual(client.retry, want) {
		t.Fatalf("defaults = %#v", client.retry)
	}
	p.MaxAttempts = 99
	if client.retry.MaxAttempts != want.MaxAttempts {
		t.Fatal("policy was not copied")
	}
}
