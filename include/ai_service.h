#pragma once

#include "ai_provider.h"
#include <string>
#include <memory>
#include <functional>

namespace NovelStudio {

// AI service for managing AI interactions
class AIService {
public:
    enum class ProviderType {
        OpenAI,
        Anthropic,
        Local
    };

    static AIService& GetInstance();

    // Initialize with provider
    bool Initialize(ProviderType type, const std::string& apiKey, const std::string& endpoint);

    // Switch provider at runtime
    void SwitchProvider(ProviderType type, const std::string& apiKey, const std::string& endpoint);

    // Get current provider
    ProviderType GetCurrentProvider() const { return currentProvider_; }

    // Generate completion
    std::string Complete(
        const std::vector<AIMessage>& messages,
        const CompletionOptions& options = {}
    );

    // Generate streaming completion
    void CompleteStreaming(
        const std::vector<AIMessage>& messages,
        std::function<void(const std::string&)> callback,
        const CompletionOptions& options = {}
    );

    // Helper: Generate from prompt
    std::string GenerateFromPrompt(
        const std::string& systemPrompt,
        const std::string& userPrompt,
        const std::string& model = "",
        int maxTokens = 2048
    );

    // Helper: Continue writing
    std::string ContinueWriting(
        const std::string& context,
        const std::string& instruction = "Continue writing naturally.",
        int maxTokens = 500
    );

private:
    AIService() = default;
    std::unique_ptr<AIProvider> provider_;
    ProviderType currentProvider_ = ProviderType::OpenAI;
};

} // namespace NovelStudio
