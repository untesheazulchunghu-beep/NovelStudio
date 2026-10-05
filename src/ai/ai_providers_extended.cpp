#include "ai_providers.h"
#include "http_client.h"
#include "json_utils.h"
#include "sse_parser.h"
#include "logger.h"
#include <algorithm>

namespace NovelStudio {

// ============================================================================
// Anthropic Claude Provider
// ============================================================================

bool AnthropicProvider::Initialize(const std::string& apiKey, const std::string& endpoint) {
    apiKey_ = apiKey;
    endpoint_ = endpoint.empty() ? "https://api.anthropic.com/v1/messages" : endpoint;

    LOG_INFO("Anthropic provider initialized with endpoint: " + endpoint_);
    return true;
}

std::string AnthropicProvider::Complete(
    const std::vector<AIMessage>& messages,
    const CompletionOptions& options
) {
    if (apiKey_.empty()) {
        LOG_ERROR("API key not set");
        return "";
    }

    // Anthropic format: separate system message from other messages
    std::string systemPrompt;
    JsonValue anthropicMessages = JsonValue::Array();

    for (const auto& msg : messages) {
        if (msg.role == AIMessage::Role::System) {
            systemPrompt = msg.content;
        } else {
            JsonValue msgJson = JsonValue::Object();
            msgJson["role"] = JsonValue(
                msg.role == AIMessage::Role::User ? "user" : "assistant"
            );
            msgJson["content"] = JsonValue(msg.content);
            anthropicMessages.Push(msgJson);
        }
    }

    // Build request
    JsonValue request = JsonValue::Object();
    request["model"] = JsonValue(options.model.empty() ? "claude-3-5-sonnet-20241022" : options.model);
    request["max_tokens"] = JsonValue(options.maxTokens);
    request["messages"] = anthropicMessages;

    if (!systemPrompt.empty()) {
        request["system"] = JsonValue(systemPrompt);
    }

    if (options.temperature > 0) {
        request["temperature"] = JsonValue(options.temperature);
    }

    std::string requestBody = request.ToString();
    LOG_DEBUG("Sending request to Anthropic API");

    // HTTP request
    HttpClient client;
    std::map<std::string, std::string> headers;
    headers["Content-Type"] = "application/json";
    headers["x-api-key"] = apiKey_;
    headers["anthropic-version"] = apiVersion_;

    HttpResponse response = client.Post(endpoint_, requestBody, headers);

    if (!response.success) {
        LOG_ERROR("HTTP request failed: " + response.error);
        LOG_ERROR("Status code: " + std::to_string(response.statusCode));
        return "";
    }

    // Parse response
    try {
        JsonValue responseJson = JsonValue::Parse(response.body);

        if (responseJson.Has("error")) {
            std::string errorMsg = responseJson["error"]["message"].AsString();
            LOG_ERROR("API error: " + errorMsg);
            return "";
        }

        if (responseJson.Has("content") && responseJson["content"].Size() > 0) {
            std::string content = responseJson["content"][0]["text"].AsString();
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

void AnthropicProvider::CompleteStreaming(
    const std::vector<AIMessage>& messages,
    StreamCallback callback,
    const CompletionOptions& options
) {
    if (apiKey_.empty()) {
        LOG_ERROR("API key not set");
        return;
    }

    // Build request with streaming
    std::string systemPrompt;
    JsonValue anthropicMessages = JsonValue::Array();

    for (const auto& msg : messages) {
        if (msg.role == AIMessage::Role::System) {
            systemPrompt = msg.content;
        } else {
            JsonValue msgJson = JsonValue::Object();
            msgJson["role"] = JsonValue(
                msg.role == AIMessage::Role::User ? "user" : "assistant"
            );
            msgJson["content"] = JsonValue(msg.content);
            anthropicMessages.Push(msgJson);
        }
    }

    JsonValue request = JsonValue::Object();
    request["model"] = JsonValue(options.model.empty() ? "claude-3-5-sonnet-20241022" : options.model);
    request["max_tokens"] = JsonValue(options.maxTokens);
    request["messages"] = anthropicMessages;
    request["stream"] = JsonValue(true);

    if (!systemPrompt.empty()) {
        request["system"] = JsonValue(systemPrompt);
    }

    std::string requestBody = request.ToString();
    LOG_DEBUG("Sending streaming request to Anthropic API");

    HttpClient client;
    std::map<std::string, std::string> headers;
    headers["Content-Type"] = "application/json";
    headers["x-api-key"] = apiKey_;
    headers["anthropic-version"] = apiVersion_;
    headers["Accept"] = "text/event-stream";

    SSEParser parser;
    std::string fullText;

    parser.OnData([&](const std::string& data) {
        try {
            JsonValue chunk = JsonValue::Parse(data);

            if (!chunk.Has("type")) return;

            std::string eventType = chunk["type"].AsString();

            if (eventType == "content_block_delta") {
                if (chunk.Has("delta") && chunk["delta"].Has("text")) {
                    std::string text = chunk["delta"]["text"].AsString();
                    if (!text.empty()) {
                        fullText += text;
                        callback(text);
                    }
                }
            }
        } catch (const std::exception& e) {
            LOG_ERROR("Failed to parse streaming chunk: " + std::string(e.what()));
        }
    });

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

// ============================================================================
// Local Provider (Ollama, LM Studio)
// ============================================================================

bool LocalProvider::Initialize(const std::string& apiKey, const std::string& endpoint) {
    endpoint_ = endpoint.empty() ? "http://localhost:11434/api/chat" : endpoint;

    // Detect if it's Ollama or OpenAI-compatible endpoint
    isOllama_ = (endpoint_.find("localhost:11434") != std::string::npos ||
                 endpoint_.find("ollama") != std::string::npos);

    LOG_INFO("Local provider initialized: " + endpoint_ +
             (isOllama_ ? " (Ollama)" : " (OpenAI-compatible)"));
    return true;
}

std::string LocalProvider::Complete(
    const std::vector<AIMessage>& messages,
    const CompletionOptions& options
) {
    JsonValue request = JsonValue::Object();
    request["model"] = JsonValue(options.model.empty() ? "llama3.2" : options.model);
    request["stream"] = JsonValue(false);

    if (isOllama_) {
        // Ollama format
        JsonValue ollamaMessages = JsonValue::Array();
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
            ollamaMessages.Push(msgJson);
        }
        request["messages"] = ollamaMessages;

        // Ollama options
        JsonValue ollamaOptions = JsonValue::Object();
        ollamaOptions["temperature"] = JsonValue(options.temperature);
        ollamaOptions["num_predict"] = JsonValue(options.maxTokens);
        request["options"] = ollamaOptions;
    } else {
        // OpenAI-compatible format
        JsonValue openaiMessages = JsonValue::Array();
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
            openaiMessages.Push(msgJson);
        }
        request["messages"] = openaiMessages;
        request["temperature"] = JsonValue(options.temperature);
        request["max_tokens"] = JsonValue(options.maxTokens);
    }

    std::string requestBody = request.ToString();
    LOG_DEBUG("Sending request to local model");

    HttpClient client;
    client.SetTimeout(120000); // 2 minutes for local models

    std::map<std::string, std::string> headers;
    headers["Content-Type"] = "application/json";

    HttpResponse response = client.Post(endpoint_, requestBody, headers);

    if (!response.success) {
        LOG_ERROR("HTTP request failed: " + response.error);
        return "";
    }

    // Parse response
    try {
        JsonValue responseJson = JsonValue::Parse(response.body);

        if (isOllama_) {
            if (responseJson.Has("message") && responseJson["message"].Has("content")) {
                return responseJson["message"]["content"].AsString();
            }
        } else {
            if (responseJson.Has("choices") && responseJson["choices"].Size() > 0) {
                return responseJson["choices"][0]["message"]["content"].AsString();
            }
        }
    } catch (const std::exception& e) {
        LOG_ERROR("Failed to parse response: " + std::string(e.what()));
        return "";
    }

    return "";
}

void LocalProvider::CompleteStreaming(
    const std::vector<AIMessage>& messages,
    StreamCallback callback,
    const CompletionOptions& options
) {
    // Similar implementation with stream=true
    // For brevity, using non-streaming fallback
    LOG_WARNING("Local streaming not fully implemented, using non-streaming");
    std::string result = Complete(messages, options);
    if (!result.empty()) {
        callback(result);
    }
}

} // namespace NovelStudio
