package apsara

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"net"
	"net/http"
	"os"
	"sync"
	"time"
)

// 环境变量名（可通过 export 替代 WithXXX 选项）。
const (
	// envAccessKeyID 是环境变量 APSARA_ACCESS_KEY_ID 的名称。
	envAccessKeyID = "APSARA_ACCESS_KEY_ID"
	// envAccessKeySecret 是环境变量 APSARA_ACCESS_KEY_SECRET 的名称。
	envAccessKeySecret = "APSARA_ACCESS_KEY_SECRET"
	// envSecurityToken 是环境变量 APSARA_SECURITY_TOKEN 的名称（STS 可选）。
	envSecurityToken = "APSARA_SECURITY_TOKEN"
	// envRegionID 是环境变量 APSARA_REGION_ID 的名称。
	envRegionID = "APSARA_REGION_ID"
	// envEndpoint 是环境变量 APSARA_ENDPOINT 的名称。
	envEndpoint = "APSARA_ENDPOINT"
)

// callerSource 是默认的 x-acs-caller-sdk-source 取值。
const callerSource = "apsara-go"

// ---------------------------------------------------------------------------
// Logger
// ---------------------------------------------------------------------------

// Logger 是 SDK 的日志接口。实现此接口的类型可接收 SDK 的内部日志。
// 通过 WithLogger 注入。
type Logger interface {
	Logf(format string, args ...any)
}

// ---------------------------------------------------------------------------
// ApsaraError
// ---------------------------------------------------------------------------

// ApsaraError 是 SDK 返回的结构化错误。
//
// StatusCode 含义：
//   - 0：网络层错误（连接超时、DNS 解析失败等），Err 包含原始错误；
//   - 400～599：服务端返回的错误，RequestID / Code / Message 从 JSON body 中解析。
type ApsaraError struct {
	// Action 是请求的 API 操作名称。
	Action string
	// StatusCode 是 HTTP 响应状态码。0 表示网络层错误。
	StatusCode int
	// RequestID 是阿里云请求的唯一标识，可从失败响应的 JSON 中提取。
	RequestID string
	// Code 是阿里云业务错误码，例如 "InvalidInstanceId"。
	Code string
	// Message 是阿里云业务错误描述。
	Message string
	// Err 是原始错误（网络层错误时不为 nil）。
	Err error
}

func (e *ApsaraError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s: request %s error: %v", e.Action, e.RequestID, e.Err)
	}

	return fmt.Sprintf("%s: [%d %s] %s (request=%s)",
		e.Action, e.StatusCode, e.Code, e.Message, e.RequestID)
}

// Unwrap 返回原始错误，使 errors.As 可穿透到 ApsaraError。
func (e *ApsaraError) Unwrap() error { return e.Err }

// ---------------------------------------------------------------------------
// ResponseMeta
// ---------------------------------------------------------------------------

// ResponseMeta 包含单次 API 调用的响应元数据。
// 通过 RequestOption 的 WithMeta 获取。
type ResponseMeta struct {
	StatusCode int
	Header     http.Header
	RequestID  string
	RawBody    []byte // 原始 JSON 响应体
}

// ---------------------------------------------------------------------------
// ClientOption
// ---------------------------------------------------------------------------

// ClientOption 是 NewClient 的配置选项。
type ClientOption func(*Client)

// WithEndpoint 设置 API 服务地址（必填）。
// 环境变量 APSARA_ENDPOINT 可替代此选项。
func WithEndpoint(endpoint string) ClientOption {
	return func(c *Client) { c.Endpoint = endpoint }
}

// WithRegion 设置地域 ID（必填）。
// 环境变量 APSARA_REGION_ID 可替代此选项。
func WithRegion(regionID string) ClientOption {
	return func(c *Client) { c.RegionID = regionID }
}

// WithCredential 手动设置访问凭证。
// 若未调用此选项，SDK 会依次尝试从环境变量
// APSARA_ACCESS_KEY_ID / APSARA_ACCESS_KEY_SECRET / APSARA_SECURITY_TOKEN 加载。
func WithCredential(cred Credential) ClientOption {
	return func(c *Client) { c.signer = NewSigner(cred) }
}

// WithInsecureSkipVerify 跳过 TLS 证书验证，适用于自签名证书的内网环境。
func WithInsecureSkipVerify(skip bool) ClientOption {
	return func(c *Client) { c.InsecureSkipVerify = skip }
}

// WithHTTPClient 设置自定义 HTTP 客户端。
// 设置后 InsecureSkipVerify 不生效，需自行在 Transport 中配置。
func WithHTTPClient(cl *http.Client) ClientOption {
	return func(c *Client) { c.HTTPClient = cl }
}

// WithOrganizationID 设置专有云的组织 ID，对应 Header x-acs-organizationid。
func WithOrganizationID(id string) ClientOption {
	return func(c *Client) { c.OrganizationID = id }
}

// WithResourceGroupID 设置专有云的资源集 ID，对应 Header x-acs-resourcegroupid。
func WithResourceGroupID(id string) ClientOption {
	return func(c *Client) { c.ResourceGroupID = id }
}

// WithInstanceID 设置实例 ID，对应 Header x-acs-instanceid。
func WithInstanceID(id string) ClientOption {
	return func(c *Client) { c.InstanceID = id }
}

// WithCallerSource 设置调用来源标识，对应 Header x-acs-caller-sdk-source。
// 未设置时默认使用 "apsara-go"。
func WithCallerSource(source string) ClientOption {
	return func(c *Client) { c.CallerSource = source }
}

// WithTimeout 设置单次 HTTP 请求的超时时间（含连接、TLS 握手、发送请求、读取响应体）。
// 默认为 0（不限时），受 context 约束。
func WithTimeout(d time.Duration) ClientOption {
	return func(c *Client) { c.requestTimeout = d }
}

// WithLogger 设置日志记录器。设置后 SDK 会在每次 API 调用时输出请求方法和 URL。
func WithLogger(l Logger) ClientOption {
	return func(c *Client) { c.logger = l }
}

// WithRetry 设置请求失败时的最大重试次数。
// 设置为 0 或 1 表示不重试（仅执行一次请求）。
// 重试退避采用指数退避 + 随机抖动，仅对网络错误和 5xx/429 响应生效。
func WithRetry(maxAttempts int) ClientOption {
	return func(c *Client) { c.retryMaxAttempts = maxAttempts }
}

// ---------------------------------------------------------------------------
// RequestOption
// ---------------------------------------------------------------------------

// RequestOption 是 Get / Post / Put / Delete 函数的额外选项。
type RequestOption func(*requestConfig)

type requestConfig struct {
	meta *ResponseMeta
}

// WithMeta 让 SDK 将响应元数据（状态码、Header、RequestId、原始 Body）写入 m。
func WithMeta(m *ResponseMeta) RequestOption {
	return func(cfg *requestConfig) { cfg.meta = m }
}

func newRequestConfig(opts []RequestOption) *requestConfig {
	cfg := &requestConfig{}
	for _, opt := range opts {
		opt(cfg)
	}

	return cfg
}

// ---------------------------------------------------------------------------
// Client
// ---------------------------------------------------------------------------

// Client 是阿里云专有云的通用 API 客户端。
// 一个 Client 实例可复用于任意产品，每次调用时传入 Action + Version。
type Client struct {
	// Endpoint 是 API 服务地址，例如 "ecs.aliyuncs.com"（不含 scheme 和 path）。
	Endpoint string
	// RegionID 是地域 ID，例如 "cn-hangzhou"。
	RegionID string
	// Scheme 是协议，默认为 "https"。
	Scheme string

	signer *Signer

	// HTTPClient 是自定义 HTTP 客户端。
	HTTPClient *http.Client
	// InsecureSkipVerify 为 true 时跳过 TLS 证书验证。
	InsecureSkipVerify bool

	// OrganizationID 对应 Header x-acs-organizationid。
	OrganizationID string
	// ResourceGroupID 对应 Header x-acs-resourcegroupid。
	ResourceGroupID string
	// InstanceID 对应 Header x-acs-instanceid。
	InstanceID string
	// CallerSource 对应 Header x-acs-caller-sdk-source。
	CallerSource string

	logger Logger

	retryMaxAttempts int // 重试次数，≤1 表示不重试

	requestTimeout time.Duration // HTTP 请求超时，0 表示不限时

	mu             sync.Mutex
	insecureClient *http.Client // 懒加载的 insecure 客户端
}

// NewClient 创建通用 API 客户端。
//
// 必填选项：WithEndpoint + WithRegion。
// 凭证可通过 WithCredential 手动设置，或通过环境变量自动加载。
//
// 使用示例：
//
//	client, err := apsara.NewClient(
//	    apsara.WithEndpoint("ecs.aliyuncs.com"),
//	    apsara.WithRegion("cn-hangzhou"),
//	)
func NewClient(opts ...ClientOption) (*Client, error) {
	c := &Client{
		Scheme:           "https",
		retryMaxAttempts: 1,
	}

	// 环境变量加载凭证
	cred := Credential{
		AccessKeyID:     os.Getenv(envAccessKeyID),
		AccessKeySecret: os.Getenv(envAccessKeySecret),
		SecurityToken:   os.Getenv(envSecurityToken),
	}
	if cred.AccessKeyID != "" && cred.AccessKeySecret != "" {
		c.signer = NewSigner(cred)
	}

	if ep := os.Getenv(envEndpoint); ep != "" {
		c.Endpoint = ep
	}

	if r := os.Getenv(envRegionID); r != "" {
		c.RegionID = r
	}

	for _, opt := range opts {
		opt(c)
	}

	if c.Endpoint == "" {
		return nil, fmt.Errorf(
			"apsara: endpoint is required (set WithEndpoint or %s env var)",
			envEndpoint,
		)
	}

	if c.RegionID == "" {
		return nil, fmt.Errorf(
			"apsara: region is required (set WithRegion or %s env var)",
			envRegionID,
		)
	}

	if c.signer == nil {
		return nil, fmt.Errorf(
			"apsara: credential is required (set WithCredential or %s/%s env vars)",
			envAccessKeyID,
			envAccessKeySecret,
		)
	}

	return c, nil
}

// httpClient 返回用于请求的 *http.Client。
// 优先级：自定义 HTTPClient > InsecureSkipVerify 客户端 > 默认客户端。
// 当 requestTimeout > 0 时，返回的客户端会携带超时设置。
func (c *Client) httpClient() *http.Client {
	if c.HTTPClient != nil {
		return c.HTTPClient
	}

	if c.InsecureSkipVerify {
		return c.insecureClientWithTimeout()
	}

	if c.requestTimeout <= 0 {
		return http.DefaultClient
	}

	// requestTimeout > 0 时不能修改 http.DefaultClient，需创建副本
	return &http.Client{
		Timeout: c.requestTimeout,
	}
}

// insecureClientWithTimeout 返回带 InsecureSkipVerify 的客户端（含超时）。
func (c *Client) insecureClientWithTimeout() *http.Client {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.insecureClient != nil && c.insecureClient.Timeout == c.requestTimeout {
		return c.insecureClient
	}

	c.insecureClient = &http.Client{
		Timeout: c.requestTimeout,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
			DialContext: (&net.Dialer{
				Timeout: 10 * time.Second,
			}).DialContext,
			TLSHandshakeTimeout:   10 * time.Second,
			ResponseHeaderTimeout: 30 * time.Second,
		},
	}

	return c.insecureClient
}

// logf 通过 Logger 输出日志。
func (c *Client) logf(format string, args ...any) {
	if c.logger != nil {
		c.logger.Logf(format, args...)
	}
}

// callerSourceValue 返回 x-acs-caller-sdk-source 的实际值。
func (c *Client) callerSourceValue() string {
	if c.CallerSource != "" {
		return c.CallerSource
	}

	return callerSource
}

// setHeaders 将公共 Header 注入到 http.Header。
func (c *Client) setHeaders(h http.Header) {
	h.Set("x-acs-regionid", c.RegionID)
	h.Set("x-acs-caller-sdk-source", c.callerSourceValue())

	if c.OrganizationID != "" {
		h.Set("x-acs-organizationid", c.OrganizationID)
	}

	if c.ResourceGroupID != "" {
		h.Set("x-acs-resourcegroupid", c.ResourceGroupID)
	}

	if c.InstanceID != "" {
		h.Set("x-acs-instanceid", c.InstanceID)
	}
}

// commonHeaders 返回公共 Header 映射表。
func (c *Client) commonHeaders() map[string]string {
	h := make(map[string]string, 6)
	h["x-acs-regionid"] = c.RegionID

	h["x-acs-caller-sdk-source"] = c.callerSourceValue()
	if c.OrganizationID != "" {
		h["x-acs-organizationid"] = c.OrganizationID
	}

	if c.ResourceGroupID != "" {
		h["x-acs-resourcegroupid"] = c.ResourceGroupID
	}

	if c.InstanceID != "" {
		h["x-acs-instanceid"] = c.InstanceID
	}

	return h
}

// ---------------------------------------------------------------------------
// Get / Post / Put / Delete
// ---------------------------------------------------------------------------

// Get 发送 GET 请求。
//
//	action:    API 操作名称（如 "DescribeInstances"）
//	version:   API 版本号（如 "2014-05-26"）
//	bizParams: 业务参数，可为 nil
//	result:    用于接收响应的值（需传入指针）
//	opts:      额外选项（如 WithMeta）
func (c *Client) Get(
	ctx context.Context,
	action, version string,
	bizParams map[string]string,
	result any,
	opts ...RequestOption,
) error {
	return c.doRequest(ctx, http.MethodGet, action, version, bizParams, result, opts...)
}

// Post 发送 POST 请求。参数同 Get。
func (c *Client) Post(
	ctx context.Context,
	action, version string,
	bizParams map[string]string,
	result any,
	opts ...RequestOption,
) error {
	return c.doRequest(ctx, http.MethodPost, action, version, bizParams, result, opts...)
}

// Put 发送 PUT 请求。参数同 Get。
func (c *Client) Put(
	ctx context.Context,
	action, version string,
	bizParams map[string]string,
	result any,
	opts ...RequestOption,
) error {
	return c.doRequest(ctx, http.MethodPut, action, version, bizParams, result, opts...)
}

// Delete 发送 DELETE 请求。参数同 Get。
func (c *Client) Delete(
	ctx context.Context,
	action, version string,
	bizParams map[string]string,
	result any,
	opts ...RequestOption,
) error {
	return c.doRequest(ctx, http.MethodDelete, action, version, bizParams, result, opts...)
}

// ---------------------------------------------------------------------------
// doRequest
// ---------------------------------------------------------------------------

// doRequest 是 Get/Post/Put/Delete 的内部实现。
func (c *Client) doRequest(
	ctx context.Context,
	method, action, version string,
	bizParams map[string]string,
	result any,
	opts ...RequestOption,
) error {
	if bizParams == nil {
		bizParams = make(map[string]string)
	}

	reqCfg := newRequestConfig(opts)

	params := c.buildParams(action, version, bizParams)
	if err := c.signer.Sign(method, params); err != nil {
		return &ApsaraError{Action: action, Err: fmt.Errorf("sign: %w", err)}
	}

	queryString := buildCanonicalQueryString(params, true)
	rawURL := fmt.Sprintf("%s://%s/?%s", c.Scheme, c.Endpoint, queryString)
	c.logf("apsara: %s %s", method, rawURL)

	var lastErr error

	attempts := max(c.retryMaxAttempts, 1)

	for attempt := range attempts {
		if attempt > 0 {
			wait := backoff(attempt)
			c.logf("apsara: retry attempt %d/%d after %v", attempt+1, attempts, wait)

			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(wait):
			}
		}

		lastErr = c.tryOnce(ctx, method, rawURL, reqCfg, result, action)
		if lastErr == nil {
			return nil
		}
		// StatusCode == 0 表示网络层错误，应重试
		// StatusCode >= 500 或 == 429 表示服务端可重试错误
		if ae, ok := lastErr.(*ApsaraError); ok {
			if ae.StatusCode != 0 && ae.StatusCode < 500 && ae.StatusCode != 429 {
				return lastErr
			}
		}
	}

	return lastErr
}

// tryOnce 执行一次 HTTP 请求，包含签名验证、状态码检查和响应反序列化。
func (c *Client) tryOnce(
	ctx context.Context,
	method, rawURL string,
	reqCfg *requestConfig,
	result any,
	action string,
) error {
	req, err := http.NewRequestWithContext(ctx, method, rawURL, nil)
	if err != nil {
		return &ApsaraError{Action: action, Err: fmt.Errorf("build request: %w", err)}
	}

	c.setHeaders(req.Header)

	resp, err := c.httpClient().Do(req)
	if err != nil {
		return &ApsaraError{Action: action, Err: fmt.Errorf("http do: %w", err)}
	}

	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return &ApsaraError{
			Action: action, StatusCode: resp.StatusCode,
			Err: fmt.Errorf("read body: %w", err),
		}
	}

	if reqCfg.meta != nil {
		reqCfg.meta.StatusCode = resp.StatusCode
		reqCfg.meta.Header = resp.Header.Clone()
		reqCfg.meta.RawBody = body
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		ae := parseErrorResponse(action, resp.StatusCode, body)
		if reqCfg.meta != nil {
			reqCfg.meta.RequestID = ae.RequestID
		}

		return ae
	}

	if err := json.Unmarshal(body, result); err != nil {
		return &ApsaraError{
			Action: action, StatusCode: resp.StatusCode,
			Err: fmt.Errorf("json decode: %w (body: %.1024s)", err, body),
		}
	}

	if reqCfg.meta != nil {
		if rid, ok := extractRequestID(body); ok {
			reqCfg.meta.RequestID = rid
		}
	}

	return nil
}

// parseErrorResponse 从失败的 HTTP 响应 body 中解析 ApsaraError，提取 RequestId/Code/Message。
func parseErrorResponse(action string, statusCode int, body []byte) *ApsaraError {
	ae := &ApsaraError{Action: action, StatusCode: statusCode}
	if len(body) == 0 {
		return ae
	}

	var m map[string]any
	if json.Unmarshal(body, &m) != nil {
		return ae
	}

	if rid, _ := m["RequestId"].(string); rid != "" {
		ae.RequestID = rid
	}

	if code, _ := m["Code"].(string); code != "" {
		ae.Code = code
	}

	if msg, _ := m["Message"].(string); msg != "" {
		ae.Message = msg
	}

	return ae
}

// extractRequestID 从 JSON body 中提取 RequestId 字段。
func extractRequestID(body []byte) (string, bool) {
	var m map[string]any
	if json.Unmarshal(body, &m) != nil {
		return "", false
	}

	rid, ok := m["RequestId"].(string)

	return rid, ok
}

// ---------------------------------------------------------------------------
// Backoff
// ---------------------------------------------------------------------------

// backoff 计算第 n 次重试的等待时间（指数退避 + 随机抖动），上限 30 秒。
func backoff(attempt int) time.Duration {
	if attempt <= 0 {
		return 0
	}

	base := min(1<<uint(attempt-1), 30)
	jitter := cryptoRandInt(base + 1)

	return time.Duration(base+jitter) * time.Second
}

// cryptoRandInt 返回 [0, n) 的安全随机整数，使用 crypto/rand 避免 gosec 告警。
func cryptoRandInt(n int) int {
	if n <= 1 {
		return 0
	}

	threshold := 256 - (256 % n) - 1

	b := make([]byte, 1)
	for {
		if _, err := io.ReadFull(rand.Reader, b); err != nil {
			return 0
		}

		val := int(b[0])
		if val <= threshold {
			return val % n
		}
	}
}

// ---------------------------------------------------------------------------
// buildParams
// ---------------------------------------------------------------------------

// buildParams 组装业务参数和公共请求参数，初始容量预留充足避免中途扩容。
func (c *Client) buildParams(
	action, version string,
	bizParams map[string]string,
) map[string]string {
	params := make(map[string]string, len(bizParams)+12)
	maps.Copy(params, bizParams)
	params["Action"] = action
	params["Format"] = "JSON"
	params["Version"] = version
	params["AccessKeyId"] = c.signer.cred.AccessKeyID
	params["RegionId"] = c.RegionID

	return params
}

// ---------------------------------------------------------------------------
// BuildSignedParams / RawRequest / Do / BuildURL / MustBuildURL
// ---------------------------------------------------------------------------

// BuildSignedParams 生成包含签名在内的完整查询参数映射。
// 返回的 map 可直接拼接到 URL 查询字符串中使用。
func (c *Client) BuildSignedParams(
	action, version string,
	bizParams map[string]string,
) (map[string]string, error) {
	if bizParams == nil {
		bizParams = make(map[string]string)
	}

	params := c.buildParams(action, version, bizParams)
	if err := c.signer.Sign("GET", params); err != nil {
		return nil, err
	}

	return params, nil
}

// RawRequest 使用已签名的参数直接发送 HTTP 请求，并返回原始响应。
// 调用方需自行关闭 resp.Body 并解析响应。
func (c *Client) RawRequest(
	ctx context.Context,
	method string,
	params map[string]string,
) (*http.Response, error) {
	queryString := buildCanonicalQueryString(params, true)
	rawURL := fmt.Sprintf("%s://%s/?%s", c.Scheme, c.Endpoint, queryString)

	req, err := http.NewRequestWithContext(ctx, method, rawURL, nil)
	if err != nil {
		return nil, err
	}

	c.setHeaders(req.Header)

	return c.httpClient().Do(req)
}

// Do 对已构建的 *http.Request 注入公共 Header、发送请求，并将 JSON 响应反序列化到 result。
// 此方法不校验 HTTP 状态码，也不解析业务错误，适合完全自定义的请求场景。
func (c *Client) Do(ctx context.Context, req *http.Request, result any) error {
	for k, v := range c.commonHeaders() {
		if req.Header.Get(k) == "" {
			req.Header.Set(k, v)
		}
	}

	resp, err := c.httpClient().Do(req)
	if err != nil {
		return fmt.Errorf("http do: %w", err)
	}

	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read body: %w", err)
	}

	if err := json.Unmarshal(body, result); err != nil {
		return fmt.Errorf("json decode: %w (body: %.1024s)", err, body)
	}

	return nil
}

// BuildURL 将带签名的参数构建为完整的请求 URL 字符串。
func (c *Client) BuildURL(action, version string, bizParams map[string]string) (string, error) {
	params, err := c.BuildSignedParams(action, version, bizParams)
	if err != nil {
		return "", err
	}

	qs := buildCanonicalQueryString(params, true)

	return fmt.Sprintf("%s://%s/?%s", c.Scheme, c.Endpoint, qs), nil
}

// MustBuildURL 类似 BuildURL，但失败时 panic。适用于静态初始化。
func (c *Client) MustBuildURL(action, version string, bizParams map[string]string) string {
	url, err := c.BuildURL(action, version, bizParams)
	if err != nil {
		panic(err)
	}

	return url
}
