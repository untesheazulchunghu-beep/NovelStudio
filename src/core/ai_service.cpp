#include "ai_service.h"
#include "ai_providers.h"
#include "logger.h"

namespace NovelStudio {

AIService& AIService::GetInstance() {
    static AIService instance;
    return instance;
}

bool AIService::Initialize(ProviderType type, const std::string& apiKey, const std::string& endpoint) {
    LOG_INFO("Initializing AI service");

    currentProvider_ = type;

    switch (type) {
        case ProviderType::OpenAI:
            provider_ = AIProvider::CreateOpenAI();
            break;
        case ProviderType::Anthropic:
            provider_ = AIProvider::CreateAnthropic();
            break;
        case ProviderType::Local:
            provider_ = AIProvider::CreateLocal();
            break;
    }

    if (!provider_) {
        LOG_ERROR("Failed to create provider");
        return false;
    }

    if (!provider_->Initialize(apiKey, endpoint)) {
        LOG_ERROR("Failed to initialize provider");
        return false;
    }

    LOG_INFO("AI service initialized successfully");
    return true;
}

void AIService::SwitchProvider(ProviderType type, const std::string& apiKey, const std::string& endpoint) {
    LOG_INFO("Switching AI provider");
    Initialize(type, apiKey, endpoint);
}

std::string AIService::Complete(
    const std::vector<AIMessage>& messages,
    const CompletionOptions& options
) {
    if (!provider_) {
        LOG_ERROR("Provider not initialized");
        return "";
    }

    return provider_->Complete(messages, options);
}

void AIService::CompleteStreaming(
    const std::vector<AIMessage>& messages,
    std::function<void(const std::string&)> callback,
    const CompletionOptions& options
) {
    if (!provider_) {
        LOG_ERROR("Provider not initialized");
        return;
    }

    provider_->CompleteStreaming(messages, callback, options);
}

std::string AIService::GenerateFromPrompt(
    const std::string& systemPrompt,
    const std::string& userPrompt,
    const std::string& model,
    int maxTokens
) {
    std::vector<AIMessage> messages;

    if (!systemPrompt.empty()) {
        messages.push_back({AIMessage::Role::System, systemPrompt});
    }

    messages.push_back({AIMessage::Role::User, userPrompt});

    CompletionOptions options;
    if (!model.empty()) {
        options.model = model;
    }
    options.maxTokens = maxTokens;
    options.temperature = 0.8f;

    return Complete(messages, options);
}

std::string AIService::ContinueWriting(
    const std::string& context,
    const std::string& instruction,
    int maxTokens
) {
    std::string systemPrompt = "You are a creative writing assistant. Continue the story naturally, "
                              "maintaining consistent style, tone, and character voices.";

    std::string userPrompt = instruction + "\n\nContext:\n" + context;

    CompletionOptions options;
    options.maxTokens = maxTokens;
    options.temperature = 0.85f; // Slightly higher for creativity
    options.topP = 0.95f;

    std::vector<AIMessage> messages;
    messages.push_back({AIMessage::Role::System, systemPrompt});
    messages.push_back({AIMessage::Role::User, userPrompt});

    return Complete(messages, options);
}

} // namespace NovelStudio
