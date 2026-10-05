#pragma once

#include <string>
#include <vector>
#include <memory>
#include <ctime>

namespace NovelStudio {

// Chapter entity
struct Chapter {
    int id = 0;
    int novelId = 0;
    int chapterNumber = 0;
    std::string title;
    std::string content;
    int wordCount = 0;
    time_t createdAt = 0;
    time_t updatedAt = 0;
};

// Novel entity
struct Novel {
    int id = 0;
    std::string title;
    std::string description;
    std::string author;
    std::string genre;
    int totalWords = 0;
    time_t createdAt = 0;
    time_t updatedAt = 0;
};

// Character entity
struct Character {
    int id = 0;
    int novelId = 0;
    std::string name;
    std::string description;
    std::string traits;
    std::string backstory;
    time_t createdAt = 0;
};

// Draft version entity
struct Draft {
    int id = 0;
    int chapterId = 0;
    int versionNumber = 0;
    std::string content;
    std::string note;
    time_t createdAt = 0;
};

// In-memory data manager (no SQLite required)
class DataManager {
public:
    static DataManager& GetInstance();

    // Novel operations
    int CreateNovel(const std::string& title, const std::string& author);
    Novel* GetNovel(int id);
    std::vector<Novel> GetAllNovels();
    bool UpdateNovel(const Novel& novel);
    bool DeleteNovel(int id);

    // Chapter operations
    int CreateChapter(int novelId, const std::string& title, const std::string& content);
    Chapter* GetChapter(int id);
    std::vector<Chapter> GetChaptersByNovel(int novelId);
    bool UpdateChapter(const Chapter& chapter);
    bool DeleteChapter(int id);

    // Character operations
    int CreateCharacter(int novelId, const std::string& name, const std::string& description);
    Character* GetCharacter(int id);
    std::vector<Character> GetCharactersByNovel(int novelId);
    bool UpdateCharacter(const Character& character);
    bool DeleteCharacter(int id);

    // Draft operations
    int CreateDraft(int chapterId, const std::string& content, const std::string& note = "");
    std::vector<Draft> GetDraftsByChapter(int chapterId);
    Draft* GetDraft(int id);

    // Utility
    int CountWords(const std::string& text);
    void UpdateWordCounts(int novelId);

private:
    DataManager() = default;

    std::vector<Novel> novels_;
    std::vector<Chapter> chapters_;
    std::vector<Character> characters_;
    std::vector<Draft> drafts_;

    int nextNovelId_ = 1;
    int nextChapterId_ = 1;
    int nextCharacterId_ = 1;
    int nextDraftId_ = 1;
};

} // namespace NovelStudio
