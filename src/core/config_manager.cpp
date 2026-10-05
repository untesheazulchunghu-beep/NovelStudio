#include "config_manager.h"
#include "json_utils.h"
#include "logger.h"
#include <fstream>
#include <sstream>

namespace NovelStudio {

ConfigManager& ConfigManager::GetInstance() {
    static ConfigManager instance;
    return instance;
}

bool ConfigManager::Load(const std::string& path) {
    configPath_ = path;

    std::ifstream file(path);
    if (!file.is_open()) {
        LOG_WARNING("Config file not found: " + path);
        return false;
    }

    std::string content((std::istreambuf_iterator<char>(file)),
                       std::istreambuf_iterator<char>());
    file.close();

    try {
        JsonValue json = JsonValue::Parse(content);

        if (!json.IsObject()) {
            LOG_ERROR("Invalid config format");
            return false;
        }

        // Extract all key-value pairs
        config_.clear();

        // Manual extraction for known keys
        const std::vector<std::string> keys = {
            "openai_api_key", "openai_endpoint",
            "anthropic_api_key", "anthropic_endpoint",
            "ollama_endpoint",
            "database_path", "log_level"
        };

        for (const auto& key : keys) {
            if (json.Has(key)) {
                config_[key] = json[key].AsString();
            }
        }

        LOG_INFO("Config loaded from: " + path);
        return true;

    } catch (const std::exception& e) {
        LOG_ERROR("Failed to parse config: " + std::string(e.what()));
        return false;
    }
}

bool ConfigManager::Save(const std::string& path) {
    JsonValue json = JsonValue::Object();

    for (const auto& [key, value] : config_) {
        json[key] = JsonValue(value);
    }

    std::string content = json.ToString(true);

    std::ofstream file(path);
    if (!file.is_open()) {
        LOG_ERROR("Failed to open config file for writing: " + path);
        return false;
    }

    file << content;
    file.close();

    LOG_INFO("Config saved to: " + path);
    return true;
}

std::string ConfigManager::Get(const std::string& key, const std::string& defaultValue) const {
    auto it = config_.find(key);
    if (it != config_.end()) {
        return it->second;
    }
    return defaultValue;
}

int ConfigManager::GetInt(const std::string& key, int defaultValue) const {
    auto it = config_.find(key);
    if (it != config_.end()) {
        try {
            return std::stoi(it->second);
        } catch (...) {
            return defaultValue;
        }
    }
    return defaultValue;
}

bool ConfigManager::GetBool(const std::string& key, bool defaultValue) const {
    auto it = config_.find(key);
    if (it != config_.end()) {
        const std::string& value = it->second;
        return (value == "true" || value == "1" || value == "yes");
    }
    return defaultValue;
}

void ConfigManager::Set(const std::string& key, const std::string& value) {
    config_[key] = value;
}

void ConfigManager::SetInt(const std::string& key, int value) {
    config_[key] = std::to_string(value);
}

void ConfigManager::SetBool(const std::string& key, bool value) {
    config_[key] = value ? "true" : "false";
}

bool ConfigManager::Has(const std::string& key) const {
    return config_.find(key) != config_.end();
}

} // namespace NovelStudio
