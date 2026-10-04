#include "ai_middleware.h"
#include "logger.h"
#include <chrono>
#include <thread>
#include <algorithm>

namespace NovelStudio {

// AIProviderRegistry implementation
AIProviderRegistry& AIProviderRegistry::GetInstance() {
    static AIProviderRegistry instance;
    return instance;
}

void AIProviderRegistry::RegisterProvider(const std::string& name, std::unique_ptr<AIProvider> provider) {
    LOG_INFO("Registering AI provider: " + name);
    providers_[name] = std::move(provider);
}

AIProvider* AIProviderRegistry::GetProvider(const std::string& name) {
    auto it = providers_.find(name);
    if (it != providers_.end()) {
        return it->second.get();
    }
    LOG_WARNING("AI provider not found: " + name);
    return nullptr;
}

std::vector<std::string> AIProviderRegistry::ListProviders() const {
    std::vector<std::string> names;
    for (const auto& [name, _] : providers_) {
        names.push_back(name);
    }
    return names;
}

bool AIProviderRegistry::HasProvider(const std::string& name) const {
    return providers_.find(name) != providers_.end();
}

// CacheMiddleware implementation
CacheMiddleware::CacheMiddleware(int ttlSeconds) : ttlSeconds_(ttlSeconds) {
    LOG_INFO("CacheMiddleware initialized with TTL: " + std::to_string(ttlSeconds) + "s");
}

std::string CacheMiddleware::Process(
    const std::string& input,
    std::function<std::string(const std::string&)> next
) {
    int64_t now = std::chrono::duration_cast<std::chrono::seconds>(
        std::chrono::system_clock::now().time_since_epoch()
    ).count();

    // Check cache
    auto it = cache_.find(input);
    if (it != cache_.end()) {
        if (now - it->second.timestamp < ttlSeconds_) {
            LOG_DEBUG("Cache hit");
            return it->second.output;
        } else {
            // Expired
            cache_.erase(it);
        }
    }

    LOG_DEBUG("Cache miss, calling next middleware");
    std::string output = next(input);

    // Store in cache
    cache_[input] = CacheEntry{output, now};

    return output;
}

void CacheMiddleware::Clear() {
    cache_.clear();
    LOG_INFO("Cache cleared");
}

size_t CacheMiddleware::GetCacheSize() const {
    return cache_.size();
}

// RetryMiddleware implementation
RetryMiddleware::RetryMiddleware(int maxRetries, int delayMs)
    : maxRetries_(maxRetries), delayMs_(delayMs) {
    LOG_INFO("RetryMiddleware initialized: max " + std::to_string(maxRetries) + " retries");
}

std::string RetryMiddleware::Process(
    const std::string& input,
    std::function<std::string(const std::string&)> next
) {
    for (int attempt = 0; attempt <= maxRetries_; ++attempt) {
        try {
            if (attempt > 0) {
                LOG_INFO("Retry attempt " + std::to_string(attempt));
                std::this_thread::sleep_for(std::chrono::milliseconds(delayMs_ * attempt));
            }

            std::string result = next(input);
            if (!result.empty()) {
                return result;
            }
        } catch (const std::exception& e) {
            LOG_WARNING("Attempt " + std::to_string(attempt) + " failed: " + e.what());
            if (attempt == maxRetries_) {
                throw;
            }
        }
    }

    return "";
}

// RateLimitMiddleware implementation
RateLimitMiddleware::RateLimitMiddleware(int requestsPerMinute)
    : requestsPerMinute_(requestsPerMinute) {
    LOG_INFO("RateLimitMiddleware initialized: " + std::to_string(requestsPerMinute) + " req/min");
}

std::string RateLimitMiddleware::Process(
    const std::string& input,
    std::function<std::string(const std::string&)> next
) {
    int64_t now = std::chrono::duration_cast<std::chrono::milliseconds>(
        std::chrono::system_clock::now().time_since_epoch()
    ).count();

    // Remove timestamps older than 1 minute
    auto cutoff = now - 60000;
    requestTimestamps_.erase(
        std::remove_if(requestTimestamps_.begin(), requestTimestamps_.end(),
            [cutoff](int64_t ts) { return ts < cutoff; }),
        requestTimestamps_.end()
    );

    // Check rate limit
    if (requestTimestamps_.size() >= static_cast<size_t>(requestsPerMinute_)) {
        int64_t oldestRequest = requestTimestamps_.front();
        int64_t waitTime = (oldestRequest + 60000) - now;

        if (waitTime > 0) {
            LOG_INFO("Rate limit reached, waiting " + std::to_string(waitTime) + "ms");
            std::this_thread::sleep_for(std::chrono::milliseconds(waitTime));
        }
    }

    requestTimestamps_.push_back(now);
    return next(input);
}

// TokenCounterMiddleware implementation
TokenCounterMiddleware::TokenCounterMiddleware() {
    stats_ = Stats{};
    LOG_INFO("TokenCounterMiddleware initialized");
}

std::string TokenCounterMiddleware::Process(
    const std::string& input,
    std::function<std::string(const std::string&)> next
) {
    stats_.totalRequests++;

    int inputTokens = EstimateTokens(input);
    stats_.totalInputTokens += inputTokens;

    std::string output = next(input);

    int outputTokens = EstimateTokens(output);
    stats_.totalOutputTokens += outputTokens;

    // Rough cost estimate (OpenAI GPT-3.5-turbo pricing)
    stats_.estimatedCost += (inputTokens * 0.0015 / 1000) + (outputTokens * 0.002 / 1000);

    LOG_DEBUG("Request tokens: " + std::to_string(inputTokens) + " in, " +
              std::to_string(outputTokens) + " out");

    return output;
}

TokenCounterMiddleware::Stats TokenCounterMiddleware::GetStats() const {
    return stats_;
}

void TokenCounterMiddleware::ResetStats() {
    stats_ = Stats{};
    LOG_INFO("Token counter stats reset");
}

int TokenCounterMiddleware::EstimateTokens(const std::string& text) const {
    // Rough estimate: ~4 characters per token
    return static_cast<int>(text.length() / 4);
}

// EnhancedAIProvider implementation
EnhancedAIProvider::EnhancedAIProvider(std::unique_ptr<AIProvider> baseProvider)
    : baseProvider_(std::move(baseProvider)) {
    LOG_INFO("EnhancedAIProvider created");
}

void EnhancedAIProvider::AddMiddleware(std::unique_ptr<AIMiddleware> middleware) {
    middlewares_.push_back(std::move(middleware));
    LOG_INFO("Middleware added, total: " + std::to_string(middlewares_.size()));
}

bool EnhancedAIProvider::Initialize(const std::string& apiKey, const std::string& endpoint) {
    return baseProvider_->Initialize(apiKey, endpoint);
}

std::string EnhancedAIProvider::Complete(
    const std::vector<AIMessage>& messages,
    const CompletionOptions& options
) {
    // Build input string for middleware
    std::string input;
    for (const auto& msg : messages) {
        input += msg.content + "\n";
    }

    // Build middleware chain
    std::function<std::string(const std::string&)> chain =
        [this, &messages, &options](const std::string&) {
            return baseProvider_->Complete(messages, options);
        };

    // Apply middlewares in reverse order
    for (auto it = middlewares_.rbegin(); it != middlewares_.rend(); ++it) {
        auto currentChain = chain;
        auto middleware = it->get();
        chain = [middleware, currentChain](const std::string& in) {
            return middleware->Process(in, currentChain);
        };
    }

    return chain(input);
}

void EnhancedAIProvider::CompleteStreaming(
    const std::vector<AIMessage>& messages,
    StreamCallback callback,
    const CompletionOptions& options
) {
    // Streaming with middleware is complex, delegate to base for now
    baseProvider_->CompleteStreaming(messages, callback, options);
}

} // namespace NovelStudio
