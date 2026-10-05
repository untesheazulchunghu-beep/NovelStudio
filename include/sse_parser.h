#pragma once

#include <string>
#include <vector>
#include <functional>

namespace NovelStudio {

// Server-Sent Events (SSE) stream parser
class SSEParser {
public:
    using DataCallback = std::function<void(const std::string& data)>;
    using EventCallback = std::function<void(const std::string& event, const std::string& data)>;

    SSEParser();

    // Feed raw bytes from HTTP stream
    void Feed(const char* data, size_t length);

    // Set callback for data events
    void OnData(DataCallback callback) { dataCallback_ = callback; }

    // Set callback for named events
    void OnEvent(EventCallback callback) { eventCallback_ = callback; }

    // Check if stream is complete (received [DONE] or similar)
    bool IsDone() const { return done_; }

    // Reset parser state
    void Reset();

private:
    void ProcessLine(const std::string& line);
    void DispatchEvent();

    std::string buffer_;
    std::string currentEvent_;
    std::string currentData_;
    std::string currentId_;

    DataCallback dataCallback_;
    EventCallback eventCallback_;

    bool done_ = false;
};

// Line-based buffer for streaming response parsing
class StreamLineBuffer {
public:
    // Push new data chunk, returns complete lines
    std::vector<std::string> Push(const char* data, size_t length);
    std::vector<std::string> Push(const std::string& data);

    // Get any remaining data (for incomplete last line)
    std::string Flush();

    // Clear buffer
    void Clear() { buffer_.clear(); }

private:
    std::string buffer_;
};

} // namespace NovelStudio
