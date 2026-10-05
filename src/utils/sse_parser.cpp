#include "sse_parser.h"
#include <sstream>
#include <algorithm>

namespace NovelStudio {

// SSEParser implementation
SSEParser::SSEParser() {
}

void SSEParser::Feed(const char* data, size_t length) {
    buffer_.append(data, length);

    // Process complete lines
    size_t pos = 0;
    while ((pos = buffer_.find('\n')) != std::string::npos) {
        std::string line = buffer_.substr(0, pos);
        buffer_.erase(0, pos + 1);

        // Remove \r if present
        if (!line.empty() && line.back() == '\r') {
            line.pop_back();
        }

        ProcessLine(line);
    }
}

void SSEParser::ProcessLine(const std::string& line) {
    // Empty line dispatches the event
    if (line.empty()) {
        DispatchEvent();
        return;
    }

    // Ignore comments (lines starting with :)
    if (line[0] == ':') {
        return;
    }

    // Find field separator
    size_t colonPos = line.find(':');
    if (colonPos == std::string::npos) {
        // Field with no value
        return;
    }

    std::string field = line.substr(0, colonPos);
    std::string value = line.substr(colonPos + 1);

    // Skip leading space in value
    if (!value.empty() && value[0] == ' ') {
        value.erase(0, 1);
    }

    // Handle different field types
    if (field == "event") {
        currentEvent_ = value;
    } else if (field == "data") {
        if (!currentData_.empty()) {
            currentData_ += '\n';
        }
        currentData_ += value;
    } else if (field == "id") {
        currentId_ = value;
    }
}

void SSEParser::DispatchEvent() {
    if (currentData_.empty()) {
        return;
    }

    // Check for [DONE] signal
    if (currentData_ == "[DONE]") {
        done_ = true;
        currentData_.clear();
        currentEvent_.clear();
        return;
    }

    // Call appropriate callback
    if (currentEvent_.empty()) {
        if (dataCallback_) {
            dataCallback_(currentData_);
        }
    } else {
        if (eventCallback_) {
            eventCallback_(currentEvent_, currentData_);
        }
    }

    // Reset current event
    currentData_.clear();
    currentEvent_.clear();
}

void SSEParser::Reset() {
    buffer_.clear();
    currentEvent_.clear();
    currentData_.clear();
    currentId_.clear();
    done_ = false;
}

// StreamLineBuffer implementation
std::vector<std::string> StreamLineBuffer::Push(const char* data, size_t length) {
    return Push(std::string(data, length));
}

std::vector<std::string> StreamLineBuffer::Push(const std::string& data) {
    std::vector<std::string> lines;
    buffer_ += data;

    size_t pos = 0;
    while ((pos = buffer_.find('\n')) != std::string::npos) {
        std::string line = buffer_.substr(0, pos);
        buffer_.erase(0, pos + 1);

        // Remove \r if present
        if (!line.empty() && line.back() == '\r') {
            line.pop_back();
        }

        lines.push_back(line);
    }

    return lines;
}

std::string StreamLineBuffer::Flush() {
    std::string remaining = buffer_;
    buffer_.clear();
    return remaining;
}

} // namespace NovelStudio
