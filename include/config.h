#pragma once

#include <string>
#include <memory>
#include <functional>
#include <chrono>
#include <unordered_map>
#include <optional>
#include <nlohmann/json.hpp>

namespace NovelStudio {

// Cache entry with TTL support
struct CacheEntry {
    nlohmann::json value;
    std::chrono::system_clock::time_point expiresAt;
    bool isExpired() const {
        return std::chrono::system_clock::now() >= expiresAt;
    }
};

// In-memory cache manager
class CacheManager {
public:
    CacheManager() = default;
    ~CacheManager() = default;

    // Set value with optional TTL (0 = no expiration)
    void Set(const std::string& key, const nlohmann::json& value, int ttlSeconds = 0);

    // Get value, returns nullopt if not found or expired
    std::optional<nlohmann::json> Get(const std::string& key);

    // Check if key exists and is not expired
    bool Has(const std::string& key);

    // Remove specific key
    void Remove(const std::string& key);

    // Clear all cache
    void Clear();

    // Remove expired entries
    void CleanExpired();

    // Get cache statistics
    struct Stats {
        size_t totalEntries;
        size_t expiredEntries;
        size_t hits;
        size_t misses;
    };
    Stats GetStats() const;

private:
    std::unordered_map<std::string, CacheEntry> cache_;
    mutable size_t hits_ = 0;
    mutable size_t misses_ = 0;
};

// Preference store for user settings
class PreferenceStore {
public:
    PreferenceStore();
    ~PreferenceStore();

    bool Open(const std::string& dbPath);
    void Close();

    // String preferences
    void SetString(const std::string& key, const std::string& value);
    std::string GetString(const std::string& key, const std::string& defaultValue = "");

    // Integer preferences
    void SetInt(const std::string& key, int value);
    int GetInt(const std::string& key, int defaultValue = 0);

    // Boolean preferences
    void SetBool(const std::string& key, bool value);
    bool GetBool(const std::string& key, bool defaultValue = false);

    // Float preferences
    void SetFloat(const std::string& key, float value);
    float GetFloat(const std::string& key, float defaultValue = 0.0f);

    // JSON preferences
    void SetJson(const std::string& key, const nlohmann::json& value);
    nlohmann::json GetJson(const std::string& key, const nlohmann::json& defaultValue = {});

    // Remove preference
    void Remove(const std::string& key);

    // Check if preference exists
    bool Has(const std::string& key);

private:
    class Impl;
    std::unique_ptr<Impl> impl_;
};

// Boot configuration (loaded before app initialization)
class BootConfig {
public:
    struct Settings {
        // Performance
        bool enableGPU = true;
        int maxMemoryMB = 2048;
        int threadPoolSize = 4;

        // Logging
        std::string logLevel = "info";
        std::string logPath = "logs/";

        // UI
        bool enableDevTools = false;
        bool enableHardwareAcceleration = true;

        // Database
        std::string databasePath = "novelstudio.db";
        int databasePoolSize = 5;
    };

    static Settings Load(const std::string& configPath);
    static bool Save(const std::string& configPath, const Settings& settings);

private:
    static Settings defaults_;
};

} // namespace NovelStudio
