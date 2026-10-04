#include "config.h"
#include <sqlite3.h>
#include <fstream>
#include <sstream>

namespace NovelStudio {

// CacheManager implementation
void CacheManager::Set(const std::string& key, const nlohmann::json& value, int ttlSeconds) {
    CacheEntry entry;
    entry.value = value;

    if (ttlSeconds > 0) {
        entry.expiresAt = std::chrono::system_clock::now() + std::chrono::seconds(ttlSeconds);
    } else {
        // Far future = no expiration
        entry.expiresAt = std::chrono::system_clock::time_point::max();
    }

    cache_[key] = entry;
}

std::optional<nlohmann::json> CacheManager::Get(const std::string& key) {
    auto it = cache_.find(key);
    if (it == cache_.end()) {
        misses_++;
        return std::nullopt;
    }

    if (it->second.isExpired()) {
        cache_.erase(it);
        misses_++;
        return std::nullopt;
    }

    hits_++;
    return it->second.value;
}

bool CacheManager::Has(const std::string& key) {
    auto it = cache_.find(key);
    if (it == cache_.end()) {
        return false;
    }

    if (it->second.isExpired()) {
        cache_.erase(it);
        return false;
    }

    return true;
}

void CacheManager::Remove(const std::string& key) {
    cache_.erase(key);
}

void CacheManager::Clear() {
    cache_.clear();
    hits_ = 0;
    misses_ = 0;
}

void CacheManager::CleanExpired() {
    auto it = cache_.begin();
    while (it != cache_.end()) {
        if (it->second.isExpired()) {
            it = cache_.erase(it);
        } else {
            ++it;
        }
    }
}

CacheManager::Stats CacheManager::GetStats() const {
    Stats stats;
    stats.totalEntries = cache_.size();
    stats.expiredEntries = 0;
    stats.hits = hits_;
    stats.misses = misses_;

    for (const auto& [key, entry] : cache_) {
        if (entry.isExpired()) {
            stats.expiredEntries++;
        }
    }

    return stats;
}

// PreferenceStore implementation
class PreferenceStore::Impl {
public:
    sqlite3* db = nullptr;

    bool Initialize() {
        const char* schema = R"(
            CREATE TABLE IF NOT EXISTS preferences (
                key TEXT PRIMARY KEY,
                value TEXT NOT NULL,
                type TEXT NOT NULL
            );
        )";

        char* errMsg = nullptr;
        int rc = sqlite3_exec(db, schema, nullptr, nullptr, &errMsg);

        if (rc != SQLITE_OK) {
            sqlite3_free(errMsg);
            return false;
        }

        return true;
    }

    void SetValue(const std::string& key, const std::string& value, const std::string& type) {
        const char* sql = "REPLACE INTO preferences (key, value, type) VALUES (?, ?, ?)";

        sqlite3_stmt* stmt;
        if (sqlite3_prepare_v2(db, sql, -1, &stmt, nullptr) != SQLITE_OK) {
            return;
        }

        sqlite3_bind_text(stmt, 1, key.c_str(), -1, SQLITE_TRANSIENT);
        sqlite3_bind_text(stmt, 2, value.c_str(), -1, SQLITE_TRANSIENT);
        sqlite3_bind_text(stmt, 3, type.c_str(), -1, SQLITE_TRANSIENT);

        sqlite3_step(stmt);
        sqlite3_finalize(stmt);
    }

    std::optional<std::string> GetValue(const std::string& key) {
        const char* sql = "SELECT value FROM preferences WHERE key = ?";

        sqlite3_stmt* stmt;
        if (sqlite3_prepare_v2(db, sql, -1, &stmt, nullptr) != SQLITE_OK) {
            return std::nullopt;
        }

        sqlite3_bind_text(stmt, 1, key.c_str(), -1, SQLITE_TRANSIENT);

        std::optional<std::string> result;
        if (sqlite3_step(stmt) == SQLITE_ROW) {
            result = reinterpret_cast<const char*>(sqlite3_column_text(stmt, 0));
        }

        sqlite3_finalize(stmt);
        return result;
    }
};

PreferenceStore::PreferenceStore() : impl_(std::make_unique<Impl>()) {}

PreferenceStore::~PreferenceStore() {
    Close();
}

bool PreferenceStore::Open(const std::string& dbPath) {
    if (impl_->db) {
        Close();
    }

    int rc = sqlite3_open(dbPath.c_str(), &impl_->db);
    if (rc != SQLITE_OK) {
        sqlite3_close(impl_->db);
        impl_->db = nullptr;
        return false;
    }

    return impl_->Initialize();
}

void PreferenceStore::Close() {
    if (impl_->db) {
        sqlite3_close(impl_->db);
        impl_->db = nullptr;
    }
}

void PreferenceStore::SetString(const std::string& key, const std::string& value) {
    impl_->SetValue(key, value, "string");
}

std::string PreferenceStore::GetString(const std::string& key, const std::string& defaultValue) {
    auto value = impl_->GetValue(key);
    return value.value_or(defaultValue);
}

void PreferenceStore::SetInt(const std::string& key, int value) {
    impl_->SetValue(key, std::to_string(value), "int");
}

int PreferenceStore::GetInt(const std::string& key, int defaultValue) {
    auto value = impl_->GetValue(key);
    if (!value) return defaultValue;
    try {
        return std::stoi(*value);
    } catch (...) {
        return defaultValue;
    }
}

void PreferenceStore::SetBool(const std::string& key, bool value) {
    impl_->SetValue(key, value ? "1" : "0", "bool");
}

bool PreferenceStore::GetBool(const std::string& key, bool defaultValue) {
    auto value = impl_->GetValue(key);
    if (!value) return defaultValue;
    return *value == "1" || *value == "true";
}

void PreferenceStore::SetFloat(const std::string& key, float value) {
    impl_->SetValue(key, std::to_string(value), "float");
}

float PreferenceStore::GetFloat(const std::string& key, float defaultValue) {
    auto value = impl_->GetValue(key);
    if (!value) return defaultValue;
    try {
        return std::stof(*value);
    } catch (...) {
        return defaultValue;
    }
}

void PreferenceStore::SetJson(const std::string& key, const nlohmann::json& value) {
    impl_->SetValue(key, value.dump(), "json");
}

nlohmann::json PreferenceStore::GetJson(const std::string& key, const nlohmann::json& defaultValue) {
    auto value = impl_->GetValue(key);
    if (!value) return defaultValue;
    try {
        return nlohmann::json::parse(*value);
    } catch (...) {
        return defaultValue;
    }
}

void PreferenceStore::Remove(const std::string& key) {
    const char* sql = "DELETE FROM preferences WHERE key = ?";

    sqlite3_stmt* stmt;
    if (sqlite3_prepare_v2(impl_->db, sql, -1, &stmt, nullptr) != SQLITE_OK) {
        return;
    }

    sqlite3_bind_text(stmt, 1, key.c_str(), -1, SQLITE_TRANSIENT);
    sqlite3_step(stmt);
    sqlite3_finalize(stmt);
}

bool PreferenceStore::Has(const std::string& key) {
    return impl_->GetValue(key).has_value();
}

// BootConfig implementation
BootConfig::Settings BootConfig::defaults_;

BootConfig::Settings BootConfig::Load(const std::string& configPath) {
    Settings settings;

    std::ifstream file(configPath);
    if (!file.is_open()) {
        return settings;  // Return defaults
    }

    try {
        nlohmann::json config;
        file >> config;

        if (config.contains("performance")) {
            auto perf = config["performance"];
            if (perf.contains("enableGPU")) settings.enableGPU = perf["enableGPU"];
            if (perf.contains("maxMemoryMB")) settings.maxMemoryMB = perf["maxMemoryMB"];
            if (perf.contains("threadPoolSize")) settings.threadPoolSize = perf["threadPoolSize"];
        }

        if (config.contains("logging")) {
            auto log = config["logging"];
            if (log.contains("level")) settings.logLevel = log["level"];
            if (log.contains("path")) settings.logPath = log["path"];
        }

        if (config.contains("ui")) {
            auto ui = config["ui"];
            if (ui.contains("enableDevTools")) settings.enableDevTools = ui["enableDevTools"];
            if (ui.contains("enableHardwareAcceleration"))
                settings.enableHardwareAcceleration = ui["enableHardwareAcceleration"];
        }

        if (config.contains("database")) {
            auto db = config["database"];
            if (db.contains("path")) settings.databasePath = db["path"];
            if (db.contains("poolSize")) settings.databasePoolSize = db["poolSize"];
        }
    } catch (...) {
        // Return defaults on parse error
    }

    return settings;
}

bool BootConfig::Save(const std::string& configPath, const Settings& settings) {
    nlohmann::json config;

    config["performance"]["enableGPU"] = settings.enableGPU;
    config["performance"]["maxMemoryMB"] = settings.maxMemoryMB;
    config["performance"]["threadPoolSize"] = settings.threadPoolSize;

    config["logging"]["level"] = settings.logLevel;
    config["logging"]["path"] = settings.logPath;

    config["ui"]["enableDevTools"] = settings.enableDevTools;
    config["ui"]["enableHardwareAcceleration"] = settings.enableHardwareAcceleration;

    config["database"]["path"] = settings.databasePath;
    config["database"]["poolSize"] = settings.databasePoolSize;

    std::ofstream file(configPath);
    if (!file.is_open()) {
        return false;
    }

    file << config.dump(2);
    return true;
}

} // namespace NovelStudio
