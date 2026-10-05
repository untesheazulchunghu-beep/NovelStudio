#include "json_utils.h"
#include <sstream>
#include <stdexcept>
#include <cstring>
#include <cctype>

namespace NovelStudio {

// Constructors
JsonValue::JsonValue(bool value) : type_(Type::Boolean), boolValue_(value) {}
JsonValue::JsonValue(int value) : type_(Type::Number), numberValue_(static_cast<double>(value)) {}
JsonValue::JsonValue(double value) : type_(Type::Number), numberValue_(value) {}
JsonValue::JsonValue(const std::string& value) : type_(Type::String), stringValue_(value) {}
JsonValue::JsonValue(const char* value) : type_(Type::String), stringValue_(value) {}

JsonValue JsonValue::Array() {
    JsonValue val;
    val.type_ = Type::Array;
    return val;
}

JsonValue JsonValue::Object() {
    JsonValue val;
    val.type_ = Type::Object;
    return val;
}

// Getters
bool JsonValue::AsBool() const {
    if (type_ != Type::Boolean) throw std::runtime_error("Not a boolean");
    return boolValue_;
}

double JsonValue::AsNumber() const {
    if (type_ != Type::Number) throw std::runtime_error("Not a number");
    return numberValue_;
}

std::string JsonValue::AsString() const {
    if (type_ != Type::String) throw std::runtime_error("Not a string");
    return stringValue_;
}

// Array operations
void JsonValue::Push(const JsonValue& value) {
    if (type_ != Type::Array) throw std::runtime_error("Not an array");
    arrayValue_.push_back(value);
}

JsonValue& JsonValue::operator[](size_t index) {
    if (type_ != Type::Array) throw std::runtime_error("Not an array");
    if (index >= arrayValue_.size()) arrayValue_.resize(index + 1);
    return arrayValue_[index];
}

const JsonValue& JsonValue::operator[](size_t index) const {
    if (type_ != Type::Array) throw std::runtime_error("Not an array");
    return arrayValue_[index];
}

size_t JsonValue::Size() const {
    if (type_ == Type::Array) return arrayValue_.size();
    if (type_ == Type::Object) return objectValue_.size();
    return 0;
}

// Object operations
void JsonValue::Set(const std::string& key, const JsonValue& value) {
    if (type_ != Type::Object) throw std::runtime_error("Not an object");
    objectValue_[key] = value;
}

JsonValue& JsonValue::operator[](const std::string& key) {
    if (type_ != Type::Object) throw std::runtime_error("Not an object");
    return objectValue_[key];
}

const JsonValue& JsonValue::operator[](const std::string& key) const {
    if (type_ != Type::Object) throw std::runtime_error("Not an object");
    auto it = objectValue_.find(key);
    if (it == objectValue_.end()) throw std::runtime_error("Key not found");
    return it->second;
}

bool JsonValue::Has(const std::string& key) const {
    if (type_ != Type::Object) return false;
    return objectValue_.find(key) != objectValue_.end();
}

// Serialization
std::string JsonValue::ToString(bool pretty, int indent) const {
    std::ostringstream oss;
    std::string indentStr(indent * 2, ' ');
    std::string nextIndentStr((indent + 1) * 2, ' ');

    switch (type_) {
        case Type::Null:
            oss << "null";
            break;
        case Type::Boolean:
            oss << (boolValue_ ? "true" : "false");
            break;
        case Type::Number:
            if (numberValue_ == static_cast<int>(numberValue_)) {
                oss << static_cast<int>(numberValue_);
            } else {
                oss << numberValue_;
            }
            break;
        case Type::String:
            oss << "\"";
            for (char c : stringValue_) {
                switch (c) {
                    case '\"': oss << "\\\""; break;
                    case '\\': oss << "\\\\"; break;
                    case '\n': oss << "\\n"; break;
                    case '\r': oss << "\\r"; break;
                    case '\t': oss << "\\t"; break;
                    default: oss << c; break;
                }
            }
            oss << "\"";
            break;
        case Type::Array:
            oss << "[";
            for (size_t i = 0; i < arrayValue_.size(); ++i) {
                if (pretty && i > 0) oss << ",\n" << nextIndentStr;
                else if (i > 0) oss << ",";
                oss << arrayValue_[i].ToString(pretty, indent + 1);
            }
            if (pretty && !arrayValue_.empty()) oss << "\n" << indentStr;
            oss << "]";
            break;
        case Type::Object:
            oss << "{";
            size_t count = 0;
            for (const auto& [key, value] : objectValue_) {
                if (count > 0) oss << ",";
                if (pretty) oss << "\n" << nextIndentStr;
                oss << "\"" << key << "\":";
                if (pretty) oss << " ";
                oss << value.ToString(pretty, indent + 1);
                ++count;
            }
            if (pretty && !objectValue_.empty()) oss << "\n" << indentStr;
            oss << "}";
            break;
    }
    return oss.str();
}

// Parsing helpers
void JsonValue::SkipWhitespace(const char*& ptr) {
    while (*ptr && std::isspace(*ptr)) ++ptr;
}

JsonValue JsonValue::ParseString(const char*& ptr) {
    ++ptr; // Skip opening quote
    std::string result;
    while (*ptr && *ptr != '\"') {
        if (*ptr == '\\') {
            ++ptr;
            switch (*ptr) {
                case '\"': result += '\"'; break;
                case '\\': result += '\\'; break;
                case 'n': result += '\n'; break;
                case 'r': result += '\r'; break;
                case 't': result += '\t'; break;
                default: result += *ptr; break;
            }
            ++ptr;
        } else {
            result += *ptr++;
        }
    }
    if (*ptr == '\"') ++ptr; // Skip closing quote
    return JsonValue(result);
}

JsonValue JsonValue::ParseNumber(const char*& ptr) {
    const char* start = ptr;
    if (*ptr == '-') ++ptr;
    while (*ptr && std::isdigit(*ptr)) ++ptr;
    if (*ptr == '.') {
        ++ptr;
        while (*ptr && std::isdigit(*ptr)) ++ptr;
    }
    std::string numStr(start, ptr - start);
    return JsonValue(std::stod(numStr));
}

JsonValue JsonValue::ParseArray(const char*& ptr) {
    ++ptr; // Skip '['
    JsonValue arr = JsonValue::Array();
    SkipWhitespace(ptr);

    if (*ptr == ']') {
        ++ptr;
        return arr;
    }

    while (*ptr) {
        arr.Push(ParseValue(ptr));
        SkipWhitespace(ptr);
        if (*ptr == ',') {
            ++ptr;
            SkipWhitespace(ptr);
        } else if (*ptr == ']') {
            ++ptr;
            break;
        }
    }
    return arr;
}

JsonValue JsonValue::ParseObject(const char*& ptr) {
    ++ptr; // Skip '{'
    JsonValue obj = JsonValue::Object();
    SkipWhitespace(ptr);

    if (*ptr == '}') {
        ++ptr;
        return obj;
    }

    while (*ptr) {
        SkipWhitespace(ptr);
        if (*ptr != '\"') break;

        std::string key = ParseString(ptr).AsString();
        SkipWhitespace(ptr);
        if (*ptr == ':') ++ptr;
        SkipWhitespace(ptr);

        obj.Set(key, ParseValue(ptr));
        SkipWhitespace(ptr);

        if (*ptr == ',') {
            ++ptr;
            SkipWhitespace(ptr);
        } else if (*ptr == '}') {
            ++ptr;
            break;
        }
    }
    return obj;
}

JsonValue JsonValue::ParseValue(const char*& ptr) {
    SkipWhitespace(ptr);

    if (*ptr == '\"') return ParseString(ptr);
    if (*ptr == '{') return ParseObject(ptr);
    if (*ptr == '[') return ParseArray(ptr);
    if (*ptr == 't') { ptr += 4; return JsonValue(true); }
    if (*ptr == 'f') { ptr += 5; return JsonValue(false); }
    if (*ptr == 'n') { ptr += 4; return JsonValue(); }
    if (*ptr == '-' || std::isdigit(*ptr)) return ParseNumber(ptr);

    return JsonValue();
}

JsonValue JsonValue::Parse(const std::string& json) {
    const char* ptr = json.c_str();
    return ParseValue(ptr);
}

} // namespace NovelStudio
