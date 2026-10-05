#pragma once

#define CROW_MAIN
#define CROW_ENABLE_SSL
#define CROW_USE_BOOST
#include "crow/crow_all.h"

// Response helper
struct ApiResponse {
    bool success;
    crow::json::wvalue data;
    std::string error;

    crow::json::wvalue to_json() const {
        crow::json::wvalue json;
        json["success"] = success;
        json["data"] = data;
        json["error"] = error;
        return json;
    }

    static crow::response ok(const crow::json::wvalue& data) {
        ApiResponse resp;
        resp.success = true;
        resp.data = data;
        return crow::response(200, resp.to_json());
    }

    static crow::response error(int code, const std::string& message) {
        ApiResponse resp;
        resp.success = false;
        resp.error = message;
        return crow::response(code, resp.to_json());
    }
};

// CORS middleware
struct CORSMiddleware {
    struct context {};

    void before_handle(crow::request& req, crow::response& res, context& ctx) {
        // Will be called before handler
    }

    void after_handle(crow::request& req, crow::response& res, context& ctx) {
        res.add_header("Access-Control-Allow-Origin", "*");
        res.add_header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS");
        res.add_header("Access-Control-Allow-Headers", "Content-Type, Authorization");
        res.add_header("Access-Control-Max-Age", "3600");
    }
};

// Logger middleware
struct LoggerMiddleware {
    struct context {};

    void before_handle(crow::request& req, crow::response& res, context& ctx) {
        std::cout << "[" << crow::utility::datetime_now() << "] "
                  << req.method << " " << req.url << std::endl;
    }

    void after_handle(crow::request& req, crow::response& res, context& ctx) {
        // Log response
    }
};
