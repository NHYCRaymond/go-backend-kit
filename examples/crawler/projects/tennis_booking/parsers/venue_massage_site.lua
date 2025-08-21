-- venue_massage_site 数据源解析脚本
-- 解析网球场地预订时间段数据
local json = require("json")

function parse()
    -- 解析JSON响应
    local success, data = pcall(json.decode, response)
    if not success then
        log_error("Failed to decode JSON: " .. tostring(data))
        return false
    end
    
    -- 检查必要字段
    if not data or not data.vn_id then
        log_error("Invalid data structure: missing vn_id")
        return false
    end
    
    local venue_id = tostring(data.vn_id)
    
    -- 从任务元数据获取请求日期（重要：API不会在响应中返回请求的日期）
    local request_date = nil
    if TASK_METADATA and TASK_METADATA.date then
        request_date = TASK_METADATA.date
        log_info("Request date from task metadata: " .. request_date)
    elseif REQUEST_BODY then
        -- 尝试从请求体中解析date参数
        local date_match = string.match(REQUEST_BODY, "date=([%d%-]+)")
        if date_match then
            request_date = date_match
            log_info("Request date from request body: " .. request_date)
        end
    end
    
    -- 如果没有找到日期，使用今天的日期（初始请求的情况）
    local base_date = request_date or os.date("%Y-%m-%d")
    
    -- 记录请求参数
    log_info("Processing venue " .. venue_id .. " for date: " .. base_date)
    
    -- 初始请求处理：生成子任务（不带date参数的请求）
    if not request_date and data.week and type(data.week) == "table" and #data.week > 0 then
        log_info("Initial request detected - generating subtasks for " .. #data.week .. " days")
        
        -- 为每个日期创建子任务（跳过第一天，因为初始请求已包含今天的数据）
        for i = 2, #data.week do
            local day_info = data.week[i]
            if day_info.time then
                -- 简化的请求体：只需要必要参数，不需要week
                local task_body = string.format(
                    "vid=%s&pid=18&date=%s&openid=o_PUd7UvwOKaxG8AHYDo6X962bvg",
                    venue_id,
                    day_info.time
                )
                
                -- 创建子任务
                local subtask = {
                    id = string.format("venue_%s_%s", venue_id, day_info.time:gsub("-", "")),
                    parent_id = TASK_ID,
                    name = string.format("Venue %s - %s", venue_id, day_info.time),
                    url = "https://wx.dududing.com/VenueMassige/sitedata",
                    method = "POST",
                    headers = {
                        ["Content-Type"] = "application/x-www-form-urlencoded",
                        ["User-Agent"] = "Mozilla/5.0 (iPhone; CPU iPhone OS 14_7_1 like Mac OS X)"
                    },
                    body = task_body,
                    type = "detail",
                    priority = 5,
                    max_retries = 3,
                    timeout = 30,
                    project_id = PROJECT_ID or "tennis_booking",
                    lua_script = "venue_massage_site.lua",
                    metadata = {
                        venue_id = venue_id,
                        date = day_info.time
                    }
                }
                
                create_task(subtask)
                log_info("Created subtask for " .. day_info.time)
            end
        end
    end
    
    -- 处理场地时间段数据
    if data.site_data and type(data.site_data) == "table" then
        local saved_count = 0
        local available_count = 0
        
        for time_id, slots in pairs(data.site_data) do
            if type(slots) == "table" then
                for _, slot in ipairs(slots) do
                    -- 只处理网球场地（过滤掉陪打和发球机）
                    if string.find(slot.s_name or "", "网球%d+号") then
                        
                        -- 保存场地详情
                        local court_detail = {
                            _id = string.format("%s_%s_%s", venue_id, slot.sid, base_date:gsub("-", "")),
                            venue_id = venue_id,
                            court_id = slot.sid,
                            court_name = slot.s_name,
                            date = base_date,
                            crawled_at = os.date("%Y-%m-%d %H:%M:%S"),
                            project_id = PROJECT_ID or "tennis_booking"
                        }
                        
                        mongo_upsert("venue_court_details", {_id = court_detail._id}, court_detail)
                        
                        -- 保存时间段信息
                        local period_id = string.format("%s_%s_%s_%s",
                            venue_id,
                            base_date:gsub("-", ""),
                            slot.sid,
                            (slot.times or ""):gsub(":", "")
                        )
                        
                        -- 判断预订状态
                        local is_ordered = tonumber(slot.is_ordered) or 0
                        local status_name = is_ordered == 0 and "available" or "booked"
                        if is_ordered == 0 then
                            available_count = available_count + 1
                        end
                        
                        local period_record = {
                            _id = period_id,
                            venue_id = venue_id,
                            court_id = slot.sid,
                            court_name = slot.s_name,
                            date = base_date,
                            
                            -- 时间信息
                            start_time = slot.times,
                            time_range = slot.time_road,
                            
                            -- 状态
                            is_ordered = is_ordered,
                            status_name = status_name,
                            
                            -- 价格
                            price = tonumber(slot.p) or 0,
                            
                            -- 支付信息
                            payment_type = slot.paytype,
                            
                            -- 元数据
                            crawled_at = os.date("%Y-%m-%d %H:%M:%S"),
                            project_id = PROJECT_ID or "tennis_booking"
                        }
                        
                        mongo_upsert("venue_court_periods", {_id = period_id}, period_record)
                        saved_count = saved_count + 1
                    end
                end
            end
        end
        
        log_info(string.format("Saved %d time slots for venue %s on %s (%d available)",
            saved_count, venue_id, base_date, available_count))
        
        -- 缓存统计信息到Redis
        local stats_key = string.format("venue:stats:%s:%s", venue_id, base_date:gsub("-", ""))
        local stats = {
            date = base_date,
            total_slots = saved_count,
            available_slots = available_count,
            booked_slots = saved_count - available_count,
            utilization_rate = saved_count > 0 and 
                math.floor((saved_count - available_count) / saved_count * 100) or 0
        }
        redis_set(stats_key, json.encode(stats), 3600)  -- 缓存1小时
    else
        log_info("No site_data field found in response")
    end
    
    return true
end

-- 执行解析
local success, err = pcall(parse)
if not success then
    error_with_prefix("Parse failed: " .. tostring(err))
    return false
end

return true