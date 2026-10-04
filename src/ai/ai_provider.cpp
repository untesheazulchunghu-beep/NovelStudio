#include "ai_provider.h"
#include <curl/curl.h>
#include <nlohmann/json.hpp>
#include <sstream>

using json = nlohmann::json;

namespace NovelStudio {

// Helper for CURL responses
struct CurlResponse {
    std::string data;
    static size_t WriteCallback(void* contents, size_t size, size_t nmemb, void* userp) {
        ((std::string*)userp)->append((char*)contents, size * nmemb);
        return size * nmemb;
    }
};

// Factory methods
std::unique_ptr<AIProvider> AIProvider::CreateOpenAI() {
    return std::make_unique<OpenAIProvider>();
}

std::unique_ptr<AIProvider> AIProvider::CreateAnthropic() {
    // TODO: Implement Anthropic provider
    return nullptr;
}

std::unique_ptr<AIProvider> AIProvider::CreateLocal() {
    // TODO: Implement local model provider
    return nullptr;
}

// OpenAI implementation
bool OpenAIProvider::Initialize(const std::string& apiKey, const std::string& endpoint) {
    apiKey_ = apiKey;
    endpoint_ = endpoint.empty() ? "https://api.openai.com/v1/chat/completions" : endpoint;

    curl_global_init(CURL_GLOBAL_DEFAULT);
    return true;
}

std::string OpenAIProvider::Complete(
    const std::vector<AIMessage>& messages,
    const CompletionOptions& options
) {
    CURL* curl = curl_easy_init();
    if (!curl) {
        return "";
    }

    // Build JSON request
    json request;
    request["model"] = options.model;
    request["temperature"] = options.temperature;
    request["max_tokens"] = options.maxTokens;
    request["top_p"] = options.topP;
    request["frequency_penalty"] = options.frequencyPenalty;
    request["presence_penalty"] = options.presencePenalty;

    json messagesJson = json::array();
    for (const auto& msg : messages) {
        json msgJson;
        switch (msg.role) {
            case AIMessage::Role::System:
                msgJson["role"] = "system";
                break;
            case AIMessage::Role::User:
                msgJson["role"] = "user";
                break;
            case AIMessage::Role::Assistant:
                msgJson["role"] = "assistant";
                break;
        }
        msgJson["content"] = msg.content;
        messagesJson.push_back(msgJson);
    }
    request["messages"] = messagesJson;

    std::string requestBody = request.dump();

    // Setup CURL
    CurlResponse response;
    struct curl_slist* headers = nullptr;
    headers = curl_slist_append(headers, "Content-Type: application/json");
    std::string authHeader = "Authorization: Bearer " + apiKey_;
    headers = curl_slist_append(headers, authHeader.c_str());

    curl_easy_setopt(curl, CURLOPT_URL, endpoint_.c_str());
    curl_easy_setopt(curl, CURLOPT_HTTPHEADER, headers);
    curl_easy_setopt(curl, CURLOPT_POSTFIELDS, requestBody.c_str());
    curl_easy_setopt(curl, CURLOPT_WRITEFUNCTION, CurlResponse::WriteCallback);
    curl_easy_setopt(curl, CURLOPT_WRITEDATA, &response.data);

    CURLcode res = curl_easy_perform(curl);

    curl_slist_free_all(headers);
    curl_easy_cleanup(curl);

    if (res != CURLE_OK) {
        return "";
    }

    // Parse response
    try {
        json responseJson = json::parse(response.data);
        if (responseJson.contains("choices") && !responseJson["choices"].empty()) {
            return responseJson["choices"][0]["message"]["content"].get<std::string>();
        }
    } catch (...) {
        return "";
    }

    return "";
}

void OpenAIProvider::CompleteStreaming(
    const std::vector<AIMessage>& messages,
    StreamCallback callback,
    const CompletionOptions& options
) {
    // TODO: Implement streaming support
    // For now, fallback to non-streaming
    std::string result = Complete(messages, options);
    if (!result.empty()) {
        callback(result);
    }
}

} // namespace NovelStudio
