package skport

import (
	"crypto/hmac"
	"crypto/md5" // #nosec G501 -- required by the upstream SKPORT signing protocol, not used for password hashing.
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

func HeaderJSON(platform, timestamp, vName string) string {
	return fmt.Sprintf(`{"platform":"%s","timestamp":"%s","dId":"","vName":"%s"}`, platform, timestamp, vName)
}

func GenerateSign(path, body, timestamp, token, platform, vName string) string {
	payload := path + body + timestamp + HeaderJSON(platform, timestamp, vName)
	mac := hmac.New(sha256.New, []byte(token))
	_, _ = mac.Write([]byte(payload))
	hmacHex := hex.EncodeToString(mac.Sum(nil))
	// MD5 is an API compatibility step in SKPORT's signing scheme, not an integrity design choice.
	digest := md5.Sum([]byte(hmacHex)) // #nosec G401
	return hex.EncodeToString(digest[:])
}
