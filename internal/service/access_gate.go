package service

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"strconv"
	"strings"
	"time"

	"github.com/ppxb/miyabi/internal/domain"
)

var ErrAccessPassword = domain.E(domain.KindUnauthorized, "访问密码错误", nil)

// accessSessionTTL bounds how long a signed cookie stays valid. Self-hosted
// single-user deployments favour convenience over short re-login cycles.
const accessSessionTTL = 30 * 24 * time.Hour

// AccessGateService checks the optional Web entry password and issues the
// signed cookie every other API route requires.
type AccessGateService struct {
	enabled      bool
	passwordHash [sha256.Size]byte
}

func NewAccessGateService(password string) *AccessGateService {
	return &AccessGateService{
		enabled: password != "", passwordHash: sha256.Sum256([]byte(password)),
	}
}

func (gate *AccessGateService) Enabled() bool {
	return gate.enabled
}

func (gate *AccessGateService) Verify(password string) error {
	if !gate.enabled {
		return nil
	}
	actual := sha256.Sum256([]byte(password))
	if subtle.ConstantTimeCompare(gate.passwordHash[:], actual[:]) != 1 {
		return ErrAccessPassword
	}
	return nil
}

// IssueSession returns a token that carries its own expiry and signature, so
// the server keeps no session table and a restart does not sign visitors out.
func (gate *AccessGateService) IssueSession() (string, time.Time) {
	expires := time.Now().Add(accessSessionTTL)
	deadline := strconv.FormatInt(expires.Unix(), 10)
	return deadline + "." + gate.sign(deadline), expires
}

func (gate *AccessGateService) ValidSession(token string) bool {
	deadline, signature, found := strings.Cut(token, ".")
	if !found {
		return false
	}
	// Compare before parsing: a forged token must not reach strconv.
	if subtle.ConstantTimeCompare([]byte(signature), []byte(gate.sign(deadline))) != 1 {
		return false
	}
	seconds, err := strconv.ParseInt(deadline, 10, 64)
	if err != nil {
		return false
	}
	return time.Now().Unix() < seconds
}

// sign binds an expiry to the password, so changing MIYABI_ACCESS_PASSWORD
// invalidates every cookie handed out under the previous one.
func (gate *AccessGateService) sign(payload string) string {
	mac := hmac.New(sha256.New, gate.passwordHash[:])
	mac.Write([]byte(payload))
	return hex.EncodeToString(mac.Sum(nil))
}
