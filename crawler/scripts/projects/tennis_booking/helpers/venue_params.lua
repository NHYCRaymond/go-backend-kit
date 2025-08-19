-- venue_params.lua - Helper functions for generating venue request parameters
-- Updated based on API behavior analysis: week parameter is not needed

-- Generate date string in YYYY-MM-DD format
function format_date(offset_days)
    offset_days = offset_days or 0
    local timestamp = os.time() + (offset_days * 86400)
    return os.date("%Y-%m-%d", timestamp)
end

-- Build venue request body (simplified - no week parameter needed)
function build_venue_request_body(vid, pid, date, openid)
    -- Build simple request body with only necessary parameters
    local params = {}
    
    -- Required parameters
    table.insert(params, string.format("vid=%s", vid or "1557"))
    table.insert(params, string.format("pid=%s", pid or "18"))
    
    -- Optional date parameter (if not provided, API returns today's data)
    if date and date ~= "" then
        table.insert(params, string.format("date=%s", date))
    end
    
    -- OpenID parameter
    table.insert(params, string.format("openid=%s", openid or "o_PUd7UvwOKaxG8AHYDo6X962bvg"))
    
    -- Combine into request body
    local body = table.concat(params, "&")
    
    return body
end

-- Generate date range for batch task creation
function generate_date_range(start_offset, end_offset)
    start_offset = start_offset or 0
    end_offset = end_offset or 6
    
    local dates = {}
    for i = start_offset, end_offset do
        table.insert(dates, format_date(i))
    end
    
    return dates
end

-- Build task metadata
function build_task_metadata(venue_id, date)
    return {
        venue_id = venue_id,
        date = date,
        timestamp = os.time()
    }
end

-- Export functions
return {
    format_date = format_date,
    build_venue_request_body = build_venue_request_body,
    generate_date_range = generate_date_range,
    build_task_metadata = build_task_metadata
}