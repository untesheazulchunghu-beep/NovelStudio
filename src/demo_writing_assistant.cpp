// Complete AI Writing Assistant Demo
#include "ai_service.h"
#include "config_manager.h"
#include "logger.h"
#include <iostream>
#include <string>

using namespace NovelStudio;

void DisplayMenu() {
    std::cout << "\n========================================" << std::endl;
    std::cout << "NovelStudio AI Writing Assistant" << std::endl;
    std::cout << "========================================" << std::endl;
    std::cout << "1. Test OpenAI Provider" << std::endl;
    std::cout << "2. Test Anthropic Provider" << std::endl;
    std::cout << "3. Test Ollama Local Provider" << std::endl;
    std::cout << "4. Continue Writing (Creative)" << std::endl;
    std::cout << "5. Generate Story Opening" << std::endl;
    std::cout << "6. Character Description" << std::endl;
    std::cout << "7. Streaming Demo" << std::endl;
    std::cout << "0. Exit" << std::endl;
    std::cout << "========================================" << std::endl;
    std::cout << "Choice: ";
}

void TestProvider(AIService::ProviderType type, const std::string& name,
                  const std::string& apiKey, const std::string& endpoint) {
    std::cout << "\n=== Testing " << name << " ===" << std::endl;

    if (apiKey.empty() && type != AIService::ProviderType::Local) {
        std::cout << "No API key configured for " << name << std::endl;
        return;
    }

    auto& aiService = AIService::GetInstance();
    if (!aiService.Initialize(type, apiKey, endpoint)) {
        std::cout << "Failed to initialize " << name << std::endl;
        return;
    }

    std::string response = aiService.GenerateFromPrompt(
        "You are a helpful assistant.",
        "Say 'Hello from " + name + "!' in one creative sentence.",
        "",
        50
    );

    if (!response.empty()) {
        std::cout << "\nResponse: " << response << std::endl;
        std::cout << "\nSuccess: " << name << " is working!" << std::endl;
    } else {
        std::cout << "\nFailed: " << name << " request failed" << std::endl;
    }
}

void ContinueWritingDemo() {
    std::cout << "\n=== Continue Writing Demo ===" << std::endl;
    std::cout << "\nContext:" << std::endl;
    std::cout << "---------------------------------------" << std::endl;

    std::string context =
        "Sarah stood at the edge of the cliff, the ocean spray "
        "stinging her face. Behind her, footsteps crunched on gravel.";

    std::cout << context << std::endl;
    std::cout << "---------------------------------------" << std::endl;

    auto& aiService = AIService::GetInstance();
    std::cout << "\nGenerating continuation..." << std::endl;

    std::string continuation = aiService.ContinueWriting(
        context,
        "Continue the story with tension and mystery.",
        300
    );

    if (!continuation.empty()) {
        std::cout << "\nContinuation:" << std::endl;
        std::cout << "---------------------------------------" << std::endl;
        std::cout << continuation << std::endl;
        std::cout << "---------------------------------------" << std::endl;
    } else {
        std::cout << "Generation failed" << std::endl;
    }
}

void GenerateStoryOpening() {
    std::cout << "\n=== Generate Story Opening ===" << std::endl;
    std::cout << "Genre: Sci-Fi Space Opera" << std::endl;

    auto& aiService = AIService::GetInstance();

    std::string opening = aiService.GenerateFromPrompt(
        "You are a creative fiction writer specializing in science fiction.",
        "Write a compelling opening paragraph for a space opera novel about a lone pilot "
        "discovering an ancient alien artifact. Make it mysterious and engaging.",
        "",
        200
    );

    if (!opening.empty()) {
        std::cout << "\nGenerated Opening:" << std::endl;
        std::cout << "---------------------------------------" << std::endl;
        std::cout << opening << std::endl;
        std::cout << "---------------------------------------" << std::endl;
    }
}

void GenerateCharacterDescription() {
    std::cout << "\n=== Generate Character Description ===" << std::endl;
    std::cout << "Character: Elena, a cybersecurity expert turned rebel" << std::endl;

    auto& aiService = AIService::GetInstance();

    std::string description = aiService.GenerateFromPrompt(
        "You are a character development expert for novels.",
        "Create a detailed character description for Elena: a 32-year-old former "
        "cybersecurity expert who became a rebel leader. Include physical appearance, "
        "personality traits, and a defining characteristic.",
        "",
        250
    );

    if (!description.empty()) {
        std::cout << "\nCharacter Description:" << std::endl;
        std::cout << "---------------------------------------" << std::endl;
        std::cout << description << std::endl;
        std::cout << "---------------------------------------" << std::endl;
    }
}

void StreamingDemo() {
    std::cout << "\n=== Streaming Demo ===" << std::endl;
    std::cout << "Watching AI think in real-time..." << std::endl;
    std::cout << "\nPrompt: Write a haiku about coding\n" << std::endl;

    auto& aiService = AIService::GetInstance();

    std::vector<AIMessage> messages;
    messages.push_back({AIMessage::Role::User, "Write a haiku about coding."});

    CompletionOptions options;
    options.maxTokens = 100;
    options.temperature = 0.8f;

    std::cout << "Response: ";
    std::cout.flush();

    aiService.CompleteStreaming(messages, [](const std::string& chunk) {
        std::cout << chunk;
        std::cout.flush();
    }, options);

    std::cout << "\n" << std::endl;
}

int main() {
    // Setup logging
    auto& logger = Logger::GetInstance();
    logger.SetLevel(LogLevel::Info);
    logger.SetLogFile("logs/demo.log");
    logger.EnableConsoleOutput(false);

    // Load configuration
    auto& config = ConfigManager::GetInstance();
    if (!config.Load("config.json")) {
        std::cout << "Warning: config.json not found, using defaults" << std::endl;
        std::cout << "Create config.json from config.example.json" << std::endl;
    }

    std::cout << "\n========================================" << std::endl;
    std::cout << "NovelStudio - AI Writing Assistant Demo" << std::endl;
    std::cout << "========================================" << std::endl;

    // Check available providers
    std::cout << "\nConfigured Providers:" << std::endl;
    std::cout << "- OpenAI: " << (!config.GetOpenAIKey().empty() ? "Available" : "Not configured (no API key)") << std::endl;
    std::cout << "- Anthropic: " << (!config.GetAnthropicKey().empty() ? "Available" : "Not configured (no API key)") << std::endl;
    std::cout << "- Ollama: " << config.GetOllamaEndpoint() << std::endl;

    // Initialize default provider
    auto& aiService = AIService::GetInstance();
    if (!config.GetOpenAIKey().empty()) {
        std::cout << "\nUsing OpenAI as default provider" << std::endl;
        aiService.Initialize(
            AIService::ProviderType::OpenAI,
            config.GetOpenAIKey(),
            config.Get("openai_endpoint", "https://api.openai.com/v1/chat/completions")
        );
    } else if (!config.GetAnthropicKey().empty()) {
        std::cout << "\nUsing Anthropic as default provider" << std::endl;
        aiService.Initialize(
            AIService::ProviderType::Anthropic,
            config.GetAnthropicKey(),
            config.Get("anthropic_endpoint", "https://api.anthropic.com/v1/messages")
        );
    } else {
        std::cout << "\nUsing Ollama as default provider" << std::endl;
        aiService.Initialize(
            AIService::ProviderType::Local,
            "",
            config.GetOllamaEndpoint()
        );
    }

    // Main loop
    bool running = true;
    while (running) {
        DisplayMenu();

        int choice;
        std::cin >> choice;
        std::cin.ignore();

        switch (choice) {
            case 1:
                TestProvider(
                    AIService::ProviderType::OpenAI,
                    "OpenAI",
                    config.GetOpenAIKey(),
                    config.Get("openai_endpoint", "https://api.openai.com/v1/chat/completions")
                );
                break;

            case 2:
                TestProvider(
                    AIService::ProviderType::Anthropic,
                    "Anthropic Claude",
                    config.GetAnthropicKey(),
                    config.Get("anthropic_endpoint", "https://api.anthropic.com/v1/messages")
                );
                break;

            case 3:
                TestProvider(
                    AIService::ProviderType::Local,
                    "Ollama",
                    "",
                    config.GetOllamaEndpoint()
                );
                break;

            case 4:
                ContinueWritingDemo();
                break;

            case 5:
                GenerateStoryOpening();
                break;

            case 6:
                GenerateCharacterDescription();
                break;

            case 7:
                StreamingDemo();
                break;

            case 0:
                running = false;
                break;

            default:
                std::cout << "Invalid choice" << std::endl;
        }

        if (running) {
            std::cout << "\nPress Enter to continue...";
            std::cin.get();
        }
    }

    std::cout << "\nThank you for using NovelStudio!" << std::endl;
    return 0;
}
