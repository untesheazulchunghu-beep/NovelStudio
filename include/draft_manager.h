#pragma once

#include <string>
#include <vector>
#include <memory>
#include <optional>
#include "database.h"

namespace NovelStudio {

// Draft management for version control
struct ChapterDraft {
    int64_t id = 0;
    int64_t chapterId = 0;
    int draftNumber = 0;
    std::string content;
    std::string source;        // "user", "ai", "unknown"
    float aiConfidence = 0.0f; // 0-1 confidence score
    int64_t createdAt = 0;
    bool isSelected = false;   // Current active draft
};

// Draft manager class
class DraftManager {
public:
    explicit DraftManager(Database* db);
    ~DraftManager();

    // Create a new draft
    bool CreateDraft(ChapterDraft& draft);

    // Get all drafts for a chapter
    std::vector<ChapterDraft> GetDrafts(int64_t chapterId);

    // Get specific draft
    std::optional<ChapterDraft> GetDraft(int64_t draftId);

    // Select a draft as active
    bool SelectDraft(int64_t draftId);

    // Get currently selected draft
    std::optional<ChapterDraft> GetSelectedDraft(int64_t chapterId);

    // Compare two drafts (returns diff)
    std::string CompareDrafts(int64_t draft1Id, int64_t draft2Id);

    // Delete a draft
    bool DeleteDraft(int64_t draftId);

    // Get draft count for chapter
    int GetDraftCount(int64_t chapterId);

private:
    class Impl;
    std::unique_ptr<Impl> impl_;
};

// Writing session tracker
struct WritingSession {
    int64_t id = 0;
    int64_t novelId = 0;
    int64_t chapterId = 0;
    int wordsWritten = 0;
    int64_t startTime = 0;
    int64_t endTime = 0;
    int64_t durationSeconds = 0;
};

// Statistics manager
class StatisticsManager {
public:
    explicit StatisticsManager(Database* db);
    ~StatisticsManager();

    // Track writing session
    bool RecordSession(WritingSession& session);

    // Get statistics
    struct DailyStats {
        int64_t date;
        int wordsWritten;
        int sessionsCount;
        int64_t totalTimeSeconds;
    };

    std::vector<DailyStats> GetDailyStats(int64_t novelId, int days = 7);

    struct NovelStats {
        int totalWords;
        int totalChapters;
        int totalCharacters;
        int64_t firstWritten;
        int64_t lastWritten;
        int64_t totalTimeSeconds;
    };

    NovelStats GetNovelStats(int64_t novelId);

private:
    class Impl;
    std::unique_ptr<Impl> impl_;
};

} // namespace NovelStudio
