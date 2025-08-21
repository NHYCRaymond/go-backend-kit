-- 网球场预订项目公共函数库

-- 标准化时间格式 (转换为 HH:MM 格式)
function normalize_time(time_str)
    if not time_str or time_str == "" then
        return ""
    end
    
    -- 处理已经是标准格式的时间
    if string.match(time_str, "^%d%d:%d%d$") then
        return time_str
    end
    
    -- 提取小时和分钟
    local hour, minute = string.match(time_str, "(%d+):(%d+)")
    
    if hour and minute then
        hour = tonumber(hour)
        minute = tonumber(minute)
        
        -- 处理上午/下午标识
        if string.find(time_str, "下午") or string.find(time_str, "PM") or string.find(time_str, "pm") then
            if hour < 12 then
                hour = hour + 12
            end
        elseif string.find(time_str, "上午") or string.find(time_str, "AM") or string.find(time_str, "am") then
            if hour == 12 then
                hour = 0
            end
        end
        
        return string.format("%02d:%02d", hour, minute)
    end
    
    -- 处理纯数字格式 (如 "0900" -> "09:00")
    local pure_number = string.match(time_str, "^(%d%d%d%d)$")
    if pure_number then
        local h = string.sub(pure_number, 1, 2)
        local m = string.sub(pure_number, 3, 4)
        return h .. ":" .. m
    end
    
    return time_str
end

-- 解析价格（提取数字）
function parse_price(price_str)
    if not price_str then
        return 0
    end
    
    -- 如果已经是数字
    if type(price_str) == "number" then
        return price_str
    end
    
    -- 转换为字符串
    price_str = tostring(price_str)
    
    -- 提取数字（支持小数）
    local num = string.match(price_str, "(%d+%.?%d*)")
    if num then
        return tonumber(num) or 0
    end
    
    return 0
end

-- 从地址提取区域
function extract_district(address)
    if not address or address == "" then
        return ""
    end
    
    -- 常见区域模式
    local districts = {
        -- 北京
        "朝阳区", "海淀区", "东城区", "西城区", "丰台区", "石景山区", 
        "通州区", "顺义区", "房山区", "大兴区", "昌平区", "怀柔区",
        -- 上海
        "浦东新区", "黄浦区", "静安区", "徐汇区", "长宁区", "普陀区",
        "虹口区", "杨浦区", "闵行区", "宝山区", "嘉定区", "松江区",
        -- 广州
        "天河区", "越秀区", "荔湾区", "海珠区", "番禺区", "白云区",
        -- 深圳
        "福田区", "罗湖区", "南山区", "宝安区", "龙岗区", "盐田区"
    }
    
    for _, district in ipairs(districts) do
        if string.find(address, district) then
            return district
        end
    end
    
    -- 尝试提取"XX区"格式
    local district = string.match(address, "([^%s]+区)")
    if district then
        return district
    end
    
    return ""
end

-- 从地址提取城市
function extract_city(address)
    if not address or address == "" then
        return ""
    end
    
    -- 直辖市和主要城市
    local cities = {
        "北京", "上海", "天津", "重庆",
        "广州", "深圳", "杭州", "南京", "武汉", "成都",
        "西安", "苏州", "郑州", "济南", "青岛", "大连"
    }
    
    for _, city in ipairs(cities) do
        if string.find(address, city) then
            return city
        end
    end
    
    -- 尝试提取"XX市"格式
    local city = string.match(address, "([^%s]+市)")
    if city then
        return city
    end
    
    return ""
end

-- 计算价格信息
function calculate_pricing(slots)
    if not slots or #slots == 0 then
        return {
            currency = "CNY",
            min_price = 0,
            max_price = 0,
            avg_price = 0,
            peak_price = 0,
            off_peak_price = 0
        }
    end
    
    local prices = {}
    local peak_prices = {}      -- 高峰时段价格 (18:00-21:00)
    local off_peak_prices = {}   -- 非高峰时段价格
    
    for _, slot in ipairs(slots) do
        if slot.price and slot.price > 0 then
            table.insert(prices, slot.price)
            
            -- 判断是否为高峰时段
            local hour = tonumber(string.sub(slot.start_time or "", 1, 2))
            if hour and hour >= 18 and hour <= 21 then
                table.insert(peak_prices, slot.price)
            else
                table.insert(off_peak_prices, slot.price)
            end
        end
    end
    
    if #prices == 0 then
        return {
            currency = "CNY",
            min_price = 0,
            max_price = 0,
            avg_price = 0,
            peak_price = 0,
            off_peak_price = 0
        }
    end
    
    -- 排序价格
    table.sort(prices)
    
    return {
        currency = "CNY",
        min_price = prices[1],
        max_price = prices[#prices],
        avg_price = calculate_average(prices),
        peak_price = #peak_prices > 0 and calculate_average(peak_prices) or 0,
        off_peak_price = #off_peak_prices > 0 and calculate_average(off_peak_prices) or 0
    }
end

-- 计算统计信息
function calculate_statistics(slots)
    if not slots then
        return {
            total_slots = 0,
            available_slots = 0,
            booked_slots = 0,
            utilization_rate = 0
        }
    end
    
    local total = #slots
    local available = 0
    local booked = 0
    
    for _, slot in ipairs(slots) do
        if slot.status == "available" then
            available = available + 1
        elseif slot.status == "booked" then
            booked = booked + 1
        end
    end
    
    local utilization = total > 0 and (booked / total) or 0
    
    return {
        total_slots = total,
        available_slots = available,
        booked_slots = booked,
        utilization_rate = math.floor(utilization * 100) / 100
    }
end

-- 计算平均值
function calculate_average(array)
    if not array or #array == 0 then
        return 0
    end
    
    local sum = 0
    for _, v in ipairs(array) do
        sum = sum + v
    end
    
    return math.floor(sum / #array * 100) / 100  -- 保留两位小数
end

-- 数组求和
function sum_array(array)
    local total = 0
    for _, v in ipairs(array) do
        total = total + (tonumber(v) or 0)
    end
    return total
end

-- 统计符合条件的元素
function count_if(array, predicate)
    local count = 0
    for _, item in ipairs(array) do
        if predicate(item) then
            count = count + 1
        end
    end
    return count
end

-- 数组映射
function map_array(array, fn)
    local result = {}
    for i, v in ipairs(array or {}) do
        result[i] = fn(v, i)
    end
    return result
end

-- 数组过滤
function filter_array(array, fn)
    local result = {}
    for _, v in ipairs(array or {}) do
        if fn(v) then
            table.insert(result, v)
        end
    end
    return result
end

-- 安全获取嵌套值
function safe_get(data, path, default)
    if not data then
        return default
    end
    
    local current = data
    for key in string.gmatch(path, "[^%.]+") do
        if type(current) == "table" and current[key] ~= nil then
            current = current[key]
        else
            return default
        end
    end
    
    return current
end

-- 验证必填字段
function validate_required_fields(record, required_fields)
    local missing = {}
    
    for _, field in ipairs(required_fields) do
        local value = safe_get(record, field, nil)
        if value == nil or value == "" then
            table.insert(missing, field)
        end
    end
    
    return #missing == 0, missing
end

-- 清理字符串（去除空格和特殊字符）
function clean_string(str)
    if not str then
        return ""
    end
    
    -- 去除首尾空格
    str = string.gsub(str, "^%s+", "")
    str = string.gsub(str, "%s+$", "")
    
    -- 替换多个空格为单个空格
    str = string.gsub(str, "%s+", " ")
    
    return str
end

-- 解析日期（确保格式为 YYYY-MM-DD）
function parse_date(date_str)
    if not date_str then
        return os.date("%Y-%m-%d")
    end
    
    -- 已经是正确格式
    if string.match(date_str, "^%d%d%d%d%-%d%d%-%d%d$") then
        return date_str
    end
    
    -- 尝试其他格式
    -- 2024/03/15 -> 2024-03-15
    local y, m, d = string.match(date_str, "(%d%d%d%d)/(%d%d)/(%d%d)")
    if y and m and d then
        return y .. "-" .. m .. "-" .. d
    end
    
    -- 20240315 -> 2024-03-15
    y, m, d = string.match(date_str, "(%d%d%d%d)(%d%d)(%d%d)")
    if y and m and d then
        return y .. "-" .. m .. "-" .. d
    end
    
    -- 默认返回今天
    return os.date("%Y-%m-%d")
end

-- 生成缓存键
function generate_cache_key(project_id, source, source_id)
    return string.format("%s:%s:%s", project_id, source, source_id)
end

-- 日志辅助函数
function log_with_prefix(message)
    local prefix = log_prefix and log_prefix() or ""
    log_info(prefix .. message)
end

function error_with_prefix(message)
    local prefix = log_prefix and log_prefix() or ""
    log_error(prefix .. message)
end