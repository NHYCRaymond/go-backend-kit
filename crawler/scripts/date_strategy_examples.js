// MongoDB 日期策略配置示例
// 展示不同场景下的日期变量策略配置

use crawler;

// 示例1: 体育场馆预订 - 只需要当天数据
db.tasks.updateOne(
  { name: "venue_booking_today" },
  {
    $set: {
      "request.variables": [{
        name: "DATE",
        type: "date",
        source: "function",
        expression: "${DATE}",
        strategy: {
          mode: "single",
          format: "2006-01-02",
          start_days: 0  // 今天
        }
      }]
    }
  },
  { upsert: false }
);

// 示例2: 历史数据爬取 - 获取过去7天的数据
db.tasks.updateOne(
  { name: "historical_data_crawler" },
  {
    $set: {
      "request.variables": [{
        name: "DATE",
        type: "date",
        source: "function",
        expression: "${DATE}",
        strategy: {
          mode: "range",
          format: "2006-01-02",
          start_days: -6,  // 6天前开始
          end_days: 0,     // 到今天
          day_step: 1      // 每天一个任务
        }
      }]
    }
  },
  { upsert: false }
);

// 示例3: 未来赛事预告 - 获取未来30天的数据
db.tasks.updateOne(
  { name: "upcoming_events" },
  {
    $set: {
      "request.variables": [{
        name: "DATE",
        type: "date",
        source: "function",
        expression: "${DATE}",
        strategy: {
          mode: "range",
          format: "2006-01-02",
          start_days: 1,   // 明天开始
          end_days: 30,    // 30天后
          day_step: 1      // 每天
        }
      }]
    }
  },
  { upsert: false }
);

// 示例4: 周报数据 - 每周一次
db.tasks.updateOne(
  { name: "weekly_report" },
  {
    $set: {
      "request.variables": [{
        name: "DATE",
        type: "date",
        source: "function",
        expression: "${DATE}",
        strategy: {
          mode: "range",
          format: "2006-01-02",
          start_days: -28,  // 4周前
          end_days: 0,      // 到今天
          day_step: 7       // 每7天（每周）
        }
      }]
    }
  },
  { upsert: false }
);

// 示例5: 特定日期列表 - 只爬取特定的几天
db.tasks.updateOne(
  { name: "custom_dates_crawler" },
  {
    $set: {
      "request.variables": [{
        name: "DATE",
        type: "date",
        source: "function",
        expression: "${DATE}",
        strategy: {
          mode: "custom",
          format: "2006-01-02",
          custom_days: [-7, -3, -1, 0, 1, 3, 7]  // 特定的日期偏移
        }
      }]
    }
  },
  { upsert: false }
);

// 示例6: 月度数据 - 每月第一天
db.tasks.updateOne(
  { name: "monthly_data" },
  {
    $set: {
      "request.variables": [{
        name: "DATE",
        type: "date",
        source: "function",
        expression: "${DATE}",
        strategy: {
          mode: "custom",
          format: "2006-01-02",
          // 过去3个月的每月1号
          custom_days: [-90, -60, -30, 0]
        }
      }]
    }
  },
  { upsert: false }
);

print("\n=== 日期策略配置示例 ===\n");
print("1. single 模式: 单个日期（如今天、昨天、明天）");
print("2. range 模式: 日期范围（如最近7天、未来30天）");
print("3. custom 模式: 自定义日期列表（如特定的几天）");
print("\n各种策略可以灵活组合使用，满足不同的爬取需求。");