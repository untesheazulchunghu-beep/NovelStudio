#include "logger.h"
#include <iostream>

#ifdef _WIN32
#include <windows.h>
#endif

namespace NovelStudio {

Logger& Logger::GetInstance() {
    static Logger instance;
    return instance;
}

Logger::Logger() {
    // Create logs directory if it doesn't exist
#ifdef _WIN32
    CreateDirectoryA("logs", NULL);
#endif
}

Logger::~Logger() {
    if (logFile_.is_open()) {
        logFile_.close();
    }
}

void Logger::SetLevel(LogLevel level) {
    std::lock_guard<std::mutex> lock(mutex_);
    minLevel_ = level;
}

void Logger::SetLogFile(const std::string& path) {
    std::lock_guard<std::mutex> lock(mutex_);

    if (logFile_.is_open()) {
        logFile_.close();
    }

    logFilePath_ = path;
    logFile_.open(path, std::ios::app);

    if (logFile_.is_open()) {
        logFile_ << "\n=== Log session started at " << GetTimestamp() << " ===\n";
    }
}

void Logger::EnableConsoleOutput(bool enable) {
    std::lock_guard<std::mutex> lock(mutex_);
    consoleOutput_ = enable;
}

void Logger::Trace(const std::string& message) {
    LogMessage(LogLevel::Trace, message);
}

void Logger::Debug(const std::string& message) {
    LogMessage(LogLevel::Debug, message);
}

void Logger::Info(const std::string& message) {
    LogMessage(LogLevel::Info, message);
}

void Logger::Warning(const std::string& message) {
    LogMessage(LogLevel::Warning, message);
}

void Logger::Error(const std::string& message) {
    LogMessage(LogLevel::Error, message);
}

void Logger::Critical(const std::string& message) {
    LogMessage(LogLevel::Critical, message);
}

void Logger::LogMessage(LogLevel level, const std::string& message) {
    if (level < minLevel_) {
        return;
    }

    std::lock_guard<std::mutex> lock(mutex_);

    std::string timestamp = GetTimestamp();
    std::string levelStr = LevelToString(level);
    std::string logLine = "[" + timestamp + "] [" + levelStr + "] " + message;

    // Console output with color
    if (consoleOutput_) {
#ifdef _WIN32
        HANDLE hConsole = GetStdHandle(STD_OUTPUT_HANDLE);
        WORD color;

        switch (level) {
            case LogLevel::Trace:
            case LogLevel::Debug:
                color = 8; // Gray
                break;
            case LogLevel::Info:
                color = 7; // White
                break;
            case LogLevel::Warning:
                color = 14; // Yellow
                break;
            case LogLevel::Error:
                color = 12; // Red
                break;
            case LogLevel::Critical:
                color = 79; // Red background
                break;
        }

        SetConsoleTextAttribute(hConsole, color);
        std::cout << logLine << std::endl;
        SetConsoleTextAttribute(hConsole, 7); // Reset to white
#else
        std::cout << GetLevelColor(level) << logLine << "\033[0m" << std::endl;
#endif
    }

    // File output
    if (logFile_.is_open()) {
        logFile_ << logLine << std::endl;
        logFile_.flush();
    }
}

std::string Logger::GetTimestamp() {
    auto now = std::chrono::system_clock::now();
    auto time = std::chrono::system_clock::to_time_t(now);
    auto ms = std::chrono::duration_cast<std::chrono::milliseconds>(
        now.time_since_epoch()) % 1000;

    std::stringstream ss;
    ss << std::put_time(std::localtime(&time), "%Y-%m-%d %H:%M:%S");
    ss << '.' << std::setfill('0') << std::setw(3) << ms.count();

    return ss.str();
}

std::string Logger::LevelToString(LogLevel level) {
    switch (level) {
        case LogLevel::Trace:    return "TRACE";
        case LogLevel::Debug:    return "DEBUG";
        case LogLevel::Info:     return "INFO ";
        case LogLevel::Warning:  return "WARN ";
        case LogLevel::Error:    return "ERROR";
        case LogLevel::Critical: return "CRIT ";
        default:                 return "?????";
    }
}

std::string Logger::GetLevelColor(LogLevel level) {
    switch (level) {
        case LogLevel::Trace:    return "\033[90m"; // Gray
        case LogLevel::Debug:    return "\033[36m"; // Cyan
        case LogLevel::Info:     return "\033[37m"; // White
        case LogLevel::Warning:  return "\033[33m"; // Yellow
        case LogLevel::Error:    return "\033[31m"; // Red
        case LogLevel::Critical: return "\033[41m"; // Red background
        default:                 return "\033[0m";
    }
}

} // namespace NovelStudio
