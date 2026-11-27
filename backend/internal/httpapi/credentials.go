package httpapi

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/echoflying/nominatim-cache/backend/internal/store"
)

func (s Server) listBatchCredentials(w http.ResponseWriter, r *http.Request) {
	items, err := s.Store.ListBatchCredentials(r.Context())
	if err != nil {
		operationError(w, err, "Failed to list batch credentials")
		return
	}
	adminOK(w, items)
}

func (s Server) createBatchCredential(w http.ResponseWriter, r *http.Request) {
	var input struct {
		CallerID string `json:"caller_id"`
		Label    string `json:"label"`
	}
	if err := decodeBody(w, r, &input); err != nil {
		adminError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	input.CallerID, input.Label = strings.TrimSpace(input.CallerID), strings.TrimSpace(input.Label)
	if !store.ValidBatchCallerID(input.CallerID) || input.Label == "" || len([]rune(input.Label)) > 100 {
		adminError(w, http.StatusBadRequest, "调用方 ID 需以小写字母开头，并仅使用小写字母、数字、点、下划线或短横线；说明最多 100 个字符")
		return
	}
	item, token, err := s.Store.CreateBatchCredential(r.Context(), input.CallerID, input.Label)
	if err != nil {
		operationError(w, err, "Failed to create batch credential")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	adminOK(w, struct {
		Credential store.BatchCredential `json:"credential"`
		Token      string                `json:"token"`
		Notice     string                `json:"notice"`
	}{item, token, "密钥只显示这一次。请立即复制并保存。"})
}

func (s Server) revokeBatchCredential(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		adminError(w, http.StatusBadRequest, "Invalid credential ID")
		return
	}
	if err := s.Store.RevokeBatchCredential(r.Context(), id); err != nil {
		if err == store.ErrCredentialNotFound {
			adminError(w, http.StatusNotFound, "Credential not found or already revoked")
			return
		}
		operationError(w, err, "Failed to revoke batch credential")
		return
	}
	adminOK(w, map[string]any{"id": id, "revoked": true})
}
