// Test program for streaming AI functionality
#include "ai_provider.h"
#include "sse_parser.h"
#include "logger.h"
#include <iostream>
#include <fstream>
#include <string>

using namespace NovelStudio;

std::string LoadApiKey() {
    // Try to load from config.json
    std::ifstream configFile("config.json");
    if (configFile.is_open()) {
        std::string content((std::istreambuf_iterator<char>(configFile)),
                           std::istreambuf_iterator<char>());
        configFile.close();

        // Simple JSON parsing to extract apiKey
        size_t pos = content.find("\"openai_api_key\"");
        if (pos != std::string::npos) {
            size_t start = content.find("\"", pos + 16);
            if (start != std::string::npos) {
                size_t end = content.find("\"", start + 1);
                if (end != std::string::npos) {
                    return content.substr(start + 1, end - start - 1);
                }
            }
        }
    }
    return "";
}

void TestStreamingCompletion() {
    std::cout << "\n=== Testing Streaming AI Completion ===" << std::endl;

    // Load API key
    std::string apiKey = LoadApiKey();
    if (apiKey.empty()) {
        std::cout << "⚠ No API key found in config.json" << std::endl;
        std::cout << "Create config.json with:" << std::endl;
        std::cout << R"({
  "openai_api_key": "sk-your-key-here",
  "openai_endpoint": "https://api.openai.com/v1/chat/completions"
})" << std::endl;
        return;
    }

    std::cout << "✓ API key loaded" << std::endl;

    // Create OpenAI provider
    auto provider = AIProvider::CreateOpenAI();
    if (!provider->Initialize(apiKey, "https://api.openai.com/v1/chat/completions")) {
        std::cout << "✗ Failed to initialize provider" << std::endl;
        return;
    }

    std::cout << "✓ Provider initialized" << std::endl;

    // Prepare messages
    std::vector<AIMessage> messages;
    messages.push_back({
        AIMessage::Role::System,
        "You are a helpful writing assistant."
    });
    messages.push_back({
        AIMessage::Role::User,
        "Write a short opening sentence for a sci-fi novel about space exploration."
    });

    // Completion options
    CompletionOptions options;
    options.model = "gpt-3.5-turbo";
    options.temperature = 0.8f;
    options.maxTokens = 100;
    options.stream = true;

    std::cout << "\nSending streaming request..." << std::endl;
    std::cout << "Response: ";
    std::cout.flush();

    std::string fullResponse;
    int chunkCount = 0;

    // Call streaming API
    provider->CompleteStreaming(messages, [&](const std::string& chunk) {
        std::cout << chunk;
        std::cout.flush();
        fullResponse += chunk;
        chunkCount++;
    }, options);

    std::cout << std::endl;
    std::cout << "\n✓ Stream complete!" << std::endl;
    std::cout << "Total chunks received: " << chunkCount << std::endl;
    std::cout << "Total characters: " << fullResponse.length() << std::endl;
}

void TestSSEParser() {
    std::cout << "\n=== Testing SSE Parser ===" << std::endl;

    SSEParser parser;
    int eventCount = 0;

    parser.OnData([&](const std::string& data) {
        std::cout << "Data event: " << data << std::endl;
        eventCount++;
    });

    // Simulate SSE stream
    const char* sseData =
        "data: {\"id\":\"1\",\"object\":\"chat.completion.chunk\",\"choices\":[{\"delta\":{\"content\":\"Hello\"}}]}\n"
        "\n"
        "data: {\"id\":\"2\",\"object\":\"chat.completion.chunk\",\"choices\":[{\"delta\":{\"content\":\" World\"}}]}\n"
        "\n"
        "data: [DONE]\n"
        "\n";

    parser.Feed(sseData, strlen(sseData));

    std::cout << "Received " << eventCount << " events" << std::endl;
    std::cout << "Stream done: " << (parser.IsDone() ? "yes" : "no") << std::endl;
}

int main() {
    std::cout << "NovelStudio - Streaming AI Test" << std::endl;
    std::cout << "================================" << std::endl;

    auto& logger = Logger::GetInstance();
    logger.SetLevel(LogLevel::Debug);
    logger.SetLogFile("logs/streaming_test.log");
    logger.EnableConsoleOutput(true);

    TestSSEParser();
    TestStreamingCompletion();

    std::cout << "\n=== All Tests Complete ===" << std::endl;
    std::cout << "\nPress Enter to exit...";
    std::cin.get();

    return 0;
}
