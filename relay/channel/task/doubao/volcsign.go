package doubao

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/url"
	"strings"
	"time"
)

// Volcengine / BytePlus universal-OpenAPI signer (HMAC-SHA256), standalone (no gin
// coupling), parameterized by region/service. Mirrors the algorithm used by
// relay/channel/jimeng/sign.go and bragi-canvas src/providers/volcengine-sig.ts, but
// for the BytePlus Ark asset library (region ap-southeast-1, service "ark").
//
// Endpoint: https://open.volcengineapi.com/?Action=<action>&Version=<version>

const volcAPIHost = "open.volcengineapi.com"

func volcHMAC(key, data []byte) []byte {
	h := hmac.New(sha256.New, key)
	h.Write(data)
	return h.Sum(nil)
}

func volcSHA256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// signVolcRequest signs a Volcengine universal-API POST request and returns the full
// request URL plus the headers to set.
func signVolcRequest(accessKey, secretKey, region, service, action, version, body string) (string, map[string]string) {
	reqURL := fmt.Sprintf("https://%s/?Action=%s&Version=%s",
		volcAPIHost, url.QueryEscape(action), url.QueryEscape(version))

	now := time.Now().UTC()
	xDate := now.Format("20060102T150405Z")
	shortDate := now.Format("20060102")

	bodyHash := volcSHA256Hex([]byte(body))

	// Canonical query string — keys sorted (Action < Version).
	canonicalQuery := fmt.Sprintf("Action=%s&Version=%s",
		url.QueryEscape(action), url.QueryEscape(version))

	// Canonical headers (sorted: content-type, host, x-content-sha256, x-date).
	headers := map[string]string{
		"content-type":     "application/json",
		"host":             volcAPIHost,
		"x-content-sha256": bodyHash,
		"x-date":           xDate,
	}
	sortedKeys := []string{"content-type", "host", "x-content-sha256", "x-date"}
	var canonicalHeaders strings.Builder
	for _, k := range sortedKeys {
		canonicalHeaders.WriteString(k)
		canonicalHeaders.WriteString(":")
		canonicalHeaders.WriteString(headers[k])
		canonicalHeaders.WriteString("\n")
	}
	signedHeaders := strings.Join(sortedKeys, ";")

	canonicalRequest := strings.Join([]string{
		"POST",
		"/",
		canonicalQuery,
		canonicalHeaders.String(),
		signedHeaders,
		bodyHash,
	}, "\n")

	credentialScope := fmt.Sprintf("%s/%s/%s/request", shortDate, region, service)
	stringToSign := strings.Join([]string{
		"HMAC-SHA256",
		xDate,
		credentialScope,
		volcSHA256Hex([]byte(canonicalRequest)),
	}, "\n")

	kDate := volcHMAC([]byte(secretKey), []byte(shortDate))
	kRegion := volcHMAC(kDate, []byte(region))
	kService := volcHMAC(kRegion, []byte(service))
	kSigning := volcHMAC(kService, []byte("request"))
	signature := hex.EncodeToString(volcHMAC(kSigning, []byte(stringToSign)))

	authorization := fmt.Sprintf("HMAC-SHA256 Credential=%s/%s, SignedHeaders=%s, Signature=%s",
		accessKey, credentialScope, signedHeaders, signature)

	return reqURL, map[string]string{
		"Content-Type":     "application/json",
		"Host":             volcAPIHost,
		"X-Date":           xDate,
		"X-Content-Sha256": bodyHash,
		"Authorization":    authorization,
	}
}
