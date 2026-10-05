# SQLite3 Amalgamation Integration

Since vcpkg is not available in the current environment, we'll use SQLite3's amalgamation (single-file distribution) which is the official way to embed SQLite.

## Download SQLite3 Amalgamation

1. Download from: https://www.sqlite.org/download.html
2. Get "sqlite-amalgamation-XXXXXXX.zip"
3. Extract `sqlite3.c` and `sqlite3.h`

## Integration Steps

```bash
# Create vendor directory
mkdir -p vendor/sqlite3

# Place sqlite3.c and sqlite3.h in vendor/sqlite3/
# These are ~250KB combined (much smaller than vcpkg)
```

## CMake Integration

```cmake
# Add to CMakeLists.txt
add_library(sqlite3 STATIC vendor/sqlite3/sqlite3.c)
target_include_directories(sqlite3 PUBLIC vendor/sqlite3)
```

## Benefits

- ✅ No vcpkg dependency
- ✅ Single file integration
- ✅ Official SQLite distribution method
- ✅ ~250KB vs ~4MB vcpkg package
- ✅ Easier to version control
- ✅ Faster builds

## Alternative: Download Script

Create `setup-sqlite3.bat`:
```batch
@echo off
echo Downloading SQLite3 amalgamation...
curl -L https://www.sqlite.org/2024/sqlite-amalgamation-3460100.zip -o sqlite3.zip
tar -xf sqlite3.zip
mkdir vendor\sqlite3
move sqlite-amalgamation-3460100\sqlite3.* vendor\sqlite3\
rmdir sqlite-amalgamation-3460100
del sqlite3.zip
echo SQLite3 ready in vendor/sqlite3/
```

## Status

For now, we'll continue developing without SQLite3 and demonstrate the architecture.
The database layer is already designed and ready - it just needs SQLite3 linked.
