package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"eddisonso.com/edd-cloud-auth/internal/db"
	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/golang-jwt/jwt/v5"
)

// webauthnUser wraps db.User to implement webauthn.User interface
type webauthnUser struct {
	*db.User
	credentials []webauthn.Credential
}

func (u *webauthnUser) WebAuthnID() []byte {
	return []byte(u.UserID)
}

func (u *webauthnUser) WebAuthnName() string {
	return u.Username
}

func (u *webauthnUser) WebAuthnDisplayName() string {
	if u.DisplayName != "" {
		return u.DisplayName
	}
	return u.Username
}

func (u *webauthnUser) WebAuthnCredentials() []webauthn.Credential {
	return u.credentials
}

func (u *webauthnUser) WebAuthnIcon() string {
	return ""
}

// ceremonySession stores temporary state during WebAuthn ceremonies
type ceremonySession struct {
	data      *webauthn.SessionData
	userID    string
	expiresAt time.Time
}

func (h *Handler) loadWebAuthnUser(user *db.User) (*webauthnUser, error) {
	creds, err := h.db.GetCredentialsByUserID(user.UserID)
	if err != nil {
		return nil, err
	}

	webCreds := make([]webauthn.Credential, len(creds))
	for i, c := range creds {
		webCreds[i] = webauthn.Credential{
			ID:              c.ID,
			PublicKey:       c.PublicKey,
			AttestationType: c.AttestationType,
			Authenticator: webauthn.Authenticator{
				AAGUID:    c.AAGUID,
				SignCount: c.SignCount,
			},
		}
	}

	return &webauthnUser{
		User:        user,
		credentials: webCreds,
	}, nil
}

func generateCeremonyID() string {
	return fmt.Sprintf("%d", time.Now().UnixNano())
}

// CleanupCeremonies removes expired ceremony sessions
func (h *Handler) CleanupCeremonies() {
	h.ceremonies.Range(func(key, value any) bool {
		session := value.(*ceremonySession)
		if time.Now().After(session.expiresAt) {
			h.ceremonies.Delete(key)
		}
		return true
	})
}

// --- Registration (adding a key from settings) ---

func (h *Handler) handleAddKeyBegin(w http.ResponseWriter, r *http.Request) {
	claims := h.getClaims(r)

	user, err := h.db.GetUserByID(claims.UserID)
	if err != nil || user == nil {
		writeError(w, "user not found", http.StatusNotFound)
		return
	}

	wUser, err := h.loadWebAuthnUser(user)
	if err != nil {
		writeError(w, "failed to load credentials", http.StatusInternalServerError)
		return
	}

	options, session, err := h.webauthn.BeginRegistration(wUser)
	if err != nil {
		writeError(w, "failed to begin registration", http.StatusInternalServerError)
		return
	}

	sessionID := generateCeremonyID()
	h.ceremonies.Store(sessionID, &ceremonySession{
		data:      session,
		userID:    user.UserID,
		expiresAt: time.Now().Add(5 * time.Minute),
	})

	writeJSON(w, map[string]interface{}{
		"options": options,
		"state":   sessionID,
	})
}

func (h *Handler) handleAddKeyFinish(w http.ResponseWriter, r *http.Request) {
	claims := h.getClaims(r)

	var req struct {
		State      string                              `json:"state"`
		Credential *protocol.CredentialCreationResponse `json:"credential"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, "invalid request", http.StatusBadRequest)
		return
	}

	val, ok := h.ceremonies.LoadAndDelete(req.State)
	if !ok {
		writeError(w, "ceremony session not found or expired", http.StatusBadRequest)
		return
	}
	session := val.(*ceremonySession)
	if time.Now().After(session.expiresAt) {
		writeError(w, "ceremony session expired", http.StatusBadRequest)
		return
	}
	if session.userID != claims.UserID {
		writeError(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	user, err := h.db.GetUserByID(claims.UserID)
	if err != nil || user == nil {
		writeError(w, "user not found", http.StatusNotFound)
		return
	}

	wUser, err := h.loadWebAuthnUser(user)
	if err != nil {
		writeError(w, "failed to load credentials", http.StatusInternalServerError)
		return
	}

	ccr, err := req.Credential.Parse()
	if err != nil {
		writeError(w, "invalid credential", http.StatusBadRequest)
		return
	}

	credential, err := h.webauthn.CreateCredential(wUser, *session.data, ccr)
	if err != nil {
		writeError(w, "registration failed: "+err.Error(), http.StatusBadRequest)
		return
	}

	cred := &db.WebAuthnCredential{
		ID:              credential.ID,
		UserID:          claims.UserID,
		PublicKey:       credential.PublicKey,
		AttestationType: credential.AttestationType,
		AAGUID:          credential.Authenticator.AAGUID,
		SignCount:       credential.Authenticator.SignCount,
	}
	if err := h.db.CreateCredential(cred); err != nil {
		writeError(w, "failed to store credential", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

// --- 2FA Login ---

// TwoFAClaims represents the claims in a 2FA challenge token
type TwoFAClaims struct {
	UserID string `json:"user_id"`
	Type   string `json:"type"` // "2fa_challenge"
	jwt.RegisteredClaims
}

func (h *Handler) handleWebAuthnLoginBegin(w http.ResponseWriter, r *http.Request) {
	// Validate challenge token
	tokenString := h.extractToken(r)
	if tokenString == "" {
		writeError(w, "challenge token required", http.StatusUnauthorized)
		return
	}

	token, err := jwt.ParseWithClaims(tokenString, &TwoFAClaims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method")
		}
		return h.jwtSecret, nil
	})
	if err != nil {
		writeError(w, "invalid or expired challenge token", http.StatusUnauthorized)
		return
	}
	tfaClaims, ok := token.Claims.(*TwoFAClaims)
	if !ok || !token.Valid || tfaClaims.Type != "2fa_challenge" {
		writeError(w, "invalid challenge token", http.StatusUnauthorized)
		return
	}

	user, err := h.db.GetUserByID(tfaClaims.UserID)
	if err != nil || user == nil {
		writeError(w, "user not found", http.StatusNotFound)
		return
	}

	wUser, err := h.loadWebAuthnUser(user)
	if err != nil {
		writeError(w, "failed to load credentials", http.StatusInternalServerError)
		return
	}

	options, session, err := h.webauthn.BeginLogin(wUser)
	if err != nil {
		writeError(w, "failed to begin login", http.StatusInternalServerError)
		return
	}

	sessionID := generateCeremonyID()
	h.ceremonies.Store(sessionID, &ceremonySession{
		data:      session,
		userID:    user.UserID,
		expiresAt: time.Now().Add(5 * time.Minute),
	})

	writeJSON(w, map[string]interface{}{
		"options": options,
		"state":   sessionID,
	})
}

func (h *Handler) handleWebAuthnLoginFinish(w http.ResponseWriter, r *http.Request) {
	var req struct {
		State      string                               `json:"state"`
		Credential *protocol.CredentialAssertionResponse `json:"credential"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, "invalid request", http.StatusBadRequest)
		return
	}

	val, ok := h.ceremonies.LoadAndDelete(req.State)
	if !ok {
		writeError(w, "ceremony session not found or expired", http.StatusBadRequest)
		return
	}
	session := val.(*ceremonySession)
	if time.Now().After(session.expiresAt) {
		writeError(w, "ceremony session expired", http.StatusBadRequest)
		return
	}

	user, err := h.db.GetUserByID(session.userID)
	if err != nil || user == nil {
		writeError(w, "user not found", http.StatusNotFound)
		return
	}

	wUser, err := h.loadWebAuthnUser(user)
	if err != nil {
		writeError(w, "failed to load credentials", http.StatusInternalServerError)
		return
	}

	car, err := req.Credential.Parse()
	if err != nil {
		writeError(w, "invalid credential", http.StatusBadRequest)
		return
	}

	credential, err := h.webauthn.ValidateLogin(wUser, *session.data, car)
	if err != nil {
		writeError(w, "authentication failed", http.StatusUnauthorized)
		return
	}

	// Update sign count
	_ = h.db.UpdateCredentialSignCount(credential.ID, credential.Authenticator.SignCount)

	// Create full session (same as normal login)
	clientIP := h.getClientIP(r)
	expires := time.Now().Add(h.sessionTTL)
	jwtClaims := JWTClaims{
		Username:    user.Username,
		DisplayName: user.DisplayName,
		UserID:      user.UserID,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expires),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Subject:   user.Username,
		},
	}

	jwtToken := jwt.NewWithClaims(jwt.SigningMethodHS256, jwtClaims)
	tokenString, err := jwtToken.SignedString(h.jwtSecret)
	if err != nil {
		writeError(w, "failed to create session", http.StatusInternalServerError)
		return
	}

	sess, err := h.db.CreateSession(user.UserID, tokenString, expires, clientIP)
	if err != nil {
		writeError(w, "failed to create session", http.StatusInternalServerError)
		return
	}

	if h.publisher != nil {
		h.publisher.PublishSessionCreated(sess.ID, user.UserID, expires)
	}

	writeJSON(w, loginResponse{
		Username:    user.Username,
		DisplayName: user.DisplayName,
		UserID:      user.UserID,
		IsAdmin:     h.isAdmin(user.Username),
		Token:       tokenString,
	})
}
