#pragma once

#include "ai_provider.h"
#include <functional>
#include <memory>
#include <unordered_map>

namespace NovelStudio {

// AI Provider Registry
class AIProviderRegistry {
public:
    static AIProviderRegistry& GetInstance();

    void RegisterProvider(const std::string& name, std::unique_ptr<AIProvider> provider);
    AIProvider* GetProvider(const std::string& name);
    std::vector<std::string> ListProviders() const;
    bool HasProvider(const std::string& name) const;

private:
    AIProviderRegistry() = default;
    std::unordered_map<std::string, std::unique_ptr<AIProvider>> providers_;
};

// AI Middleware interface
class AIMiddleware {
public:
    virtual ~AIMiddleware() = default;

    virtual std::string Process(
        const std::string& input,
        std::function<std::string(const std::string&)> next
    ) = 0;
};

// Cache middleware
class CacheMiddleware : public AIMiddleware {
public:
    explicit CacheMiddleware(int ttlSeconds = 3600);

    std::string Process(
        const std::string& input,
        std::function<std::string(const std::string&)> next
    ) override;

    void Clear();
    size_t GetCacheSize() const;

private:
    struct CacheEntry {
        std::string output;
        int64_t timestamp;
    };

    std::unordered_map<std::string, CacheEntry> cache_;
    int ttlSeconds_;
};

// Retry middleware
class RetryMiddleware : public AIMiddleware {
public:
    explicit RetryMiddleware(int maxRetries = 3, int delayMs = 1000);

    std::string Process(
        const std::string& input,
        std::function<std::string(const std::string&)> next
    ) override;

private:
    int maxRetries_;
    int delayMs_;
};

// Rate limit middleware
class RateLimitMiddleware : public AIMiddleware {
public:
    explicit RateLimitMiddleware(int requestsPerMinute);

    std::string Process(
        const std::string& input,
        std::function<std::string(const std::string&)> next
    ) override;

private:
    int requestsPerMinute_;
    std::vector<int64_t> requestTimestamps_;
};

// Token counter middleware
class TokenCounterMiddleware : public AIMiddleware {
public:
    TokenCounterMiddleware();

    std::string Process(
        const std::string& input,
        std::function<std::string(const std::string&)> next
    ) override;

    struct Stats {
        int64_t totalRequests;
        int64_t totalInputTokens;
        int64_t totalOutputTokens;
        double estimatedCost;
    };

    Stats GetStats() const;
    void ResetStats();

private:
    int EstimateTokens(const std::string& text) const;

    Stats stats_;
};

// Enhanced AI Provider with middleware support
class EnhancedAIProvider : public AIProvider {
public:
    explicit EnhancedAIProvider(std::unique_ptr<AIProvider> baseProvider);

    void AddMiddleware(std::unique_ptr<AIMiddleware> middleware);

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
    std::unique_ptr<AIProvider> baseProvider_;
    std::vector<std::unique_ptr<AIMiddleware>> middlewares_;
};

} // namespace NovelStudio
