package novel

import (
	"context"
	"errors"
	"net/http"
	"time"

	"novelstudio/internal/llm"
)

func (s *Server) getLLMSettings(w http.ResponseWriter, r *http.Request) {
	userID, ok := s.userID(w, r)
	if !ok {
		return
	}
	if s.llmRuntime == nil {
		writeError(w, http.StatusServiceUnavailable, "SETTINGS_UNAVAILABLE", "模型设置服务暂不可用")
		return
	}
	canManage, err := s.isLocalSettingsAdmin(userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "SETTINGS_READ_FAILED", "读取本机设置权限失败")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"settings": s.llmRuntime.Settings(), "canManage": canManage})
}

func (s *Server) updateLLMSettings(w http.ResponseWriter, r *http.Request) {
	userID, ok := s.userID(w, r)
	if !ok {
		return
	}
	if s.llmRuntime == nil {
		writeError(w, http.StatusServiceUnavailable, "SETTINGS_UNAVAILABLE", "模型设置服务暂不可用")
		return
	}
	if !s.requireLocalSettingsAdmin(w, userID) {
		return
	}
	var input llm.SettingsInput
	if !decodeJSON(w, r, &input) {
		return
	}
	settings, err := s.llmRuntime.Update(input)
	if err != nil {
		var validationError *llm.SettingsValidationError
		if errors.As(err, &validationError) {
			writeError(w, http.StatusBadRequest, "INVALID_MODEL_SETTINGS", validationError.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, "SETTINGS_SAVE_FAILED", "无法保存本机模型设置，请检查 .env 文件权限")
		return
	}
	writeJSON(w, http.StatusOK, settings)
}

func (s *Server) testLLMConnection(w http.ResponseWriter, r *http.Request) {
	userID, ok := s.userID(w, r)
	if !ok {
		return
	}
	if s.llmRuntime == nil {
		writeError(w, http.StatusServiceUnavailable, "SETTINGS_UNAVAILABLE", "模型设置服务暂不可用")
		return
	}
	if !s.requireLocalSettingsAdmin(w, userID) {
		return
	}
	var input llm.SettingsInput
	if !decodeJSON(w, r, &input) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	if err := s.llmRuntime.TestConnection(ctx, input); err != nil {
		var validationError *llm.SettingsValidationError
		if errors.As(err, &validationError) {
			writeError(w, http.StatusBadRequest, "INVALID_MODEL_SETTINGS", validationError.Error())
			return
		}
		var userFacing interface{ UserMessage() string }
		if errors.As(err, &userFacing) {
			writeError(w, http.StatusBadGateway, "MODEL_CONNECTION_FAILED", userFacing.UserMessage())
			return
		}
		writeError(w, http.StatusBadGateway, "MODEL_CONNECTION_FAILED", "无法连接模型服务，请检查地址和 API Key")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "message": "连接成功"})
}

func (s *Server) listLLMProviders(w http.ResponseWriter, r *http.Request) {
	userID, ok := s.userID(w, r)
	if !ok {
		return
	}
	catalog := s.providerCatalog(w)
	if catalog == nil {
		return
	}
	canManage, err := s.isLocalSettingsAdmin(userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "SETTINGS_READ_FAILED", "读取本机设置权限失败")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"providers":         catalog.List(),
		"defaultProviderId": catalog.DefaultID(),
		"canManage":         canManage,
	})
}

func (s *Server) createLLMProvider(w http.ResponseWriter, r *http.Request) {
	userID, ok := s.userID(w, r)
	if !ok || !s.requireLocalSettingsAdmin(w, userID) {
		return
	}
	catalog := s.providerCatalog(w)
	if catalog == nil {
		return
	}
	var input llm.ProviderProfileInput
	if !decodeJSON(w, r, &input) {
		return
	}
	provider, err := catalog.Save("", input)
	if err != nil {
		s.writeProviderError(w, err, "SETTINGS_SAVE_FAILED")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"provider": provider})
}

func (s *Server) updateLLMProvider(w http.ResponseWriter, r *http.Request) {
	userID, ok := s.userID(w, r)
	if !ok || !s.requireLocalSettingsAdmin(w, userID) {
		return
	}
	catalog := s.providerCatalog(w)
	if catalog == nil {
		return
	}
	var input llm.ProviderProfileInput
	if !decodeJSON(w, r, &input) {
		return
	}
	provider, err := catalog.Save(r.PathValue("providerID"), input)
	if err != nil {
		s.writeProviderError(w, err, "SETTINGS_SAVE_FAILED")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"provider": provider})
}

func (s *Server) deleteLLMProvider(w http.ResponseWriter, r *http.Request) {
	userID, ok := s.userID(w, r)
	if !ok || !s.requireLocalSettingsAdmin(w, userID) {
		return
	}
	catalog := s.providerCatalog(w)
	if catalog == nil {
		return
	}
	if err := catalog.Delete(r.PathValue("providerID")); err != nil {
		s.writeProviderError(w, err, "SETTINGS_DELETE_FAILED")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "defaultProviderId": catalog.DefaultID()})
}

func (s *Server) setDefaultLLMProvider(w http.ResponseWriter, r *http.Request) {
	userID, ok := s.userID(w, r)
	if !ok || !s.requireLocalSettingsAdmin(w, userID) {
		return
	}
	catalog := s.providerCatalog(w)
	if catalog == nil {
		return
	}
	if err := catalog.SetDefault(r.PathValue("providerID")); err != nil {
		s.writeProviderError(w, err, "SETTINGS_SAVE_FAILED")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "defaultProviderId": catalog.DefaultID()})
}

func (s *Server) testLLMProvider(w http.ResponseWriter, r *http.Request) {
	userID, ok := s.userID(w, r)
	if !ok || !s.requireLocalSettingsAdmin(w, userID) {
		return
	}
	catalog := s.providerCatalog(w)
	if catalog == nil {
		return
	}
	var input struct {
		ID string `json:"id"`
		llm.ProviderProfileInput
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	if err := catalog.TestConnection(ctx, input.ID, input.ProviderProfileInput); err != nil {
		var validationError *llm.SettingsValidationError
		if errors.As(err, &validationError) {
			writeError(w, http.StatusBadRequest, "INVALID_MODEL_SETTINGS", validationError.Error())
			return
		}
		var userFacing interface{ UserMessage() string }
		if errors.As(err, &userFacing) {
			writeError(w, http.StatusBadGateway, "MODEL_CONNECTION_FAILED", userFacing.UserMessage())
			return
		}
		writeError(w, http.StatusBadGateway, "MODEL_CONNECTION_FAILED", "无法连接模型服务，请检查地址和 API Key")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "message": "连接成功"})
}

func (s *Server) providerCatalog(w http.ResponseWriter) *llm.ProviderCatalog {
	if s.llmRuntime == nil || s.llmRuntime.ProviderCatalog() == nil {
		writeError(w, http.StatusServiceUnavailable, "SETTINGS_UNAVAILABLE", "多模型设置服务暂不可用")
		return nil
	}
	return s.llmRuntime.ProviderCatalog()
}

func (s *Server) writeProviderError(w http.ResponseWriter, err error, code string) {
	var validationError *llm.SettingsValidationError
	if errors.As(err, &validationError) {
		writeError(w, http.StatusBadRequest, "INVALID_MODEL_SETTINGS", validationError.Error())
		return
	}
	writeError(w, http.StatusInternalServerError, code, "无法保存本机模型服务商设置")
}

func (s *Server) requireLocalSettingsAdmin(w http.ResponseWriter, userID string) bool {
	isAdmin, err := s.isLocalSettingsAdmin(userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "SETTINGS_READ_FAILED", "读取本机设置权限失败")
		return false
	}
	if !isAdmin {
		writeError(w, http.StatusForbidden, "FORBIDDEN", "只有本机首个账户可以修改模型设置")
		return false
	}
	return true
}

func (s *Server) isLocalSettingsAdmin(userID string) (bool, error) {
	var firstUserID string
	if err := s.db.QueryRow(`SELECT id FROM users ORDER BY created_at, id LIMIT 1`).Scan(&firstUserID); err != nil {
		return false, err
	}
	return firstUserID == userID, nil
}
