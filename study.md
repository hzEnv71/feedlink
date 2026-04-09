# Feed 项目学习手册（study.md）

> 目标：让你从 0 开始，系统理解这个项目的**设计逻辑、技术路线、核心流程与扩展方向**。  
> 建议学习方式：按本文顺序走一遍，然后边看代码边跑接口。

---

## 1. 先建立全局认知：这个项目到底在做什么？

这是一个典型的社交动态系统（朋友圈/信息流），包含四条核心业务线：

1. **身份线**：注册登录 + JWT 鉴权
2. **关系线**：关注/取关 + 粉丝/关注列表
3. **内容线**：发布动态（图文视频）+ 时间线聚合
4. **互动线**：点赞、评论、转发、私信

它不是简单 CRUD，而是围绕“**信息流分发**”做了核心设计：
- 用 **Redis** 做收件箱/发件箱缓存
- 用 **RabbitMQ** 做异步分发
- 用“**推拉混合**”平衡读写压力

---

## 2. 技术路线（为什么是这套栈）

### 后端
- **Go + Gin**：高并发友好，接口开发效率高
- **GORM + MySQL**：关系数据（用户、关注、动态、评论）稳定可靠
- **Redis**：时间线缓存、关系状态缓存
- **RabbitMQ**：异步 fanout，避免发动态时阻塞请求
- **JWT**：前后端分离场景常用登录态方案

### 前端
- **Vue3 + Router + Pinia + Axios + Element Plus + Vite**
- 目标是快速完成完整社交交互链路，而不是重型工程化脚手架

这条路线很适合学习“中小型社交系统从功能到架构”的过渡。

---

## 3. 启动链路（从程序入口看系统骨架）

看 `backend/main.go`：

启动顺序是：
1. `InitConfig()` 读配置
2. `InitDB()` 初始化 MySQL + 自动迁移
3. `InitRedis()` 初始化 Redis
4. `InitMQ()` 初始化 RabbitMQ（发布者 + 多消费者）
5. `SetupRouter()` 注册路由并启动 Gin

这个顺序体现了一个原则：
**先准备依赖，再对外提供服务**。

---

## 4. 分层设计（你该怎么读代码）

后端是经典分层：

- `router`：路由注册
- `handlers`：HTTP 入参/出参、错误码
- `services`：业务逻辑核心
- `models`：DB 模型
- `cache`：Redis 封装
- `mq`：异步消息分发
- `middleware`：鉴权、跨域
- `utils`：JWT、统一响应

建议阅读顺序：
1. `router/router.go`（先知道有哪些 API）
2. `handlers/*`（请求如何进入）
3. `services/*`（业务怎么做）
4. `cache/redis.go` + `mq/fanout.go`（性能关键）

---

## 5. 项目核心设计：推拉混合 Feed

这是你最该深入的部分。

### 5.1 为什么要推拉混合？

信息流两种极端：
- **全推**：作者发一条，给所有粉丝写一遍（写放大）
- **全拉**：用户看一页，每次都实时查关注对象（读放大）

本项目做法：
- 普通用户：偏推（发文后推给粉丝收件箱）
- 大 V 用户：偏拉（只写自己发件箱，粉丝读取时拉）

### 5.2 判定依据

通过 `feed.big_v_threshold`（粉丝数阈值）区分是否大 V。

### 5.3 具体实现在哪里？

`backend/mq/fanout.go` 的 `processFanout`：
1. 所有人先写作者 `outbox`
2. 判断是否 bigV
3. bigV：跳过粉丝收件箱推送
4. 普通用户：fanout 到粉丝 `inbox` + timeline 持久化

### 5.4 时间线读取

`services/feed_service.go -> GetTimeline()`：
1. 拉本人动态
2. 读自己 inbox（推模式结果）
3. 拉关注的大 V outbox（拉模式结果）
4. 合并去重排序
5. 回查详情与点赞状态

这条链路是这个项目最重要的“架构思维”体现。

---

## 6. RabbitMQ 在项目里的角色

你现在这个项目是 RabbitMQ，不是内存队列。

### 6.1 发布
`PublishFeed(feedID, authorID)`：把消息发到 queue（持久化消息）。

### 6.2 消费
`InitMQ()` 启动多个消费者 goroutine，`consumerLoop -> runConsumer`：
- `Qos(prefetch)` 控制预取
- 手动 `Ack` 消息
- 失败可重连

### 6.3 配置参数
在 `config.yaml` + `config/config.go`：
- `rabbitmq.host/port/username/password/vhost`
- `rabbitmq.queue`
- `rabbitmq.prefetch`
- `rabbitmq.consumers`

学习建议：改动 `consumers/prefetch`，观察吞吐和数据库压力变化。

---

## 7. 数据模型怎么理解

优先理解这些表：

1. `users`：用户基本信息 + 关注统计 + 大V状态
2. `follows`：关注关系
3. `feeds`：动态主体
4. `timelines`：收件箱持久化副本
5. `likes` / `comments`：互动关系
6. `profile_visits`：访客记录
7. `messages`：私信

理解重点：
- `feeds` 是内容源
- `timelines` 是分发后的投递结果
- `inbox/outbox` 是 Redis 加速层

---

## 8. 端到端流程拆解（建议你动手画时序图）

### 8.1 发布动态
前端上传媒体 -> `POST /feeds` -> DB 写 feed -> MQ 发布 -> 消费者 fanout。

### 8.2 看时间线
`GET /timeline` -> 聚合 own + inbox + bigV outbox -> 排序分页 -> 返回。

### 8.3 关注用户
写 follow -> 更新计数 -> 缓存更新 -> 可选回填/清理 timeline。

### 8.4 私信聊天
主页点击“私信” -> 跳 `messages?target=...` -> 首次 `POST /messages` -> 后续 `GET /messages/conversations` 展示会话。

---

## 9. 前端设计逻辑（你该怎么读 Vue 代码）

核心页面：
- `Timeline.vue`：发布 + 信息流
- `Profile.vue`：用户主页 + 访客 + 私信入口
- `Messages.vue`：会话列表 + 聊天窗口
- `FeedCard.vue`：动态卡片复用组件

前端设计重点：
1. **按领域封装 API**（`api/index.js`）
2. **用户态集中在 Pinia**（`stores/user.js`）
3. **页面只处理交互，不放重业务规则**

---

## 10. 学习路径（从0到深入）

### Phase A：跑通项目（1~2天）
- 启动 MySQL/Redis/RabbitMQ
- 跑后端 + 前端
- 两个账号互关、发动态、点赞评论、私信

### Phase B：读懂主链路（2~4天）
- 顺序读：`router -> handlers -> services -> mq/cache`
- 重点断点：`PublishFeed`、`GetTimeline`、`processFanout`

### Phase C：理解架构取舍（2~3天）
- 思考：为什么不是全推/全拉
- 思考：为什么需要 MQ
- 思考：Redis 和 MySQL 谁是主数据源

### Phase D：做小改造（3~7天）
建议从这三个任务开始：
1. 给消息系统加未读计数
2. 给 timeline 加游标分页（替代 page/size）
3. 增加消息实时推送（WebSocket）

---

## 11. 你可以重点训练的“工程能力”

1. **分层思维**：Handler 不写复杂业务
2. **一致性思维**：DB 为准，缓存可回源
3. **性能思维**：异步化、批量化、缓存化
4. **演进思维**：先可用，再优化

---

## 12. 常见问题与排查

### 时间线没数据
- 查是否发文成功（DB feeds）
- 查 MQ 消费者日志
- 查 Redis inbox/outbox 是否有数据
- 查 timeline 表是否有落库

### 私信不显示
- 检查 `/messages/conversations` 返回
- 检查目标用户 id 是否正确
- 检查前端 query 参数 `target`

### 登录态异常
- 检查请求头 `Authorization: Bearer xxx`
- 检查 JWT secret 是否一致

---

## 13. 后续进阶建议（你学完后该做什么）

1. **可靠性**：消息重试、死信队列、幂等键
2. **可观测性**：Prometheus + Grafana + tracing
3. **高可用**：Redis 哨兵/集群、MQ 镜像队列
4. **业务进化**：推荐流、黑名单、消息已读/撤回

---

## 14. 一句话总结

这个项目最有价值的不是“功能多”，而是它已经具备了社交系统的核心工程骨架：

- 关系 + 内容 + 分发 + 互动 + 消息
- 同时有缓存、异步、策略分流

你把这套吃透后，再学中大型社交/推荐系统会非常顺。

---

如果你愿意，我下一步可以给你再做一个 `study-checklist.md`，按“每天学什么 + 看哪些文件 + 做什么小实验”的形式，直接当 14 天学习计划执行。