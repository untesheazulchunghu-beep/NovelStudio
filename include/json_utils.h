#pragma once

#include <string>
#include <vector>
#include <map>
#include <variant>
#include <memory>

namespace NovelStudio {

// Lightweight JSON implementation - no external dependencies
class JsonValue {
public:
    enum class Type {
        Null,
        Boolean,
        Number,
        String,
        Array,
        Object
    };

    JsonValue() : type_(Type::Null) {}
    JsonValue(bool value);
    JsonValue(int value);
    JsonValue(double value);
    JsonValue(const std::string& value);
    JsonValue(const char* value);

    // Factory methods
    static JsonValue Array();
    static JsonValue Object();

    // Type checking
    Type GetType() const { return type_; }
    bool IsNull() const { return type_ == Type::Null; }
    bool IsBool() const { return type_ == Type::Boolean; }
    bool IsNumber() const { return type_ == Type::Number; }
    bool IsString() const { return type_ == Type::String; }
    bool IsArray() const { return type_ == Type::Array; }
    bool IsObject() const { return type_ == Type::Object; }

    // Getters
    bool AsBool() const;
    double AsNumber() const;
    std::string AsString() const;

    // Array operations
    void Push(const JsonValue& value);
    JsonValue& operator[](size_t index);
    const JsonValue& operator[](size_t index) const;
    size_t Size() const;

    // Object operations
    void Set(const std::string& key, const JsonValue& value);
    JsonValue& operator[](const std::string& key);
    const JsonValue& operator[](const std::string& key) const;
    bool Has(const std::string& key) const;

    // Serialization
    std::string ToString(bool pretty = false, int indent = 0) const;

    // Parsing
    static JsonValue Parse(const std::string& json);

private:
    Type type_;
    bool boolValue_ = false;
    double numberValue_ = 0.0;
    std::string stringValue_;
    std::vector<JsonValue> arrayValue_;
    std::map<std::string, JsonValue> objectValue_;

    static JsonValue ParseValue(const char*& ptr);
    static JsonValue ParseObject(const char*& ptr);
    static JsonValue ParseArray(const char*& ptr);
    static JsonValue ParseString(const char*& ptr);
    static JsonValue ParseNumber(const char*& ptr);
    static void SkipWhitespace(const char*& ptr);
};

} // namespace NovelStudio
