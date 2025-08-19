// 配置Lua脚本任务
// 运行: mongosh crawler < configure_lua_tasks.js

db = db.getSiblingDB('crawler');

print("=" + "=".repeat(80));
print("配置 Lua 脚本任务");
print("=" + "=".repeat(80) + "\n");

// 定义要更新的任务
const tasks = [
    {
        name: "drip_ground_board",
        lua_script: "drip_ground_board.lua",
        description: "Drip Ground Board 场地预订数据"
    },
    {
        name: "Venue Massage Site Data",
        lua_script: "venue_massage_site.lua",
        description: "Venue Massage 网球场数据"
    },
    {
        name: "Pospal Venue Class Room Appointment",
        lua_script: "pospal_venue.lua",
        description: "Pospal 场地预约数据"
    },
    {
        name: "Pospal Venue Appointment Store 5662377",
        lua_script: "pospal_store_5662377.lua",
        description: "Pospal 特定店铺(5662377)数据"
    }
];

// 批量更新任务配置
print("更新任务配置...\n");

tasks.forEach(task => {
    // 先确保metadata字段存在
    db.crawler_tasks.updateOne(
        { name: task.name, metadata: null },
        { $set: { metadata: {} } }
    );
    
    const result = db.crawler_tasks.updateOne(
        { name: task.name },
        { 
            $set: { 
                project_id: "tennis_booking",
                lua_script: task.lua_script,
                description: task.description,
                "metadata.lua_enabled": true,
                "metadata.dual_storage": true,  // 标记使用双层存储
                updated_at: new Date()
            }
        }
    );
    
    if (result.matchedCount > 0) {
        print(`✅ ${task.name}`);
        print(`   - Lua脚本: ${task.lua_script}`);
        print(`   - 描述: ${task.description}`);
        print(`   - 更新成功: ${result.modifiedCount > 0 ? '是' : '否(无变化)'}`);
    } else {
        print(`❌ ${task.name} - 未找到任务`);
    }
    print("");
});

// 验证配置
print("\n" + "=" + "=".repeat(80));
print("验证配置结果");
print("=" + "=".repeat(80) + "\n");

const configuredTasks = db.crawler_tasks.find({
    project_id: "tennis_booking",
    lua_script: { $exists: true, $ne: "" }
}).toArray();

print(`已配置 Lua 脚本的任务数: ${configuredTasks.length}\n`);

configuredTasks.forEach(task => {
    print(`任务: ${task.name}`);
    print(`  - Project ID: ${task.project_id}`);
    print(`  - Lua Script: ${task.lua_script}`);
    print(`  - 启用状态: ${task.status.enabled ? '✅ 启用' : '❌ 禁用'}`);
    print(`  - 上次运行: ${task.status.last_run || 'N/A'}`);
    print("");
});

// 显示存储配置
print("\n" + "=" + "=".repeat(80));
print("存储架构说明");
print("=" + "=".repeat(80) + "\n");

print("双层存储架构:");
print("");
print("1. 原始数据存储 (保留完整响应):");
print("   - drip_ground_board → court_booking_boards");
print("   - venue_massage_site → venue_tennis_courts");
print("   - pospal_venue → pospal_venue_slots");
print("   - pospal_store_5662377 → pospal_venue_slots_5662377");
print("");
print("2. 统一格式存储 (标准化数据):");
print("   - 所有数据源 → unified_venues");
print("");
print("优势:");
print("  ✅ 原始数据完整保留，便于调试和回溯");
print("  ✅ 统一格式便于跨数据源查询和分析");
print("  ✅ 通过 project_id + source + source_id 关联两层数据");

// 创建索引（如果需要）
print("\n" + "=" + "=".repeat(80));
print("创建索引");
print("=" + "=".repeat(80) + "\n");

// 为统一数据集合创建索引
const indexesToCreate = [
    { collection: "unified_venues", index: { project_id: 1, source: 1, source_id: 1 }, options: { unique: true } },
    { collection: "unified_venues", index: { "venue.district": 1 } },
    { collection: "unified_venues", index: { "venue.city": 1 } },
    { collection: "unified_venues", index: { "slots.date": 1 } },
    { collection: "unified_venues", index: { "metadata.crawled_at": -1 } },
    { collection: "unified_venues", index: { "statistics.available_slots": 1 } }
];

indexesToCreate.forEach(idx => {
    try {
        db[idx.collection].createIndex(idx.index, idx.options || {});
        print(`✅ 创建索引: ${idx.collection} - ${JSON.stringify(idx.index)}`);
    } catch (e) {
        if (e.codeName === 'IndexOptionsConflict' || e.code === 85) {
            print(`⚠️ 索引已存在: ${idx.collection} - ${JSON.stringify(idx.index)}`);
        } else {
            print(`❌ 创建索引失败: ${idx.collection} - ${e.message}`);
        }
    }
});

print("\n✅ Lua 脚本任务配置完成！");
print("\n使用说明:");
print("1. 任务执行时会自动调用对应的 Lua 脚本");
print("2. 原始数据保存在各自的集合中");
print("3. 统一格式数据保存在 unified_venues 集合中");
print("4. 可通过 project_id='tennis_booking' 查询所有相关数据");