package webhook

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"

	"hookly/internal/db"

	"github.com/go-chi/chi/v5"
	gonanoid "github.com/matoous/go-nanoid/v2"
)

const maxPayloadSize = 100 * 1024 * 1024 // 100MB

// Handler handles webhook ingestion.
type Handler struct {
	queries       *db.Queries
	secretManager *db.SecretManager
}

// NewHandler creates a new webhook handler.
func NewHandler(queries *db.Queries, secretManager *db.SecretManager) *Handler {
	return &Handler{
		queries:       queries,
		secretManager: secretManager,
	}
}

// ServeHTTP handles incoming webhooks at POST /h/{endpoint-id}
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	endpointID := chi.URLParam(r, "endpointID")
	if endpointID == "" {
		http.Error(w, "Endpoint ID required", http.StatusBadRequest)
		return
	}

	ctx := r.Context()

	// Look up endpoint
	endpoint, err := h.queries.GetEndpointByID(ctx, endpointID)
	if err != nil {
		slog.Debug("endpoint not found", "endpoint_id", endpointID, "error", err)
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}

	// Check if muted
	if endpoint.Muted != 0 {
		slog.Debug("endpoint is muted, ignoring webhook", "endpoint_id", endpointID)
		w.WriteHeader(http.StatusOK)
		return
	}

	// Read payload with size limit
	r.Body = http.MaxBytesReader(w, r.Body, maxPayloadSize)
	payload, err := io.ReadAll(r.Body)
	if err != nil {
		slog.Warn("failed to read payload", "error", err)
		http.Error(w, "Payload too large", http.StatusRequestEntityTooLarge)
		return
	}

	// Extract headers
	headers := make(map[string]string)
	for name, values := range r.Header {
		if len(values) > 0 {
			headers[name] = values[0]
		}
	}

	// Verify signature
	secret, err := h.secretManager.DecryptSecret(endpoint.SignatureSecretEncrypted)
	if err != nil {
		slog.Error("failed to decrypt secret", "endpoint_id", endpointID, "error", err)
		// Still store webhook but mark as invalid
		h.storeWebhook(ctx, endpointID, headers, payload, false)
		w.WriteHeader(http.StatusOK)
		return
	}

	verifier := NewVerifier(endpoint.ProviderType)
	signatureValid := verifier.Verify(payload, headers, secret)

	if !signatureValid {
		slog.Warn("webhook signature verification failed",
			"endpoint_id", endpointID,
			"provider_type", endpoint.ProviderType,
		)
	}

	// Store webhook
	webhookID, err := h.storeWebhook(ctx, endpointID, headers, payload, signatureValid)
	if err != nil {
		slog.Error("failed to store webhook", "error", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	slog.Info("webhook received",
		"webhook_id", webhookID,
		"endpoint_id", endpointID,
		"signature_valid", signatureValid,
		"payload_size", len(payload),
	)

	w.WriteHeader(http.StatusOK)
}

func (h *Handler) storeWebhook(ctx context.Context, endpointID string, headers map[string]string, payload []byte, signatureValid bool) (string, error) {
	webhookID, err := gonanoid.New()
	if err != nil {
		return "", err
	}

	headersJSON, err := json.Marshal(headers)
	if err != nil {
		return "", err
	}

	sigValid := int64(0)
	if signatureValid {
		sigValid = 1
	}

	_, err = h.queries.CreateWebhook(ctx, db.CreateWebhookParams{
		ID:             webhookID,
		EndpointID:     endpointID,
		Headers:        string(headersJSON),
		Payload:        payload,
		SignatureValid: sigValid,
	})
	if err != nil {
		return "", err
	}

	return webhookID, nil
}
