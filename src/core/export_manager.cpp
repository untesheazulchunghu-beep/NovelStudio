#include "export_manager.h"
#include "logger.h"
#include <fstream>
#include <sstream>

namespace NovelStudio {

// ExportManager implementation
ExportManager::ExportManager(Database* db) : db_(db) {
    LOG_INFO("ExportManager initialized");
}

ExportManager::~ExportManager() = default;

bool ExportManager::ExportToText(int64_t novelId, const std::string& outputPath) {
    LOG_INFO("Exporting novel " + std::to_string(novelId) + " to text: " + outputPath);

    auto novel = db_->GetNovel(novelId);
    if (!novel) {
        LOG_ERROR("Novel not found");
        return false;
    }

    auto chapters = db_->GetChaptersByNovel(novelId);

    std::ofstream file(outputPath);
    if (!file.is_open()) {
        LOG_ERROR("Failed to open output file");
        return false;
    }

    // Write header
    file << novel->title << "\n";
    file << "by " << novel->author << "\n";
    file << std::string(50, '=') << "\n\n";

    // Write chapters
    for (const auto& chapter : chapters) {
        file << "Chapter " << chapter.chapterNumber << ": " << chapter.title << "\n";
        file << std::string(50, '-') << "\n\n";
        file << chapter.content << "\n\n";
    }

    file.close();
    LOG_INFO("Export to text completed successfully");
    return true;
}

bool ExportManager::ExportToMarkdown(int64_t novelId, const std::string& outputPath) {
    LOG_INFO("Exporting novel " + std::to_string(novelId) + " to Markdown: " + outputPath);

    auto novel = db_->GetNovel(novelId);
    if (!novel) {
        LOG_ERROR("Novel not found");
        return false;
    }

    auto chapters = db_->GetChaptersByNovel(novelId);

    std::ofstream file(outputPath);
    if (!file.is_open()) {
        LOG_ERROR("Failed to open output file");
        return false;
    }

    // Write header
    file << "# " << novel->title << "\n\n";
    file << "**Author:** " << novel->author << "\n\n";

    if (!novel->description.empty()) {
        file << "> " << novel->description << "\n\n";
    }

    file << "---\n\n";

    // Write table of contents
    file << "## Table of Contents\n\n";
    for (const auto& chapter : chapters) {
        file << "- [Chapter " << chapter.chapterNumber << ": " << chapter.title << "](#chapter-"
             << chapter.chapterNumber << ")\n";
    }
    file << "\n---\n\n";

    // Write chapters
    for (const auto& chapter : chapters) {
        file << "## Chapter " << chapter.chapterNumber << ": " << chapter.title << " {#chapter-"
             << chapter.chapterNumber << "}\n\n";
        file << chapter.content << "\n\n";
    }

    file.close();
    LOG_INFO("Export to Markdown completed successfully");
    return true;
}

bool ExportManager::ExportToHTML(int64_t novelId, const std::string& outputPath) {
    LOG_INFO("Exporting novel " + std::to_string(novelId) + " to HTML: " + outputPath);

    auto novel = db_->GetNovel(novelId);
    if (!novel) {
        LOG_ERROR("Novel not found");
        return false;
    }

    auto chapters = db_->GetChaptersByNovel(novelId);

    std::ofstream file(outputPath);
    if (!file.is_open()) {
        LOG_ERROR("Failed to open output file");
        return false;
    }

    // Write HTML header
    file << "<!DOCTYPE html>\n<html lang=\"en\">\n<head>\n";
    file << "    <meta charset=\"UTF-8\">\n";
    file << "    <meta name=\"viewport\" content=\"width=device-width, initial-scale=1.0\">\n";
    file << "    <title>" << novel->title << "</title>\n";
    file << "    <style>\n";
    file << "        body { font-family: Georgia, serif; max-width: 800px; margin: 0 auto; padding: 20px; }\n";
    file << "        h1 { text-align: center; }\n";
    file << "        .author { text-align: center; font-style: italic; }\n";
    file << "        .chapter { margin-top: 40px; }\n";
    file << "        .chapter-title { border-bottom: 2px solid #333; padding-bottom: 10px; }\n";
    file << "        .content { line-height: 1.8; text-align: justify; }\n";
    file << "    </style>\n";
    file << "</head>\n<body>\n";

    // Write novel info
    file << "    <h1>" << novel->title << "</h1>\n";
    file << "    <p class=\"author\">by " << novel->author << "</p>\n";

    if (!novel->description.empty()) {
        file << "    <p style=\"text-align: center; font-style: italic;\">" << novel->description << "</p>\n";
    }

    file << "    <hr>\n";

    // Write chapters
    for (const auto& chapter : chapters) {
        file << "    <div class=\"chapter\">\n";
        file << "        <h2 class=\"chapter-title\">Chapter " << chapter.chapterNumber << ": "
             << chapter.title << "</h2>\n";
        file << "        <div class=\"content\">\n";

        // Simple paragraph conversion
        std::istringstream iss(chapter.content);
        std::string line;
        while (std::getline(iss, line)) {
            if (!line.empty()) {
                file << "            <p>" << line << "</p>\n";
            }
        }

        file << "        </div>\n";
        file << "    </div>\n";
    }

    file << "</body>\n</html>\n";
    file.close();

    LOG_INFO("Export to HTML completed successfully");
    return true;
}

bool ExportManager::ExportToDOCX(int64_t novelId, const std::string& outputPath) {
    LOG_WARNING("DOCX export not yet implemented");
    // Would use a library like libdocx or generate OpenXML directly
    return false;
}

bool ExportManager::ExportToPDF(int64_t novelId, const std::string& outputPath) {
    LOG_WARNING("PDF export not yet implemented");
    // Would use a library like libharu or wkhtmltopdf
    return false;
}

bool ExportManager::ExportToEPUB(int64_t novelId, const std::string& outputPath) {
    LOG_WARNING("EPUB export not yet implemented");
    // Would generate EPUB container structure
    return false;
}

ExportManager::ExportStats ExportManager::GetExportStats(int64_t novelId) {
    LOG_DEBUG("Calculating export stats for novel " + std::to_string(novelId));

    ExportStats stats{};

    auto novel = db_->GetNovel(novelId);
    if (!novel) {
        return stats;
    }

    auto chapters = db_->GetChaptersByNovel(novelId);

    stats.totalChapters = static_cast<int>(chapters.size());

    for (const auto& chapter : chapters) {
        std::istringstream iss(chapter.content);
        std::string word;
        while (iss >> word) {
            stats.totalWords++;
        }
        stats.totalCharacters += chapter.content.length();
    }

    return stats;
}

} // namespace NovelStudio
