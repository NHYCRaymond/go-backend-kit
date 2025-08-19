-- 网球场预订项目配置
-- 定义统一的数据结构

-- 存储配置
-- 原始数据集合映射
RAW_COLLECTIONS = {
    drip_ground_board = "court_booking_boards",
    venue_massage_site = "venue_tennis_courts", 
    pospal_venue = "pospal_venue_slots",
    pospal_store_5662377 = "pospal_venue_slots_5662377"
}

-- 统一数据集合名
UNIFIED_COLLECTION = "unified_venues"

-- 创建统一记录结构
function create_unified_record()
    -- 获取当前时间
    local now = os.date("!%Y-%m-%dT%H:%M:%SZ")
    
    return {
        -- 项目和数据源标识
        project_id = PROJECT_ID or "tennis_booking",     -- 项目ID（从全局变量获取）
        source = "",                                     -- 数据源标识（将从脚本名提取）
        source_id = "",                                  -- 原始数据ID
        
        -- 场地基本信息
        venue = {
            id = "",                      -- 场地ID
            name = "",                    -- 场地名称
            type = "",                    -- 场地类型 (tennis/badminton/basketball/etc)
            address = "",                 -- 地址
            district = "",                -- 区域
            city = "",                    -- 城市
            coordinates = {               -- 坐标
                lat = 0,
                lng = 0
            },
            contact = "",                 -- 联系方式
            description = ""              -- 描述
        },
        
        -- 时间槽信息（核心数据）
        slots = {},  -- 数组，每个元素包含: {date, start_time, end_time, court_no, status, price, extra}
        
        -- 价格信息汇总
        pricing = {
            currency = "CNY",             -- 货币
            min_price = 0,                -- 最低价
            max_price = 0,                -- 最高价
            avg_price = 0,                -- 平均价
            peak_price = 0,               -- 高峰价
            off_peak_price = 0            -- 非高峰价
        },
        
        -- 统计信息
        statistics = {
            total_slots = 0,              -- 总时间槽数
            available_slots = 0,          -- 可用时间槽数
            booked_slots = 0,             -- 已预订时间槽数
            utilization_rate = 0          -- 利用率
        },
        
        -- 元数据
        metadata = {
            crawled_at = now,             -- 爬取时间
            data_date = os.date("%Y-%m-%d"),  -- 数据日期
            validity_period = 86400,      -- 有效期（秒）
            confidence_score = 1.0,       -- 数据置信度 (0-1)
            version = "1.0",              -- 数据版本
            raw_count = 0                 -- 原始数据条数
        },
        
        -- 原始数据（可选，用于调试）
        raw_data = nil
    }
end

-- 时间槽结构
function create_time_slot()
    return {
        date = "",                        -- 日期 (YYYY-MM-DD)
        start_time = "",                  -- 开始时间 (HH:MM)
        end_time = "",                    -- 结束时间 (HH:MM)
        court_no = "",                    -- 场地编号
        court_name = "",                  -- 场地名称（可选）
        status = "",                      -- 状态 (available/booked/maintenance)
        price = 0,                        -- 价格
        original_price = 0,               -- 原价（如有折扣）
        discount = 0,                     -- 折扣率
        capacity = 0,                     -- 容量（如适用）
        booked = 0,                       -- 已预订数量
        extra = {}                        -- 额外信息（各数据源特有字段）
    }
end

-- 设置数据源信息
function set_source_info(record, source_name)
    -- 从脚本名提取数据源
    if SCRIPT_NAME then
        record.source = SCRIPT_NAME:gsub("%.lua$", "")
    elseif source_name then
        record.source = source_name
    end
end

-- 索引定义（供MongoDB使用）
INDEXES = {
    {"project_id", 1},
    {"source", 1},
    {"source_id", 1},
    {"venue.district", 1},
    {"venue.city", 1},
    {"slots.date", 1},
    {"slots.status", 1},
    {"metadata.crawled_at", -1},
    {{"project_id", 1}, {"source", 1}, {"source_id", 1}},  -- 复合唯一索引
}

-- 数据质量要求
QUALITY_REQUIREMENTS = {
    min_fields = 5,                      -- 最少必填字段数
    min_slots = 1,                       -- 最少时间槽数
    required_fields = {                  -- 必填字段
        "project_id",
        "source",
        "source_id",
        "venue.name"
    }
}

-- 日志前缀
function log_prefix()
    return "[" .. (PROJECT_ID or "unknown") .. "/" .. (SCRIPT_NAME or "unknown") .. "] "
end