# Crawler System

A distributed web crawler system with task scheduling, worker nodes, and monitoring.

## Architecture

```
MongoDB (Task Storage) → Scheduler (Hub) → Redis Queue → Worker Nodes → Storage
                                ↑                              ↓
                               TUI ←────────────────────── Monitoring
```

## Components

1. **Hub** - Central coordinator with integrated scheduler
2. **Node** - Worker nodes that execute crawling tasks  
3. **TUI** - Terminal UI for monitoring and control

## Quick Start

### Prerequisites
```bash
# Start required services
brew services start redis
brew services start mongodb-community
```

### Configuration Files
All components use YAML configuration files located in `examples/crawler/config/`:
- `hub.yaml` - Hub configuration
- `node.yaml` - Node configuration (set `crawler.mode: "enhanced"` for real execution)
- `tui.yaml` - TUI configuration

### Running the System

#### 1. Start Hub (with Scheduler)
```bash
go run examples/crawler/distributed/hub/main.go -config examples/crawler/config/hub.yaml
```

#### 2. Start Node (Worker)
```bash
go run examples/crawler/distributed/node/main.go -config examples/crawler/config/node.yaml
```

#### 3. Monitor with TUI
```bash
go run examples/crawler/tui/main.go -config examples/crawler/config/tui.yaml
```

## Building

```bash
# Build all components
go build -o bin/crawler-hub examples/crawler/distributed/hub/main.go
go build -o bin/crawler-node examples/crawler/distributed/node/main.go
go build -o bin/crawler-tui examples/crawler/tui/main.go

# Run built binaries
./bin/crawler-hub -config examples/crawler/config/hub.yaml
./bin/crawler-node -config examples/crawler/config/node.yaml  
./bin/crawler-tui -config examples/crawler/config/tui.yaml
```

## Node Modes

- **Enhanced Mode**: Real task execution with HTTP fetching, data extraction, and storage
  - Requires MongoDB connection
  - Set `crawler.mode: "enhanced"` in node.yaml
  
- **Basic Mode**: Test mode that simulates execution without actual work
  - No external dependencies required
  - Used for testing task distribution

## Task Flow

1. **Define Tasks**: Create task definitions in MongoDB
2. **Schedule**: Hub's scheduler loads and schedules tasks based on cron/interval
3. **Queue**: Tasks are pushed to Redis queue
4. **Execute**: Nodes fetch tasks from queue and execute them
5. **Store**: Results stored in configured storage (MongoDB/MySQL/Redis)

## TUI Controls

- **F1-F6**: Switch between views (Dashboard, Nodes, Tasks, Logs, Config, Task Manager)
- **R**: Refresh current view / Run task (in Task Manager)
- **E**: Edit task (in Task Manager)
- **D**: Delete task (in Task Manager)
- **ESC**: Return to list / Clear filter
- **Ctrl+W W**: Switch focus between menu and content
- **F10**: Quit

## Configuration Override

Command-line flags override configuration file values:

```bash
# Override MongoDB URI
go run examples/crawler/distributed/node/main.go -config examples/crawler/config/node.yaml -mongo "mongodb://192.168.1.100:27017"

# Override worker count
go run examples/crawler/distributed/node/main.go -config examples/crawler/config/node.yaml -workers 20
```

## Production Tasks

### 滴球场地预订板任务 (drip_ground_board)

**Target API**: https://zd.drip.im/api-zd-wap/booking/order/getGroundBoard
- **Method**: POST
- **Schedule**: Every 30 seconds
- **Type**: API data collection
- **Storage**: MongoDB `crawler.court_booking_boards`

## Monitoring

- View system status in TUI Dashboard (F1)
- Check node status in Nodes view (F2)  
- Monitor task queue in Tasks view (F3)
- View logs in Logs view (F4)
- Manage tasks in Task Manager (F6)

## Troubleshooting

### Tasks Not Executing
1. Ensure node is running in **enhanced mode** (check node.yaml)
2. Verify MongoDB connection
3. Check Redis queue: `crawler:queue:pending`
4. View node logs for errors

### Node Shows "Basic Mode"
- MongoDB connection failed
- Set `crawler.mode: "enhanced"` in node.yaml
- Check MongoDB URI is correct

### TUI Not Showing Tasks
- Verify MongoDB connection
- Check MongoDB URI in TUI config or command line