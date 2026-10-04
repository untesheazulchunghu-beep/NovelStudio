#pragma once

#include <string>
#include <memory>
#include <vector>

namespace NovelStudio {

// Forward declarations
class Database;
class AIProvider;
class Novel;
class Chapter;

// Version info
constexpr const char* VERSION = "0.1.0";
constexpr const char* APP_NAME = "NovelStudio";

// Configuration
struct AppConfig {
    std::string databasePath;
    std::string aiApiKey;
    std::string aiEndpoint;
    std::string aiModel;
    int maxTokens = 4096;
    float temperature = 0.7f;
};

// Main application class
class Application {
public:
    Application();
    ~Application();

    bool Initialize(const AppConfig& config);
    void Run();
    void Shutdown();

    Database* GetDatabase() const { return database_.get(); }
    AIProvider* GetAIProvider() const { return aiProvider_.get(); }

private:
    std::unique_ptr<Database> database_;
    std::unique_ptr<AIProvider> aiProvider_;
    AppConfig config_;
    bool initialized_ = false;
};

} // namespace NovelStudio
