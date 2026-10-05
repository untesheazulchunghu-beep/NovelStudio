# Tests require SQLite3
if(NOT SQLite3_FOUND)
    message(STATUS "Skipping tests (requires SQLite3)")
    return()
endif()

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
