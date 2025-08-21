# 分布式并发控制方案

## 问题分析

在分布式爬虫环境中，多个Node可能同时处理相同的场地数据，导致：
1. **重复推送**：同一个变更被多个Node检测并推送
2. **竞态条件**：并发写入导致数据不一致
3. **资源浪费**：重复的查询和计算

## 解决方案

### 1. 三层防护机制

```
第一层：任务级分布式锁
第二层：通知去重（基于状态MD5）
第三层：事件级去重（基于事件ID）
```

### 2. 核心组件

#### 2.1 分布式锁（DistributedLock）

**任务处理锁**
```go
// 防止多个Node同时处理同一个任务结果
lockValue, acquired := cd.distLock.AcquireProcessingLock(ctx, taskID, 30*time.Second)
if !acquired {
    // 其他Node正在处理，直接跳过
    return nil
}
defer cd.distLock.ReleaseProcessingLock(ctx, taskID, lockValue)
```

**特点**：
- 使用Redis SETNX实现原子锁获取
- 使用UUID作为锁值，防止误释放
- Lua脚本保证释放操作的原子性
- 自动过期防止死锁（30秒TTL）

#### 2.2 通知去重（CheckAndSetLastNotification）

**基于状态MD5的去重**
```go
shouldNotify := cd.distLock.CheckAndSetLastNotification(
    ctx,
    source,
    timeSlot,
    venueID,
    stateMD5,
    5*time.Minute, // 5分钟内相同状态不重复通知
)
```

**工作原理**：
- 记录每个场地最后通知的状态MD5
- 相同状态在5分钟内不重复通知
- 使用Lua脚本保证原子性检查和设置

#### 2.3 事件去重（DeduplicationChecker）

**基于事件ID的精确去重**
```go
eventID := cd.dedup.GenerateEventID(
    source, timeSlot, venueID,
    oldMD5, newMD5,
    timestamp.Truncate(time.Minute), // 分钟级精度
)

if !cd.dedup.IsEventProcessed(ctx, eventID) {
    cd.dedup.MarkEventProcessed(ctx, eventID)
    // 处理事件
}
```

**事件ID组成**：
- source:timeSlot:venueID:oldMD5:newMD5:timestamp
- 时间戳精确到分钟，允许相同变更在不同时间段重新通知

### 3. 并发场景分析

#### 场景1：多个Node同时收到相同任务结果

```
Node1 ──┐
        ├─→ AcquireProcessingLock(taskID)
Node2 ──┘
        ↓
    只有一个成功
        ↓
    获锁Node处理
    其他Node跳过
```

#### 场景2：同一场地在短时间内多次变更

```
时间线：
T0: 状态A → 状态B （通知）
T1: 状态B → 状态A （通知）
T2: 状态A → 状态B （5分钟内，不通知）
T6: 状态B → 状态C （超过5分钟，通知）
```

#### 场景3：网络延迟导致的乱序处理

```
实际顺序：Event1 → Event2 → Event3
处理顺序：Event2 → Event1 → Event3

解决：每个事件独立去重，不依赖顺序
```

### 4. Redis Key设计

```
# 任务处理锁
crawler:monitor:processing:{taskID}
TTL: 30秒

# 通知状态跟踪
crawler:monitor:notify:{source}:{timeSlot}:{venueID}
Value: stateMD5
TTL: 5分钟

# 事件处理标记
crawler:monitor:event:processed:{eventID}
TTL: 5分钟
```

### 5. 性能优化

#### 5.1 批量操作
- 批量查询历史快照（MongoDB聚合）
- 批量保存新快照（InsertMany）
- 批量检查事件去重（Pipeline）

#### 5.2 缓存策略
- 订阅者查询结果缓存60秒
- 处理过的事件ID缓存5分钟
- DingTalk客户端实例复用

#### 5.3 异步处理
- 变更检测后异步推送
- Worker Pool并发发送通知
- 失败重试也是异步的

### 6. 监控指标

```go
// 并发控制相关指标
metrics := map[string]interface{}{
    "lock_acquired_count": 1234,      // 成功获取锁的次数
    "lock_rejected_count": 56,        // 锁冲突次数
    "duplicate_events": 89,           // 重复事件数
    "unique_events": 1145,           // 唯一事件数
    "notification_dedup": 23,        // 去重的通知数
}
```

### 7. 故障恢复

#### 锁超时处理
- 处理锁30秒自动释放
- 防止Node崩溃导致死锁

#### 重试机制
- 获锁失败直接跳过，等待下次扫描
- 推送失败异步重试3次
- 重试间隔5秒

#### 数据一致性
- MongoDB操作失败不影响后续处理
- Redis操作失败降级为允许处理（宁可重复不可丢失）

### 8. 配置建议

```yaml
monitor:
  distributed:
    lock_ttl: 30s              # 处理锁超时时间
    notification_interval: 5m   # 相同状态通知间隔
    event_dedup_ttl: 5m        # 事件去重有效期
    
  performance:
    batch_size: 100            # 批量操作大小
    max_concurrent: 10         # 最大并发处理数
```

## 总结

通过三层防护机制，系统可以：
1. ✅ **防止重复推送**：多重去重机制确保每个变更只通知一次
2. ✅ **保证数据一致**：分布式锁防止并发写入冲突
3. ✅ **高效利用资源**：避免重复计算和查询
4. ✅ **容错能力强**：锁超时自动释放，操作失败优雅降级