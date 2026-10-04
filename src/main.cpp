#include "novel_studio.h"
#include "logger.h"
#include <windows.h>
#include <iostream>

int WINAPI WinMain(
    _In_ HINSTANCE hInstance,
    _In_opt_ HINSTANCE hPrevInstance,
    _In_ LPSTR lpCmdLine,
    _In_ int nCmdShow
) {
    using namespace NovelStudio;

    // Initialize logger
    auto& logger = Logger::GetInstance();
    logger.SetLevel(LogLevel::Info);
    logger.SetLogFile("logs/novelstudio.log");
    logger.EnableConsoleOutput(true);

    LOG_INFO("NovelStudio v" + std::string(VERSION) + " starting...");

    // Configure application
    AppConfig config;
    config.databasePath = "novelstudio.db";
    config.aiModel = "gpt-3.5-turbo";
    config.maxTokens = 4096;
    config.temperature = 0.7f;

    // TODO: Load API key from secure storage
    config.aiApiKey = "";
    config.aiEndpoint = "";

    // Initialize application
    Application app;
    if (!app.Initialize(config)) {
        LOG_CRITICAL("Failed to initialize application");
        MessageBoxA(nullptr, "Failed to initialize application", "Error", MB_OK | MB_ICONERROR);
        return 1;
    }

    LOG_INFO("Application initialized successfully");

    // Run application
    app.Run();

    LOG_INFO("Application shutting down");
    return 0;
}

// Console entry point for development
int main(int argc, char* argv[]) {
    using namespace NovelStudio;

    // Initialize logger
    auto& logger = Logger::GetInstance();
    logger.SetLevel(LogLevel::Debug);
    logger.SetLogFile("logs/novelstudio.log");
    logger.EnableConsoleOutput(true);

    LOG_INFO("=================================================");
    LOG_INFO("NovelStudio v" + std::string(VERSION));
    LOG_INFO("Windows AI Novel Writing Platform");
    LOG_INFO("=================================================");

    // Load configuration
    auto bootConfig = BootConfig::Load("config.json");

    AppConfig config;
    config.databasePath = bootConfig.databasePath;
    config.aiModel = "gpt-3.5-turbo";
    config.maxTokens = 4096;
    config.temperature = 0.7f;

    Application app;
    if (!app.Initialize(config)) {
        LOG_CRITICAL("Failed to initialize application");
        std::cerr << "Failed to initialize application" << std::endl;
        return 1;
    }

    LOG_INFO("Application initialized successfully");
    LOG_INFO("Database: " + config.databasePath);

    // Test database operations
    auto db = app.GetDatabase();
    if (db && db->IsOpen()) {
        LOG_INFO("Database connection verified");

        // Get all novels
        auto novels = db->GetAllNovels();
        LOG_INFO("Found " + std::to_string(novels.size()) + " novel(s) in database");

        if (novels.empty()) {
            LOG_INFO("Creating sample novel...");

            Novel novel;
            novel.title = "Test Novel";
            novel.description = "A sample novel created by NovelStudio";
            novel.author = "NovelStudio User";

            if (db->CreateNovel(novel)) {
                LOG_INFO("Sample novel created with ID: " + std::to_string(novel.id));

                // Create a sample chapter
                Chapter chapter;
                chapter.novelId = novel.id;
                chapter.chapterNumber = 1;
                chapter.title = "Chapter 1: The Beginning";
                chapter.content = "This is the beginning of our story...";

                if (db->CreateChapter(chapter)) {
                    LOG_INFO("Sample chapter created with ID: " + std::to_string(chapter.id));
                }
            }
        }
    }

    LOG_INFO("");
    LOG_INFO("Press Enter to exit...");
    std::cin.get();

    app.Shutdown();
    LOG_INFO("Application shutdown complete");

    return 0;
}
