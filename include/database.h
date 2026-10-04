#pragma once

#include <string>
#include <vector>
#include <memory>
#include <optional>

struct sqlite3;

namespace NovelStudio {

// Entity classes
struct Novel {
    int64_t id = 0;
    std::string title;
    std::string description;
    std::string author;
    int64_t createdAt = 0;
    int64_t updatedAt = 0;
};

struct Chapter {
    int64_t id = 0;
    int64_t novelId = 0;
    int chapterNumber = 0;
    std::string title;
    std::string content;
    int64_t createdAt = 0;
    int64_t updatedAt = 0;
};

struct Character {
    int64_t id = 0;
    int64_t novelId = 0;
    std::string name;
    std::string description;
    std::string traits;
    std::string notes;
};

// Database manager
class Database {
public:
    Database();
    ~Database();

    bool Open(const std::string& path);
    void Close();
    bool IsOpen() const;

    // Novel operations
    bool CreateNovel(Novel& novel);
    std::optional<Novel> GetNovel(int64_t id);
    std::vector<Novel> GetAllNovels();
    bool UpdateNovel(const Novel& novel);
    bool DeleteNovel(int64_t id);

    // Chapter operations
    bool CreateChapter(Chapter& chapter);
    std::optional<Chapter> GetChapter(int64_t id);
    std::vector<Chapter> GetChaptersByNovel(int64_t novelId);
    bool UpdateChapter(const Chapter& chapter);
    bool DeleteChapter(int64_t id);

    // Character operations
    bool CreateCharacter(Character& character);
    std::optional<Character> GetCharacter(int64_t id);
    std::vector<Character> GetCharactersByNovel(int64_t novelId);
    bool UpdateCharacter(const Character& character);
    bool DeleteCharacter(int64_t id);

private:
    bool InitializeTables();
    int64_t GetLastInsertRowId();

    sqlite3* db_ = nullptr;
};

} // namespace NovelStudio
