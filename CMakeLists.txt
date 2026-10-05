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
find_package(SQLite3)

# Windows HTTP library (WinHTTP) - no extra package needed, built-in
if(WIN32)
    # WinHTTP is linked via pragma comment in http_client.cpp
endif()

if(NOT SQLite3_FOUND)
    message(WARNING "SQLite3 not found. Database features will be disabled. Install via: vcpkg install sqlite3:x64-windows")
endif()

# Include directories
include_directories(${PROJECT_SOURCE_DIR}/include)
include_directories(${PROJECT_SOURCE_DIR}/third_party)

# Source files
file(GLOB_RECURSE CORE_SOURCES "src/core/*.cpp")
file(GLOB_RECURSE UI_SOURCES "src/ui/*.cpp")
file(GLOB AI_SOURCES "src/ai/*.cpp")
file(GLOB_RECURSE DATABASE_SOURCES "src/database/*.cpp")
file(GLOB_RECURSE UTILS_SOURCES "src/utils/*.cpp")

# Header files
file(GLOB_RECURSE HEADER_FILES "include/*.h")

# Main executable
if(SQLite3_FOUND)
    add_executable(NovelStudio
        src/main.cpp
        ${CORE_SOURCES}
        ${UI_SOURCES}
        ${AI_SOURCES}
        ${DATABASE_SOURCES}
        ${UTILS_SOURCES}
        ${HEADER_FILES}
    )

    target_link_libraries(NovelStudio PRIVATE
        SQLite::SQLite3
    )

    # Enable warnings
    if(MSVC)
        target_compile_options(NovelStudio PRIVATE /W4)
    else()
        target_compile_options(NovelStudio PRIVATE -Wall -Wextra)
    endif()

    # Windows specific settings
    if(WIN32)
        target_compile_definitions(NovelStudio PRIVATE UNICODE _UNICODE)
        set_target_properties(NovelStudio PROPERTIES
            WIN32_EXECUTABLE ON
        )
    endif()

    # Install
    install(TARGETS NovelStudio DESTINATION bin)
else()
    message(STATUS "Skipping NovelStudio main executable (requires SQLite3)")
endif()

# Test executable for native implementations (no SQLite3 needed)
add_executable(TestNative
    src/test_native.cpp
    src/utils/http_client.cpp
    src/utils/json_utils.cpp
    src/utils/logger.cpp
)

if(WIN32)
    target_compile_definitions(TestNative PRIVATE UNICODE _UNICODE)
endif()

# Test executable for streaming AI (no SQLite3 needed)
add_executable(TestStreaming
    src/test_streaming.cpp
    src/ai/ai_provider_native.cpp
    src/ai/ai_providers_extended.cpp
    src/ai/ai_provider_factory.cpp
    src/utils/http_client.cpp
    src/utils/json_utils.cpp
    src/utils/sse_parser.cpp
    src/utils/logger.cpp
)

if(WIN32)
    target_compile_definitions(TestStreaming PRIVATE UNICODE _UNICODE)
endif()

# Test executable for all AI providers
add_executable(TestProviders
    src/test_providers.cpp
    src/ai/ai_provider_native.cpp
    src/ai/ai_providers_extended.cpp
    src/ai/ai_provider_factory.cpp
    src/utils/http_client.cpp
    src/utils/json_utils.cpp
    src/utils/sse_parser.cpp
    src/utils/logger.cpp
)

if(WIN32)
    target_compile_definitions(TestProviders PRIVATE UNICODE _UNICODE)
endif()

# Tests
if(BUILD_TESTS)
    enable_testing()
    add_subdirectory(tests)
endif()
