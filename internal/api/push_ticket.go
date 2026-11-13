package api

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/neul-labs/m9m/internal/auth"
)

// The push channel (the WebSocket behind /api/v1/push) is the one
// endpoint the browser cannot authenticate the usual way: the
// WebSocket API of the browser cannot set an Authorization header.
//
// Instead of weakening the auth middleware, the client exchanges its
// bearer token for a one-time ticket:
//
//	POST /api/v1/push/ticket   (bearer token, ordinary API call)
//	  -> {"ticket":"...","expiresIn":30}
//	GET  /api/v1/push?ticket=...  (the ticket authenticates the handshake)
//
// The ticket is random, single-use and short-lived: it authorises
// exactly ONE handshake and expires 30 seconds after it was minted. No
// long-lived credential ever travels in the URL.
const (
	// pushTicketTTL is how long a push ticket stays usable.
	pushTicketTTL = 30 * time.Second

	// pushTicketBytes is the entropy of a push ticket.
	pushTicketBytes = 32
)

// pushTicket is an outstanding connection ticket.
type pushTicket struct {
	userID    string
	expiresAt time.Time
}

// pushTicketStore holds the outstanding push tickets.
//
// The raw ticket never lives in the store - only its SHA-256 digest
// does, so a memory dump of the process cannot be replayed as a
// credential.
type pushTicketStore struct {
	mu      sync.Mutex
	tickets map[string]pushTicket
	ttl     time.Duration
}

// newPushTicketStore creates an empty ticket store.
func newPushTicketStore(ttl time.Duration) *pushTicketStore {
	return &pushTicketStore{
		tickets: make(map[string]pushTicket),
		ttl:     ttl,
	}
}

// issue mints a new ticket for the given user.
func (s *pushTicketStore) issue(userID string) (string, error) {
	raw, err := auth.GenerateRandomToken(pushTicketBytes)
	if err != nil {
		return "", err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.purgeExpiredLocked()
	s.tickets[hashPushTicket(raw)] = pushTicket{
		userID:    userID,
		expiresAt: time.Now().Add(s.ttl),
	}
	return raw, nil
}

// consume validates a ticket and burns it: a ticket authorises
// exactly one handshake, no matter what happens next.
func (s *pushTicketStore) consume(raw string) (string, bool) {
	if raw == "" {
		return "", false
	}
	key := hashPushTicket(raw)

	s.mu.Lock()
	defer s.mu.Unlock()

	ticket, ok := s.tickets[key]
	if !ok {
		return "", false
	}
	// Burn it first: the ticket is spent even when it turns out to be
	// expired.
	delete(s.tickets, key)

	if time.Now().After(ticket.expiresAt) {
		return "", false
	}
	return ticket.userID, true
}

// purgeExpiredLocked drops the tickets that are past their TTL.
func (s *pushTicketStore) purgeExpiredLocked() {
	now := time.Now()
	for key, ticket := range s.tickets {
		if now.After(ticket.expiresAt) {
			delete(s.tickets, key)
		}
	}
}

// hashPushTicket returns the digest a ticket is stored under.
func hashPushTicket(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

// pushTicketStoreFor returns the ticket store of the server.
//
// The lazy init keeps a zero-value APIServer (some unit tests build
// one by hand) usable.
func (s *APIServer) pushTicketStoreFor() *pushTicketStore {
	s.pushTicketsMu.Lock()
	defer s.pushTicketsMu.Unlock()

	if s.pushTickets == nil {
		s.pushTickets = newPushTicketStore(pushTicketTTL)
	}
	return s.pushTickets
}

// HandlePushTicket issues a one-time ticket for the push WebSocket.
//
// The endpoint is registered on the /api/v1 subrouter, so it sits
// behind the regular authentication middleware: only an authenticated
// caller can mint a ticket.
func (s *APIServer) HandlePushTicket(w http.ResponseWriter, r *http.Request) {
	userID := ""
	if authCtx := AuthFromContext(r.Context()); authCtx != nil {
		userID = authCtx.GetUserID()
	}

	store := s.pushTicketStoreFor()
	ticket, err := store.issue(userID)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":true,"message":"could not issue a push ticket","code":500}`))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"ticket":    ticket,
		"expiresIn": int(store.ttl.Seconds()),
	})
}

// HandlePushWebSocket authenticates the push handshake and hands over
// to the WebSocket handler.
//
// A handshake without a valid ticket is rejected with 401: the push
// channel is never open to an unauthenticated caller.
func (s *APIServer) HandlePushWebSocket(w http.ResponseWriter, r *http.Request) {
	userID, ok := s.pushTicketStoreFor().consume(r.URL.Query().Get("ticket"))
	if !ok {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":true,"message":"push authentication required","code":401}`))
		return
	}

	// The ticket belongs to a session; the stream itself is the
	// authenticated part from here on.
	_ = userID
	s.HandleWebSocket(w, r)
}
