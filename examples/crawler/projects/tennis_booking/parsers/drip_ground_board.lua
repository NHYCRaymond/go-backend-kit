-- drip_ground_board 数据源解析脚本
local json = require("json")

function parse()
    -- 解析JSON响应
    local success, resp = pcall(json.decode, response)
    if not success then
        log_error("Failed to decode JSON: " .. tostring(resp))
        return false
    end
    
    -- 检查响应是否成功
    if not resp.success then
        log_error("API returned error")
        return false
    end
    
    local data = resp.result  -- 实际数据在 result 字段中
    
    -- 检查是否有 groundDatas 字段
    if data.groundDatas and type(data.groundDatas) == "table" then
        log_info("Found groundDatas field with " .. #data.groundDatas .. " grounds")
        log_info("Group: " .. (data.groupName or "unknown") .. ", Date: " .. (data.date or "unknown"))
        
        -- 遍历每个 ground 数据
        for i, ground in ipairs(data.groundDatas) do
            -- 展开 ground 数据的各个字段
            local ground_record = {
                -- 基本信息
                _id = tostring(ground.groundId) .. "_" .. (data.date or os.date("%Y-%m-%d")),
                task_id = TASK_ID or "",
                ground_index = i,
                
                -- 场地基本信息（展开字段）
                group_id = ground.groupId,
                group_name = data.groupName,  -- 从外层获取
                ground_id = ground.groundId,
                ground_name = ground.groundName,
                full_ground_name = ground.fullGroundName,
                sub_ground_id = ground.subGroundId,
                sub_ground_name = ground.subGroundName,
                
                -- 场地属性
                state = ground.state,  -- 1=可用
                sort_no = ground.sortNo,
                ground_option_ids = ground.groundOptionIds,
                
                -- 日期和时间信息
                date = data.date,
                time_scale = data.timeScale,  -- 时间间隔（分钟）
                time_scales = data.timeScales,  -- 时间刻度数组
                
                -- 时间段（periods）处理
                periods_count = ground.periods and #ground.periods or 0,
                periods_summary = {},  -- 将在下面填充
                
                -- 元数据
                raw_data = ground,  -- 保存完整的原始数据
                crawled_at = os.date("%Y-%m-%d %H:%M:%S"),
                url = URL or "",
                project_id = PROJECT_ID or "tennis_booking"
            }
            
            -- 处理 periods 数据，生成摘要
            if ground.periods and type(ground.periods) == "table" then
                local available_count = 0
                local booked_count = 0
                local unavailable_count = 0
                local min_price = nil
                local max_price = nil
                
                for _, period in ipairs(ground.periods) do
                    -- status: -1=不可用(关闭), 1=可预订, 2=已预订
                    if period.status == 1 then
                        available_count = available_count + 1
                    elseif period.status == 2 then
                        booked_count = booked_count + 1
                    elseif period.status == -1 then
                        unavailable_count = unavailable_count + 1
                    end
                    
                    -- 价格统计（单位是分，需要转换）
                    local price = period.price / 100
                    if not min_price or price < min_price then
                        min_price = price
                    end
                    if not max_price or price > max_price then
                        max_price = price
                    end
                end
                
                ground_record.periods_summary = {
                    total = #ground.periods,
                    available = available_count,
                    booked = booked_count,
                    unavailable = unavailable_count,
                    min_price = min_price,
                    max_price = max_price
                }
            end
            
            -- 使用 upsert 保存到 ground_board_details（存在则更新，不存在则插入）
            local filter = {
                _id = ground_record._id
            }
            local success = mongo_upsert("ground_board_details", filter, ground_record)
            if success then
                log_info("Upserted ground " .. i .. " (" .. ground.groundName .. ") to ground_board_details")
            else
                log_error("Failed to upsert ground " .. i)
            end
            
            -- 同时保存详细的时间段数据到另一个集合
            if ground.periods and type(ground.periods) == "table" then
                for j, period in ipairs(ground.periods) do
                    -- 生成唯一ID: 日期_场地ID_开始时间
                    -- 格式: 20250819_6038_060000 (对应 2025-08-19 场地6038 06:00:00)
                    local date_clean = data.date:gsub("-", "")  -- 2025-08-19 -> 20250819
                    local time_clean = period.startAt:gsub("[%-%s:]", "")  -- 2025-08-19 06:00:00 -> 20250819060000
                    -- 从完整时间戳中提取时间部分
                    local time_part = time_clean:sub(9)  -- 取 060000 部分
                    
                    local period_id = string.format("%s_%s_%s", 
                        date_clean,  -- 日期: 20250819
                        tostring(ground.groundId),  -- 场地ID: 6038
                        time_part  -- 时间: 060000
                    )
                    
                    local period_record = {
                        _id = period_id,
                        ground_id = ground.groundId,
                        ground_name = ground.groundName,
                        group_id = ground.groupId,
                        group_name = data.groupName,
                        date = data.date,
                        
                        -- 时间段信息
                        start_at = period.startAt,
                        end_at = period.endAt,
                        status = period.status,  -- -1=不可用(关闭), 1=可预订, 2=已预订
                        status_name = period.status == 1 and "available" or (period.status == 2 and "booked" or "unavailable"),
                        
                        -- 价格信息（转换为元）
                        price = period.price / 100,
                        non_member_price = period.nonMemberPrice / 100,
                        
                        -- 价格标签
                        price_tag_enabled = period.priceTag and period.priceTag.enable or false,
                        price_tag_desc = period.priceTag and period.priceTag.desc or "",
                        price_tag_color = period.priceTag and period.priceTag.color or "",
                        
                        -- 元数据
                        crawled_at = os.date("%Y-%m-%d %H:%M:%S"),
                        project_id = PROJECT_ID or "tennis_booking"
                    }
                    
                    -- 使用 upsert 保存时间段数据
                    local period_filter = {
                        _id = period_id
                    }
                    mongo_upsert("ground_board_periods", period_filter, period_record)
                end
                log_info("Saved " .. #ground.periods .. " periods for ground " .. ground.groundName)
            end
        end
    else
        log_info("No groundDatas field found in response")
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