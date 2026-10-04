#include "novel_studio.h"
#include "database.h"
#include "ai_provider.h"

namespace NovelStudio {

Application::Application() = default;

Application::~Application() {
    Shutdown();
}

bool Application::Initialize(const AppConfig& config) {
    if (initialized_) {
        return true;
    }

    config_ = config;

    // Initialize database
    database_ = std::make_unique<Database>();
    if (!database_->Open(config.databasePath)) {
        return false;
    }

    // Initialize AI provider
    aiProvider_ = AIProvider::CreateOpenAI();
    if (!aiProvider_) {
        return false;
    }

    if (!aiProvider_->Initialize(config.aiApiKey, config.aiEndpoint)) {
        return false;
    }

    initialized_ = true;
    return true;
}

void Application::Run() {
    // Main application loop will be implemented here
    // This will handle UI events and user interactions
}

void Application::Shutdown() {
    if (database_) {
        database_->Close();
    }
    initialized_ = false;
}

} // namespace NovelStudio
