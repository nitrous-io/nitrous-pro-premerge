package messageverifier

import (
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"fmt"
)

// MessageVerifier is currently missing a "Verify()" method because we don't
// have a use for it yet (i.e. we only generate signed messages).
type MessageVerifier struct {
	secret string
}

func (v *MessageVerifier) Generate(data ...interface{}) (string, error) {
	b, err := json.Marshal(data)
	if err != nil {
		return "", err
	}
	encoded := base64.StdEncoding.EncodeToString(b)
	signature := v.sign(encoded)
	return fmt.Sprintf("%s--%x", encoded, signature), nil
}

func (v *MessageVerifier) sign(s string) string {
	mac := hmac.New(sha1.New, []byte(v.secret))
	mac.Write([]byte(s))
	return string(mac.Sum(nil))
}

func New(secret string) *MessageVerifier {
	return &MessageVerifier{secret: secret}
}
