-- pospal_store_5662377 数据源解析脚本（特定店铺）
local json = require("json")

function parse()
    -- 解析JSON响应
    local success, data = pcall(json.decode, response)
    if not success then
        log_error("Failed to decode JSON: " .. tostring(data))
        return false
    end
    
    -- Pospal API响应结构
    local pospal_data = data.data or data
    
    -- 获取基本信息
    local store_id = "5662377"  -- 固定的店铺ID
    local venue_name = clean_string(pospal_data.storeName or "Pospal Venue Store 5662377")
    
    -- 保存店铺基本信息
    local store_info = {
        _id = store_id,
        store_name = venue_name,
        store_type = "appointment_venue",
        address = clean_string(pospal_data.storeAddress or pospal_data.address or ""),
        phone = pospal_data.storePhone or pospal_data.contact or "",
        description = pospal_data.storeInfo or pospal_data.description or "",
        crawled_at = os.date("%Y-%m-%d %H:%M:%S"),
        task_id = TASK_ID or "",
        url = URL or "",
        project_id = PROJECT_ID or "tennis_booking"
    }
    
    -- 保存店铺信息
    local store_filter = {_id = store_id}
    local store_success = mongo_upsert("pospal_stores", store_filter, store_info)
    if not store_success then
        log_error("Failed to save store info")
    end
    
    -- 处理这个特定店铺的预约数据
    local appointment_count = 0
    local available_count = 0
    
    if pospal_data.appointmentList and type(pospal_data.appointmentList) == "table" then
        for _, apt in ipairs(pospal_data.appointmentList) do
            appointment_count = appointment_count + 1
            -- 生成唯一ID
            local apt_id = string.format("%s_%s_%s_%s",
                store_id,
                (apt.appointmentDate or apt.date or os.date("%Y-%m-%d")):gsub("-", ""),
                tostring(apt.roomId or apt.venueId or "unknown"),
                (apt.startTime or ""):gsub(":", "")
            )
            
            -- 容量和状态计算
            local capacity = tonumber(apt.maxCapacity or apt.capacity) or 0
            local booked = tonumber(apt.currentBooked or apt.bookedCount) or 0
            local remaining = capacity - booked
            local status = remaining > 0 and "available" or "booked"
            
            if status == "available" then
                available_count = available_count + 1
            end
            
            -- 保存预约时段数据
            local appointment_record = {
                _id = apt_id,
                store_id = store_id,
                store_name = venue_name,
                
                -- 日期和时间
                date = parse_date(apt.appointmentDate or apt.date),
                start_time = normalize_time(apt.startTime),
                end_time = normalize_time(apt.endTime),
                
                -- 房间/场地信息  
                room_id = tostring(apt.roomId or apt.venueId or ""),
                room_name = apt.roomName or apt.venueName or "",
                
                -- 容量和状态
                capacity = capacity,
                booked = booked,
                remaining = remaining,
                status = status,
                
                -- 价格
                member_price = parse_price(apt.memberPrice or apt.price),
                non_member_price = parse_price(apt.nonMemberPrice or apt.originalPrice),
                
                -- 额外信息
                service_type = apt.serviceType or apt.classType or "",
                instructor = apt.instructorName or apt.teacherName or "",
                duration_minutes = apt.duration or 60,
                booking_status = apt.bookingStatus or "",
                allow_waiting_list = apt.allowWaitingList or false,
                is_peak_hour = apt.isHotTime or apt.isPeakHour or false,
                
                -- 元数据
                crawled_at = os.date("%Y-%m-%d %H:%M:%S"),
                task_id = TASK_ID or "",
                project_id = PROJECT_ID or "tennis_booking"
            }
            
            -- 使用 upsert 保存预约数据
            local apt_filter = {_id = apt_id}
            mongo_upsert("pospal_appointments_5662377", apt_filter, appointment_record)
        end
        
        log_info(string.format("Processed %d appointments for store 5662377 (%d available)",
            appointment_count, available_count))
    end
    
    -- 保存原始数据
    local raw_data = {
        _id = store_id .. "_" .. os.time(),
        store_id = store_id,
        task_id = TASK_ID or "",
        raw_response = response,
        parsed_data = data,
        crawled_at = os.date("%Y-%m-%d %H:%M:%S"),
        url = URL or "",
        project_id = PROJECT_ID or "tennis_booking"
    }
    
    local raw_filter = {_id = raw_data._id}
    local raw_success = mongo_upsert("pospal_raw_data_5662377", raw_filter, raw_data)
    if not raw_success then
        log_error("Failed to save raw data")
    else
        log_info("Raw data saved")
    end
    
    -- 缓存店铺统计信息
    if appointment_count > 0 then
        local cache_key = string.format("pospal:store:5662377:%s", os.date("%Y%m%d"))
        local stats = {
            date = os.date("%Y-%m-%d"),
            total_appointments = appointment_count,
            available_appointments = available_count,
            utilization_rate = appointment_count > 0 and 
                math.floor((appointment_count - available_count) / appointment_count * 100) / 100 or 0
        }
        redis_set(cache_key, json.encode(stats), 3600)  -- 缓存1小时
        
        -- 如果利用率高，记录日志
        if stats.utilization_rate > 0.8 then
            log_info(string.format("High utilization for store 5662377: %.1f%%", 
                stats.utilization_rate * 100))
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