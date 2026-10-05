// Complete Novel Writing Workflow Demo
#include "data_manager.h"
#include "ai_service.h"
#include "config_manager.h"
#include "logger.h"
#include <iostream>
#include <iomanip>

using namespace NovelStudio;

void DisplayNovelInfo(const Novel& novel) {
    std::cout << "\n+-------------------------------------+" << std::endl;
    std::cout << "| " << novel.title << std::endl;
    std::cout << "+-------------------------------------+" << std::endl;
    std::cout << "| Author: " << novel.author << std::endl;
    std::cout << "| Total Words: " << novel.totalWords << std::endl;
    std::cout << "+-------------------------------------+" << std::endl;
}

void DisplayChaptersList(const std::vector<Chapter>& chapters) {
    std::cout << "\nChapters:" << std::endl;
    std::cout << "-------------------------------------" << std::endl;
    for (const auto& ch : chapters) {
        std::cout << ch.chapterNumber << ". " << ch.title
                  << " (" << ch.wordCount << " words)" << std::endl;
    }
}

void CreateNovelWorkflow() {
    auto& data = DataManager::GetInstance();
    auto& ai = AIService::GetInstance();

    std::cout << "\n=== Create New Novel ===" << std::endl;

    std::cout << "Novel title: ";
    std::string title;
    std::getline(std::cin, title);

    std::cout << "Author name: ";
    std::string author;
    std::getline(std::cin, author);

    int novelId = data.CreateNovel(title, author);
    std::cout << "\n✓ Novel created (ID: " << novelId << ")" << std::endl;

    // Generate description with AI
    std::cout << "\nGenerating novel description with AI..." << std::endl;
    std::string description = ai.GenerateFromPrompt(
        "You are a literary editor.",
        "Create a compelling 2-sentence book description for a novel titled: " + title,
        "",
        100
    );

    if (!description.empty()) {
        Novel* novel = data.GetNovel(novelId);
        if (novel) {
            novel->description = description;
            data.UpdateNovel(*novel);
            std::cout << "\nDescription: " << description << std::endl;
        }
    }
}

void WriteChapterWorkflow() {
    auto& data = DataManager::GetInstance();
    auto& ai = AIService::GetInstance();

    std::cout << "\n=== Write New Chapter ===" << std::endl;

    auto novels = data.GetAllNovels();
    if (novels.empty()) {
        std::cout << "No novels found. Create a novel first." << std::endl;
        return;
    }

    // Select novel
    std::cout << "\nAvailable novels:" << std::endl;
    for (size_t i = 0; i < novels.size(); i++) {
        std::cout << (i + 1) << ". " << novels[i].title << std::endl;
    }

    std::cout << "Select novel: ";
    int choice;
    std::cin >> choice;
    std::cin.ignore();

    if (choice < 1 || choice > static_cast<int>(novels.size())) {
        std::cout << "Invalid choice" << std::endl;
        return;
    }

    Novel& novel = novels[choice - 1];

    std::cout << "\nChapter title: ";
    std::string chapterTitle;
    std::getline(std::cin, chapterTitle);

    std::cout << "\nOptions:" << std::endl;
    std::cout << "1. Write manually" << std::endl;
    std::cout << "2. Generate with AI" << std::endl;
    std::cout << "Choice: ";
    int writeChoice;
    std::cin >> writeChoice;
    std::cin.ignore();

    std::string content;

    if (writeChoice == 2) {
        std::cout << "\nBrief description of what happens: ";
        std::string prompt;
        std::getline(std::cin, prompt);

        std::cout << "\nGenerating chapter content..." << std::endl;
        std::cout << "Content: ";
        std::cout.flush();

        CompletionOptions genOpts;
        genOpts.maxTokens = 1000;
        genOpts.temperature = 0.85f;

        ai.CompleteStreaming({
            {AIMessage::Role::System, "You are a creative fiction writer."},
            {AIMessage::Role::User, "Write a chapter titled '" + chapterTitle +
             "' for the novel '" + novel.title + "'. " + prompt}
        }, [&content](const std::string& chunk) {
            std::cout << chunk;
            std::cout.flush();
            content += chunk;
        }, genOpts);

        std::cout << std::endl;
    } else {
        std::cout << "\nEnter content (end with Ctrl+Z on new line):" << std::endl;
        std::string line;
        while (std::getline(std::cin, line)) {
            content += line + "\n";
        }
        std::cin.clear();
    }

    int chapterId = data.CreateChapter(novel.id, chapterTitle, content);
    std::cout << "\n✓ Chapter created (ID: " << chapterId << ")" << std::endl;
    std::cout << "Word count: " << data.CountWords(content) << std::endl;
}

void ContinueChapterWorkflow() {
    auto& data = DataManager::GetInstance();
    auto& ai = AIService::GetInstance();

    std::cout << "\n=== Continue Chapter ===" << std::endl;

    auto novels = data.GetAllNovels();
    if (novels.empty()) {
        std::cout << "No novels found." << std::endl;
        return;
    }

    // Select novel
    std::cout << "\nAvailable novels:" << std::endl;
    for (size_t i = 0; i < novels.size(); i++) {
        std::cout << (i + 1) << ". " << novels[i].title << std::endl;
    }

    std::cout << "Select novel: ";
    int novelChoice;
    std::cin >> novelChoice;
    std::cin.ignore();

    if (novelChoice < 1 || novelChoice > static_cast<int>(novels.size())) return;

    Novel& novel = novels[novelChoice - 1];
    auto chapters = data.GetChaptersByNovel(novel.id);

    if (chapters.empty()) {
        std::cout << "No chapters found." << std::endl;
        return;
    }

    DisplayChaptersList(chapters);

    std::cout << "\nSelect chapter: ";
    int chapterChoice;
    std::cin >> chapterChoice;
    std::cin.ignore();

    if (chapterChoice < 1 || chapterChoice > static_cast<int>(chapters.size())) return;

    Chapter& chapter = chapters[chapterChoice - 1];

    // Save current version as draft
    data.CreateDraft(chapter.id, chapter.content, "Before AI continuation");

    std::cout << "\nCurrent content (last 200 chars):" << std::endl;
    std::cout << "-------------------------------------" << std::endl;
    size_t start = chapter.content.length() > 200 ? chapter.content.length() - 200 : 0;
    std::cout << chapter.content.substr(start) << std::endl;
    std::cout << "-------------------------------------" << std::endl;

    std::cout << "\nInstruction for AI: ";
    std::string instruction;
    std::getline(std::cin, instruction);

    std::cout << "\nGenerating continuation..." << std::endl;
    std::cout << "New content: ";
    std::cout.flush();

    std::string continuation;
    ai.CompleteStreaming({
        {AIMessage::Role::System, "You are a creative fiction writer. Continue the story naturally."},
        {AIMessage::Role::User, "Current chapter content:\n" + chapter.content +
         "\n\nInstruction: " + instruction}
    }, [&continuation](const std::string& chunk) {
        std::cout << chunk;
        std::cout.flush();
        continuation += chunk;
    });

    CompletionOptions contOpts;
    contOpts.maxTokens = 500;
    contOpts.temperature = 0.85f;

    ai.CompleteStreaming({
        {AIMessage::Role::System, "You are a creative fiction writer. Continue the story naturally."},
        {AIMessage::Role::User, "Current chapter content:\n" + chapter.content +
         "\n\nInstruction: " + instruction}
    }, [&continuation](const std::string& chunk) {
        std::cout << chunk;
        std::cout.flush();
        continuation += chunk;
    }, contOpts);

    std::cout << std::endl;

    chapter.content += "\n\n" + continuation;
    data.UpdateChapter(chapter);

    std::cout << "\n✓ Chapter updated" << std::endl;
    std::cout << "New word count: " << chapter.wordCount << std::endl;
}

void ManageCharactersWorkflow() {
    auto& data = DataManager::GetInstance();
    auto& ai = AIService::GetInstance();

    std::cout << "\n=== Manage Characters ===" << std::endl;

    auto novels = data.GetAllNovels();
    if (novels.empty()) {
        std::cout << "No novels found." << std::endl;
        return;
    }

    std::cout << "\nAvailable novels:" << std::endl;
    for (size_t i = 0; i < novels.size(); i++) {
        std::cout << (i + 1) << ". " << novels[i].title << std::endl;
    }

    std::cout << "Select novel: ";
    int choice;
    std::cin >> choice;
    std::cin.ignore();

    if (choice < 1 || choice > static_cast<int>(novels.size())) return;

    Novel& novel = novels[choice - 1];

    std::cout << "\n1. Add character" << std::endl;
    std::cout << "2. List characters" << std::endl;
    std::cout << "Choice: ";
    int action;
    std::cin >> action;
    std::cin.ignore();

    if (action == 1) {
        std::cout << "\nCharacter name: ";
        std::string name;
        std::getline(std::cin, name);

        std::cout << "Generate description with AI? (y/n): ";
        char useAI;
        std::cin >> useAI;
        std::cin.ignore();

        std::string description;
        if (useAI == 'y' || useAI == 'Y') {
            std::cout << "\nBrief character description: ";
            std::string charDesc;
            std::getline(std::cin, charDesc);

            description = ai.GenerateFromPrompt(
                "You are a character development expert.",
                "Create a detailed character description for: " + name + ". " + charDesc,
                "",
                200
            );

            std::cout << "\nGenerated description:" << std::endl;
            std::cout << description << std::endl;
        } else {
            std::cout << "Description: ";
            std::getline(std::cin, description);
        }

        int charId = data.CreateCharacter(novel.id, name, description);
        std::cout << "\n✓ Character created (ID: " << charId << ")" << std::endl;

    } else if (action == 2) {
        auto characters = data.GetCharactersByNovel(novel.id);

        std::cout << "\nCharacters in " << novel.title << ":" << std::endl;
        std::cout << "-------------------------------------" << std::endl;

        for (const auto& character : characters) {
            std::cout << "\n" << character.name << std::endl;
            std::cout << character.description << std::endl;
        }
    }
}

void ViewStatsWorkflow() {
    auto& data = DataManager::GetInstance();

    std::cout << "\n=== Novel Statistics ===" << std::endl;

    auto novels = data.GetAllNovels();
    if (novels.empty()) {
        std::cout << "No novels found." << std::endl;
        return;
    }

    for (const auto& novel : novels) {
        DisplayNovelInfo(novel);

        auto chapters = data.GetChaptersByNovel(novel.id);
        auto characters = data.GetCharactersByNovel(novel.id);

        std::cout << "Chapters: " << chapters.size() << std::endl;
        std::cout << "Characters: " << characters.size() << std::endl;

        if (!chapters.empty()) {
            DisplayChaptersList(chapters);
        }

        std::cout << std::endl;
    }
}

int main() {
    // Setup
    auto& logger = Logger::GetInstance();
    logger.SetLevel(LogLevel::Info);
    logger.SetLogFile("logs/workflow.log");
    logger.EnableConsoleOutput(false);

    auto& config = ConfigManager::GetInstance();
    config.Load("config.json");

    auto& ai = AIService::GetInstance();

    // Initialize AI provider
    if (!config.GetOpenAIKey().empty()) {
        ai.Initialize(AIService::ProviderType::OpenAI, config.GetOpenAIKey(),
                     "https://api.openai.com/v1/chat/completions");
    } else if (!config.GetAnthropicKey().empty()) {
        ai.Initialize(AIService::ProviderType::Anthropic, config.GetAnthropicKey(),
                     "https://api.anthropic.com/v1/messages");
    } else {
        ai.Initialize(AIService::ProviderType::Local, "", config.GetOllamaEndpoint());
    }

    std::cout << "========================================" << std::endl;
    std::cout << "   NovelStudio - Writing Workflow     " << std::endl;
    std::cout << "========================================" << std::endl;

    bool running = true;
    while (running) {
        std::cout << "\n========================================" << std::endl;
        std::cout << "1. Create New Novel" << std::endl;
        std::cout << "2. Write New Chapter" << std::endl;
        std::cout << "3. Continue Chapter (AI)" << std::endl;
        std::cout << "4. Manage Characters" << std::endl;
        std::cout << "5. View Statistics" << std::endl;
        std::cout << "0. Exit" << std::endl;
        std::cout << "========================================" << std::endl;
        std::cout << "Choice: ";

        int choice;
        std::cin >> choice;
        std::cin.ignore();

        switch (choice) {
            case 1: CreateNovelWorkflow(); break;
            case 2: WriteChapterWorkflow(); break;
            case 3: ContinueChapterWorkflow(); break;
            case 4: ManageCharactersWorkflow(); break;
            case 5: ViewStatsWorkflow(); break;
            case 0: running = false; break;
            default: std::cout << "Invalid choice" << std::endl;
        }
    }

    std::cout << "\nThank you for using NovelStudio!" << std::endl;
    return 0;
}
