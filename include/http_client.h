#pragma once

#include <string>
#include <map>
#include <functional>

namespace NovelStudio {

// HTTP response structure
struct HttpResponse {
    int statusCode = 0;
    std::string body;
    std::map<std::string, std::string> headers;
    bool success = false;
    std::string error;
};

// HTTP client using Windows WinHTTP API
class HttpClient {
public:
    HttpClient();
    ~HttpClient();

    // Synchronous request
    HttpResponse Post(
        const std::string& url,
        const std::string& body,
        const std::map<std::string, std::string>& headers = {}
    );

    HttpResponse Get(
        const std::string& url,
        const std::map<std::string, std::string>& headers = {}
    );

    // Streaming callback for chunked responses
    using StreamCallback = std::function<void(const std::string& chunk)>;

    bool PostStreaming(
        const std::string& url,
        const std::string& body,
        const std::map<std::string, std::string>& headers,
        StreamCallback callback
    );

    void SetTimeout(int timeoutMs);

private:
    struct Impl;
    Impl* impl_ = nullptr;
    int timeoutMs_ = 30000; // 30 seconds default
};

} // namespace NovelStudio
