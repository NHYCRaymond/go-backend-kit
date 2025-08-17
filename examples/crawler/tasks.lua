-- Lua script for defining crawler tasks
-- This script demonstrates how to define crawler tasks using Lua

-- Note: All constants are automatically available in the Lua environment
-- Task types: TYPE_SEED, TYPE_DETAIL, TYPE_LIST, TYPE_API, TYPE_BROWSER, TYPE_AGGREGATE
-- Priorities: PRIORITY_LOW, PRIORITY_NORMAL, PRIORITY_HIGH, PRIORITY_URGENT
-- Status: STATUS_PENDING, STATUS_QUEUED, STATUS_RUNNING, STATUS_COMPLETED, etc.

-- Define a table to store task definitions
tasks = {}

-- Helper function to create a task definition
function create_task(id, name, url_pattern, extract_rules)
    return {
        id = id,
        name = name,
        type = TYPE_DETAIL,  -- Use constant
        priority = PRIORITY_NORMAL,  -- Use constant
        url_pattern = url_pattern,
        extract_rules = extract_rules,
        headers = {
            ["User-Agent"] = "Mozilla/5.0 (compatible; Crawler/1.0)",
            ["Accept"] = "text/html,application/json"
        },
        timeout = 30,
        max_retries = 3
    }
end

-- Define GitHub repository crawler task
tasks.github_repo = create_task(
    "github-repo",
    "GitHub Repository Crawler",
    "https://api.github.com/repos/{owner}/{repo}",
    {
        {field = "name", selector = "$.name", type = "json"},
        {field = "description", selector = "$.description", type = "json"},
        {field = "stars", selector = "$.stargazers_count", type = "json"},
        {field = "forks", selector = "$.forks_count", type = "json"},
        {field = "language", selector = "$.language", type = "json"},
        {field = "created_at", selector = "$.created_at", type = "json"},
        {field = "updated_at", selector = "$.updated_at", type = "json"}
    }
)

-- Define e-commerce product crawler task
tasks.product_crawler = {
    id = "product-crawler",
    name = "E-commerce Product Crawler",
    type = TYPE_DETAIL,  -- Use constant
    priority = PRIORITY_HIGH,  -- Use constant
    extract_rules = {
        {field = "title", selector = "h1.product-title", type = "css"},
        {field = "price", selector = ".price-now", type = "css"},
        {field = "description", selector = ".product-description", type = "css"},
        {field = "images", selector = ".product-image img", type = "css", attribute = "src", multiple = true},
        {field = "rating", selector = ".rating-value", type = "css"},
        {field = "reviews", selector = ".review-count", type = "css"}
    },
    link_rules = {
        {
            name = "related_products",
            selector = ".related-products a",
            type = "css",
            attribute = "href",
            task_type = TYPE_DETAIL,  -- Use constant
            priority = PRIORITY_NORMAL,  -- Use constant
            depth = 2
        }
    }
}

-- Define news article crawler task
tasks.news_crawler = {
    id = "news-crawler",
    name = "News Article Crawler",
    type = TYPE_DETAIL,  -- Use constant
    priority = PRIORITY_NORMAL,  -- Use constant
    extract_rules = {
        {field = "title", selector = "//h1[@class='article-title']", type = "xpath"},
        {field = "author", selector = "//span[@class='author-name']", type = "xpath"},
        {field = "publish_date", selector = "//time[@class='publish-date']", type = "xpath", attribute = "datetime"},
        {field = "content", selector = "//div[@class='article-content']", type = "xpath"},
        {field = "tags", selector = "//a[@class='tag']", type = "xpath", multiple = true}
    }
}

-- Dynamic task generation function
function generate_search_task(keyword, page)
    local task_id = string.format("search-%s-%d", keyword, page)
    local url = string.format("https://example.com/search?q=%s&page=%d", keyword, page)
    
    return {
        id = task_id,
        url = url,
        type = TYPE_LIST,  -- Use constant
        priority = PRIORITY_NORMAL,  -- Use constant
        extract_rules = {
            {field = "items", selector = ".search-result", type = "css", multiple = true},
            {field = "next_page", selector = ".pagination .next", type = "css", attribute = "href"}
        },
        metadata = {
            keyword = keyword,
            page = page
        }
    }
end

-- Task validation function
function validate_task(task)
    if not task.id or task.id == "" then
        return false, "Task ID is required"
    end
    
    if not task.type then
        return false, "Task type is required"
    end
    
    if not task.extract_rules or #task.extract_rules == 0 then
        return false, "At least one extraction rule is required"
    end
    
    return true, nil
end

-- Task preprocessing function
function preprocess_task(task)
    -- Add default headers if not present
    if not task.headers then
        task.headers = {}
    end
    
    if not task.headers["User-Agent"] then
        task.headers["User-Agent"] = "Mozilla/5.0 (compatible; LuaCrawler/1.0)"
    end
    
    -- Set default timeout
    if not task.timeout then
        task.timeout = 30
    end
    
    -- Set default priority
    if not task.priority then
        task.priority = PRIORITY_NORMAL  -- Use constant
    end
    
    return task
end

-- Task result processing function
function process_result(task_id, result)
    log("info", "Processing result for task: " .. task_id)
    
    -- Extract data
    if result.status_code == 200 then
        local data = json_decode(result.body)
        
        -- Save to storage
        crawler.save_data(task_id, data)
        
        -- Generate child tasks for pagination
        if data.next_page then
            local next_task = {
                url = data.next_page,
                type = TYPE_LIST,  -- Use constant
                parent_id = task_id,
                priority = PRIORITY_LOW  -- Use constant
            }
            crawler.submit_task(next_task)
        end
        
        return true
    else
        log("error", "Task failed with status: " .. result.status_code)
        return false
    end
end

-- Custom extraction function
function extract_price(text)
    -- Extract price from text like "$99.99" or "￥299"
    local patterns = {
        "%$([%d,]+%.?%d*)",      -- USD
        "￥([%d,]+%.?%d*)",       -- CNY
        "€([%d,]+%.?%d*)",        -- EUR
        "£([%d,]+%.?%d*)"         -- GBP
    }
    
    for _, pattern in ipairs(patterns) do
        local price = string.match(text, pattern)
        if price then
            -- Remove commas and convert to number
            price = string.gsub(price, ",", "")
            return tonumber(price)
        end
    end
    
    return nil
end

-- Rate limiting function
rate_limits = {}

function check_rate_limit(domain)
    local current_time = os.time()
    local limit = rate_limits[domain]
    
    if not limit then
        rate_limits[domain] = {
            last_request = current_time,
            count = 1
        }
        return true
    end
    
    -- Reset counter every minute
    if current_time - limit.last_request > 60 then
        limit.count = 1
        limit.last_request = current_time
        return true
    end
    
    -- Check if within rate limit (10 requests per minute)
    if limit.count < 10 then
        limit.count = limit.count + 1
        return true
    end
    
    return false
end

-- URL filtering function
function should_crawl_url(url)
    -- Skip certain file types
    local skip_extensions = {".pdf", ".doc", ".xls", ".ppt", ".zip", ".exe"}
    for _, ext in ipairs(skip_extensions) do
        if string.sub(url, -#ext) == ext then
            return false
        end
    end
    
    -- Skip certain domains
    local blocked_domains = {"ads.example.com", "tracking.example.com"}
    for _, domain in ipairs(blocked_domains) do
        if string.find(url, domain) then
            return false
        end
    end
    
    return true
end

-- Main execution function
function main()
    log("info", "Starting Lua crawler tasks")
    
    -- Register all tasks
    for name, task in pairs(tasks) do
        task = preprocess_task(task)
        
        local valid, err = validate_task(task)
        if valid then
            crawler.create_definition(task)
            log("info", "Registered task: " .. name)
        else
            log("error", "Invalid task " .. name .. ": " .. err)
        end
    end
    
    -- Generate and submit initial tasks
    local keywords = {"golang", "web scraping", "distributed systems"}
    for _, keyword in ipairs(keywords) do
        for page = 1, 5 do
            local task = generate_search_task(keyword, page)
            crawler.submit_task(task)
        end
    end
    
    log("info", "Lua crawler tasks initialized")
end

-- Execute main function
main()