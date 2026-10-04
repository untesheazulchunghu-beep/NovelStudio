cmake_minimum_required(VERSION 3.20)

add_executable(NovelStudioTests
    test_database.cpp
)

# Link against Google Test (if available) or just the main library
target_link_libraries(NovelStudioTests PRIVATE
    SQLite::SQLite3
)

target_include_directories(NovelStudioTests PRIVATE
    ${PROJECT_SOURCE_DIR}/include
)

add_test(NAME NovelStudioTests COMMAND NovelStudioTests)
