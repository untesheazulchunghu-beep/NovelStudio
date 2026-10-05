#pragma once

#include "ai_provider.h"

namespace NovelStudio {

// Anthropic Claude provider implementation
class AnthropicProvider : public AIProvider {
public:
    bool Initialize(const std::string& apiKey, const std::string& endpoint) override;

    std::string Complete(
        const std::vector<AIMessage>& messages,
        const CompletionOptions& options
    ) override;

    void CompleteStreaming(
        const std::vector<AIMessage>& messages,
        StreamCallback callback,
        const CompletionOptions& options
    ) override;

private:
    std::string apiKey_;
    std::string endpoint_;
    std::string apiVersion_ = "2023-06-01";
};

// Local model provider (Ollama, LM Studio)
class LocalProvider : public AIProvider {
public:
    bool Initialize(const std::string& apiKey, const std::string& endpoint) override;

    std::string Complete(
        const std::vector<AIMessage>& messages,
        const CompletionOptions& options
    ) override;

    void CompleteStreaming(
        const std::vector<AIMessage>& messages,
        StreamCallback callback,
        const CompletionOptions& options
    ) override;

private:
    std::string endpoint_;
    bool isOllama_ = false;
};

} // namespace NovelStudio
