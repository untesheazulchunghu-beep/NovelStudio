// Simple test program to verify native HTTP and JSON work
#include "http_client.h"
#include "json_utils.h"
#include "logger.h"
#include <iostream>

using namespace NovelStudio;

void TestJsonParser() {
    std::cout << "\n=== Testing JSON Parser ===" << std::endl;

    // Create JSON
    JsonValue obj = JsonValue::Object();
    obj["name"] = JsonValue("NovelStudio");
    obj["version"] = JsonValue(1.0);
    obj["active"] = JsonValue(true);

    JsonValue features = JsonValue::Array();
    features.Push(JsonValue("AI Writing"));
    features.Push(JsonValue("Chapter Management"));
    features.Push(JsonValue("Export"));
    obj["features"] = features;

    std::string json = obj.ToString(true);
    std::cout << "Created JSON:\n" << json << std::endl;

    // Parse JSON
    const char* testJson = R"({
        "model": "gpt-4",
        "temperature": 0.7,
        "max_tokens": 2048,
        "messages": [
            {"role": "system", "content": "You are a helpful assistant."},
            {"role": "user", "content": "Hello!"}
        ]
    })";

    try {
        JsonValue parsed = JsonValue::Parse(testJson);
        std::cout << "\nParsed JSON successfully!" << std::endl;
        std::cout << "Model: " << parsed["model"].AsString() << std::endl;
        std::cout << "Temperature: " << parsed["temperature"].AsNumber() << std::endl;
        std::cout << "Messages count: " << parsed["messages"].Size() << std::endl;
    } catch (const std::exception& e) {
        std::cout << "Parse error: " << e.what() << std::endl;
    }
}

void TestHttpClient() {
    std::cout << "\n=== Testing HTTP Client ===" << std::endl;

    HttpClient client;

    // Test a simple API call (httpbin.org echo)
    JsonValue testRequest = JsonValue::Object();
    testRequest["test"] = JsonValue("NovelStudio HTTP Test");
    testRequest["timestamp"] = JsonValue(1234567890);

    std::map<std::string, std::string> headers;
    headers["Content-Type"] = "application/json";
    headers["User-Agent"] = "NovelStudio/1.0";

    std::cout << "Sending test request to httpbin.org..." << std::endl;

    HttpResponse response = client.Post(
        "https://httpbin.org/post",
        testRequest.ToString(),
        headers
    );

    if (response.success) {
        std::cout << "✓ HTTP request succeeded!" << std::endl;
        std::cout << "Status code: " << response.statusCode << std::endl;
        std::cout << "Response length: " << response.body.length() << " bytes" << std::endl;

        try {
            JsonValue responseJson = JsonValue::Parse(response.body);
            if (responseJson.Has("url")) {
                std::cout << "Echo URL: " << responseJson["url"].AsString() << std::endl;
            }
        } catch (...) {
            std::cout << "Could not parse response as JSON" << std::endl;
        }
    } else {
        std::cout << "✗ HTTP request failed: " << response.error << std::endl;
    }
}

int main() {
    std::cout << "NovelStudio - Native Implementation Test" << std::endl;
    std::cout << "=========================================" << std::endl;

    auto& logger = Logger::GetInstance();
    logger.SetLevel(LogLevel::Debug);
    logger.EnableConsoleOutput(true);

    TestJsonParser();
    TestHttpClient();

    std::cout << "\n=== All Tests Complete ===" << std::endl;
    std::cout << "\nPress Enter to exit...";
    std::cin.get();

    return 0;
}
