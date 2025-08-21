# Storage Architecture

## Overview

The crawler system implements a flexible storage architecture that supports multiple storage backends through different mechanisms.

## Current Storage Implementation

### 1. Direct Storage via Lua Scripts

The primary storage mechanism is through Lua scripts that directly write to MongoDB:

```lua
-- Example: Saving crawled data to MongoDB
mongo_save("collection_name", {
    url = url,
    data = extracted_data,
    crawled_at = os.time()
})
```

**Location**: `/crawler/parser/lua_engine.go`

Available Lua functions:
- `mongo_save(collection, data)` - Save single document
- `mongo_save_batch(collection, data_array)` - Save multiple documents
- `mongo_upsert(collection, query, data)` - Upsert document
- `redis_set(key, value, ttl)` - Cache data in Redis
- `redis_get(key)` - Retrieve cached data

### 2. Storage Interface (crawler.go)

The Storage interface in `crawler.go` is designed for crawler metadata and state management, NOT for actual crawled data storage:

```go
type Storage interface {
    SaveTask(task *Task) error
    GetTask(id string) (*Task, error)
    SaveResult(result *Result) error
    GetResult(id string) (*Result, error)
}
```

**Current implementations**:
- ✅ `RedisStorage` - Implemented for task/result caching
- ❌ `MongoDBStorage` - TODO (not critical, as data storage is handled by Lua)
- ❌ `MySQLStorage` - TODO (not critical, as data storage is handled by Lua)

## Why This Architecture?

1. **Flexibility**: Lua scripts can implement complex, project-specific storage logic
2. **Performance**: Direct database writes from Lua avoid Go marshaling overhead
3. **Customization**: Each project can define its own storage schema and logic
4. **Separation of Concerns**: 
   - Crawler framework manages tasks and coordination
   - Lua scripts handle business logic and data persistence

## Storage Flow

```
Crawler Node → Executes Task → Fetches Data
                ↓
        Lua Script Parser
                ↓
    ┌───────────┴───────────┐
    │                       │
Direct MongoDB Write    Redis Cache
(via mongo_save)       (via redis_set)
```

## Should We Implement MongoDB/MySQL Storage Interfaces?

**Not necessary for crawled data** because:
1. Lua scripts already handle data storage effectively
2. Adding another storage layer would create redundancy
3. Project-specific storage requirements are better handled in Lua

**Could be useful for** crawler metadata like:
- Task history
- Crawler statistics
- System metrics
- Audit logs

## Recommendations

1. **Keep Lua-based storage** for crawled data
2. **Remove or clarify TODOs** in crawler.go to avoid confusion
3. **Consider implementing** MongoDB/MySQL storage only for system metadata if needed
4. **Document clearly** that data storage is handled by Lua scripts

## Example Project Storage Pattern

```lua
-- projects/e-commerce/parsers/product.lua

function parse_product(response)
    local product = extract_product_data(response)
    
    -- Direct storage with project-specific schema
    mongo_save("products", {
        sku = product.sku,
        name = product.name,
        price = product.price,
        inventory = product.inventory,
        updated_at = os.time()
    })
    
    -- Cache for deduplication
    redis_set("product:" .. product.sku, "1", 3600)
end
```

This architecture provides maximum flexibility while keeping the core crawler framework generic and reusable.