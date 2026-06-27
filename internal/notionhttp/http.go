package notionhttp

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	http "github.com/bogdanfinn/fhttp"
	tls_client "github.com/bogdanfinn/tls-client"
	"github.com/bogdanfinn/tls-client/profiles"
)

type Client struct {
	httpClient tls_client.HttpClient
	timeout    time.Duration
}

func NewClient() (*Client, error) {
	jar := tls_client.NewCookieJar()
	options := []tls_client.HttpClientOption{
		tls_client.WithTimeoutSeconds(300),
		tls_client.WithClientProfile(profiles.Chrome_131),
		tls_client.WithNotFollowRedirects(),
		tls_client.WithCookieJar(jar),
	}
	client, err := tls_client.NewHttpClient(tls_client.NewNoopLogger(), options...)
	if err != nil {
		return nil, fmt.Errorf("tls client: %w", err)
	}
	return &Client{httpClient: client, timeout: 300 * time.Second}, nil
}

func (c *Client) Close() error {
	return nil
}

func (c *Client) PostJSON(url string, body map[string]any, headers map[string]string) (map[string]any, int, string, error) {
	data, status, respBody, _, err := c.PostJSONWithCookies(url, body, headers)
	return data, status, respBody, err
}

func (c *Client) PostJSONWithCookies(url string, body map[string]any, headers map[string]string) (map[string]any, int, string, []*http.Cookie, error) {
	b, err := json.Marshal(body)
	if err != nil {
		return nil, 0, "", nil, err
	}
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(b))
	if err != nil {
		return nil, 0, "", nil, err
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, 0, "", nil, err
	}
	defer resp.Body.Close()
	setCookies := ParseSetCookieHeaders(resp.Header)
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, "", setCookies, err
	}
	if resp.StatusCode != 200 {
		return nil, resp.StatusCode, string(respBody), setCookies, nil
	}
	var data map[string]any
	if err := json.Unmarshal(respBody, &data); err != nil {
		return nil, resp.StatusCode, string(respBody), setCookies, err
	}
	return data, resp.StatusCode, string(respBody), setCookies, nil
}

func (c *Client) PostStream(url string, body map[string]any, headers map[string]string) (io.ReadCloser, int, string, error) {
	b, err := json.Marshal(body)
	if err != nil {
		return nil, 0, "", err
	}
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(b))
	if err != nil {
		return nil, 0, "", err
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, 0, "", err
	}
	if resp.StatusCode != 200 {
		bodyBytes, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, resp.StatusCode, string(bodyBytes), nil
	}
	stream := io.ReadCloser(resp.Body)
	if enc := strings.ToLower(resp.Header.Get("Content-Encoding")); strings.Contains(enc, "gzip") {
		gz, err := gzip.NewReader(resp.Body)
		if err != nil {
			resp.Body.Close()
			return nil, 0, "", fmt.Errorf("gzip reader: %w", err)
		}
		stream = &gzipReadCloser{Reader: gz, underlying: resp.Body}
	}
	return stream, resp.StatusCode, "", nil
}

type gzipReadCloser struct {
	*gzip.Reader
	underlying io.Closer
}

func (g *gzipReadCloser) Close() error {
	_ = g.Reader.Close()
	return g.underlying.Close()
}

func ReadLines(r io.Reader, onLine func(string) error) error {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if err := onLine(line); err != nil {
			return err
		}
	}
	return scanner.Err()
}

func BuildHeaders(accHeaders map[string]string) map[string]string {
	out := make(map[string]string, len(accHeaders))
	for k, v := range accHeaders {
		out[k] = v
	}
	return out
}

func TruncateBody(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return s[:n]
}