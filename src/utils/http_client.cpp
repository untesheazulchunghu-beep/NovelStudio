#include "http_client.h"
#include "logger.h"

#ifdef _WIN32
#include <windows.h>
#include <winhttp.h>
#pragma comment(lib, "winhttp.lib")
#endif

namespace NovelStudio {

struct HttpClient::Impl {
    HINTERNET hSession = nullptr;
};

HttpClient::HttpClient() : impl_(new Impl()) {
#ifdef _WIN32
    impl_->hSession = WinHttpOpen(
        L"NovelStudio/1.0",
        WINHTTP_ACCESS_TYPE_DEFAULT_PROXY,
        WINHTTP_NO_PROXY_NAME,
        WINHTTP_NO_PROXY_BYPASS,
        0
    );
#endif
}

HttpClient::~HttpClient() {
#ifdef _WIN32
    if (impl_->hSession) {
        WinHttpCloseHandle(impl_->hSession);
    }
#endif
    delete impl_;
}

void HttpClient::SetTimeout(int timeoutMs) {
    timeoutMs_ = timeoutMs;
}

#ifdef _WIN32
static std::wstring StringToWide(const std::string& str) {
    if (str.empty()) return std::wstring();
    int size = MultiByteToWideChar(CP_UTF8, 0, str.c_str(), -1, nullptr, 0);
    std::wstring result(size, 0);
    MultiByteToWideChar(CP_UTF8, 0, str.c_str(), -1, &result[0], size);
    return result;
}

static std::string WideToString(const std::wstring& wstr) {
    if (wstr.empty()) return std::string();
    int size = WideCharToMultiByte(CP_UTF8, 0, wstr.c_str(), -1, nullptr, 0, nullptr, nullptr);
    std::string result(size, 0);
    WideCharToMultiByte(CP_UTF8, 0, wstr.c_str(), -1, &result[0], size, nullptr, nullptr);
    return result;
}
#endif

HttpResponse HttpClient::Post(
    const std::string& url,
    const std::string& body,
    const std::map<std::string, std::string>& headers
) {
    HttpResponse response;

#ifdef _WIN32
    if (!impl_->hSession) {
        response.error = "HTTP session not initialized";
        return response;
    }

    // Parse URL
    std::wstring wUrl = StringToWide(url);
    URL_COMPONENTS urlComp = {};
    urlComp.dwStructSize = sizeof(urlComp);

    wchar_t hostName[256] = {};
    wchar_t urlPath[1024] = {};
    urlComp.lpszHostName = hostName;
    urlComp.dwHostNameLength = sizeof(hostName) / sizeof(wchar_t);
    urlComp.lpszUrlPath = urlPath;
    urlComp.dwUrlPathLength = sizeof(urlPath) / sizeof(wchar_t);

    if (!WinHttpCrackUrl(wUrl.c_str(), 0, 0, &urlComp)) {
        response.error = "Failed to parse URL";
        return response;
    }

    // Connect
    HINTERNET hConnect = WinHttpConnect(
        impl_->hSession,
        hostName,
        urlComp.nPort,
        0
    );

    if (!hConnect) {
        response.error = "Failed to connect";
        return response;
    }

    // Open request
    DWORD flags = (urlComp.nScheme == INTERNET_SCHEME_HTTPS) ? WINHTTP_FLAG_SECURE : 0;
    HINTERNET hRequest = WinHttpOpenRequest(
        hConnect,
        L"POST",
        urlPath,
        nullptr,
        WINHTTP_NO_REFERER,
        WINHTTP_DEFAULT_ACCEPT_TYPES,
        flags
    );

    if (!hRequest) {
        WinHttpCloseHandle(hConnect);
        response.error = "Failed to open request";
        return response;
    }

    // Set timeout
    WinHttpSetTimeouts(hRequest, timeoutMs_, timeoutMs_, timeoutMs_, timeoutMs_);

    // Add headers
    std::wstring allHeaders;
    for (const auto& [key, value] : headers) {
        allHeaders += StringToWide(key) + L": " + StringToWide(value) + L"\r\n";
    }

    if (!allHeaders.empty()) {
        WinHttpAddRequestHeaders(
            hRequest,
            allHeaders.c_str(),
            -1,
            WINHTTP_ADDREQ_FLAG_ADD
        );
    }

    // Send request
    BOOL result = WinHttpSendRequest(
        hRequest,
        WINHTTP_NO_ADDITIONAL_HEADERS,
        0,
        (LPVOID)body.c_str(),
        body.length(),
        body.length(),
        0
    );

    if (!result) {
        WinHttpCloseHandle(hRequest);
        WinHttpCloseHandle(hConnect);
        response.error = "Failed to send request";
        return response;
    }

    // Receive response
    if (!WinHttpReceiveResponse(hRequest, nullptr)) {
        WinHttpCloseHandle(hRequest);
        WinHttpCloseHandle(hConnect);
        response.error = "Failed to receive response";
        return response;
    }

    // Get status code
    DWORD statusCode = 0;
    DWORD statusCodeSize = sizeof(statusCode);
    WinHttpQueryHeaders(
        hRequest,
        WINHTTP_QUERY_STATUS_CODE | WINHTTP_QUERY_FLAG_NUMBER,
        nullptr,
        &statusCode,
        &statusCodeSize,
        nullptr
    );
    response.statusCode = statusCode;

    // Read response body
    std::string responseBody;
    DWORD bytesAvailable = 0;
    do {
        bytesAvailable = 0;
        if (!WinHttpQueryDataAvailable(hRequest, &bytesAvailable)) {
            break;
        }

        if (bytesAvailable > 0) {
            std::vector<char> buffer(bytesAvailable + 1);
            DWORD bytesRead = 0;
            if (WinHttpReadData(hRequest, buffer.data(), bytesAvailable, &bytesRead)) {
                responseBody.append(buffer.data(), bytesRead);
            }
        }
    } while (bytesAvailable > 0);

    response.body = responseBody;
    response.success = (statusCode >= 200 && statusCode < 300);

    WinHttpCloseHandle(hRequest);
    WinHttpCloseHandle(hConnect);
#endif

    return response;
}

HttpResponse HttpClient::Get(
    const std::string& url,
    const std::map<std::string, std::string>& headers
) {
    HttpResponse response;
    // Similar implementation to Post but with GET method
    // For brevity, returning empty response for now
    response.error = "GET not implemented yet";
    return response;
}

bool HttpClient::PostStreaming(
    const std::string& url,
    const std::string& body,
    const std::map<std::string, std::string>& headers,
    StreamCallback callback
) {
#ifdef _WIN32
    if (!impl_->hSession) {
        return false;
    }

    // Parse URL
    std::wstring wUrl = StringToWide(url);
    URL_COMPONENTS urlComp = {};
    urlComp.dwStructSize = sizeof(urlComp);

    wchar_t hostName[256] = {};
    wchar_t urlPath[1024] = {};
    urlComp.lpszHostName = hostName;
    urlComp.dwHostNameLength = sizeof(hostName) / sizeof(wchar_t);
    urlComp.lpszUrlPath = urlPath;
    urlComp.dwUrlPathLength = sizeof(urlPath) / sizeof(wchar_t);

    if (!WinHttpCrackUrl(wUrl.c_str(), 0, 0, &urlComp)) {
        return false;
    }

    // Connect
    HINTERNET hConnect = WinHttpConnect(
        impl_->hSession,
        hostName,
        urlComp.nPort,
        0
    );

    if (!hConnect) {
        return false;
    }

    // Open request
    DWORD flags = (urlComp.nScheme == INTERNET_SCHEME_HTTPS) ? WINHTTP_FLAG_SECURE : 0;
    HINTERNET hRequest = WinHttpOpenRequest(
        hConnect,
        L"POST",
        urlPath,
        nullptr,
        WINHTTP_NO_REFERER,
        WINHTTP_DEFAULT_ACCEPT_TYPES,
        flags
    );

    if (!hRequest) {
        WinHttpCloseHandle(hConnect);
        return false;
    }

    // Set longer timeout for streaming
    int streamTimeout = 300000; // 5 minutes
    WinHttpSetTimeouts(hRequest, timeoutMs_, timeoutMs_, streamTimeout, streamTimeout);

    // Add headers
    std::wstring allHeaders;
    for (const auto& [key, value] : headers) {
        allHeaders += StringToWide(key) + L": " + StringToWide(value) + L"\r\n";
    }

    if (!allHeaders.empty()) {
        WinHttpAddRequestHeaders(
            hRequest,
            allHeaders.c_str(),
            -1,
            WINHTTP_ADDREQ_FLAG_ADD
        );
    }

    // Send request
    BOOL result = WinHttpSendRequest(
        hRequest,
        WINHTTP_NO_ADDITIONAL_HEADERS,
        0,
        (LPVOID)body.c_str(),
        body.length(),
        body.length(),
        0
    );

    if (!result) {
        WinHttpCloseHandle(hRequest);
        WinHttpCloseHandle(hConnect);
        return false;
    }

    // Receive response
    if (!WinHttpReceiveResponse(hRequest, nullptr)) {
        WinHttpCloseHandle(hRequest);
        WinHttpCloseHandle(hConnect);
        return false;
    }

    // Stream response chunks
    bool success = true;
    DWORD bytesAvailable = 0;
    do {
        bytesAvailable = 0;
        if (!WinHttpQueryDataAvailable(hRequest, &bytesAvailable)) {
            success = false;
            break;
        }

        if (bytesAvailable > 0) {
            std::vector<char> buffer(bytesAvailable);
            DWORD bytesRead = 0;
            if (WinHttpReadData(hRequest, buffer.data(), bytesAvailable, &bytesRead)) {
                std::string chunk(buffer.data(), bytesRead);
                callback(chunk);
            } else {
                success = false;
                break;
            }
        }
    } while (bytesAvailable > 0);

    WinHttpCloseHandle(hRequest);
    WinHttpCloseHandle(hConnect);

    return success;
#else
    return false;
#endif
}

} // namespace NovelStudio
