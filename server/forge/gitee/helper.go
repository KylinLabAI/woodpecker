// Copyright 2026 Woodpecker Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package gitee

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"go.woodpecker-ci.org/woodpecker/v3/shared/httputil"
)

const (
	// DefaultPageSize is the amount of items requested per Gitee API page.
	// Gitee caps this at 100.
	defaultPageSize = 100
	// MaxPages bounds the paging of fetchAllPages to protect against a
	// Gitee instance that keeps returning full pages forever.
	maxPages = 1000
	// RequestTimeout is the fallback timeout of a single Gitee API request.
	requestTimeout = 30 * time.Second
	// MaxResponseSize is the maximum size of a Gitee API response body.
	maxResponseSize = 32 << 20
	// IdleConnTimeout is how long an idle connection is kept in the pool.
	idleConnTimeout = 90 * time.Second
	// MaxIdleConnsPerHost is the amount of idle connections kept per host.
	maxIdleConnsPerHost = 10
)

// apiError reports a non successful response of the Gitee API.
type apiError struct {
	StatusCode int
	Message    string
}

// Error implements the error interface.
func (e *apiError) Error() string {
	if e.Message == "" {
		return fmt.Sprintf("gitee api responded with status %d", e.StatusCode)
	}
	return fmt.Sprintf("gitee api responded with status %d: %s", e.StatusCode, e.Message)
}

// isNotFound reports whether err was caused by a 404 response of the Gitee API.
func isNotFound(err error) bool {
	var apiErr *apiError
	if errors.As(err, &apiErr) {
		return apiErr.StatusCode == http.StatusNotFound
	}
	return false
}

// apiURL returns the base url of the Gitee API.
func (c *Gitee) apiURL() string {
	return strings.TrimSuffix(c.url, "/") + "/api/v5"
}

// httpClient returns the shared http client of this forge.
// The client is built once per forge instance, creating a new transport per
// request would leak connections as idle transports are never collected.
func (c *Gitee) httpClient() *http.Client {
	c.clientOnce.Do(func() {
		c.client = httputil.WrapClient(&http.Client{
			Timeout: requestTimeout,
			Transport: &http.Transport{
				TLSClientConfig:       &tls.Config{InsecureSkipVerify: c.skipVerify},
				Proxy:                 http.ProxyFromEnvironment,
				IdleConnTimeout:       idleConnTimeout,
				MaxIdleConnsPerHost:   maxIdleConnsPerHost,
				ResponseHeaderTimeout: requestTimeout,
			},
		}, "forge-gitee")
	})
	return c.client
}

// get performs a GET request against the Gitee API and decodes the json
// response into out. A nil out skips the decoding.
//
// Gitee authenticates requests with an access_token query parameter, no
// bearer token is used.
func (c *Gitee) get(ctx context.Context, accessToken, path string, params url.Values, out any) error {
	query := maps.Clone(params)
	if query == nil {
		query = url.Values{}
	}
	if accessToken != "" {
		query.Set("access_token", accessToken)
	}

	target := c.apiURL() + path
	if len(query) != 0 {
		target += "?" + query.Encode()
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return sanitizeError(err, path)
	}

	resp, err := c.httpClient().Do(req)
	if err != nil {
		return sanitizeError(err, path)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseSize+1))
	if err != nil {
		return fmt.Errorf("could not read gitee api response of %s: %w", path, err)
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return &apiError{StatusCode: resp.StatusCode, Message: errorMessage(body)}
	}

	if len(body) > maxResponseSize {
		return fmt.Errorf("gitee api response of %s exceeds %d bytes", path, maxResponseSize)
	}

	if out == nil || len(body) == 0 {
		return nil
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("could not decode gitee api response of %s: %w", path, err)
	}
	return nil
}

// fetchAllPages fetches every page of a paginated Gitee API endpoint.
// Gitee does not send a Link header, so paging stops as soon as a page
// returns fewer items than requested.
func fetchAllPages[T any](ctx context.Context, c *Gitee, accessToken, path string, params url.Values) ([]T, error) {
	items := make([]T, 0, defaultPageSize)

	for page := 1; page <= maxPages; page++ {
		query := maps.Clone(params)
		if query == nil {
			query = url.Values{}
		}
		query.Set("page", strconv.Itoa(page))
		query.Set("per_page", strconv.Itoa(defaultPageSize))

		var batch []T
		if err := c.get(ctx, accessToken, path, query, &batch); err != nil {
			return nil, err
		}

		items = append(items, batch...)
		if len(batch) < defaultPageSize {
			return items, nil
		}
	}

	return nil, fmt.Errorf("gitee api paging of %s exceeded %d pages", path, maxPages)
}

// sanitizeError hides the request url of a transport error as it carries the
// access token of the user.
func sanitizeError(err error, path string) error {
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		if urlErr.Err == nil {
			return fmt.Errorf("gitee api request to %s failed", path)
		}
		return fmt.Errorf("gitee api request to %s failed: %w", path, urlErr.Err)
	}
	return fmt.Errorf("gitee api request to %s failed: %w", path, err)
}

// errorMessage extracts the message of a Gitee API error response.
func errorMessage(body []byte) string {
	var payload struct {
		Message string `json:"message"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return ""
	}
	return payload.Message
}
