#pragma once

#include <string>
#include <fstream>
#include <sstream>
#include <chrono>
#include <iomanip>
#include <mutex>
#include <memory>

namespace NovelStudio {

// Log levels
enum class LogLevel {
    Trace,
    Debug,
    Info,
    Warning,
    Error,
    Critical
};

// Logger class
class Logger {
public:
    static Logger& GetInstance();

    void SetLevel(LogLevel level);
    void SetLogFile(const std::string& path);
    void EnableConsoleOutput(bool enable);

    void Trace(const std::string& message);
    void Debug(const std::string& message);
    void Info(const std::string& message);
    void Warning(const std::string& message);
    void Error(const std::string& message);
    void Critical(const std::string& message);

    // Template for formatted logging
    template<typename... Args>
    void Log(LogLevel level, const std::string& format, Args... args) {
        char buffer[512];
        snprintf(buffer, sizeof(buffer), format.c_str(), args...);
        LogMessage(level, buffer);
    }

private:
    Logger();
    ~Logger();

    void LogMessage(LogLevel level, const std::string& message);
    std::string GetTimestamp();
    std::string LevelToString(LogLevel level);
    std::string GetLevelColor(LogLevel level);

    LogLevel minLevel_ = LogLevel::Info;
    std::string logFilePath_;
    bool consoleOutput_ = true;
    std::ofstream logFile_;
    std::mutex mutex_;
};

// Convenience macros
#define LOG_TRACE(msg) NovelStudio::Logger::GetInstance().Trace(msg)
#define LOG_DEBUG(msg) NovelStudio::Logger::GetInstance().Debug(msg)
#define LOG_INFO(msg) NovelStudio::Logger::GetInstance().Info(msg)
#define LOG_WARNING(msg) NovelStudio::Logger::GetInstance().Warning(msg)
#define LOG_ERROR(msg) NovelStudio::Logger::GetInstance().Error(msg)
#define LOG_CRITICAL(msg) NovelStudio::Logger::GetInstance().Critical(msg)

} // namespace NovelStudio
