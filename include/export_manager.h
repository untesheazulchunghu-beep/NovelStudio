#pragma once

#include <string>
#include "database.h"

namespace NovelStudio {

// Export formats
enum class ExportFormat {
    Text,
    Markdown,
    HTML,
    DOCX,
    PDF,
    EPUB
};

// Export manager class
class ExportManager {
public:
    explicit ExportManager(Database* db);
    ~ExportManager();

    // Export to different formats
    bool ExportToText(int64_t novelId, const std::string& outputPath);
    bool ExportToMarkdown(int64_t novelId, const std::string& outputPath);
    bool ExportToHTML(int64_t novelId, const std::string& outputPath);
    bool ExportToDOCX(int64_t novelId, const std::string& outputPath);
    bool ExportToPDF(int64_t novelId, const std::string& outputPath);
    bool ExportToEPUB(int64_t novelId, const std::string& outputPath);

    // Export statistics
    struct ExportStats {
        int totalChapters;
        int totalWords;
        int totalCharacters;
    };

    ExportStats GetExportStats(int64_t novelId);

private:
    Database* db_;
};

} // namespace NovelStudio
