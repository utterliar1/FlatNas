package handlers

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/net/proxy"
)

const maxWallpaperResponseBytes = 20 << 20

func shouldDropProxyResponseHeader(header string) bool {
	switch strings.ToLower(strings.TrimSpace(header)) {
	case "content-security-policy",
		"content-security-policy-report-only",
		"x-frame-options",
		"x-content-security-policy",
		"x-webkit-csp",
		"frame-options",
		"cross-origin-embedder-policy",
		"cross-origin-opener-policy",
		"cross-origin-resource-policy",
		"permissions-policy",
		"clear-site-data",
		"report-to",
		"nel",
		"set-cookie",
		"set-cookie2",
		"www-authenticate",
		"authorization",
		"connection",
		"keep-alive",
		"proxy-authenticate",
		"proxy-authorization",
		"te",
		"trailer",
		"transfer-encoding",
		"upgrade":
		return true
	default:
		return false
	}
}

func ProxyWallpaper(c *gin.Context) {
	targetURL := c.Query("url")
	requestUUID := c.Query("uuid")

	if targetURL == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "URL is required"})
		return
	}

	parsed, err := url.Parse(targetURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || parsed.User != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid URL"})
		return
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Unsupported protocol"})
		return
	}
	h := parsed.Hostname()
	if IsBlockedHost(h) {
		c.JSON(http.StatusForbidden, gin.H{"error": "Target host is not allowed"})
		return
	}

	req, err := http.NewRequest("GET", parsed.String(), nil)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create request"})
		return
	}

	// Forward necessary headers? Or just simple GET.
	// User-Agent might be needed for some APIs
	req.Header.Set("User-Agent", "FlatNas/1.0")

	// Reuse shared client or use a dedicated global one if needed.
	// For now, let's use the shared proxy client to support environments behind proxy.
	client := newSafeHTTPClient(10 * time.Second)
	client = cloneProxyClientWithoutRedirects(client)

	resp, err := client.Do(req)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "Failed to fetch upstream URL"})
		return
	}
	defer resp.Body.Close()

	// Copy headers
	c.Header("Content-Type", resp.Header.Get("Content-Type"))
	if cc := resp.Header.Get("Cache-Control"); cc != "" {
		c.Header("Cache-Control", cc)
	}
	if etag := resp.Header.Get("ETag"); etag != "" {
		c.Header("ETag", etag)
	}

	// Set UUID if provided
	if requestUUID != "" {
		c.Header("X-Request-UUID", requestUUID)
	}

	c.Status(resp.StatusCode)
	_, err = io.Copy(c.Writer, io.LimitReader(resp.Body, maxWallpaperResponseBytes))
	if err != nil {
		fmt.Printf("Error streaming response: %v\n", err)
	}
}

func GetProxyStatus(c *gin.Context) {
	proxyURL, err := getProxyURL()
	if err != nil || proxyURL == nil {
		c.JSON(http.StatusOK, gin.H{"available": false})
		return
	}
	c.JSON(http.StatusOK, gin.H{"available": true})
}

func ProxyRequest(c *gin.Context) {
	targetURL := c.Query("url")
	if targetURL == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "URL is required"})
		return
	}
	parsed, err := url.Parse(targetURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || parsed.User != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid URL"})
		return
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Unsupported protocol"})
		return
	}
	if IsBlockedHost(parsed.Hostname()) {
		c.JSON(http.StatusForbidden, gin.H{"error": "Target host is not allowed"})
		return
	}

	method := c.Request.Method
	if strings.EqualFold(method, "CONNECT") {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Unsupported method"})
		return
	}

	req, err := http.NewRequestWithContext(c.Request.Context(), method, parsed.String(), c.Request.Body)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create request"})
		return
	}
	allowedRequestHeaders := map[string]bool{
		"Accept": true, "Accept-Encoding": true, "Accept-Language": true,
		"Content-Type": true, "If-Modified-Since": true, "If-None-Match": true,
		"Range": true, "User-Agent": true,
	}
	for k, v := range c.Request.Header {
		key := http.CanonicalHeaderKey(k)
		if !allowedRequestHeaders[key] {
			continue
		}
		for _, vv := range v {
			req.Header.Add(k, vv)
		}
	}
	if req.Header.Get("User-Agent") == "" {
		req.Header.Set("User-Agent", "FlatNas/1.0")
	}

	client := newSafeHTTPClient(20 * time.Second)
	resp, err := client.Do(req)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "Failed to fetch upstream URL"})
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 && resp.StatusCode < 400 {
		location := resp.Header.Get("Location")
		if location == "" {
			c.JSON(http.StatusBadGateway, gin.H{"error": "Upstream redirect is not allowed"})
			return
		}
		redirectURL, err := parsed.Parse(location)
		if err != nil || (redirectURL.Scheme != "http" && redirectURL.Scheme != "https") || IsBlockedHost(redirectURL.Hostname()) {
			c.JSON(http.StatusForbidden, gin.H{"error": "Upstream redirect target is not allowed"})
			return
		}
		c.JSON(http.StatusBadGateway, gin.H{"error": "Upstream redirect is not allowed"})
		return
	}

	for k, v := range resp.Header {
		if shouldDropProxyResponseHeader(k) {
			continue
		}
		for _, vv := range v {
			c.Header(k, vv)
		}
	}
	c.Header("X-Frame-Options", "")
	c.Header("Content-Security-Policy", "")
	c.Header("Content-Security-Policy-Report-Only", "")
	c.Header("Cross-Origin-Opener-Policy", "unsafe-none")
	c.Header("Cross-Origin-Embedder-Policy", "unsafe-none")
	c.Header("Cross-Origin-Resource-Policy", "cross-origin")
	c.Status(resp.StatusCode)
	_, err = io.Copy(c.Writer, resp.Body)
	if err != nil {
		fmt.Printf("Error streaming response: %v\n", err)
	}
}

func cloneProxyClientWithoutRedirects(client *http.Client) *http.Client {
	if client == nil {
		return newSafeHTTPClient(20 * time.Second)
	}
	clone := *client
	clone.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}
	var transport *http.Transport
	if existing, ok := client.Transport.(*http.Transport); ok {
		transport = existing.Clone()
	} else if client.Transport == nil {
		transport = (&http.Transport{
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 10,
			IdleConnTimeout:     90 * time.Second,
		}).Clone()
	}
	if transport != nil {
		if transport.Proxy == nil {
			transport.DialContext = safeProxyDialContext(transport.DialContext)
		}
		clone.Transport = transport
	}
	return &clone
}

func newSafeHTTPClient(timeout time.Duration) *http.Client {
	transport := &http.Transport{
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 10,
		IdleConnTimeout:     90 * time.Second,
	}
	transport.DialContext = safeProxyDialContext(nil)
	return cloneProxyClientWithoutRedirects(&http.Client{Timeout: timeout, Transport: transport})
}

func safeProxyDialContext(parentDial func(context.Context, string, string) (net.Conn, error)) func(context.Context, string, string) (net.Conn, error) {
	dialer := &net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}
	return func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, err
		}
		ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
		if err != nil || len(ips) == 0 {
			return nil, fmt.Errorf("target host resolution failed")
		}
		for _, resolved := range ips {
			if isBlockedIP(resolved.IP) {
				return nil, fmt.Errorf("target host is not allowed")
			}
		}
		if parentDial != nil {
			return parentDial(ctx, network, net.JoinHostPort(ips[0].IP.String(), port))
		}
		return dialer.DialContext(ctx, network, net.JoinHostPort(ips[0].IP.String(), port))
	}
}

type safeProxyDialer struct {
	dialer proxy.Dialer
}

func (d safeProxyDialer) Dial(network, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil || len(ips) == 0 {
		return nil, fmt.Errorf("target host resolution failed")
	}
	for _, resolved := range ips {
		if isBlockedIP(resolved.IP) {
			return nil, fmt.Errorf("target host is not allowed")
		}
	}
	return d.dialer.Dial(network, net.JoinHostPort(ips[0].IP.String(), port))
}

func getProxyURL() (*url.URL, error) {
	keys := []string{"PROXY_URL", "HTTP_PROXY", "HTTPS_PROXY", "http_proxy", "https_proxy"}
	var lastErr error
	for _, key := range keys {
		value := strings.TrimSpace(os.Getenv(key))
		if value == "" {
			continue
		}
		parsed, err := parseProxyURL(value)
		if err != nil {
			lastErr = err
			continue
		}
		return parsed, nil
	}
	if lastErr != nil {
		return nil, lastErr
	}
	return nil, nil
}

func parseProxyURL(value string) (*url.URL, error) {
	normalized := strings.TrimSpace(value)
	if normalized == "" {
		return nil, fmt.Errorf("empty proxy url")
	}
	if !strings.Contains(normalized, "://") {
		normalized = "http://" + normalized
	}
	parsed, err := url.Parse(normalized)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return nil, fmt.Errorf("invalid proxy url")
	}
	switch parsed.Scheme {
	case "http", "https", "socks5", "socks5h":
		return parsed, nil
	default:
		return nil, fmt.Errorf("unsupported proxy protocol")
	}
}

var (
	globalProxyClient *http.Client
	globalProxyURLStr string
	proxyClientMu     sync.RWMutex
)

func getSharedProxyClient() (*http.Client, error) {
	proxyURL, err := getProxyURL()
	if err != nil {
		return nil, err
	}

	currentURLStr := ""
	if proxyURL != nil {
		currentURLStr = proxyURL.String()
	}

	proxyClientMu.RLock()
	client := globalProxyClient
	cachedURLStr := globalProxyURLStr
	proxyClientMu.RUnlock()

	if client != nil && cachedURLStr == currentURLStr {
		return client, nil
	}

	proxyClientMu.Lock()
	defer proxyClientMu.Unlock()

	// Double check
	if globalProxyClient != nil && globalProxyURLStr == currentURLStr {
		return globalProxyClient, nil
	}

	// Rebuild client
	transport := &http.Transport{
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 10,
		IdleConnTimeout:     90 * time.Second,
	}

	if proxyURL == nil {
		newClient := cloneProxyClientWithoutRedirects(&http.Client{Timeout: 20 * time.Second, Transport: transport})
		globalProxyClient = newClient
		globalProxyURLStr = ""
		return newClient, nil
	}

	switch proxyURL.Scheme {
	case "http", "https":
		transport.Proxy = http.ProxyURL(proxyURL)
	case "socks5", "socks5h":
		dialer, err := proxy.FromURL(proxyURL, proxy.Direct)
		if err != nil {
			return nil, err
		}
		safeDialer := safeProxyDialer{dialer: dialer}
		transport.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
			return safeDialer.Dial(network, addr)
		}
	default:
		return nil, fmt.Errorf("unsupported proxy protocol")
	}

	newClient := cloneProxyClientWithoutRedirects(&http.Client{Timeout: 20 * time.Second, Transport: transport})
	globalProxyClient = newClient
	globalProxyURLStr = currentURLStr
	return newClient, nil
}

func buildProxyClient() (*http.Client, error) {
	return getSharedProxyClient()
}

func IsBlockedHost(host string) bool {
	if host == "" {
		return true
	}
	host = strings.TrimSpace(strings.ToLower(host))
	if host == "localhost" || host == "localhost." {
		return true
	}
	if ip := net.ParseIP(host); ip != nil {
		return isBlockedIP(ip)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil || len(ips) == 0 {
		return true
	}
	for _, item := range ips {
		if item.IP != nil && isBlockedIP(item.IP) {
			return true
		}
	}
	return false
}

func validateExternalHTTPURL(raw string) (*url.URL, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme == "" || parsed.Hostname() == "" || parsed.User != nil {
		return nil, fmt.Errorf("invalid URL")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, fmt.Errorf("unsupported URL scheme")
	}
	if IsBlockedHost(parsed.Hostname()) {
		return nil, fmt.Errorf("target host is not allowed")
	}
	return parsed, nil
}

func isBlockedIP(ip net.IP) bool {
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast()
}
