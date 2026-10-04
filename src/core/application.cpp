#include "novel_studio.h"
#include "database.h"
#include "ai_provider.h"
#include "logger.h"
#include "config.h"

namespace NovelStudio {

Application::Application() = default;

Application::~Application() {
    Shutdown();
}

bool Application::Initialize(const AppConfig& config) {
    if (initialized_) {
        LOG_WARNING("Application already initialized");
        return true;
    }

    LOG_INFO("Initializing NovelStudio v" + std::string(VERSION));
    config_ = config;

    // Setup logger
    auto& logger = Logger::GetInstance();
    logger.SetLogFile("logs/novelstudio.log");
    logger.SetLevel(LogLevel::Info);

    // Initialize database
    LOG_INFO("Opening database: " + config.databasePath);
    database_ = std::make_unique<Database>();
    if (!database_->Open(config.databasePath)) {
        LOG_ERROR("Failed to open database");
        return false;
    }
    LOG_INFO("Database initialized successfully");

    // Initialize AI provider
    LOG_INFO("Initializing AI provider");
    aiProvider_ = AIProvider::CreateOpenAI();
    if (!aiProvider_) {
        LOG_ERROR("Failed to create AI provider");
        return false;
    }

    if (!config.aiApiKey.empty()) {
        if (!aiProvider_->Initialize(config.aiApiKey, config.aiEndpoint)) {
            LOG_WARNING("Failed to initialize AI provider (API key may be invalid)");
        } else {
            LOG_INFO("AI provider initialized successfully");
        }
    } else {
        LOG_WARNING("No AI API key provided, AI features will be disabled");
    }

    initialized_ = true;
    LOG_INFO("Application initialization complete");
    return true;
}

void Application::Run() {
    LOG_INFO("Application running");
    // Main application loop will be implemented here
    // This will handle UI events and user interactions
}

void Application::Shutdown() {
    LOG_INFO("Shutting down application");

    if (database_) {
        database_->Close();
        LOG_INFO("Database closed");
    }

    initialized_ = false;
    LOG_INFO("Shutdown complete");
}

} // namespace NovelStudio
