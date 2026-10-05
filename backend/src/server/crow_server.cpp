#include "crow_config.h"
#include "../../include/ai_service.h"
#include "../../include/data_manager.h"
#include "../../include/config_manager.h"
#include <iostream>
#include <fstream>
#include <sstream>

using namespace crow;

class NovelStudioServer {
private:
    crow::SimpleApp app;
    int port;
    std::string static_dir;

public:
    NovelStudioServer(int p = 3000) : port(p) {
        static_dir = "../frontend/dist";
    }

    void setupRoutes() {
        // Health check
        CROW_ROUTE(app, "/api/health")
        ([]() {
            return ApiResponse::ok(json::wvalue{{"status", "ok"}});
        });

        // ===== Novel Routes =====

        // Get all novels
        CROW_ROUTE(app, "/api/novels").methods(HTTPMethod::Get)
        ([](const request& req) {
            auto& dm = DataManager::GetInstance();
            auto novels = dm.GetAllNovels();

            json::wvalue::list novel_list;
            for (const auto& novel : novels) {
                json::wvalue n;
                n["id"] = novel.id;
                n["title"] = novel.title;
                n["author"] = novel.author;
                n["description"] = novel.description;
                n["created_at"] = novel.created_at;
                n["updated_at"] = novel.updated_at;
                novel_list.push_back(std::move(n));
            }

            json::wvalue data;
            data["novels"] = std::move(novel_list);
            return ApiResponse::ok(data);
        });

        // Create novel
        CROW_ROUTE(app, "/api/novels").methods(HTTPMethod::Post)
        ([](const request& req) {
            auto body = json::load(req.body);
            if (!body) {
                return ApiResponse::error(400, "Invalid JSON");
            }

            std::string title = body["title"].s();
            std::string author = body.has("author") ? body["author"].s() : "Unknown";
            std::string description = body.has("description") ? body["description"].s() : "";

            auto& dm = DataManager::GetInstance();
            int novel_id = dm.CreateNovel(title, author, description);

            json::wvalue data;
            data["id"] = novel_id;
            data["title"] = title;
            data["author"] = author;
            data["description"] = description;

            return ApiResponse::ok(data);
        });

        // Get single novel
        CROW_ROUTE(app, "/api/novels/<int>").methods(HTTPMethod::Get)
        ([](int id) {
            auto& dm = DataManager::GetInstance();
            auto novel = dm.GetNovel(id);

            if (!novel) {
                return ApiResponse::error(404, "Novel not found");
            }

            json::wvalue data;
            data["id"] = novel->id;
            data["title"] = novel->title;
            data["author"] = novel->author;
            data["description"] = novel->description;
            data["created_at"] = novel->created_at;
            data["updated_at"] = novel->updated_at;

            return ApiResponse::ok(data);
        });

        // Update novel
        CROW_ROUTE(app, "/api/novels/<int>").methods(HTTPMethod::Put)
        ([](const request& req, int id) {
            auto body = json::load(req.body);
            if (!body) {
                return ApiResponse::error(400, "Invalid JSON");
            }

            auto& dm = DataManager::GetInstance();
            auto novel = dm.GetNovel(id);

            if (!novel) {
                return ApiResponse::error(404, "Novel not found");
            }

            if (body.has("title")) novel->title = body["title"].s();
            if (body.has("author")) novel->author = body["author"].s();
            if (body.has("description")) novel->description = body["description"].s();

            if (!dm.UpdateNovel(*novel)) {
                return ApiResponse::error(500, "Failed to update novel");
            }

            json::wvalue data;
            data["id"] = novel->id;
            data["title"] = novel->title;
            data["author"] = novel->author;
            data["description"] = novel->description;

            return ApiResponse::ok(data);
        });

        // Delete novel
        CROW_ROUTE(app, "/api/novels/<int>").methods(HTTPMethod::Delete)
        ([](int id) {
            auto& dm = DataManager::GetInstance();

            if (!dm.DeleteNovel(id)) {
                return ApiResponse::error(404, "Novel not found");
            }

            json::wvalue data;
            data["deleted"] = true;
            data["id"] = id;

            return ApiResponse::ok(data);
        });

        // ===== Chapter Routes =====

        // Get chapters for a novel
        CROW_ROUTE(app, "/api/novels/<int>/chapters").methods(HTTPMethod::Get)
        ([](int novel_id) {
            auto& dm = DataManager::GetInstance();
            auto chapters = dm.GetChaptersByNovel(novel_id);

            json::wvalue::list chapter_list;
            for (const auto& chapter : chapters) {
                json::wvalue c;
                c["id"] = chapter.id;
                c["novel_id"] = chapter.novel_id;
                c["title"] = chapter.title;
                c["content"] = chapter.content;
                c["chapter_number"] = chapter.chapter_number;
                c["word_count"] = chapter.word_count;
                c["created_at"] = chapter.created_at;
                c["updated_at"] = chapter.updated_at;
                chapter_list.push_back(std::move(c));
            }

            json::wvalue data;
            data["chapters"] = std::move(chapter_list);
            return ApiResponse::ok(data);
        });

        // Create chapter
        CROW_ROUTE(app, "/api/novels/<int>/chapters").methods(HTTPMethod::Post)
        ([](const request& req, int novel_id) {
            auto body = json::load(req.body);
            if (!body) {
                return ApiResponse::error(400, "Invalid JSON");
            }

            std::string title = body["title"].s();
            std::string content = body.has("content") ? body["content"].s() : "";
            int chapter_number = body.has("chapter_number") ? body["chapter_number"].i() : 1;

            auto& dm = DataManager::GetInstance();
            int chapter_id = dm.CreateChapter(novel_id, title, content, chapter_number);

            json::wvalue data;
            data["id"] = chapter_id;
            data["novel_id"] = novel_id;
            data["title"] = title;
            data["content"] = content;
            data["chapter_number"] = chapter_number;

            return ApiResponse::ok(data);
        });

        // Update chapter
        CROW_ROUTE(app, "/api/chapters/<int>").methods(HTTPMethod::Put)
        ([](const request& req, int id) {
            auto body = json::load(req.body);
            if (!body) {
                return ApiResponse::error(400, "Invalid JSON");
            }

            auto& dm = DataManager::GetInstance();
            auto chapter = dm.GetChapter(id);

            if (!chapter) {
                return ApiResponse::error(404, "Chapter not found");
            }

            if (body.has("title")) chapter->title = body["title"].s();
            if (body.has("content")) chapter->content = body["content"].s();
            if (body.has("chapter_number")) chapter->chapter_number = body["chapter_number"].i();

            if (!dm.UpdateChapter(*chapter)) {
                return ApiResponse::error(500, "Failed to update chapter");
            }

            json::wvalue data;
            data["id"] = chapter->id;
            data["title"] = chapter->title;
            data["content"] = chapter->content;

            return ApiResponse::ok(data);
        });

        // Delete chapter
        CROW_ROUTE(app, "/api/chapters/<int>").methods(HTTPMethod::Delete)
        ([](int id) {
            auto& dm = DataManager::GetInstance();

            if (!dm.DeleteChapter(id)) {
                return ApiResponse::error(404, "Chapter not found");
            }

            json::wvalue data;
            data["deleted"] = true;
            data["id"] = id;

            return ApiResponse::ok(data);
        });

        // ===== Character Routes =====

        // Get characters for a novel
        CROW_ROUTE(app, "/api/novels/<int>/characters").methods(HTTPMethod::Get)
        ([](int novel_id) {
            auto& dm = DataManager::GetInstance();
            auto characters = dm.GetCharactersByNovel(novel_id);

            json::wvalue::list char_list;
            for (const auto& character : characters) {
                json::wvalue c;
                c["id"] = character.id;
                c["novel_id"] = character.novel_id;
                c["name"] = character.name;
                c["description"] = character.description;
                c["role"] = character.role;
                c["created_at"] = character.created_at;
                c["updated_at"] = character.updated_at;
                char_list.push_back(std::move(c));
            }

            json::wvalue data;
            data["characters"] = std::move(char_list);
            return ApiResponse::ok(data);
        });

        // Create character
        CROW_ROUTE(app, "/api/novels/<int>/characters").methods(HTTPMethod::Post)
        ([](const request& req, int novel_id) {
            auto body = json::load(req.body);
            if (!body) {
                return ApiResponse::error(400, "Invalid JSON");
            }

            std::string name = body["name"].s();
            std::string description = body.has("description") ? body["description"].s() : "";
            std::string role = body.has("role") ? body["role"].s() : "Supporting";

            auto& dm = DataManager::GetInstance();
            int char_id = dm.CreateCharacter(novel_id, name, description, role);

            json::wvalue data;
            data["id"] = char_id;
            data["novel_id"] = novel_id;
            data["name"] = name;
            data["description"] = description;
            data["role"] = role;

            return ApiResponse::ok(data);
        });

        // ===== AI Routes =====

        // AI Complete (non-streaming)
        CROW_ROUTE(app, "/api/ai/complete").methods(HTTPMethod::Post)
        ([](const request& req) {
            auto body = json::load(req.body);
            if (!body) {
                return ApiResponse::error(400, "Invalid JSON");
            }

            std::string system_prompt = body.has("system") ? body["system"].s() : "You are a creative writer.";
            std::string user_prompt = body["prompt"].s();
            std::string model = body.has("model") ? body["model"].s() : "gpt-3.5-turbo";
            int max_tokens = body.has("max_tokens") ? body["max_tokens"].i() : 2000;

            auto& ai = AIService::GetInstance();
            std::string result = ai.GenerateFromPrompt(system_prompt, user_prompt, model, max_tokens);

            json::wvalue data;
            data["text"] = result;
            data["model"] = model;

            return ApiResponse::ok(data);
        });

        // AI Continue Writing
        CROW_ROUTE(app, "/api/ai/continue").methods(HTTPMethod::Post)
        ([](const request& req) {
            auto body = json::load(req.body);
            if (!body) {
                return ApiResponse::error(400, "Invalid JSON");
            }

            std::string existing_text = body["text"].s();
            std::string model = body.has("model") ? body["model"].s() : "gpt-3.5-turbo";

            auto& ai = AIService::GetInstance();
            std::string result = ai.ContinueWriting(existing_text, model);

            json::wvalue data;
            data["continuation"] = result;

            return ApiResponse::ok(data);
        });

        // ===== Statistics Routes =====

        // Get novel statistics
        CROW_ROUTE(app, "/api/novels/<int>/stats").methods(HTTPMethod::Get)
        ([](int novel_id) {
            auto& dm = DataManager::GetInstance();
            auto chapters = dm.GetChaptersByNovel(novel_id);

            int total_chapters = chapters.size();
            int total_words = 0;
            for (const auto& chapter : chapters) {
                total_words += chapter.word_count;
            }

            json::wvalue data;
            data["novel_id"] = novel_id;
            data["total_chapters"] = total_chapters;
            data["total_words"] = total_words;

            return ApiResponse::ok(data);
        });

        // Static files (serve frontend)
        CROW_ROUTE(app, "/")
        ([this]() {
            return serveFile("index.html");
        });

        CROW_ROUTE(app, "/<path>")
        ([this](std::string path) {
            return serveFile(path);
        });
    }

    crow::response serveFile(const std::string& path) {
        std::string full_path = static_dir + "/" + path;
        std::ifstream file(full_path, std::ios::binary);

        if (!file.is_open()) {
            return crow::response(404, "File not found");
        }

        std::ostringstream contents;
        contents << file.rdbuf();
        file.close();

        crow::response res(contents.str());

        // Set content type
        if (path.ends_with(".html")) res.add_header("Content-Type", "text/html");
        else if (path.ends_with(".js")) res.add_header("Content-Type", "application/javascript");
        else if (path.ends_with(".css")) res.add_header("Content-Type", "text/css");
        else if (path.ends_with(".json")) res.add_header("Content-Type", "application/json");

        return res;
    }

    void run() {
        setupRoutes();

        std::cout << "========================================" << std::endl;
        std::cout << "NovelStudio Web Server" << std::endl;
        std::cout << "========================================" << std::endl;
        std::cout << "Server starting on http://localhost:" << port << std::endl;
        std::cout << "API endpoint: http://localhost:" << port << "/api" << std::endl;
        std::cout << "Press Ctrl+C to stop" << std::endl;
        std::cout << "========================================" << std::endl;

        app.port(port)
           .multithreaded()
           .run();
    }
};

int main() {
    try {
        // Initialize services
        auto& config = ConfigManager::GetInstance();
        config.Load("config.json");

        auto& ai = AIService::GetInstance();
        std::string api_key = config.GetString("openai_api_key", "");
        if (!api_key.empty()) {
            ai.Initialize(AIService::ProviderType::OpenAI, api_key);
        }

        // Start server
        NovelStudioServer server(3000);
        server.run();

    } catch (const std::exception& e) {
        std::cerr << "Fatal error: " << e.what() << std::endl;
        return 1;
    }

    return 0;
}
