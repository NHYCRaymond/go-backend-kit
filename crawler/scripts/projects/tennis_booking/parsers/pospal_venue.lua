-- pospal_venue 数据源解析脚本（Pospal预约系统通用场地）
local json = require("json")

function parse()
    -- 解析JSON响应
    local success, data = pcall(json.decode, response)
    if not success then
        log_error("Failed to decode JSON: " .. tostring(data))
        return false
    end
    
    -- Pospal API通常返回带有data字段的响应
    local pospal_data = data.data or data
    
    -- 获取场地ID，支持多种可能的字段名
    local venue_id = pospal_data.storeId or 
                     pospal_data.store_id or 
                     pospal_data.id or 
                     pospal_data.vn_id or
                     pospal_data.venueId or
                     pospal_data.venue_id
    
    -- 如果没有ID，生成一个
    if not venue_id or venue_id == "" then
        venue_id = "pospal_" .. os.time()
        log_info("Generated fallback venue_id: " .. venue_id)
    end
    venue_id = tostring(venue_id)
    
    -- 获取场地名称
    local venue_name = pospal_data.storeName or 
                       pospal_data.store_name or 
                       pospal_data.name or
                       pospal_data.venueName or
                       pospal_data.venue_name or
                       ("Pospal Venue " .. venue_id)
    
    -- 保存场地基本信息
    local venue_info = {
        _id = venue_id,
        venue_name = clean_string(venue_name),
        venue_type = "appointment_venue",
        address = clean_string(pospal_data.storeAddress or pospal_data.address or ""),
        phone = pospal_data.storePhone or pospal_data.phone or "",
        description = pospal_data.storeDescription or pospal_data.description or "",
        latitude = tonumber(pospal_data.latitude) or 0,
        longitude = tonumber(pospal_data.longitude) or 0,
        crawled_at = os.date("%Y-%m-%d %H:%M:%S"),
        task_id = TASK_ID or "",
        url = URL or "",
        project_id = PROJECT_ID or "tennis_booking"
    }
    
    -- 保存场地信息
    local venue_filter = {_id = venue_id}
    local venue_success = mongo_upsert("pospal_venues", venue_filter, venue_info)
    if not venue_success then
        log_error("Failed to save venue info")
    else
        log_info("Saved venue info for " .. venue_id)
    end
    
    -- 处理预约列表
    local appointment_count = 0
    local available_count = 0
    
    if pospal_data.appointmentList and type(pospal_data.appointmentList) == "table" then
        for _, apt in ipairs(pospal_data.appointmentList) do
            appointment_count = appointment_count + 1
            
            -- 生成唯一ID
            local apt_id = string.format("%s_%s_%s_%s",
                venue_id,
                (apt.appointmentDate or apt.date or os.date("%Y-%m-%d")):gsub("-", ""),
                tostring(apt.roomId or apt.room_id or "unknown"),
                (apt.startTime or apt.start_time or ""):gsub(":", "")
            )
            
            -- 容量和状态计算
            local capacity = tonumber(apt.capacity or apt.maxCapacity) or 0
            local booked = tonumber(apt.bookedCount or apt.booked_count or apt.currentBooked) or 0
            local remaining = capacity - booked
            local status = remaining > 0 and "available" or "booked"
            
            if status == "available" then
                available_count = available_count + 1
            end
            
            -- 保存预约时段数据
            local appointment_record = {
                _id = apt_id,
                venue_id = venue_id,
                venue_name = venue_name,
                
                -- 日期和时间
                date = parse_date(apt.appointmentDate or apt.date),
                start_time = normalize_time(apt.startTime or apt.start_time),
                end_time = normalize_time(apt.endTime or apt.end_time),
                
                -- 房间/场地信息
                room_id = tostring(apt.roomId or apt.room_id or ""),
                room_name = apt.roomName or apt.room_name or "",
                
                -- 容量和状态
                capacity = capacity,
                booked = booked,
                remaining = remaining,
                status = status,
                
                -- 价格
                price = parse_price(apt.price or apt.appointmentPrice),
                original_price = parse_price(apt.originalPrice),
                
                -- 额外信息
                teacher = apt.teacherName or apt.coach_name or "",
                class_type = apt.classType or apt.appointment_type or "",
                
                -- 元数据
                crawled_at = os.date("%Y-%m-%d %H:%M:%S"),
                task_id = TASK_ID or "",
                project_id = PROJECT_ID or "tennis_booking"
            }
            
            -- 使用 upsert 保存预约数据
            local apt_filter = {_id = apt_id}
            mongo_upsert("pospal_appointments", apt_filter, appointment_record)
        end
        
        log_info(string.format("Processed %d appointments for venue %s (%d available)",
            appointment_count, venue_id, available_count))
    
    elseif pospal_data.slots and type(pospal_data.slots) == "table" then
        -- 备用字段结构
        for _, slot in ipairs(pospal_data.slots) do
            appointment_count = appointment_count + 1
            
            -- 生成唯一ID
            local slot_id = string.format("%s_%s_%s_%s",
                venue_id,
                (slot.date or os.date("%Y-%m-%d")):gsub("-", ""),
                tostring(slot.venue_id or slot.room_id or "unknown"),
                (slot.start_time or ""):gsub(":", "")
            )
            
            -- 状态计算
            local capacity = tonumber(slot.capacity) or 0
            local booked = tonumber(slot.booked) or 0
            local status = (capacity - booked) > 0 and "available" or "booked"
            
            if status == "available" then
                available_count = available_count + 1
            end
            
            -- 保存时段数据
            local slot_record = {
                _id = slot_id,
                venue_id = venue_id,
                venue_name = venue_name,
                date = parse_date(slot.date),
                start_time = normalize_time(slot.start_time),
                end_time = normalize_time(slot.end_time),
                room_id = tostring(slot.venue_id or slot.room_id or ""),
                room_name = slot.venue_name or slot.room_name or "",
                capacity = capacity,
                booked = booked,
                status = status,
                price = parse_price(slot.price),
                crawled_at = os.date("%Y-%m-%d %H:%M:%S"),
                task_id = TASK_ID or "",
                project_id = PROJECT_ID or "tennis_booking"
            }
            
            local slot_filter = {_id = slot_id}
            mongo_upsert("pospal_appointments", slot_filter, slot_record)
        end
        
        log_info(string.format("Processed %d slots for venue %s (%d available)",
            appointment_count, venue_id, available_count))
    end
    
    -- 保存原始数据
    local raw_data = {
        _id = venue_id .. "_" .. os.time(),
        venue_id = venue_id,
        task_id = TASK_ID or "",
        raw_response = response,
        parsed_data = data,
        crawled_at = os.date("%Y-%m-%d %H:%M:%S"),
        url = URL or "",
        project_id = PROJECT_ID or "tennis_booking"
    }
    
    local raw_filter = {_id = raw_data._id}
    local raw_success = mongo_upsert("pospal_raw_data", raw_filter, raw_data)
    if not raw_success then
        log_error("Failed to save raw data")
    else
        log_info("Raw data saved")
    end
    
    -- 缓存场地统计信息
    if appointment_count > 0 then
        local cache_key = string.format("pospal:venue:%s:%s", venue_id, os.date("%Y%m%d"))
        local stats = {
            date = os.date("%Y-%m-%d"),
            venue_id = venue_id,
            venue_name = venue_name,
            total_appointments = appointment_count,
            available_appointments = available_count,
            utilization_rate = appointment_count > 0 and 
                math.floor((appointment_count - available_count) / appointment_count * 100) / 100 or 0
        }
        redis_set(cache_key, json.encode(stats), 3600)  -- 缓存1小时
        
        -- 记录高利用率
        if stats.utilization_rate > 0.8 then
            log_info(string.format("High utilization for venue %s: %.1f%%", 
                venue_id, stats.utilization_rate * 100))
        end
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