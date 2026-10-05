// Test program for all AI providers (OpenAI, Anthropic, Ollama)
#include "ai_providers.h"
#include "logger.h"
#include <iostream>
#include <fstream>
#include <string>

using namespace NovelStudio;

struct ProviderConfig {
    std::string openaiKey;
    std::string anthropicKey;
    std::string ollamaEndpoint;
};

ProviderConfig LoadConfig() {
    ProviderConfig config;

    std::ifstream configFile("config.json");
    if (configFile.is_open()) {
        std::string content((std::istreambuf_iterator<char>(configFile)),
                           std::istreambuf_iterator<char>());
        configFile.close();

        // Simple extraction
        auto extractValue = [&](const std::string& key) -> std::string {
            size_t pos = content.find("\"" + key + "\"");
            if (pos != std::string::npos) {
                size_t start = content.find("\"", pos + key.length() + 2);
                if (start != std::string::npos) {
                    size_t end = content.find("\"", start + 1);
                    if (end != std::string::npos) {
                        return content.substr(start + 1, end - start - 1);
                    }
                }
            }
            return "";
        };

        config.openaiKey = extractValue("openai_api_key");
        config.anthropicKey = extractValue("anthropic_api_key");
        config.ollamaEndpoint = extractValue("ollama_endpoint");
    }

    return config;
}

void TestOpenAI(const std::string& apiKey) {
    std::cout << "\n=== Testing OpenAI Provider ===" << std::endl;

    if (apiKey.empty()) {
        std::cout << "⚠ No OpenAI API key in config.json" << std::endl;
        return;
    }

    auto provider = AIProvider::CreateOpenAI();
    provider->Initialize(apiKey, "https://api.openai.com/v1/chat/completions");

    std::vector<AIMessage> messages;
    messages.push_back({AIMessage::Role::User, "Say 'Hello from OpenAI!' in one sentence."});

    CompletionOptions options;
    options.model = "gpt-3.5-turbo";
    options.maxTokens = 50;

    std::cout << "Sending request..." << std::endl;
    std::string response = provider->Complete(messages, options);

    if (!response.empty()) {
        std::cout << "✓ Response: " << response << std::endl;
    } else {
        std::cout << "✗ Request failed" << std::endl;
    }
}

void TestAnthropic(const std::string& apiKey) {
    std::cout << "\n=== Testing Anthropic Claude Provider ===" << std::endl;

    if (apiKey.empty()) {
        std::cout << "⚠ No Anthropic API key in config.json" << std::endl;
        return;
    }

    auto provider = AIProvider::CreateAnthropic();
    provider->Initialize(apiKey, "https://api.anthropic.com/v1/messages");

    std::vector<AIMessage> messages;
    messages.push_back({AIMessage::Role::User, "Say 'Hello from Claude!' in one sentence."});

    CompletionOptions options;
    options.model = "claude-3-5-sonnet-20241022";
    options.maxTokens = 50;

    std::cout << "Sending request..." << std::endl;
    std::string response = provider->Complete(messages, options);

    if (!response.empty()) {
        std::cout << "✓ Response: " << response << std::endl;
    } else {
        std::cout << "✗ Request failed" << std::endl;
    }
}

void TestOllama(const std::string& endpoint) {
    std::cout << "\n=== Testing Ollama Local Provider ===" << std::endl;

    std::string ollamaUrl = endpoint.empty() ? "http://localhost:11434/api/chat" : endpoint;

    auto provider = AIProvider::CreateLocal();
    provider->Initialize("", ollamaUrl);

    std::vector<AIMessage> messages;
    messages.push_back({AIMessage::Role::User, "Say 'Hello from Ollama!' in one sentence."});

    CompletionOptions options;
    options.model = "llama3.2";
    options.maxTokens = 50;
    options.temperature = 0.7f;

    std::cout << "Sending request to " << ollamaUrl << "..." << std::endl;
    std::string response = provider->Complete(messages, options);

    if (!response.empty()) {
        std::cout << "✓ Response: " << response << std::endl;
    } else {
        std::cout << "✗ Request failed (make sure Ollama is running)" << std::endl;
    }
}

void TestStreamingComparison(const ProviderConfig& config) {
    std::cout << "\n=== Testing Streaming (OpenAI vs Anthropic) ===" << std::endl;

    if (!config.openaiKey.empty()) {
        std::cout << "\n--- OpenAI Streaming ---" << std::endl;
        auto provider = AIProvider::CreateOpenAI();
        provider->Initialize(config.openaiKey, "https://api.openai.com/v1/chat/completions");

        std::vector<AIMessage> messages;
        messages.push_back({AIMessage::Role::User, "Count from 1 to 5."});

        CompletionOptions options;
        options.model = "gpt-3.5-turbo";
        options.maxTokens = 50;

        std::cout << "Response: ";
        provider->CompleteStreaming(messages, [](const std::string& chunk) {
            std::cout << chunk;
            std::cout.flush();
        }, options);
        std::cout << std::endl;
    }

    if (!config.anthropicKey.empty()) {
        std::cout << "\n--- Anthropic Streaming ---" << std::endl;
        auto provider = AIProvider::CreateAnthropic();
        provider->Initialize(config.anthropicKey, "https://api.anthropic.com/v1/messages");

        std::vector<AIMessage> messages;
        messages.push_back({AIMessage::Role::User, "Count from 1 to 5."});

        CompletionOptions options;
        options.model = "claude-3-5-sonnet-20241022";
        options.maxTokens = 50;

        std::cout << "Response: ";
        provider->CompleteStreaming(messages, [](const std::string& chunk) {
            std::cout << chunk;
            std::cout.flush();
        }, options);
        std::cout << std::endl;
    }
}

int main() {
    std::cout << "NovelStudio - Multi-Provider AI Test" << std::endl;
    std::cout << "=====================================" << std::endl;

    auto& logger = Logger::GetInstance();
    logger.SetLevel(LogLevel::Info);
    logger.SetLogFile("logs/providers_test.log");
    logger.EnableConsoleOutput(false);

    ProviderConfig config = LoadConfig();

    std::cout << "\nConfiguration:" << std::endl;
    std::cout << "- OpenAI API Key: " << (config.openaiKey.empty() ? "Not set" : "✓ Set") << std::endl;
    std::cout << "- Anthropic API Key: " << (config.anthropicKey.empty() ? "Not set" : "✓ Set") << std::endl;
    std::cout << "- Ollama Endpoint: " << (config.ollamaEndpoint.empty() ? "Default (localhost:11434)" : config.ollamaEndpoint) << std::endl;

    // Test each provider
    TestOpenAI(config.openaiKey);
    TestAnthropic(config.anthropicKey);
    TestOllama(config.ollamaEndpoint);

    // Test streaming
    if (!config.openaiKey.empty() || !config.anthropicKey.empty()) {
        TestStreamingComparison(config);
    }

    std::cout << "\n=== All Tests Complete ===" << std::endl;
    std::cout << "\nPress Enter to exit...";
    std::cin.get();

    return 0;
}
