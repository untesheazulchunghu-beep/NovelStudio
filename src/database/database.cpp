#include "database.h"
#include <sqlite3.h>
#include <ctime>
#include <stdexcept>

namespace NovelStudio {

Database::Database() = default;

Database::~Database() {
    Close();
}

bool Database::Open(const std::string& path) {
    if (db_) {
        Close();
    }

    int rc = sqlite3_open(path.c_str(), &db_);
    if (rc != SQLITE_OK) {
        sqlite3_close(db_);
        db_ = nullptr;
        return false;
    }

    return InitializeTables();
}

void Database::Close() {
    if (db_) {
        sqlite3_close(db_);
        db_ = nullptr;
    }
}

bool Database::IsOpen() const {
    return db_ != nullptr;
}

bool Database::InitializeTables() {
    const char* schema = R"(
        CREATE TABLE IF NOT EXISTS novels (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            title TEXT NOT NULL,
            description TEXT,
            author TEXT,
            created_at INTEGER NOT NULL,
            updated_at INTEGER NOT NULL
        );

        CREATE TABLE IF NOT EXISTS chapters (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            novel_id INTEGER NOT NULL,
            chapter_number INTEGER NOT NULL,
            title TEXT NOT NULL,
            content TEXT,
            created_at INTEGER NOT NULL,
            updated_at INTEGER NOT NULL,
            FOREIGN KEY (novel_id) REFERENCES novels(id) ON DELETE CASCADE
        );

        CREATE TABLE IF NOT EXISTS characters (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            novel_id INTEGER NOT NULL,
            name TEXT NOT NULL,
            description TEXT,
            traits TEXT,
            notes TEXT,
            FOREIGN KEY (novel_id) REFERENCES novels(id) ON DELETE CASCADE
        );

        CREATE INDEX IF NOT EXISTS idx_chapters_novel ON chapters(novel_id);
        CREATE INDEX IF NOT EXISTS idx_characters_novel ON characters(novel_id);
    )";

    char* errMsg = nullptr;
    int rc = sqlite3_exec(db_, schema, nullptr, nullptr, &errMsg);

    if (rc != SQLITE_OK) {
        sqlite3_free(errMsg);
        return false;
    }

    return true;
}

int64_t Database::GetLastInsertRowId() {
    return sqlite3_last_insert_rowid(db_);
}

// Novel operations
bool Database::CreateNovel(Novel& novel) {
    const char* sql = "INSERT INTO novels (title, description, author, created_at, updated_at) "
                      "VALUES (?, ?, ?, ?, ?)";

    sqlite3_stmt* stmt;
    if (sqlite3_prepare_v2(db_, sql, -1, &stmt, nullptr) != SQLITE_OK) {
        return false;
    }

    int64_t now = std::time(nullptr);
    novel.createdAt = now;
    novel.updatedAt = now;

    sqlite3_bind_text(stmt, 1, novel.title.c_str(), -1, SQLITE_TRANSIENT);
    sqlite3_bind_text(stmt, 2, novel.description.c_str(), -1, SQLITE_TRANSIENT);
    sqlite3_bind_text(stmt, 3, novel.author.c_str(), -1, SQLITE_TRANSIENT);
    sqlite3_bind_int64(stmt, 4, novel.createdAt);
    sqlite3_bind_int64(stmt, 5, novel.updatedAt);

    bool success = sqlite3_step(stmt) == SQLITE_DONE;
    if (success) {
        novel.id = GetLastInsertRowId();
    }

    sqlite3_finalize(stmt);
    return success;
}

std::optional<Novel> Database::GetNovel(int64_t id) {
    const char* sql = "SELECT id, title, description, author, created_at, updated_at "
                      "FROM novels WHERE id = ?";

    sqlite3_stmt* stmt;
    if (sqlite3_prepare_v2(db_, sql, -1, &stmt, nullptr) != SQLITE_OK) {
        return std::nullopt;
    }

    sqlite3_bind_int64(stmt, 1, id);

    std::optional<Novel> result;
    if (sqlite3_step(stmt) == SQLITE_ROW) {
        Novel novel;
        novel.id = sqlite3_column_int64(stmt, 0);
        novel.title = reinterpret_cast<const char*>(sqlite3_column_text(stmt, 1));
        novel.description = reinterpret_cast<const char*>(sqlite3_column_text(stmt, 2));
        novel.author = reinterpret_cast<const char*>(sqlite3_column_text(stmt, 3));
        novel.createdAt = sqlite3_column_int64(stmt, 4);
        novel.updatedAt = sqlite3_column_int64(stmt, 5);
        result = novel;
    }

    sqlite3_finalize(stmt);
    return result;
}

std::vector<Novel> Database::GetAllNovels() {
    const char* sql = "SELECT id, title, description, author, created_at, updated_at "
                      "FROM novels ORDER BY updated_at DESC";

    sqlite3_stmt* stmt;
    if (sqlite3_prepare_v2(db_, sql, -1, &stmt, nullptr) != SQLITE_OK) {
        return {};
    }

    std::vector<Novel> novels;
    while (sqlite3_step(stmt) == SQLITE_ROW) {
        Novel novel;
        novel.id = sqlite3_column_int64(stmt, 0);
        novel.title = reinterpret_cast<const char*>(sqlite3_column_text(stmt, 1));
        novel.description = reinterpret_cast<const char*>(sqlite3_column_text(stmt, 2));
        novel.author = reinterpret_cast<const char*>(sqlite3_column_text(stmt, 3));
        novel.createdAt = sqlite3_column_int64(stmt, 4);
        novel.updatedAt = sqlite3_column_int64(stmt, 5);
        novels.push_back(novel);
    }

    sqlite3_finalize(stmt);
    return novels;
}

bool Database::UpdateNovel(const Novel& novel) {
    const char* sql = "UPDATE novels SET title = ?, description = ?, author = ?, "
                      "updated_at = ? WHERE id = ?";

    sqlite3_stmt* stmt;
    if (sqlite3_prepare_v2(db_, sql, -1, &stmt, nullptr) != SQLITE_OK) {
        return false;
    }

    int64_t now = std::time(nullptr);
    sqlite3_bind_text(stmt, 1, novel.title.c_str(), -1, SQLITE_TRANSIENT);
    sqlite3_bind_text(stmt, 2, novel.description.c_str(), -1, SQLITE_TRANSIENT);
    sqlite3_bind_text(stmt, 3, novel.author.c_str(), -1, SQLITE_TRANSIENT);
    sqlite3_bind_int64(stmt, 4, now);
    sqlite3_bind_int64(stmt, 5, novel.id);

    bool success = sqlite3_step(stmt) == SQLITE_DONE;
    sqlite3_finalize(stmt);
    return success;
}

bool Database::DeleteNovel(int64_t id) {
    const char* sql = "DELETE FROM novels WHERE id = ?";

    sqlite3_stmt* stmt;
    if (sqlite3_prepare_v2(db_, sql, -1, &stmt, nullptr) != SQLITE_OK) {
        return false;
    }

    sqlite3_bind_int64(stmt, 1, id);
    bool success = sqlite3_step(stmt) == SQLITE_DONE;
    sqlite3_finalize(stmt);
    return success;
}

// Chapter operations
bool Database::CreateChapter(Chapter& chapter) {
    const char* sql = "INSERT INTO chapters (novel_id, chapter_number, title, content, "
                      "created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)";

    sqlite3_stmt* stmt;
    if (sqlite3_prepare_v2(db_, sql, -1, &stmt, nullptr) != SQLITE_OK) {
        return false;
    }

    int64_t now = std::time(nullptr);
    chapter.createdAt = now;
    chapter.updatedAt = now;

    sqlite3_bind_int64(stmt, 1, chapter.novelId);
    sqlite3_bind_int(stmt, 2, chapter.chapterNumber);
    sqlite3_bind_text(stmt, 3, chapter.title.c_str(), -1, SQLITE_TRANSIENT);
    sqlite3_bind_text(stmt, 4, chapter.content.c_str(), -1, SQLITE_TRANSIENT);
    sqlite3_bind_int64(stmt, 5, chapter.createdAt);
    sqlite3_bind_int64(stmt, 6, chapter.updatedAt);

    bool success = sqlite3_step(stmt) == SQLITE_DONE;
    if (success) {
        chapter.id = GetLastInsertRowId();
    }

    sqlite3_finalize(stmt);
    return success;
}

std::optional<Chapter> Database::GetChapter(int64_t id) {
    const char* sql = "SELECT id, novel_id, chapter_number, title, content, "
                      "created_at, updated_at FROM chapters WHERE id = ?";

    sqlite3_stmt* stmt;
    if (sqlite3_prepare_v2(db_, sql, -1, &stmt, nullptr) != SQLITE_OK) {
        return std::nullopt;
    }

    sqlite3_bind_int64(stmt, 1, id);

    std::optional<Chapter> result;
    if (sqlite3_step(stmt) == SQLITE_ROW) {
        Chapter chapter;
        chapter.id = sqlite3_column_int64(stmt, 0);
        chapter.novelId = sqlite3_column_int64(stmt, 1);
        chapter.chapterNumber = sqlite3_column_int(stmt, 2);
        chapter.title = reinterpret_cast<const char*>(sqlite3_column_text(stmt, 3));
        chapter.content = reinterpret_cast<const char*>(sqlite3_column_text(stmt, 4));
        chapter.createdAt = sqlite3_column_int64(stmt, 5);
        chapter.updatedAt = sqlite3_column_int64(stmt, 6);
        result = chapter;
    }

    sqlite3_finalize(stmt);
    return result;
}

std::vector<Chapter> Database::GetChaptersByNovel(int64_t novelId) {
    const char* sql = "SELECT id, novel_id, chapter_number, title, content, "
                      "created_at, updated_at FROM chapters WHERE novel_id = ? "
                      "ORDER BY chapter_number ASC";

    sqlite3_stmt* stmt;
    if (sqlite3_prepare_v2(db_, sql, -1, &stmt, nullptr) != SQLITE_OK) {
        return ;
    }

    sqlite3_bind_int64(stmt, 1, novelId);

    std::vector<Chapter> chapters;
    while (sqlite3_step(stmt) == SQLITE_ROW) {
        Chapter chapter;
        chapter.id = sqlite3_column_int64(stmt, 0);
        chapter.novelId = sqlite3_column_int64(stmt, 1);
        chapter.chapterNumber = sqlite3_column_int(stmt, 2);
        chapter.title = reinterpret_cast<const char*>(sqlite3_column_text(stmt, 3));
        chapter.content = reinterpret_cast<const char*>(sqlite3_column_text(stmt, 4));
        chapter.createdAt = sqlite3_column_int64(stmt, 5);
        chapter.updatedAt = sqlite3_column_int64(stmt, 6);
        chapters.push_back(chapter);
    }

    sqlite3_finalize(stmt);
    return chapters;
}

bool Database::UpdateChapter(const Chapter& chapter) {
    const char* sql = "UPDATE chapters SET chapter_number = ?, title = ?, content = ?, "
                      "updated_at = ? WHERE id = ?";

    sqlite3_stmt* stmt;
    if (sqlite3_prepare_v2(db_, sql, -1, &stmt, nullptr) != SQLITE_OK) {
        return false;
    }

    int64_t now = std::time(nullptr);
    sqlite3_bind_int(stmt, 1, chapter.chapterNumber);
    sqlite3_bind_text(stmt, 2, chapter.title.c_str(), -1, SQLITE_TRANSIENT);
    sqlite3_bind_text(stmt, 3, chapter.content.c_str(), -1, SQLITE_TRANSIENT);
    sqlite3_bind_int64(stmt, 4, now);
    sqlite3_bind_int64(stmt, 5, chapter.id);

    bool success = sqlite3_step(stmt) == SQLITE_DONE;
    sqlite3_finalize(stmt);
    return success;
}

bool Database::DeleteChapter(int64_t id) {
    const char* sql = "DELETE FROM chapters WHERE id = ?";

    sqlite3_stmt* stmt;
    if (sqlite3_prepare_v2(db_, sql, -1, &stmt, nullptr) != SQLITE_OK) {
        return false;
    }

    sqlite3_bind_int64(stmt, 1, id);
    bool success = sqlite3_step(stmt) == SQLITE_DONE;
    sqlite3_finalize(stmt);
    return success;
}

// Character operations (similar pattern)
bool Database::CreateCharacter(Character& character) {
    const char* sql = "INSERT INTO characters (novel_id, name, description, traits, notes) "
                      "VALUES (?, ?, ?, ?, ?)";

    sqlite3_stmt* stmt;
    if (sqlite3_prepare_v2(db_, sql, -1, &stmt, nullptr) != SQLITE_OK) {
        return false;
    }

    sqlite3_bind_int64(stmt, 1, character.novelId);
    sqlite3_bind_text(stmt, 2, character.name.c_str(), -1, SQLITE_TRANSIENT);
    sqlite3_bind_text(stmt, 3, character.description.c_str(), -1, SQLITE_TRANSIENT);
    sqlite3_bind_text(stmt, 4, character.traits.c_str(), -1, SQLITE_TRANSIENT);
    sqlite3_bind_text(stmt, 5, character.notes.c_str(), -1, SQLITE_TRANSIENT);

    bool success = sqlite3_step(stmt) == SQLITE_DONE;
    if (success) {
        character.id = GetLastInsertRowId();
    }

    sqlite3_finalize(stmt);
    return success;
}

std::optional<Character> Database::GetCharacter(int64_t id) {
    const char* sql = "SELECT id, novel_id, name, description, traits, notes "
                      "FROM characters WHERE id = ?";

    sqlite3_stmt* stmt;
    if (sqlite3_prepare_v2(db_, sql, -1, &stmt, nullptr) != SQLITE_OK) {
        return std::nullopt;
    }

    sqlite3_bind_int64(stmt, 1, id);

    std::optional<Character> result;
    if (sqlite3_step(stmt) == SQLITE_ROW) {
        Character character;
        character.id = sqlite3_column_int64(stmt, 0);
        character.novelId = sqlite3_column_int64(stmt, 1);
        character.name = reinterpret_cast<const char*>(sqlite3_column_text(stmt, 2));
        character.description = reinterpret_cast<const char*>(sqlite3_column_text(stmt, 3));
        character.traits = reinterpret_cast<const char*>(sqlite3_column_text(stmt, 4));
        character.notes = reinterpret_cast<const char*>(sqlite3_column_text(stmt, 5));
        result = character;
    }

    sqlite3_finalize(stmt);
    return result;
}

std::vector<Character> Database::GetCharactersByNovel(int64_t novelId) {
    const char* sql = "SELECT id, novel_id, name, description, traits, notes "
                      "FROM characters WHERE novel_id = ? ORDER BY name ASC";

    sqlite3_stmt* stmt;
    if (sqlite3_prepare_v2(db_, sql, -1, &stmt, nullptr) != SQLITE_OK) {
        return {};
    }

    sqlite3_bind_int64(stmt, 1, novelId);

    std::vector<Character> characters;
    while (sqlite3_step(stmt) == SQLITE_ROW) {
        Character character;
        character.id = sqlite3_column_int64(stmt, 0);
        character.novelId = sqlite3_column_int64(stmt, 1);
        character.name = reinterpret_cast<const char*>(sqlite3_column_text(stmt, 2));
        character.description = reinterpret_cast<const char*>(sqlite3_column_text(stmt, 3));
        character.traits = reinterpret_cast<const char*>(sqlite3_column_text(stmt, 4));
        character.notes = reinterpret_cast<const char*>(sqlite3_column_text(stmt, 5));
        characters.push_back(character);
    }

    sqlite3_finalize(stmt);
    return characters;
}

bool Database::UpdateCharacter(const Character& character) {
    const char* sql = "UPDATE characters SET name = ?, description = ?, traits = ?, "
                      "notes = ? WHERE id = ?";

    sqlite3_stmt* stmt;
    if (sqlite3_prepare_v2(db_, sql, -1, &stmt, nullptr) != SQLITE_OK) {
        return false;
    }

    sqlite3_bind_text(stmt, 1, character.name.c_str(), -1, SQLITE_TRANSIENT);
    sqlite3_bind_text(stmt, 2, character.description.c_str(), -1, SQLITE_TRANSIENT);
    sqlite3_bind_text(stmt, 3, character.traits.c_str(), -1, SQLITE_TRANSIENT);
    sqlite3_bind_text(stmt, 4, character.notes.c_str(), -1, SQLITE_TRANSIENT);
    sqlite3_bind_int64(stmt, 5, character.id);

    bool success = sqlite3_step(stmt) == SQLITE_DONE;
    sqlite3_finalize(stmt);
    return success;
}

bool Database::DeleteCharacter(int64_t id) {
    const char* sql = "DELETE FROM characters WHERE id = ?";

    sqlite3_stmt* stmt;
    if (sqlite3_prepare_v2(db_, sql, -1, &stmt, nullptr) != SQLITE_OK) {
        return false;
    }

    sqlite3_bind_int64(stmt, 1, id);
    bool success = sqlite3_step(stmt) == SQLITE_DONE;
    sqlite3_finalize(stmt);
    return success;
}

} // namespace NovelStudio
