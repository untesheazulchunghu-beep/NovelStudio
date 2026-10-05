#include "data_manager.h"
#include "logger.h"
#include <algorithm>
#include <sstream>

namespace NovelStudio {

DataManager& DataManager::GetInstance() {
    static DataManager instance;
    return instance;
}

// Novel operations
int DataManager::CreateNovel(const std::string& title, const std::string& author) {
    Novel novel;
    novel.id = nextNovelId_++;
    novel.title = title;
    novel.author = author;
    novel.createdAt = std::time(nullptr);
    novel.updatedAt = novel.createdAt;

    novels_.push_back(novel);
    LOG_INFO("Created novel: " + title + " (ID: " + std::to_string(novel.id) + ")");
    return novel.id;
}

Novel* DataManager::GetNovel(int id) {
    auto it = std::find_if(novels_.begin(), novels_.end(),
        [id](const Novel& n) { return n.id == id; });
    return it != novels_.end() ? &(*it) : nullptr;
}

std::vector<Novel> DataManager::GetAllNovels() {
    return novels_;
}

bool DataManager::UpdateNovel(const Novel& novel) {
    auto it = std::find_if(novels_.begin(), novels_.end(),
        [&novel](const Novel& n) { return n.id == novel.id; });

    if (it != novels_.end()) {
        *it = novel;
        it->updatedAt = std::time(nullptr);
        LOG_INFO("Updated novel ID: " + std::to_string(novel.id));
        return true;
    }
    return false;
}

bool DataManager::DeleteNovel(int id) {
    auto it = std::find_if(novels_.begin(), novels_.end(),
        [id](const Novel& n) { return n.id == id; });

    if (it != novels_.end()) {
        // Delete associated chapters
        chapters_.erase(
            std::remove_if(chapters_.begin(), chapters_.end(),
                [id](const Chapter& c) { return c.novelId == id; }),
            chapters_.end()
        );

        // Delete associated characters
        characters_.erase(
            std::remove_if(characters_.begin(), characters_.end(),
                [id](const Character& c) { return c.novelId == id; }),
            characters_.end()
        );

        novels_.erase(it);
        LOG_INFO("Deleted novel ID: " + std::to_string(id));
        return true;
    }
    return false;
}

// Chapter operations
int DataManager::CreateChapter(int novelId, const std::string& title, const std::string& content) {
    Chapter chapter;
    chapter.id = nextChapterId_++;
    chapter.novelId = novelId;
    chapter.title = title;
    chapter.content = content;
    chapter.wordCount = CountWords(content);
    chapter.createdAt = std::time(nullptr);
    chapter.updatedAt = chapter.createdAt;

    // Auto-assign chapter number
    auto novelChapters = GetChaptersByNovel(novelId);
    chapter.chapterNumber = novelChapters.size() + 1;

    chapters_.push_back(chapter);
    UpdateWordCounts(novelId);

    LOG_INFO("Created chapter: " + title + " (ID: " + std::to_string(chapter.id) + ")");
    return chapter.id;
}

Chapter* DataManager::GetChapter(int id) {
    auto it = std::find_if(chapters_.begin(), chapters_.end(),
        [id](const Chapter& c) { return c.id == id; });
    return it != chapters_.end() ? &(*it) : nullptr;
}

std::vector<Chapter> DataManager::GetChaptersByNovel(int novelId) {
    std::vector<Chapter> result;
    for (const auto& chapter : chapters_) {
        if (chapter.novelId == novelId) {
            result.push_back(chapter);
        }
    }

    // Sort by chapter number
    std::sort(result.begin(), result.end(),
        [](const Chapter& a, const Chapter& b) {
            return a.chapterNumber < b.chapterNumber;
        });

    return result;
}

bool DataManager::UpdateChapter(const Chapter& chapter) {
    auto it = std::find_if(chapters_.begin(), chapters_.end(),
        [&chapter](const Chapter& c) { return c.id == chapter.id; });

    if (it != chapters_.end()) {
        *it = chapter;
        it->wordCount = CountWords(chapter.content);
        it->updatedAt = std::time(nullptr);
        UpdateWordCounts(chapter.novelId);
        LOG_INFO("Updated chapter ID: " + std::to_string(chapter.id));
        return true;
    }
    return false;
}

bool DataManager::DeleteChapter(int id) {
    auto it = std::find_if(chapters_.begin(), chapters_.end(),
        [id](const Chapter& c) { return c.id == id; });

    if (it != chapters_.end()) {
        int novelId = it->novelId;

        // Delete associated drafts
        drafts_.erase(
            std::remove_if(drafts_.begin(), drafts_.end(),
                [id](const Draft& d) { return d.chapterId == id; }),
            drafts_.end()
        );

        chapters_.erase(it);
        UpdateWordCounts(novelId);
        LOG_INFO("Deleted chapter ID: " + std::to_string(id));
        return true;
    }
    return false;
}

// Character operations
int DataManager::CreateCharacter(int novelId, const std::string& name, const std::string& description) {
    Character character;
    character.id = nextCharacterId_++;
    character.novelId = novelId;
    character.name = name;
    character.description = description;
    character.createdAt = std::time(nullptr);

    characters_.push_back(character);
    LOG_INFO("Created character: " + name + " (ID: " + std::to_string(character.id) + ")");
    return character.id;
}

Character* DataManager::GetCharacter(int id) {
    auto it = std::find_if(characters_.begin(), characters_.end(),
        [id](const Character& c) { return c.id == id; });
    return it != characters_.end() ? &(*it) : nullptr;
}

std::vector<Character> DataManager::GetCharactersByNovel(int novelId) {
    std::vector<Character> result;
    for (const auto& character : characters_) {
        if (character.novelId == novelId) {
            result.push_back(character);
        }
    }
    return result;
}

bool DataManager::UpdateCharacter(const Character& character) {
    auto it = std::find_if(characters_.begin(), characters_.end(),
        [&character](const Character& c) { return c.id == character.id; });

    if (it != characters_.end()) {
        *it = character;
        LOG_INFO("Updated character ID: " + std::to_string(character.id));
        return true;
    }
    return false;
}

bool DataManager::DeleteCharacter(int id) {
    auto it = std::find_if(characters_.begin(), characters_.end(),
        [id](const Character& c) { return c.id == id; });

    if (it != characters_.end()) {
        characters_.erase(it);
        LOG_INFO("Deleted character ID: " + std::to_string(id));
        return true;
    }
    return false;
}

// Draft operations
int DataManager::CreateDraft(int chapterId, const std::string& content, const std::string& note) {
    Draft draft;
    draft.id = nextDraftId_++;
    draft.chapterId = chapterId;
    draft.content = content;
    draft.note = note;
    draft.createdAt = std::time(nullptr);

    // Auto-assign version number
    auto chapterDrafts = GetDraftsByChapter(chapterId);
    draft.versionNumber = chapterDrafts.size() + 1;

    drafts_.push_back(draft);
    LOG_INFO("Created draft version " + std::to_string(draft.versionNumber) +
             " for chapter " + std::to_string(chapterId));
    return draft.id;
}

std::vector<Draft> DataManager::GetDraftsByChapter(int chapterId) {
    std::vector<Draft> result;
    for (const auto& draft : drafts_) {
        if (draft.chapterId == chapterId) {
            result.push_back(draft);
        }
    }

    // Sort by version number
    std::sort(result.begin(), result.end(),
        [](const Draft& a, const Draft& b) {
            return a.versionNumber < b.versionNumber;
        });

    return result;
}

Draft* DataManager::GetDraft(int id) {
    auto it = std::find_if(drafts_.begin(), drafts_.end(),
        [id](const Draft& d) { return d.id == id; });
    return it != drafts_.end() ? &(*it) : nullptr;
}

// Utility
int DataManager::CountWords(const std::string& text) {
    if (text.empty()) return 0;

    int count = 0;
    bool inWord = false;

    for (char c : text) {
        if (std::isspace(c) || std::ispunct(c)) {
            if (inWord) {
                count++;
                inWord = false;
            }
        } else {
            inWord = true;
        }
    }

    if (inWord) count++;
    return count;
}

void DataManager::UpdateWordCounts(int novelId) {
    auto chapters = GetChaptersByNovel(novelId);
    int totalWords = 0;

    for (const auto& chapter : chapters) {
        totalWords += chapter.wordCount;
    }

    Novel* novel = GetNovel(novelId);
    if (novel) {
        novel->totalWords = totalWords;
        novel->updatedAt = std::time(nullptr);
    }
}

} // namespace NovelStudio
