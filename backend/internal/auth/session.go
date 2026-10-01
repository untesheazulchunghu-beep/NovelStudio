package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const cookieName = "novel_session"

type Sessions struct {
	secret []byte
	secure bool
}

func NewSessions(secret []byte, secure bool) *Sessions {
	return &Sessions{secret: secret, secure: secure}
}

func (s *Sessions) Set(w http.ResponseWriter, userID string) {
	s.SetVersion(w, userID, 1)
}

func (s *Sessions) SetVersion(w http.ResponseWriter, userID string, version int64) {
	body := base64.RawURLEncoding.EncodeToString([]byte(userID + "|" + strconv.FormatInt(version, 10) + "|" + strconv.FormatInt(time.Now().Add(7*24*time.Hour).Unix(), 10)))
	mac := hmac.New(sha256.New, s.secret)
	mac.Write([]byte(body))
	token := body + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	http.SetCookie(w, &http.Cookie{
		Name:     cookieName,
		Value:    token,
		Path:     "/",
		Expires:  time.Now().Add(7 * 24 * time.Hour),
		MaxAge:   7 * 24 * 60 * 60,
		HttpOnly: true,
		Secure:   s.secure,
		SameSite: http.SameSiteLaxMode,
	})
}

func (s *Sessions) Clear(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     cookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   s.secure,
		SameSite: http.SameSiteLaxMode,
	})
}

func (s *Sessions) UserID(r *http.Request) (string, bool) {
	id, _, ok := s.UserIDVersion(r)
	return id, ok
}

func (s *Sessions) UserIDVersion(r *http.Request) (string, int64, bool) {
	cookie, err := r.Cookie(cookieName)
	if err != nil {
		return "", 0, false
	}
	parts := strings.SplitN(cookie.Value, ".", 2)
	if len(parts) != 2 {
		return "", 0, false
	}
	signature, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return "", 0, false
	}
	mac := hmac.New(sha256.New, s.secret)
	mac.Write([]byte(parts[0]))
	if !hmac.Equal(signature, mac.Sum(nil)) {
		return "", 0, false
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return "", 0, false
	}
	identity := strings.Split(string(payload), "|")
	if len(identity) != 3 {
		return "", 0, false
	}
	version, versionErr := strconv.ParseInt(identity[1], 10, 64)
	expiresAt, err := strconv.ParseInt(identity[2], 10, 64)
	if versionErr != nil || err != nil || time.Now().Unix() >= expiresAt || identity[0] == "" {
		return "", 0, false
	}
	return identity[0], version, true
}
