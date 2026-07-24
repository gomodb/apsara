package apsara

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base64"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"
)

// Credential 表示阿里云 API 的访问凭证。
// 支持 AccessKey 和 STS 两种认证方式。
type Credential struct {
	AccessKeyID     string
	AccessKeySecret string
	// SecurityToken 仅在 STS 临时凭证时需要填写。
	SecurityToken string
}

// Signer 执行阿里云 RPC API 的 HMAC-SHA1 签名计算。
type Signer struct {
	cred Credential
}

// NewSigner 创建签名器。
func NewSigner(cred Credential) *Signer {
	return &Signer{cred: cred}
}

// Sign 对所有查询参数生成阿里云 RPC 签名，并将签名相关字段写入 params。
//
// 入参 params 应包含除 Signature 自身外的所有查询参数
// （Action、Format、Version、AccessKeyId、RegionId 及业务参数）。
// Sign 会向 params 中写入 SignatureNonce、Timestamp、SignatureMethod、
// SignatureVersion、Signature（及可选的 SecurityToken）。
//
// 签名算法：
//
//	StringToSign = HTTPMethod + "&" + percentEncode("/") + "&" + percentEncode(CanonicalizedQueryString)
//	Signature = Base64( HMAC-SHA1( StringToSign, AccessKeySecret + "&" ) )
func (s *Signer) Sign(httpMethod string, params map[string]string) error {
	nonce, err := newUUID()
	if err != nil {
		return fmt.Errorf("generate uuid: %w", err)
	}

	params["SignatureNonce"] = nonce
	params["Timestamp"] = time.Now().UTC().Format("2006-01-02T15:04:05Z")
	params["SignatureMethod"] = "HMAC-SHA1"

	params["SignatureVersion"] = "1.0"
	if s.cred.SecurityToken != "" {
		params["SecurityToken"] = s.cred.SecurityToken
	}

	canonicalQuery := buildCanonicalQueryString(params, false)
	stringToSign := buildStringToSign(httpMethod, canonicalQuery)

	key := s.cred.AccessKeySecret + "&"
	mac := hmac.New(sha1.New, []byte(key))
	mac.Write([]byte(stringToSign))
	signatureBytes := mac.Sum(nil)

	params["Signature"] = base64.StdEncoding.EncodeToString(signatureBytes)

	return nil
}

// percentEncode 执行 RFC 3986 百分比编码。
//
// 规则：
//   - A-Z、a-z、0-9、-、_、.、~ 不编码；
//   - 空格编码为 %20；
//   - 其余字节编码为 %XY。
func percentEncode(s string) string {
	var buf strings.Builder
	buf.Grow(len(s) * 2)

	for i := 0; i < len(s); i++ {
		b := s[i]
		switch {
		case (b >= 'A' && b <= 'Z') ||
			(b >= 'a' && b <= 'z') ||
			(b >= '0' && b <= '9') ||
			b == '-' || b == '_' || b == '.' || b == '~':
			buf.WriteByte(b)
		case b == ' ':
			buf.WriteString("%20")
		default:
			buf.WriteByte('%')
			buf.WriteByte(hexTable[b>>4])
			buf.WriteByte(hexTable[b&0x0f])
		}
	}

	return buf.String()
}

// hexTable 是 percentEncode 用的十六进制字符查表。
var hexTable = [16]byte{
	'0', '1', '2', '3', '4', '5', '6', '7',
	'8', '9', 'A', 'B', 'C', 'D', 'E', 'F',
}

// buildCanonicalQueryString 对 params 按键名排序后执行百分比编码，
// 并拼接成规范化查询字符串（k1=v1&k2=v2…）。
// 当 includeSignature 为 false 时，Signature 参数不参与拼接（用于签名计算）；
// 为 true 时包含所有参数（用于最终 URL）。
func buildCanonicalQueryString(params map[string]string, includeSignature bool) string {
	keys := make([]string, 0, len(params))
	for k := range params {
		if !includeSignature && k == "Signature" {
			continue
		}

		keys = append(keys, k)
	}

	sort.Strings(keys)

	var buf strings.Builder
	buf.Grow(len(keys) * 40)

	for i, k := range keys {
		if i > 0 {
			buf.WriteByte('&')
		}

		buf.WriteString(percentEncode(k))
		buf.WriteByte('=')
		buf.WriteString(percentEncode(params[k]))
	}

	return buf.String()
}

// buildStringToSign 构造待签名字符串：
//
//	StringToSign = HTTPMethod + "&" + percentEncode("/") + "&" + percentEncode(CanonicalizedQueryString)
func buildStringToSign(httpMethod, canonicalQuery string) string {
	return httpMethod + "&" +
		percentEncode("/") + "&" +
		percentEncode(canonicalQuery)
}

// newUUID 生成 RFC 4122 UUID v4，用于 SignatureNonce。
func newUUID() (string, error) {
	var uuid [16]byte
	if _, err := io.ReadFull(rand.Reader, uuid[:]); err != nil {
		return "", err
	}

	uuid[6] = (uuid[6] & 0x0f) | 0x40
	uuid[8] = (uuid[8] & 0x3f) | 0x80

	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		uuid[0:4], uuid[4:6], uuid[6:8], uuid[8:10], uuid[10:16]), nil
}
