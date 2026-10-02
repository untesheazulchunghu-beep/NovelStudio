package llm

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"novelstudio/internal/config"
	"novelstudio/internal/database"
)

type ProviderProfileInput struct {
	Name                 string  `json:"name"`
	BaseURL              string  `json:"baseUrl"`
	Model                string  `json:"model"`
	APIKey               string  `json:"apiKey"`
	ClearAPIKey          bool    `json:"clearApiKey"`
	Temperature          float64 `json:"temperature"`
	MaxTokens            int     `json:"maxTokens"`
	TimeoutSec           int     `json:"timeoutSec"`
	InputCostPerMillion  float64 `json:"inputCostPerMillion"`
	OutputCostPerMillion float64 `json:"outputCostPerMillion"`
}

type ProviderProfileView struct {
	ID                   string  `json:"id"`
	Name                 string  `json:"name"`
	BaseURL              string  `json:"baseUrl"`
	Model                string  `json:"model"`
	Temperature          float64 `json:"temperature"`
	MaxTokens            int     `json:"maxTokens"`
	TimeoutSec           int     `json:"timeoutSec"`
	InputCostPerMillion  float64 `json:"inputCostPerMillion"`
	OutputCostPerMillion float64 `json:"outputCostPerMillion"`
	APIKeyConfigured     bool    `json:"apiKeyConfigured"`
	IsDefault            bool    `json:"isDefault"`
}

type providerProfile struct {
	view     ProviderProfileView
	settings RuntimeSettings
	provider *Provider
}

// ProviderCatalog stores model credentials encrypted in SQLite and keeps the
// decrypted settings only in process memory while providers are running.
type ProviderCatalog struct {
	mu                 sync.RWMutex
	db                 *sql.DB
	box                cipher.AEAD
	encryptionKey      []byte
	encryptionKeyIsNew bool
	envPath            string
	profiles           map[string]providerProfile
	defaultProviderID  string
}

func NewProviderCatalog(db *sql.DB, encryptionKey []byte, encryptionKeyIsNew bool, envPath string, seed RuntimeSettings) (*ProviderCatalog, error) {
	if db == nil {
		return nil, errors.New("model provider database is required")
	}
	if len(encryptionKey) != 32 {
		return nil, errors.New("model encryption key must be 32 bytes")
	}
	if envPath == "" {
		envPath = ".env"
	}
	key := sha256.Sum256(append([]byte("novelstudio/model-provider-key/v1\x00"), encryptionKey...))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, fmt.Errorf("create model credential cipher: %w", err)
	}
	box, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create model credential cipher: %w", err)
	}
	catalog := &ProviderCatalog{
		db:                 db,
		box:                box,
		encryptionKey:      append([]byte(nil), encryptionKey...),
		encryptionKeyIsNew: encryptionKeyIsNew,
		envPath:            envPath,
		profiles:           make(map[string]providerProfile),
	}
	if err := catalog.load(); err != nil {
		return nil, err
	}
	if len(catalog.profiles) == 0 {
		_, err := catalog.Save("", ProviderProfileInput{
			Name:        "Default provider",
			BaseURL:     seed.BaseURL,
			Model:       seed.Model,
			APIKey:      seed.APIKey,
			Temperature: seed.Temperature,
			MaxTokens:   seed.MaxTokens,
			TimeoutSec:  seed.TimeoutSec,
		})
		if err != nil {
			return nil, fmt.Errorf("initialize default model provider: %w", err)
		}
	}
	if catalog.defaultProviderID == "" {
		profiles := catalog.List()
		if len(profiles) == 0 {
			return nil, errors.New("model provider catalog is empty")
		}
		if err := catalog.SetDefault(profiles[0].ID); err != nil {
			return nil, fmt.Errorf("select default model provider: %w", err)
		}
	}
	return catalog, nil
}

func (c *ProviderCatalog) load() error {
	rows, err := c.db.Query(`SELECT id, name, base_url, api_key_ciphertext, model, temperature, max_tokens, timeout_sec, is_default,
		input_cost_per_million, output_cost_per_million
		FROM model_providers ORDER BY is_default DESC, created_at, id`)
	if err != nil {
		return fmt.Errorf("read model providers: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id, name, baseURL, encryptedKey, model string
		var temperature float64
		var inputCostPerMillion, outputCostPerMillion float64
		var maxTokens, timeoutSec int
		var isDefault bool
		if err := rows.Scan(&id, &name, &baseURL, &encryptedKey, &model, &temperature, &maxTokens, &timeoutSec, &isDefault,
			&inputCostPerMillion, &outputCostPerMillion); err != nil {
			return fmt.Errorf("read model provider: %w", err)
		}
		apiKey, err := c.openKey(id, encryptedKey)
		if err != nil {
			return fmt.Errorf("decrypt model provider %q credentials; restore its LLM_ENCRYPTION_KEY: %w", id, err)
		}
		settings := RuntimeSettings{
			BaseURL:              baseURL,
			APIKey:               apiKey,
			Model:                model,
			Temperature:          temperature,
			MaxTokens:            maxTokens,
			TimeoutSec:           timeoutSec,
			InputCostPerMillion:  inputCostPerMillion,
			OutputCostPerMillion: outputCostPerMillion,
		}
		c.profiles[id] = c.makeProfile(id, name, settings, isDefault)
		if isDefault {
			c.defaultProviderID = id
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("read model providers: %w", err)
	}
	return nil
}

func (c *ProviderCatalog) List() []ProviderProfileView {
	c.mu.RLock()
	defer c.mu.RUnlock()
	profiles := make([]ProviderProfileView, 0, len(c.profiles))
	for _, profile := range c.profiles {
		profiles = append(profiles, profile.view)
	}
	sort.Slice(profiles, func(i, j int) bool {
		if profiles[i].IsDefault != profiles[j].IsDefault {
			return profiles[i].IsDefault
		}
		if profiles[i].Name != profiles[j].Name {
			return profiles[i].Name < profiles[j].Name
		}
		return profiles[i].ID < profiles[j].ID
	})
	return profiles
}

func (c *ProviderCatalog) DefaultID() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.defaultProviderID
}

func (c *ProviderCatalog) Has(id string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	_, ok := c.profiles[id]
	return ok
}

func (c *ProviderCatalog) Settings(id string) (RuntimeSettings, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if id == "" {
		id = c.defaultProviderID
	}
	profile, ok := c.profiles[id]
	return profile.settings, ok
}

func (c *ProviderCatalog) Provider(id string) (*Provider, RuntimeSettings, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if id == "" {
		id = c.defaultProviderID
	}
	profile, ok := c.profiles[id]
	return profile.provider, profile.settings, ok
}

func (c *ProviderCatalog) Save(id string, input ProviderProfileInput) (ProviderProfileView, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	name := strings.TrimSpace(input.Name)
	if name == "" || len([]rune(name)) > 80 || strings.ContainsAny(name, "\r\n\x00") {
		return ProviderProfileView{}, &SettingsValidationError{message: "请填写不超过 80 字的服务商名称"}
	}
	current, exists := c.profiles[id]
	if id != "" && !exists {
		return ProviderProfileView{}, &SettingsValidationError{message: "模型服务商不存在"}
	}
	settings, err := mergeProviderInput(current.settings, exists, input)
	if err != nil {
		return ProviderProfileView{}, err
	}
	var duplicate bool
	if err := c.db.QueryRow(`SELECT EXISTS(SELECT 1 FROM model_providers WHERE name = ? AND id <> ?)`, name, id).Scan(&duplicate); err != nil {
		return ProviderProfileView{}, fmt.Errorf("check duplicate model provider name: %w", err)
	}
	if duplicate {
		return ProviderProfileView{}, &SettingsValidationError{message: "已存在同名模型服务商"}
	}
	if id == "" {
		id = database.NewID()
	}
	isDefault := exists && current.view.IsDefault || !exists && len(c.profiles) == 0
	if c.encryptionKeyIsNew && settings.APIKey != "" {
		if err := config.SaveLLMEncryptionKey(c.envPath, c.encryptionKey); err != nil {
			return ProviderProfileView{}, fmt.Errorf("persist model credential encryption key: %w", err)
		}
		c.encryptionKeyIsNew = false
	}
	encryptedKey, err := c.sealKey(id, settings.APIKey)
	if err != nil {
		return ProviderProfileView{}, fmt.Errorf("encrypt model provider credentials: %w", err)
	}
	now := database.Now()
	tx, err := c.db.Begin()
	if err != nil {
		return ProviderProfileView{}, fmt.Errorf("begin model provider save: %w", err)
	}
	defer tx.Rollback()
	if isDefault {
		if _, err := tx.Exec(`UPDATE model_providers SET is_default = 0 WHERE is_default = 1`); err != nil {
			return ProviderProfileView{}, fmt.Errorf("clear default model provider: %w", err)
		}
	}
	_, err = tx.Exec(`INSERT INTO model_providers (
		id, name, base_url, api_key_ciphertext, model, temperature, max_tokens, timeout_sec, is_default,
		input_cost_per_million, output_cost_per_million, created_at, updated_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT(id) DO UPDATE SET name = excluded.name, base_url = excluded.base_url,
		api_key_ciphertext = excluded.api_key_ciphertext, model = excluded.model,
		temperature = excluded.temperature, max_tokens = excluded.max_tokens,
		timeout_sec = excluded.timeout_sec, is_default = excluded.is_default,
		input_cost_per_million = excluded.input_cost_per_million,
		output_cost_per_million = excluded.output_cost_per_million, updated_at = excluded.updated_at`,
		id, name, settings.BaseURL, encryptedKey, settings.Model, settings.Temperature, settings.MaxTokens,
		settings.TimeoutSec, providerDefaultFlag(isDefault), settings.InputCostPerMillion,
		settings.OutputCostPerMillion, now, now)
	if err != nil {
		return ProviderProfileView{}, fmt.Errorf("save model provider: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return ProviderProfileView{}, fmt.Errorf("commit model provider: %w", err)
	}
	profile := c.makeProfile(id, name, settings, isDefault)
	c.profiles[id] = profile
	if isDefault {
		c.defaultProviderID = id
	}
	return profile.view, nil
}

func (c *ProviderCatalog) SetDefault(id string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, exists := c.profiles[id]; !exists {
		return &SettingsValidationError{message: "模型服务商不存在"}
	}
	if c.defaultProviderID == id {
		return nil
	}
	tx, err := c.db.Begin()
	if err != nil {
		return fmt.Errorf("begin default provider update: %w", err)
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`UPDATE model_providers SET is_default = 0 WHERE is_default = 1`); err != nil {
		return fmt.Errorf("clear current default provider: %w", err)
	}
	if _, err := tx.Exec(`UPDATE model_providers SET is_default = 1, updated_at = ? WHERE id = ?`, database.Now(), id); err != nil {
		return fmt.Errorf("set default provider: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit default provider update: %w", err)
	}
	for profileID, profile := range c.profiles {
		profile.view.IsDefault = profileID == id
		c.profiles[profileID] = profile
	}
	c.defaultProviderID = id
	return nil
}

func (c *ProviderCatalog) Delete(id string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	profile, exists := c.profiles[id]
	if !exists {
		return &SettingsValidationError{message: "模型服务商不存在"}
	}
	if len(c.profiles) == 1 {
		return &SettingsValidationError{message: "至少需要保留一个模型服务商"}
	}
	var inUse bool
	needle := `"providerId":"` + id + `"`
	if err := c.db.QueryRow(`SELECT EXISTS(
		SELECT 1 FROM tasks WHERE status IN ('queued', 'running') AND instr(request_json, ?) > 0
	)`, needle).Scan(&inUse); err != nil {
		return fmt.Errorf("check active model provider tasks: %w", err)
	}
	if inUse {
		return &SettingsValidationError{message: "有正在运行的任务使用此服务商，暂时不能删除"}
	}
	nextDefault := c.defaultProviderID
	if profile.view.IsDefault {
		ids := make([]string, 0, len(c.profiles)-1)
		for profileID := range c.profiles {
			if profileID != id {
				ids = append(ids, profileID)
			}
		}
		sort.Slice(ids, func(i, j int) bool {
			left, right := c.profiles[ids[i]].view, c.profiles[ids[j]].view
			if left.Name != right.Name {
				return left.Name < right.Name
			}
			return left.ID < right.ID
		})
		nextDefault = ids[0]
	}
	tx, err := c.db.Begin()
	if err != nil {
		return fmt.Errorf("begin provider deletion: %w", err)
	}
	defer tx.Rollback()
	if profile.view.IsDefault {
		if _, err := tx.Exec(`UPDATE model_providers SET is_default = 0 WHERE id = ?`, id); err != nil {
			return fmt.Errorf("clear deleted default provider: %w", err)
		}
	}
	if _, err := tx.Exec(`DELETE FROM model_providers WHERE id = ?`, id); err != nil {
		return fmt.Errorf("delete model provider: %w", err)
	}
	if profile.view.IsDefault {
		if _, err := tx.Exec(`UPDATE model_providers SET is_default = 1, updated_at = ? WHERE id = ?`, database.Now(), nextDefault); err != nil {
			return fmt.Errorf("promote replacement default provider: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit provider deletion: %w", err)
	}
	delete(c.profiles, id)
	if profile.view.IsDefault {
		c.defaultProviderID = nextDefault
		for profileID, next := range c.profiles {
			next.view.IsDefault = profileID == nextDefault
			c.profiles[profileID] = next
		}
	}
	return nil
}

func (c *ProviderCatalog) TestConnection(ctx context.Context, id string, input ProviderProfileInput) error {
	c.mu.RLock()
	current, exists := c.profiles[id]
	if id != "" && !exists {
		c.mu.RUnlock()
		return &SettingsValidationError{message: "模型服务商不存在"}
	}
	settings, err := mergeProviderInput(current.settings, exists, input)
	c.mu.RUnlock()
	if err != nil {
		return err
	}
	if settings.APIKey == "" {
		return &SettingsValidationError{message: "请先填写 API Key"}
	}
	provider := New(settings.BaseURL, settings.APIKey, settings.Model, Options{
		Temperature: settings.Temperature,
		MaxTokens:   settings.MaxTokens,
		Timeout:     15 * time.Second,
	})
	return provider.TestConnection(ctx)
}

func (c *ProviderCatalog) sealKey(id, value string) (string, error) {
	if value == "" {
		return "", nil
	}
	nonce := make([]byte, c.box.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	associatedData := []byte("novelstudio/model-provider/v1/" + id)
	sealed := c.box.Seal(nonce, nonce, []byte(value), associatedData)
	return base64.RawURLEncoding.EncodeToString(sealed), nil
}

func (c *ProviderCatalog) openKey(id, value string) (string, error) {
	if value == "" {
		return "", nil
	}
	sealed, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil || len(sealed) < c.box.NonceSize()+c.box.Overhead() {
		return "", errors.New("invalid encrypted credential")
	}
	nonce, ciphertext := sealed[:c.box.NonceSize()], sealed[c.box.NonceSize():]
	plaintext, err := c.box.Open(nil, nonce, ciphertext, []byte("novelstudio/model-provider/v1/"+id))
	if err != nil {
		return "", err
	}
	return string(plaintext), nil
}

func (c *ProviderCatalog) makeProfile(id, name string, settings RuntimeSettings, isDefault bool) providerProfile {
	return providerProfile{
		view: ProviderProfileView{
			ID:                   id,
			Name:                 name,
			BaseURL:              settings.BaseURL,
			Model:                settings.Model,
			Temperature:          settings.Temperature,
			MaxTokens:            settings.MaxTokens,
			TimeoutSec:           settings.TimeoutSec,
			InputCostPerMillion:  settings.InputCostPerMillion,
			OutputCostPerMillion: settings.OutputCostPerMillion,
			APIKeyConfigured:     settings.APIKey != "",
			IsDefault:            isDefault,
		},
		settings: settings,
		provider: newRuntimeProvider(settings),
	}
}

func mergeProviderInput(current RuntimeSettings, exists bool, input ProviderProfileInput) (RuntimeSettings, error) {
	next := current
	next.BaseURL = strings.TrimRight(strings.TrimSpace(input.BaseURL), "/")
	next.Model = strings.TrimSpace(input.Model)
	next.Temperature = input.Temperature
	next.MaxTokens = input.MaxTokens
	next.TimeoutSec = input.TimeoutSec
	next.InputCostPerMillion = input.InputCostPerMillion
	next.OutputCostPerMillion = input.OutputCostPerMillion
	if input.ClearAPIKey {
		if strings.TrimSpace(input.APIKey) != "" {
			return RuntimeSettings{}, &SettingsValidationError{message: "清除密钥时不能同时填写新密钥"}
		}
		next.APIKey = ""
	} else if strings.TrimSpace(input.APIKey) != "" {
		next.APIKey = strings.TrimSpace(input.APIKey)
	} else if !exists {
		next.APIKey = ""
	}
	if err := validateRuntimeSettings(next); err != nil {
		return RuntimeSettings{}, err
	}
	return next, nil
}

func providerProfileInput(name string, settings RuntimeSettings) ProviderProfileInput {
	return ProviderProfileInput{
		Name:                 name,
		BaseURL:              settings.BaseURL,
		Model:                settings.Model,
		APIKey:               settings.APIKey,
		ClearAPIKey:          settings.APIKey == "",
		Temperature:          settings.Temperature,
		MaxTokens:            settings.MaxTokens,
		TimeoutSec:           settings.TimeoutSec,
		InputCostPerMillion:  settings.InputCostPerMillion,
		OutputCostPerMillion: settings.OutputCostPerMillion,
	}
}

func providerDefaultFlag(isDefault bool) int {
	if isDefault {
		return 1
	}
	return 0
}
