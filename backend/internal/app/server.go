package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Server struct {
	cfg    Config
	store  *Store
	mux    *http.ServeMux
}

func NewServer(ctx context.Context, cfg Config) (*Server, error) {
	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return nil, err
	}
	store := NewStore(pool)
	if err := store.Ping(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	if err := store.EnsureSchema(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	if err := store.Seed(ctx, cfg.OfficerID, cfg.MemberID, cfg.MemberPlate, cfg.MemberBalance); err != nil {
		pool.Close()
		return nil, err
	}
	server := &Server{cfg: cfg, store: store, mux: http.NewServeMux()}
	server.routes()
	return server, nil
}

func (s *Server) Close() {
	s.store.pool.Close()
}

func (s *Server) Handler() http.Handler {
	return s.withMiddleware(s.mux)
}

func (s *Server) routes() {
	s.mux.HandleFunc("/healthz", s.handleHealth)
	s.mux.HandleFunc("/api/bootstrap", s.handleBootstrap)
	s.mux.HandleFunc("/api/users", s.handleUsers)
	s.mux.HandleFunc("/api/rules/active", s.handleActiveRule)
	s.mux.HandleFunc("/api/rules", s.handleRules)
	s.mux.HandleFunc("/api/violations", s.handleViolations)
	s.mux.HandleFunc("/api/violations/", s.handleViolationActions)
	s.mux.HandleFunc("/api/notifications", s.handleNotifications)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleBootstrap(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	currentUser, err := s.currentUser(r.Context(), r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err.Error())
		return
	}
	users, err := s.store.ListUsers(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	rule, err := s.store.ActiveRuleVersion(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	violations, err := s.store.ListViolations(r.Context(), currentUser)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	payments := make([]PaymentRecord, 0)
	for _, violation := range violations {
		if violation.PaymentTransaction != nil {
			payments = append(payments, *violation.PaymentTransaction)
		}
	}
	notifications, err := s.store.ListNotifications(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"current_user":  currentUser,
		"users":         users,
		"active_rule":    rule,
		"violations":     violations,
		"payments":       payments,
		"notifications":  notifications,
	})
}

func (s *Server) handleUsers(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	users, err := s.store.ListUsers(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, users)
}

func (s *Server) handleActiveRule(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	rule, err := s.store.ActiveRuleVersion(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, rule)
}

func (s *Server) handleRules(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		rules, err := s.store.ListRuleVersions(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, rules)
	case http.MethodPost:
		currentUser, err := s.currentUser(r.Context(), r)
		if err != nil {
			writeError(w, http.StatusUnauthorized, err.Error())
			return
		}
		if currentUser.Role != RoleOfficer {
			writeError(w, http.StatusForbidden, "officer role required")
			return
		}
		var input RuleInput
		if err := decodeRequest(r, &input); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		rule := FineRules{BaseAmounts: input.BaseAmounts, TimeWindows: input.TimeWindows, RepeatMultipliers: input.RepeatMultipliers}
		published, err := s.store.PublishRuleVersion(r.Context(), rule)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusCreated, published)
	default:
		methodNotAllowed(w)
	}
}

func (s *Server) handleViolations(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		currentUser, err := s.currentUser(r.Context(), r)
		if err != nil {
			writeError(w, http.StatusUnauthorized, err.Error())
			return
		}
		violations, err := s.store.ListViolations(r.Context(), currentUser)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, violations)
	case http.MethodPost:
		currentUser, err := s.currentUser(r.Context(), r)
		if err != nil {
			writeError(w, http.StatusUnauthorized, err.Error())
			return
		}
		if currentUser.Role != RoleOfficer {
			writeError(w, http.StatusForbidden, "officer role required")
			return
		}
		var input ViolationInput
		if err := decodeRequest(r, &input); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if strings.TrimSpace(input.Plate) == "" || strings.TrimSpace(input.Location) == "" || strings.TrimSpace(input.PhotoData) == "" {
			writeError(w, http.StatusBadRequest, "plate, location, and photo_data are required")
			return
		}
		violation, notification, err := s.store.CreateViolation(r.Context(), input)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusCreated, map[string]any{
			"violation":    violation,
			"notification": notification,
		})
	default:
		methodNotAllowed(w)
	}
}

func (s *Server) handleViolationActions(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/violations/")
	if strings.HasSuffix(path, "/pay") {
		violationID := strings.TrimSuffix(path, "/pay")
		s.handlePayment(w, r, strings.TrimSuffix(violationID, "/"))
		return
	}
	writeError(w, http.StatusNotFound, "not found")
}

func (s *Server) handlePayment(w http.ResponseWriter, r *http.Request, violationID string) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	currentUser, err := s.currentUser(r.Context(), r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err.Error())
		return
	}
	if currentUser.Role != RoleMember {
		writeError(w, http.StatusForbidden, "member role required")
		return
	}
	var input struct {
		Scenario PaymentScenario `json:"scenario"`
	}
	if err := decodeRequest(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if input.Scenario != PaymentScenarioSuccess && input.Scenario != PaymentScenarioFailed {
		writeError(w, http.StatusBadRequest, "scenario must be success or failed")
		return
	}
	violations, err := s.store.ListViolationsByIDs(r.Context(), []string{violationID})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if len(violations) == 0 {
		writeError(w, http.StatusNotFound, "violation not found")
		return
	}
	violation := violations[0]
	if violation.OwnerUserID == nil || *violation.OwnerUserID != currentUser.ID {
		writeError(w, http.StatusForbidden, "violation does not belong to this member")
		return
	}
	status := PaymentFailed
	if input.Scenario == PaymentScenarioSuccess {
		status = PaymentPaid
	}
	payment, err := s.recordMockPayment(r.Context(), violation, currentUser, input.Scenario, status)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	updated, err := s.store.ListViolationsByIDs(r.Context(), []string{violationID})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"payment":   payment,
		"violation": updated[0],
	})
}

func (s *Server) handleNotifications(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	notifications, err := s.store.ListNotifications(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, notifications)
}

func (s *Server) recordMockPayment(ctx context.Context, violation Violation, member *User, scenario PaymentScenario, status PaymentStatus) (*PaymentRecord, error) {
	transactionID := fmt.Sprintf("txn_%s", uuid.NewString())
	if status == PaymentPaid && member.BalanceCents < violation.FineCents {
		return nil, fmt.Errorf("insufficient account balance")
	}
	payment, err := s.store.RecordPayment(ctx, violation.ID, member, violation.FineCents, scenario, status, transactionID)
	if err != nil {
		return nil, err
	}
	return payment, nil
}

func (s *Server) currentUser(ctx context.Context, r *http.Request) (*User, error) {
	userID := strings.TrimSpace(r.Header.Get("X-User-ID"))
	if userID == "" {
		return nil, errors.New("missing X-User-ID header")
	}
	user, err := s.store.GetUser(ctx, userID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, errors.New("unknown user")
		}
		return nil, err
	}
	return user, nil
}

func (s *Server) withMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin == "" {
			origin = s.cfg.FrontendOrigin
		}
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-User-ID")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Credentials", "true")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func decodeRequest(r *http.Request, target any) error {
	defer r.Body.Close()
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	return nil
}

func writeJSON(w http.ResponseWriter, statusCode int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, statusCode int, message string) {
	writeJSON(w, statusCode, map[string]string{"error": message})
}

func methodNotAllowed(w http.ResponseWriter) {
	writeError(w, http.StatusMethodNotAllowed, "method not allowed")
}

func (s *Server) LogStartup() {
	log.Printf("api gateway listening on %s", s.cfg.Address)
}
