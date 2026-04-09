# Repository 层说明

本目录负责**数据访问**，是 `service` 与持久化实现（MySQL/GORM）之间的边界。

## 设计目标

1. service 不直接写 `models.DB`，统一通过 repository 接口访问数据。
2. 业务规则留在 service；repository 只做查询、写入、事务原子操作。
3. 通过接口隔离，后续可替换实现（例如 mock / 其他存储）。

---

## 文件职责

- `user_repository.go`：用户、搜索、资料更新、访问记录（visit）
- `follow_repository.go`：关注关系、关注计数、回填相关查询
- `feed_repository.go`：动态、时间线、点赞、评论、转发计数
- `message_repository.go`：私信会话与消息查询

---

## 事务边界约定（重点）

### 原则

- **事务由 service 层开启与提交/回滚**。
- repository 提供 `BeginTx()` 及 `tx *gorm.DB` 版本方法，用于组合原子操作。

### 示例

- `FollowService.Follow()`：
  - 开事务
  - 创建/恢复关注关系
  - 更新双方计数
  - 提交事务

- `FeedService.LikeFeed()`：
  - 开事务
  - 写 `likes`
  - 增加 `feeds.like_count`
  - 提交事务

---

## 方法命名规范

- 查询：`Get* / List*`
- 新增：`Create*`
- 更新：`Update* / Increase* / Decrease*`
- 删除：`Delete*`
- 事务入口：`BeginTx`

命名要求：**语义明确、单一职责、避免缩写歧义**。

---

## 扩展建议

1. 若后续引入缓存读写穿透，可在 service 处理，repository 保持纯 DB。
2. 若需要复杂报表查询，可增加 `query_repository.go`（只读聚合）。
3. 单测建议以 service 为主，repository 用集成测试或 mock。
