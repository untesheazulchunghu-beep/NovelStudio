#include "database.h"
#include <iostream>
#include <cassert>

using namespace NovelStudio;

void TestNovelOperations() {
    std::cout << "Testing Novel operations..." << std::endl;

    Database db;
    assert(db.Open("test.db"));

    // Create novel
    Novel novel;
    novel.title = "Test Novel";
    novel.description = "A test novel";
    novel.author = "Test Author";

    assert(db.CreateNovel(novel));
    assert(novel.id > 0);

    // Get novel
    auto retrieved = db.GetNovel(novel.id);
    assert(retrieved.has_value());
    assert(retrieved->title == "Test Novel");

    // Update novel
    novel.title = "Updated Novel";
    assert(db.UpdateNovel(novel));

    retrieved = db.GetNovel(novel.id);
    assert(retrieved->title == "Updated Novel");

    // Delete novel
    assert(db.DeleteNovel(novel.id));
    retrieved = db.GetNovel(novel.id);
    assert(!retrieved.has_value());

    std::cout << "Novel operations: PASSED" << std::endl;
}

void TestChapterOperations() {
    std::cout << "Testing Chapter operations..." << std::endl;

    Database db;
    assert(db.Open("test.db"));

    // Create novel first
    Novel novel;
    novel.title = "Test Novel";
    assert(db.CreateNovel(novel));

    // Create chapter
    Chapter chapter;
    chapter.novelId = novel.id;
    chapter.chapterNumber = 1;
    chapter.title = "Chapter 1";
    chapter.content = "This is chapter 1 content";

    assert(db.CreateChapter(chapter));
    assert(chapter.id > 0);

    // Get chapter
    auto retrieved = db.GetChapter(chapter.id);
    assert(retrieved.has_value());
    assert(retrieved->title == "Chapter 1");

    // Get chapters by novel
    auto chapters = db.GetChaptersByNovel(novel.id);
    assert(chapters.size() == 1);

    // Update chapter
    chapter.content = "Updated content";
    assert(db.UpdateChapter(chapter));

    // Clean up
    db.DeleteChapter(chapter.id);
    db.DeleteNovel(novel.id);

    std::cout << "Chapter operations: PASSED" << std::endl;
}

int main() {
    std::cout << "Running NovelStudio tests..." << std::endl;

    TestNovelOperations();
    TestChapterOperations();

    std::cout << "All tests passed!" << std::endl;
    return 0;
}
