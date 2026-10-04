cmake_minimum_required(VERSION 3.20)
project(NovelStudio VERSION 0.1.0 LANGUAGES CXX)

set(CMAKE_CXX_STANDARD 20)
set(CMAKE_CXX_STANDARD_REQUIRED ON)
set(CMAKE_CXX_EXTENSIONS OFF)

# Output directories
set(CMAKE_RUNTIME_OUTPUT_DIRECTORY ${CMAKE_BINARY_DIR}/bin)
set(CMAKE_LIBRARY_OUTPUT_DIRECTORY ${CMAKE_BINARY_DIR}/lib)
set(CMAKE_ARCHIVE_OUTPUT_DIRECTORY ${CMAKE_BINARY_DIR}/lib)

# Options
option(BUILD_TESTS "Build tests" ON)
option(USE_WINUI3 "Use WinUI 3 for UI" ON)

# Find packages
find_package(SQLite3 REQUIRED)

# Include directories
include_directories(${PROJECT_SOURCE_DIR}/include)
include_directories(${PROJECT_SOURCE_DIR}/third_party)

# Source files
file(GLOB_RECURSE CORE_SOURCES "src/core/*.cpp")
file(GLOB_RECURSE UI_SOURCES "src/ui/*.cpp")
file(GLOB_RECURSE AI_SOURCES "src/ai/*.cpp")
file(GLOB_RECURSE DATABASE_SOURCES "src/database/*.cpp")
file(GLOB_RECURSE UTILS_SOURCES "src/utils/*.cpp")

# Main executable
add_executable(NovelStudio
    src/main.cpp
    ${CORE_SOURCES}
    ${UI_SOURCES}
    ${AI_SOURCES}
    ${DATABASE_SOURCES}
    ${UTILS_SOURCES}
)

target_link_libraries(NovelStudio PRIVATE
    SQLite::SQLite3
)

# Windows specific settings
if(WIN32)
    target_compile_definitions(NovelStudio PRIVATE UNICODE _UNICODE)
    set_target_properties(NovelStudio PROPERTIES
        WIN32_EXECUTABLE ON
    )
endif()

# Tests
if(BUILD_TESTS)
    enable_testing()
    add_subdirectory(tests)
endif()

# Install
install(TARGETS NovelStudio DESTINATION bin)
