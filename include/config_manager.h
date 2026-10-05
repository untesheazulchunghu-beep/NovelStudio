#pragma once

#include <string>
#include <memory>
#include <map>

namespace NovelStudio {

// Configuration manager for application settings
class ConfigManager {
public:
    static ConfigManager& GetInstance();

    // Load configuration from file
    bool Load(const std::string& path = "config.json");

    // Save configuration to file
    bool Save(const std::string& path = "config.json");

    // Get configuration value
    std::string Get(const std::string& key, const std::string& defaultValue = "") const;
    int GetInt(const std::string& key, int defaultValue = 0) const;
    bool GetBool(const std::string& key, bool defaultValue = false) const;

    // Set configuration value
    void Set(const std::string& key, const std::string& value);
    void SetInt(const std::string& key, int value);
    void SetBool(const std::string& key, bool value);

    // Check if key exists
    bool Has(const std::string& key) const;

    // Helper methods for common settings
    std::string GetDatabasePath() const { return Get("database_path", "novelstudio.db"); }
    std::string GetOpenAIKey() const { return Get("openai_api_key", ""); }
    std::string GetAnthropicKey() const { return Get("anthropic_api_key", ""); }
    std::string GetOllamaEndpoint() const { return Get("ollama_endpoint", "http://localhost:11434/api/chat"); }
    std::string GetLogLevel() const { return Get("log_level", "info"); }

private:
    ConfigManager() = default;
    std::map<std::string, std::string> config_;
    std::string configPath_;
};

} // namespace NovelStudio
