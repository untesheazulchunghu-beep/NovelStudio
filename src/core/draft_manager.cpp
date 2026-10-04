#include "draft_manager.h"
#include "logger.h"
#include <sqlite3.h>
#include <ctime>
#include <sstream>
#include <algorithm>

namespace NovelStudio {

// DraftManager implementation
class DraftManager::Impl {
public:
    Database* db;

    bool Initialize() {
        const char* schema = R"(
            CREATE TABLE IF NOT EXISTS chapter_drafts (
                id INTEGER PRIMARY KEY AUTOINCREMENT,
                chapter_id INTEGER NOT NULL,
                draft_number INTEGER NOT NULL,
                content TEXT,
                source TEXT NOT NULL DEFAULT 'user',
                ai_confidence REAL DEFAULT 0.0,
                created_at INTEGER NOT NULL,
                is_selected INTEGER DEFAULT 0,
                FOREIGN KEY (chapter_id) REFERENCES chapters(id) ON DELETE CASCADE
            );

            CREATE INDEX IF NOT EXISTS idx_drafts_chapter ON chapter_drafts(chapter_id);
            CREATE INDEX IF NOT EXISTS idx_drafts_selected ON chapter_drafts(chapter_id, is_selected);
        )";

        // Execute using Database's connection
        sqlite3* dbConn = GetDatabaseConnection();
        if (!dbConn) return false;

        char* errMsg = nullptr;
        int rc = sqlite3_exec(dbConn, schema, nullptr, nullptr, &errMsg);

        if (rc != SQLITE_OK) {
            LOG_ERROR("Failed to create drafts table: " + std::string(errMsg));
            sqlite3_free(errMsg);
            return false;
        }

        return true;
    }

    sqlite3* GetDatabaseConnection() {
        // Access the database connection through a friend or public method
        // For now, we'll need to add this capability to Database class
        return nullptr; // Placeholder
    }
};

DraftManager::DraftManager(Database* db) : impl_(std::make_unique<Impl>()) {
    impl_->db = db;
    impl_->Initialize();
    LOG_INFO("DraftManager initialized");
}

DraftManager::~DraftManager() = default;

bool DraftManager::CreateDraft(ChapterDraft& draft) {
    LOG_DEBUG("Creating draft for chapter " + std::to_string(draft.chapterId));

    // Get next draft number
    int nextNumber = GetDraftCount(draft.chapterId) + 1;
    draft.draftNumber = nextNumber;
    draft.createdAt = std::time(nullptr);

    // Implementation would use database here
    // For now, returning success
    draft.id = nextNumber; // Placeholder

    LOG_INFO("Draft created: " + std::to_string(draft.id));
    return true;
}

std::vector<ChapterDraft> DraftManager::GetDrafts(int64_t chapterId) {
    LOG_DEBUG("Getting drafts for chapter " + std::to_string(chapterId));

    std::vector<ChapterDraft> drafts;
    // Implementation would query database here

    return drafts;
}

std::optional<ChapterDraft> DraftManager::GetDraft(int64_t draftId) {
    LOG_DEBUG("Getting draft " + std::to_string(draftId));

    // Implementation would query database here
    return std::nullopt;
}

bool DraftManager::SelectDraft(int64_t draftId) {
    LOG_INFO("Selecting draft " + std::to_string(draftId));

    // Implementation would:
    // 1. Unselect all drafts for this chapter
    // 2. Select the specified draft
    // 3. Update chapter content

    return true;
}

std::optional<ChapterDraft> DraftManager::GetSelectedDraft(int64_t chapterId) {
    LOG_DEBUG("Getting selected draft for chapter " + std::to_string(chapterId));

    // Implementation would query database here
    return std::nullopt;
}

std::string DraftManager::CompareDrafts(int64_t draft1Id, int64_t draft2Id) {
    LOG_DEBUG("Comparing drafts " + std::to_string(draft1Id) + " and " + std::to_string(draft2Id));

    // Simple diff implementation
    // A real implementation would use a proper diff algorithm

    return "Diff comparison not yet implemented";
}

bool DraftManager::DeleteDraft(int64_t draftId) {
    LOG_INFO("Deleting draft " + std::to_string(draftId));

    // Implementation would delete from database
    return true;
}

int DraftManager::GetDraftCount(int64_t chapterId) {
    // Implementation would count drafts in database
    return 0;
}

// StatisticsManager implementation
class StatisticsManager::Impl {
public:
    Database* db;

    bool Initialize() {
        const char* schema = R"(
            CREATE TABLE IF NOT EXISTS writing_sessions (
                id INTEGER PRIMARY KEY AUTOINCREMENT,
                novel_id INTEGER NOT NULL,
                chapter_id INTEGER,
                words_written INTEGER NOT NULL,
                start_time INTEGER NOT NULL,
                end_time INTEGER NOT NULL,
                duration_seconds INTEGER NOT NULL,
                FOREIGN KEY (novel_id) REFERENCES novels(id) ON DELETE CASCADE,
                FOREIGN KEY (chapter_id) REFERENCES chapters(id) ON DELETE SET NULL
            );

            CREATE INDEX IF NOT EXISTS idx_sessions_novel ON writing_sessions(novel_id);
            CREATE INDEX IF NOT EXISTS idx_sessions_time ON writing_sessions(start_time);
        )";

        sqlite3* dbConn = nullptr; // Would get from Database
        if (!dbConn) return false;

        char* errMsg = nullptr;
        int rc = sqlite3_exec(dbConn, schema, nullptr, nullptr, &errMsg);

        if (rc != SQLITE_OK) {
            LOG_ERROR("Failed to create statistics tables: " + std::string(errMsg));
            sqlite3_free(errMsg);
            return false;
        }

        return true;
    }
};

StatisticsManager::StatisticsManager(Database* db) : impl_(std::make_unique<Impl>()) {
    impl_->db = db;
    impl_->Initialize();
    LOG_INFO("StatisticsManager initialized");
}

StatisticsManager::~StatisticsManager() = default;

bool StatisticsManager::RecordSession(WritingSession& session) {
    LOG_INFO("Recording writing session: " + std::to_string(session.wordsWritten) + " words");

    session.durationSeconds = session.endTime - session.startTime;
    session.id = 1; // Placeholder

    return true;
}

std::vector<StatisticsManager::DailyStats> StatisticsManager::GetDailyStats(int64_t novelId, int days) {
    LOG_DEBUG("Getting daily stats for novel " + std::to_string(novelId));

    std::vector<DailyStats> stats;
    // Implementation would aggregate data from database

    return stats;
}

StatisticsManager::NovelStats StatisticsManager::GetNovelStats(int64_t novelId) {
    LOG_DEBUG("Getting novel stats for " + std::to_string(novelId));

    NovelStats stats{};
    // Implementation would calculate from database

    return stats;
}

} // namespace NovelStudio
