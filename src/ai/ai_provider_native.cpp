#include "ai_provider.h"
#include "http_client.h"
#include "json_utils.h"
#include "sse_parser.h"
#include "logger.h"
#include <sstream>

namespace NovelStudio {

// Factory methods
std::unique_ptr<AIProvider> AIProvider::CreateOpenAI() {
    return std::make_unique<OpenAIProvider>();
}

std::unique_ptr<AIProvider> AIProvider::CreateAnthropic() {
    extern std::unique_ptr<AIProvider> CreateAnthropicProviderImpl();
    return CreateAnthropicProviderImpl();
}

std::unique_ptr<AIProvider> AIProvider::CreateLocal() {
    extern std::unique_ptr<AIProvider> CreateLocalProviderImpl();
    return CreateLocalProviderImpl();
}

// OpenAI implementation using native HTTP client
bool OpenAIProvider::Initialize(const std::string& apiKey, const std::string& endpoint) {
    apiKey_ = apiKey;
    endpoint_ = endpoint.empty() ? "https://api.openai.com/v1/chat/completions" : endpoint;

    LOG_INFO("OpenAI provider initialized with endpoint: " + endpoint_);
    return true;
}

std::string OpenAIProvider::Complete(
    const std::vector<AIMessage>& messages,
    const CompletionOptions& options
) {
    if (apiKey_.empty()) {
        LOG_ERROR("API key not set");
        return "";
    }

    // Build JSON request using our lightweight JSON library
    JsonValue request = JsonValue::Object();
    request["model"] = JsonValue(options.model);
    request["temperature"] = JsonValue(options.temperature);
    request["max_tokens"] = JsonValue(options.maxTokens);
    request["top_p"] = JsonValue(options.topP);
    request["frequency_penalty"] = JsonValue(options.frequencyPenalty);
    request["presence_penalty"] = JsonValue(options.presencePenalty);

    JsonValue messagesJson = JsonValue::Array();
    for (const auto& msg : messages) {
        JsonValue msgJson = JsonValue::Object();

        switch (msg.role) {
            case AIMessage::Role::System:
                msgJson["role"] = JsonValue("system");
                break;
            case AIMessage::Role::User:
                msgJson["role"] = JsonValue("user");
                break;
            case AIMessage::Role::Assistant:
                msgJson["role"] = JsonValue("assistant");
                break;
        }
        msgJson["content"] = JsonValue(msg.content);
        messagesJson.Push(msgJson);
    }
    request["messages"] = messagesJson;

    std::string requestBody = request.ToString();

    LOG_DEBUG("Sending request to OpenAI API");

    // Use native HTTP client
    HttpClient client;
    std::map<std::string, std::string> headers;
    headers["Content-Type"] = "application/json";
    headers["Authorization"] = "Bearer " + apiKey_;

    HttpResponse response = client.Post(endpoint_, requestBody, headers);

    if (!response.success) {
        LOG_ERROR("HTTP request failed: " + response.error);
        LOG_ERROR("Status code: " + std::to_string(response.statusCode));
        return "";
    }

    // Parse response using our lightweight JSON library
    try {
        JsonValue responseJson = JsonValue::Parse(response.body);

        if (responseJson.Has("error")) {
            std::string errorMsg = responseJson["error"]["message"].AsString();
            LOG_ERROR("API error: " + errorMsg);
            return "";
        }

        if (responseJson.Has("choices") && responseJson["choices"].Size() > 0) {
            std::string content = responseJson["choices"][0]["message"]["content"].AsString();
            LOG_DEBUG("Received response: " + std::to_string(content.length()) + " characters");
            return content;
        }
    } catch (const std::exception& e) {
        LOG_ERROR("Failed to parse response: " + std::string(e.what()));
        LOG_DEBUG("Response body: " + response.body);
        return "";
    }

    return "";
}

void OpenAIProvider::CompleteStreaming(
    const std::vector<AIMessage>& messages,
    StreamCallback callback,
    const CompletionOptions& options
) {
    if (apiKey_.empty()) {
        LOG_ERROR("API key not set");
        return;
    }

    // Build JSON request with streaming enabled
    JsonValue request = JsonValue::Object();
    request["model"] = JsonValue(options.model);
    request["temperature"] = JsonValue(options.temperature);
    request["max_tokens"] = JsonValue(options.maxTokens);
    request["top_p"] = JsonValue(options.topP);
    request["frequency_penalty"] = JsonValue(options.frequencyPenalty);
    request["presence_penalty"] = JsonValue(options.presencePenalty);
    request["stream"] = JsonValue(true); // Enable streaming

    JsonValue messagesJson = JsonValue::Array();
    for (const auto& msg : messages) {
        JsonValue msgJson = JsonValue::Object();

        switch (msg.role) {
            case AIMessage::Role::System:
                msgJson["role"] = JsonValue("system");
                break;
            case AIMessage::Role::User:
                msgJson["role"] = JsonValue("user");
                break;
            case AIMessage::Role::Assistant:
                msgJson["role"] = JsonValue("assistant");
                break;
        }
        msgJson["content"] = JsonValue(msg.content);
        messagesJson.Push(msgJson);
    }
    request["messages"] = messagesJson;

    std::string requestBody = request.ToString();

    LOG_DEBUG("Sending streaming request to OpenAI API");

    // Use native HTTP client with streaming
    HttpClient client;
    std::map<std::string, std::string> headers;
    headers["Content-Type"] = "application/json";
    headers["Authorization"] = "Bearer " + apiKey_;
    headers["Accept"] = "text/event-stream";

    // Setup SSE parser
    SSEParser parser;
    std::string fullText;

    parser.OnData([&](const std::string& data) {
        try {
            // Parse JSON chunk
            JsonValue chunk = JsonValue::Parse(data);

            if (chunk.Has("choices") && chunk["choices"].Size() > 0) {
                JsonValue choice = chunk["choices"][0];

                if (choice.Has("delta")) {
                    JsonValue delta = choice["delta"];

                    if (delta.Has("content")) {
                        std::string content = delta["content"].AsString();
                        if (!content.empty()) {
                            fullText += content;
                            callback(content);
                        }
                    }
                }

                // Check finish reason
                if (choice.Has("finish_reason") && !choice["finish_reason"].IsNull()) {
                    LOG_DEBUG("Stream finished: " + choice["finish_reason"].AsString());
                }
            }
        } catch (const std::exception& e) {
            LOG_ERROR("Failed to parse streaming chunk: " + std::string(e.what()));
        }
    });

    // Stream the response
    bool success = client.PostStreaming(endpoint_, requestBody, headers,
        [&](const std::string& chunk) {
            parser.Feed(chunk.data(), chunk.length());
        }
    );

    if (!success) {
        LOG_ERROR("Streaming request failed");
    } else {
        LOG_DEBUG("Streaming complete, total length: " + std::to_string(fullText.length()));
    }
}

} // namespace NovelStudio
