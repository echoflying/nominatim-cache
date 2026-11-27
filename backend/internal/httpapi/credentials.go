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
	if len(s.Config.CredentialEncryptionKey) != 32 {
		adminError(w, http.StatusServiceUnavailable, "凭据加密密钥未配置")
		return
	}
	item, token, err := s.Store.CreateBatchCredential(r.Context(), input.CallerID, input.Label, s.Config.CredentialEncryptionKey)
	if err != nil {
		operationError(w, err, "Failed to create batch credential")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	adminOK(w, struct {
		Credential store.BatchCredential `json:"credential"`
		Token      string                `json:"token"`
		Notice     string                `json:"notice"`
	}{item, token, "密钥已加密保存，之后可在列表中按需查看。"})
}

func (s Server) revealBatchCredential(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		adminError(w, http.StatusBadRequest, "Invalid credential ID")
		return
	}
	if len(s.Config.CredentialEncryptionKey) != 32 {
		adminError(w, http.StatusServiceUnavailable, "凭据加密密钥未配置")
		return
	}
	token, err := s.Store.RevealBatchCredential(r.Context(), id, s.Config.CredentialEncryptionKey)
	if err != nil {
		switch err {
		case store.ErrCredentialNotFound:
			adminError(w, http.StatusNotFound, "Credential not found")
		case store.ErrCredentialSecretUnavailable:
			adminError(w, http.StatusConflict, "这把凭据在启用可查看存储前创建，请新建替代密钥")
		default:
			operationError(w, err, "Failed to reveal batch credential")
		}
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	adminOK(w, map[string]any{"id": id, "token": token})
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
