#pragma once

#include <string>
#include <vector>
#include <functional>
#include <memory>

namespace NovelStudio {

// AI message structure
struct AIMessage {
    enum class Role { System, User, Assistant };
    Role role;
    std::string content;
};

// AI completion options
struct CompletionOptions {
    std::string model = "gpt-3.5-turbo";
    float temperature = 0.7f;
    int maxTokens = 2048;
    float topP = 1.0f;
    float frequencyPenalty = 0.0f;
    float presencePenalty = 0.0f;
    bool stream = false;
};

// Streaming callback
using StreamCallback = std::function<void(const std::string& chunk)>;

// AI Provider interface
class AIProvider {
public:
    virtual ~AIProvider() = default;

    virtual bool Initialize(const std::string& apiKey, const std::string& endpoint) = 0;
    virtual std::string Complete(
        const std::vector<AIMessage>& messages,
        const CompletionOptions& options = {}
    ) = 0;
    virtual void CompleteStreaming(
        const std::vector<AIMessage>& messages,
        StreamCallback callback,
        const CompletionOptions& options = {}
    ) = 0;

    // Helper methods
    static std::unique_ptr<AIProvider> CreateOpenAI();
    static std::unique_ptr<AIProvider> CreateAnthropic();
    static std::unique_ptr<AIProvider> CreateLocal();
};

// OpenAI implementation
class OpenAIProvider : public AIProvider {
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
};

} // namespace NovelStudio
