#include "novel_studio.h"
#include <windows.h>
#include <iostream>

int WINAPI WinMain(
    _In_ HINSTANCE hInstance,
    _In_opt_ HINSTANCE hPrevInstance,
    _In_ LPSTR lpCmdLine,
    _In_ int nCmdShow
) {
    using namespace NovelStudio;

    // Configure application
    AppConfig config;
    config.databasePath = "novelstudio.db";
    config.aiModel = "gpt-3.5-turbo";
    config.maxTokens = 4096;
    config.temperature = 0.7f;

    // TODO: Load API key from config file
    config.aiApiKey = "";

    // Initialize application
    Application app;
    if (!app.Initialize(config)) {
        MessageBoxA(nullptr, "Failed to initialize application", "Error", MB_OK | MB_ICONERROR);
        return 1;
    }

    // Run application
    app.Run();

    return 0;
}

// Console entry point for development
int main(int argc, char* argv[]) {
    std::cout << "NovelStudio v" << NovelStudio::VERSION << std::endl;
    std::cout << "Starting application..." << std::endl;

    NovelStudio::AppConfig config;
    config.databasePath = "novelstudio.db";
    config.aiModel = "gpt-3.5-turbo";
    config.maxTokens = 4096;
    config.temperature = 0.7f;

    NovelStudio::Application app;
    if (!app.Initialize(config)) {
        std::cerr << "Failed to initialize application" << std::endl;
        return 1;
    }

    std::cout << "Application initialized successfully" << std::endl;
    std::cout << "Press Enter to exit..." << std::endl;
    std::cin.get();

    return 0;
}
