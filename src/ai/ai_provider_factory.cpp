#include "ai_providers.h"
#include <memory>

// Factory implementations for extended providers
std::unique_ptr<NovelStudio::AIProvider> CreateAnthropicProviderImpl() {
    return std::make_unique<NovelStudio::AnthropicProvider>();
}

std::unique_ptr<NovelStudio::AIProvider> CreateLocalProviderImpl() {
    return std::make_unique<NovelStudio::LocalProvider>();
}
