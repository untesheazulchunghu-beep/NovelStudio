cmake_minimum_required(VERSION 3.20)
project(NovelStudioServer VERSION 1.0.0 LANGUAGES CXX)

# C++20 Standard
set(CMAKE_CXX_STANDARD 20)
set(CMAKE_CXX_STANDARD_REQUIRED ON)
set(CMAKE_CXX_EXTENSIONS OFF)

# Build type
if(NOT CMAKE_BUILD_TYPE)
    set(CMAKE_BUILD_TYPE Release)
endif()

# Output directories
set(CMAKE_RUNTIME_OUTPUT_DIRECTORY ${CMAKE_BINARY_DIR}/bin)
set(CMAKE_LIBRARY_OUTPUT_DIRECTORY ${CMAKE_BINARY_DIR}/lib)
set(CMAKE_ARCHIVE_OUTPUT_DIRECTORY ${CMAKE_BINARY_DIR}/lib)

# Platform-specific settings
if(WIN32)
    add_definitions(-D_WIN32_WINNT=0x0A00)  # Windows 10
    add_definitions(-DWIN32_LEAN_AND_MEAN)
    add_definitions(-DNOMINMAX)
    set(PLATFORM_LIBS ws2_32 wsock32)
endif()

# Include directories
include_directories(
    ${CMAKE_SOURCE_DIR}/include
    ${CMAKE_SOURCE_DIR}/include/server
    ${CMAKE_SOURCE_DIR}/../include
)

# Find Boost (required by Crow)
find_package(Boost REQUIRED COMPONENTS system thread)
if(Boost_FOUND)
    include_directories(${Boost_INCLUDE_DIRS})
endif()

# Source files
set(CORE_SOURCES
    ../src/core/config_manager.cpp
    ../src/data/data_manager.cpp
    ../src/ai/ai_service.cpp
    ../src/ai/ai_provider_native.cpp
    ../src/ai/ai_providers_extended.cpp
    ../src/utils/http_client.cpp
    ../src/utils/json_utils.cpp
    ../src/utils/sse_parser.cpp
    ../src/utils/logger.cpp
)

# Web Server executable
add_executable(NovelStudioServer
    src/server/crow_server.cpp
    ${CORE_SOURCES}
)

target_link_libraries(NovelStudioServer
    ${Boost_LIBRARIES}
    ${PLATFORM_LIBS}
)

# Compiler warnings
if(MSVC)
    target_compile_options(NovelStudioServer PRIVATE /W4)
else()
    target_compile_options(NovelStudioServer PRIVATE -Wall -Wextra -Wpedantic)
endif()

# Install rules
install(TARGETS NovelStudioServer
    RUNTIME DESTINATION bin
)

# Print configuration
message(STATUS "===========================================")
message(STATUS "NovelStudio Web Server Configuration")
message(STATUS "===========================================")
message(STATUS "Build Type: ${CMAKE_BUILD_TYPE}")
message(STATUS "C++ Standard: ${CMAKE_CXX_STANDARD}")
message(STATUS "Boost Found: ${Boost_FOUND}")
message(STATUS "Boost Version: ${Boost_VERSION}")
message(STATUS "Output Directory: ${CMAKE_RUNTIME_OUTPUT_DIRECTORY}")
message(STATUS "===========================================")
