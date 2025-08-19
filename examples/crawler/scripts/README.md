# Crawler Scripts

这个目录包含爬虫系统的管理和配置脚本。

## 目录结构

```
scripts/
├── README.md           # 本文档
├── setup/              # 安装和初始化脚本
│   └── init_tasks.js   # 初始化任务配置
├── config/             # 配置脚本
│   └── configure_lua.js # 配置Lua脚本任务
└── maintenance/        # 维护脚本（临时脚本应放在这里）
```

## 使用说明

### 初始化任务
```bash
mongosh crawler < scripts/setup/init_tasks.js
```

### 配置Lua脚本
```bash
mongosh crawler < scripts/config/configure_lua.js
```

## Lua脚本路径配置

Lua脚本路径可以通过以下方式配置（优先级从高到低）：

1. **命令行参数**
   ```bash
   ./node --scripts=/custom/path
   ```

2. **环境变量**
   ```bash
   export CRAWLER_SCRIPT_BASE=/custom/path
   ./node
   ```

3. **配置文件** (node.yaml)
   ```yaml
   crawler:
     script_base: "crawler/scripts"
   ```

4. **默认值**: `crawler/scripts`

## 脚本目录结构

```
${script_base}/
└── projects/
    └── ${project_id}/           # 项目目录
        ├── config.lua            # 项目配置
        ├── common.lua            # 公共函数
        └── parsers/              # 解析脚本
            └── *.lua             # 各数据源解析器
```

## 注意事项

- 临时脚本应放在 `maintenance/` 目录
- 生产环境脚本应放在 `setup/` 或 `config/` 目录
- 定期清理 `maintenance/` 目录中的过期脚本