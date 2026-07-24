package apsara

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestSign_DescribeRegions(t *testing.T) {
	cred := Credential{
		AccessKeyID:     "testid",
		AccessKeySecret: "testsecret",
	}
	signer := NewSigner(cred)

	params := map[string]string{
		"Action":           "DescribeRegions",
		"Format":           "XML",
		"Version":          "2014-05-26",
		"AccessKeyId":      "testid",
		"SignatureMethod":  "HMAC-SHA1",
		"SignatureNonce":   "3ee8c1b8-83d3-44af-a94f-4e0ad82fd6cf",
		"SignatureVersion": "1.0",
		"Timestamp":        "2016-02-23T12:46:24Z",
	}

	if err := signer.Sign("GET", params); err != nil {
		t.Fatal(err)
	}

	sig := params["Signature"]
	t.Logf("Signature: %s", sig)

	if sig == "" {
		t.Error("signature should not be empty")
	}

	if len(sig) < 8 {
		t.Errorf("signature too short: %s", sig)
	}
}

func TestSign_ExactStringToSign(t *testing.T) {
	params := map[string]string{
		"Action":           "DescribeRegions",
		"Format":           "XML",
		"Version":          "2014-05-26",
		"AccessKeyId":      "testid",
		"SignatureMethod":  "HMAC-SHA1",
		"SignatureNonce":   "3ee8c1b8-83d3-44af-a94f-4e0ad82fd6cf",
		"SignatureVersion": "1.0",
		"Timestamp":        "2016-02-23T12:46:24Z",
	}

	canonical := buildCanonicalQueryString(params, false)
	t.Logf("CanonicalQuery: %s", canonical)

	stringToSign := buildStringToSign("GET", canonical)
	t.Logf("StringToSign: %s", stringToSign)

	if !strings.HasPrefix(stringToSign, "GET&") {
		t.Errorf("StringToSign should start with GET&, got: %s", stringToSign[:4])
	}
}

func TestSign_HMAC(t *testing.T) {
	cred := Credential{
		AccessKeyID:     "testid",
		AccessKeySecret: "testsecret",
	}
	signer := NewSigner(cred)

	params := map[string]string{
		"Action":           "DescribeRegions",
		"Format":           "XML",
		"Version":          "2014-05-26",
		"AccessKeyId":      "testid",
		"SignatureMethod":  "HMAC-SHA1",
		"SignatureNonce":   "3ee8c1b8-83d3-44af-a94f-4e0ad82fd6cf",
		"SignatureVersion": "1.0",
		"Timestamp":        "2016-02-23T12:46:24Z",
	}

	if err := signer.Sign("GET", params); err != nil {
		t.Fatal(err)
	}

	t.Logf("Generated Signature: %s", params["Signature"])

	_, err := base64.StdEncoding.DecodeString(params["Signature"])
	if err != nil {
		t.Errorf("signature is not valid base64: %v", err)
	}
}

func TestPercentEncode(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"abc", "abc"},
		{"ABC", "ABC"},
		{"123", "123"},
		{"-_.~", "-_.~"},
		{" ", "%20"},
		{"/", "%2F"},
		{"+", "%2B"},
		{"*", "%2A"},
		{"中文", "%E4%B8%AD%E6%96%87"},
		{"a=b", "a%3Db"},
		{"a&b", "a%26b"},
		{"\x00", "%00"},
		{"\xff", "%FF"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := percentEncode(tt.input)
			if got != tt.expected {
				t.Errorf("percentEncode(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestBuildCanonicalQueryString(t *testing.T) {
	params := map[string]string{
		"Action":  "DescribeRegions",
		"Version": "2014-05-26",
		"Format":  "XML",
	}
	got := buildCanonicalQueryString(params, false)

	expected := "Action=DescribeRegions&Format=XML&Version=2014-05-26"
	if got != expected {
		t.Errorf("got %q, want %q", got, expected)
	}
}

func TestBuildStringToSign(t *testing.T) {
	canonical := "Action=DescribeRegions&Format=XML&Version=2014-05-26"
	s := buildStringToSign("GET", canonical)
	t.Logf("StringToSign: %s", s)

	if s == "" {
		t.Error("string to sign should not be empty")
	}

	expected := "GET&%2F&Action%3DDescribeRegions%26Format%3DXML%26Version%3D2014-05-26"
	if s != expected {
		t.Errorf("got %q, want %q", s, expected)
	}
}

func TestBuildCanonicalQuery_ExcludesSignature(t *testing.T) {
	params := map[string]string{
		"Action":    "DescribeInstances",
		"Signature": "some-sig-value",
		"Format":    "JSON",
	}

	canonical := buildCanonicalQueryString(params, false)
	if strings.Contains(canonical, "Signature") {
		t.Errorf("canonical query should not contain Signature, got: %s", canonical)
	}
}

func TestBuildCanonicalQuery_IncludesSignature(t *testing.T) {
	params := map[string]string{
		"Action":    "DescribeInstances",
		"Signature": "some-sig-value",
		"Format":    "JSON",
	}

	full := buildCanonicalQueryString(params, true)
	if !strings.Contains(full, "Signature") {
		t.Errorf("full query should contain Signature, got: %s", full)
	}

	if !strings.Contains(full, "Signature="+percentEncode("some-sig-value")) {
		t.Errorf("full query should contain encoded Signature, got: %s", full)
	}
}

func TestBuildCanonicalQuery_SignatureOrder(t *testing.T) {
	params := map[string]string{
		"A":         "a",
		"Z":         "z",
		"Signature": "sig",
		"M":         "m",
	}
	full := buildCanonicalQueryString(params, true)

	expected := "A=a&M=m&Signature=sig&Z=z"
	if full != expected {
		t.Errorf("got %q, want %q", full, expected)
	}
}

func TestParseErrorResponse(t *testing.T) {
	body := []byte(`{"RequestId":"r-abc","Code":"InvalidParam","Message":"bad param"}`)

	ae := parseErrorResponse("TestOp", 400, body)
	if ae.RequestID != "r-abc" {
		t.Errorf("RequestID = %q, want %q", ae.RequestID, "r-abc")
	}

	if ae.Code != "InvalidParam" {
		t.Errorf("Code = %q, want %q", ae.Code, "InvalidParam")
	}

	if ae.Message != "bad param" {
		t.Errorf("Message = %q, want %q", ae.Message, "bad param")
	}

	if ae.StatusCode != 400 {
		t.Errorf("StatusCode = %d, want %d", ae.StatusCode, 400)
	}
}

func TestExtractRequestID(t *testing.T) {
	body := []byte(`{"RequestId":"r-xyz","Data":"ok"}`)

	rid, ok := extractRequestID(body)
	if !ok || rid != "r-xyz" {
		t.Errorf("got (%q, %v), want (%q, true)", rid, ok, "r-xyz")
	}
}
