package typesafe

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

const mixedResponse = `{"model":"jev-test","answers":{
"route":{"type":"choice","choice":"billing","confidence":0.8,"probabilities":{"billing":0.9,"support":0.1}},
"priority":{"type":"score","score":0.0,"confidence":1.0,"legend":{"0":"low","1":{"severity":"high"}},"probabilities":{"0":1.0,"1":0.0}},
"urgent":{"type":"noul","noul":0.0}},"usage":{"input_tokens":123,"output_tokens":0}}`

const mixedRequestJSON = `{"model":"jev-latest",
"state":{"message":"Please refund the duplicate charge","history":["hello",null]},
"questions":{
"route":{"type":"choice","instructions":{"task":"Choose a team"},"criteria":{"billing":null,"support":["technical"]}},
"priority":{"type":"score","instructions":"Rate urgency","criteria":["low",{"severity":"high"}]},
"urgent":{"type":"noul","instructions":"Is it urgent?","criteria":{"true":"Time sensitive","false":"Can wait"}}}}`

func mixedRequest() *Request {
	return &Request{
		State: map[string]any{"message": "Please refund the duplicate charge", "history": []any{"hello", nil}},
		Questions: map[string]Question{
			"route": Choice{
				Instructions: map[string]any{"task": "Choose a team"},
				Criteria:     map[string]any{"billing": nil, "support": []any{"technical"}},
			},
			"priority": &Score{
				Instructions: "Rate urgency",
				Criteria:     []any{"low", map[string]any{"severity": "high"}},
			},
			"urgent": Noul{
				Instructions: "Is it urgent?",
				Criteria:     &NoulCriteria{True: "Time sensitive", False: "Can wait"},
			},
		},
	}
}

func testClient(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	client, err := New(&Options{APIKey: "test-key", BaseURL: server.URL, HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	return client
}

func TestEvaluateMixedRequest(t *testing.T) {
	t.Parallel()
	request := mixedRequest()
	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/systemone" {
			t.Errorf("unexpected endpoint: %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-key" || r.Header.Get("Content-Type") != "application/json" {
			t.Error("missing authentication or content type")
		}
		var body, want map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if err := json.Unmarshal([]byte(mixedRequestJSON), &want); err != nil {
			t.Error(err)
		}
		if !reflect.DeepEqual(body, want) {
			t.Errorf("request = %#v, want %#v", body, want)
		}
		fmt.Fprint(w, mixedResponse)
	})
	response, err := client.Evaluate(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
	choice, ok := response.Answers["route"].(ChoiceAnswer)
	if !ok || choice.Choice != "billing" || choice.Confidence != 0.8 || choice.Probabilities["support"] != 0.1 {
		t.Fatalf("choice lost data: %#v", response.Answers["route"])
	}
	score, ok := response.Answers["priority"].(ScoreAnswer)
	if !ok || score.Score != 0 || !reflect.DeepEqual(score.Legend["1"], map[string]any{"severity": "high"}) {
		t.Fatalf("score lost data: %#v", response.Answers["priority"])
	}
	noul, ok := response.Answers["urgent"].(NoulAnswer)
	if !ok || noul.Noul != 0 || response.Usage.OutputTokens != 0 || response.Model != "jev-test" {
		t.Fatalf("response lost zero values or metadata: %#v", response)
	}
	if request.Model != "" {
		t.Fatal("Evaluate mutated the request")
	}
}

func TestModelOverrides(t *testing.T) {
	t.Parallel()
	for _, override := range []string{"", "jev-pinned"} {
		t.Run(override, func(t *testing.T) {
			t.Parallel()
			client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
				var body struct {
					Model string `json:"model"`
				}
				json.NewDecoder(r.Body).Decode(&body)
				want := "jev-client"
				if override != "" {
					want = override
				}
				if body.Model != want {
					t.Errorf("model = %q, want %q", body.Model, want)
				}
				fmt.Fprint(w, mixedResponse)
			})
			client.model = "jev-client"
			request := mixedRequest()
			request.Model = override
			if _, err := client.Evaluate(t.Context(), request); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestEvaluateConcurrent(t *testing.T) {
	t.Parallel()
	request := mixedRequest()
	client := testClient(t, func(w http.ResponseWriter, _ *http.Request) { fmt.Fprint(w, mixedResponse) })
	for i := range 8 {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			t.Parallel()
			response, err := client.Evaluate(t.Context(), request)
			if err != nil {
				t.Fatal(err)
			}
			if len(response.Answers) != len(request.Questions) || request.Model != "" {
				t.Fatal("concurrent evaluation lost answers or changed the shared request")
			}
		})
	}
}

func TestEvaluateInvalidRequests(t *testing.T) {
	t.Parallel()
	var nilChoice *Choice
	many := make(map[string]any)
	for i := range 256 {
		many[strconv.Itoa(i)] = nil
	}
	tests := map[string]*Request{
		"nil":           nil,
		"nil state":     {Questions: map[string]Question{"a": Noul{}}},
		"numeric state": {State: 5, Questions: map[string]Question{"a": Noul{}}},
		"no questions":  {State: "text"},
		"nil question":  {State: "text", Questions: map[string]Question{"a": nil}},
		"typed nil":     {State: "text", Questions: map[string]Question{"a": nilChoice}},
		"empty choice":  {State: "text", Questions: map[string]Question{"a": Choice{}}},
		"large choice":  {State: "text", Questions: map[string]Question{"a": Choice{Criteria: many}}},
		"empty score":   {State: "text", Questions: map[string]Question{"a": Score{}}},
		"null level": {
			State:     "text",
			Questions: map[string]Question{"a": Score{Criteria: []any{"low", nil}}},
		},
		"numeric instructions": {State: "text", Questions: map[string]Question{"a": Noul{Instructions: 5}}},
		"unencodable state":    {State: make(chan int), Questions: map[string]Question{"a": Noul{}}},
	}
	var calls atomic.Int32
	client := testClient(t, func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	})
	t.Cleanup(func() {
		if calls.Load() != 0 {
			t.Error("invalid requests reached the server")
		}
	})
	for name, request := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if _, err := client.Evaluate(t.Context(), request); err == nil {
				t.Error("expected validation error")
			}
		})
	}
}

func TestEvaluateRejectsMalformedResponses(t *testing.T) {
	t.Parallel()
	tests := map[string]string{
		"invalid JSON": `{`,
		"missing answer": strings.Replace(
			mixedResponse,
			`"urgent":{"type":"noul","noul":0.0}`,
			`"other":{"type":"noul","noul":0.0}`,
			1,
		),
		"missing probability": strings.Replace(mixedResponse, `"noul":0.0`, `"ignored":0.0`, 1),
		"null probability":    strings.Replace(mixedResponse, `"noul":0.0`, `"noul":null`, 1),
		"out of range":        strings.Replace(mixedResponse, `"noul":0.0`, `"noul":1.1`, 1),
		"wrong type": strings.Replace(
			mixedResponse,
			`"urgent":{"type":"noul","noul":0.0}`,
			`"urgent":{"type":"choice","choice":"billing","confidence":1,"probabilities":{"billing":1}}`,
			1,
		),
		"unknown type":         strings.Replace(mixedResponse, `"type":"noul"`, `"type":"future"`, 1),
		"unexpected choice":    strings.Replace(mixedResponse, `"choice":"billing"`, `"choice":"admin"`, 1),
		"null distribution":    strings.Replace(mixedResponse, `"1":0.0`, `"1":null`, 1),
		"negative probability": strings.Replace(mixedResponse, `"billing":0.9`, `"billing":-0.5`, 1),
		"invalid legend":       strings.Replace(mixedResponse, `"0":"low"`, `"0":null`, 1),
		"missing usage":        strings.Replace(mixedResponse, `"output_tokens":0`, `"ignored":0`, 1),
		"negative usage":       strings.Replace(mixedResponse, `"output_tokens":0`, `"output_tokens":-1`, 1),
		"score outside rubric": strings.Replace(mixedResponse, `"score":0.0`, `"score":2.0`, 1),
	}
	for name, body := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			client := testClient(t, func(w http.ResponseWriter, _ *http.Request) { fmt.Fprint(w, body) })
			if _, err := client.Evaluate(t.Context(), mixedRequest()); err == nil {
				t.Error("accepted malformed response")
			}
		})
	}
}

func TestAPIErrors(t *testing.T) {
	t.Parallel()
	for _, status := range []int{401, 422, 429, 529} {
		t.Run(strconv.Itoa(status), func(t *testing.T) {
			t.Parallel()
			var calls atomic.Int32
			client := testClient(t, func(w http.ResponseWriter, _ *http.Request) {
				calls.Add(1)
				w.Header().Set("Retry-After", "3")
				w.WriteHeader(status)
				fmt.Fprint(w, `{"detail":"private request content"}`)
			})
			_, err := client.Evaluate(t.Context(), mixedRequest())
			var apiError *APIError
			if !errors.As(err, &apiError) || apiError.StatusCode != status || apiError.RetryAfter != "3" {
				t.Fatalf("unexpected error: %v", err)
			}
			if !strings.Contains(apiError.Body, "private request content") ||
				strings.Contains(err.Error(), "private request content") {
				t.Fatal("error body handling is wrong")
			}
			if calls.Load() != 1 {
				t.Fatal("unexpected automatic retry")
			}
		})
	}
}

func TestCancellationAndTimeout(t *testing.T) {
	t.Parallel()
	release := make(chan struct{})
	client := testClient(t, func(_ http.ResponseWriter, r *http.Request) {
		io.Copy(io.Discard, r.Body)
		select {
		case <-r.Context().Done():
		case <-release:
		}
	})
	t.Cleanup(func() { close(release) })
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := client.Evaluate(ctx, mixedRequest()); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation lost: %v", err)
	}
	ctx, cancel = context.WithTimeout(t.Context(), 10*time.Millisecond)
	defer cancel()
	if _, err := client.Evaluate(ctx, mixedRequest()); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("deadline lost: %v", err)
	}
}

func TestEvaluatePreservesRoundedProbabilities(t *testing.T) {
	t.Parallel()
	client := testClient(t, func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, strings.Replace(mixedResponse, `"billing":0.9`, `"billing":0.8995`, 1))
	})
	response, err := client.Evaluate(t.Context(), mixedRequest())
	if err != nil {
		t.Fatal(err)
	}
	choice, ok := response.Answers["route"].(ChoiceAnswer)
	if !ok || choice.Probabilities["billing"] != 0.8995 {
		t.Fatal("changed the API's rounded probabilities")
	}
}

func TestRedirectsAreNotFollowed(t *testing.T) {
	t.Parallel()
	var redirected atomic.Bool
	target := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { redirected.Store(true) }))
	defer target.Close()
	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
	})
	_, err := client.Evaluate(t.Context(), mixedRequest())
	var apiError *APIError
	if !errors.As(err, &apiError) || apiError.StatusCode != http.StatusTemporaryRedirect || redirected.Load() {
		t.Fatalf("redirect followed: %v", err)
	}
}

func TestResponseSizeLimit(t *testing.T) {
	t.Parallel()
	client := testClient(
		t,
		func(w http.ResponseWriter, _ *http.Request) { fmt.Fprint(w, strings.Repeat(" ", maxResponseBytes+1)) },
	)
	if _, err := client.Evaluate(t.Context(), mixedRequest()); err == nil || !strings.Contains(err.Error(), "8 MiB") {
		t.Fatalf("size limit not enforced: %v", err)
	}
}

func TestNewOptions(t *testing.T) {
	t.Setenv("TYPESAFE_API_KEY", " env-key ")
	client, err := New(nil)
	if err != nil || client.apiKey != "env-key" || client.http.Timeout != defaultTimeout {
		t.Fatalf("defaults: %v", err)
	}
	provided := &http.Client{Timeout: time.Second}
	client, err = New(&Options{APIKey: "explicit", HTTPClient: provided, BaseURL: "https://example.com/prefix/"})
	if err != nil || client.apiKey != "explicit" || client.endpoint != "https://example.com/prefix/v1/systemone" {
		t.Fatalf("options: %v", err)
	}
	if provided.CheckRedirect != nil {
		t.Fatal("mutated the caller's HTTP client")
	}
	t.Setenv("TYPESAFE_API_KEY", "")
	if _, err := New(nil); err == nil {
		t.Fatal("missing key accepted")
	}
	for _, key := range []string{"key\nsecret", "key space", "héllo"} {
		if _, err := New(&Options{APIKey: key}); err == nil || strings.Contains(err.Error(), key) {
			t.Fatal("invalid key accepted or disclosed")
		}
	}
	for _, base := range []string{"relative", "ftp://example.com", "https://u:p@example.com", "https://example.com?q=1", "https://example.com/#x"} {
		if _, err := New(&Options{APIKey: "key", BaseURL: base}); err == nil {
			t.Errorf("invalid base URL accepted: %s", base)
		}
	}
}
