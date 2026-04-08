# Feed 社交系统（推拉混合 Feed Flow）

一个面向高并发场景设计的社交 Feed 系统示例，支持：

- 用户注册/登录（JWT）
- 关注/取关
- 朋友圈发布（文本/图片/视频）
- 时间线聚合（推拉混合）
- 点赞、评论、转发
- 上传媒体文件（图片/视频）

---

## 1. 项目目标与定位

本项目目标是实现一个具备**工程可落地性**的 Feed 流系统，核心关注：

1. **读写路径清晰**：发布、分发、读取链路明确。
2. **可扩展策略**：通过「推拉混合」适配普通用户与大 V 用户。
3. **体验完整**：前端具备社交产品核心交互（点赞、评论、转发、个人主页跳转、媒体展示）。
4. **实现简洁**：技术栈以 Go + Vue 为主，便于理解与二次开发。

---

## 2. 系统架构设计

### 2.1 总体架构

```text
┌─────────────────────────────────────────────────────────────┐
│                         Frontend (Vue3)                    │
│   Vue3 + Vue Router + Pinia + Axios + Element Plus + Vite                                                 │
└──────────────────────────────┬──────────────────────────────┘
                               │ HTTP/JSON
┌──────────────────────────────▼───────────────────────
│                     Backend API (Gin)                                                                     
│  Handlers -> Services -> Cache/DB/MQ                        
│                                                             
│  - Auth/User/Follow/Feed/Upload                             
│  - JWT 鉴权 + CORS                                           
└───────────────┬───────────────────────┬──────────────
                │                       │
       ┌────────▼────────┐      ┌──────▼───────┐
       │   MySQL 8.0                      │      │          Redis 7           │
       │ 持久化数据存储                           │         收件箱/发件箱      │
       └────────┬────────┘      └──────┬───────┘
                │                                               │
                └──────────┬────────——────┘
                                      │
                    ┌──────▼─────——─┐
                    │ In-Process MQ                │
                    │  (Go channel)                │
                    └──────────────—┘
```

### 2.2 架构分层

- **Handler 层**（`backend/handlers`）
  - 参数解析、鉴权上下文读取、返回统一响应。
- **Service 层**（`backend/services`）
  - 承载核心业务（发布、时间线聚合、关注关系维护等）。
- **Cache 层**（`backend/cache`）
  - Redis 操作封装（收件箱/发件箱/关系缓存）。
- **Model 层**（`backend/models`）
  - GORM 模型定义 + 自动迁移。
- **MQ 层**（`backend/mq`）
  - 基于 Go channel 的异步扇出消费。

---

## 3. 技术选型

## 3.1 后端

- **Go 1.21+**：高并发友好、性能稳定。
- **Gin**：轻量高效 Web 框架，路由与中间件简洁。
- **GORM**：快速建模与数据访问，降低样板代码。
- **JWT**：无状态认证，适合前后端分离。
- **Redis**：高性能缓存，支持 Sorted Set 实现时间序列表达。
- **In-Process MQ（channel）**：快速实现异步推送链路。

### 3.2 前端

- **Vue 3 + Composition API**：组件复用与状态组织灵活。
- **Vue Router**：页面路由与权限控制。
- **Pinia**：全局用户态管理。
- **Axios**：请求封装 + 拦截器统一异常处理。
- **Element Plus**：快速构建中后台/社交页面组件。
- **Vite**：开发体验佳、构建速度快。

### 3.3 数据与运维

- **MySQL**：关系型数据一致性与查询能力。
- **Redis**：时间线缓存、关系缓存、读性能加速。
- **Docker Compose**：本地一键拉起基础依赖（MySQL/Redis/RabbitMQ）。

> 注：当前业务 MQ 使用的是 `backend/mq/fanout.go` 中的 Go channel；`docker-compose.yml` 中的 RabbitMQ 为预留扩展组件，可用于替换为外部消息队列。

---

## 4. 推拉混合策略（核心设计）

Feed 系统的关键是读写放大之间的平衡：

- 全推模式：写放大严重（发一条给大量粉丝写大量收件箱）。
- 全拉模式：读放大严重（每次读都要查大量关注对象）。

本项目采用**推拉混合**：


| 用户类型 | 判定条件                     | 发布时策略      | 读取时策略            |
| ---- | ------------------------ | ---------- | ---------------- |
| 普通用户 | 粉丝数 < `big_v_threshold`  | 推：异步写粉丝收件箱 | 主要从收件箱读取         |
| 大V用户 | 粉丝数 >= `big_v_threshold` | 拉：只写自己的发件箱 | 时间线读取时按关注关系拉取并合并 |


关键参数见 `backend/config.yaml`：

- `feed.big_v_threshold`
- `feed.push_fan_limit`
- `feed.inbox_max_size`
- `feed.outbox_max_size`

---

## 5. 数据模型设计

### 5.1 核心表

1. `users`
  - 用户基础信息、粉丝数、关注数、大 V 标记。
2. `follows`
  - 关注关系（`user_id + followed_id` 唯一索引，软删除）。
3. `feeds`
  - 动态主表：
    - `content` 文案
    - `images` 图片 URL 列表（JSON 字符串）
    - `videos` 视频 URL 列表（JSON 字符串）
    - `feed_type`（原创/转发）
    - `original_id`（转发源）
    - `like_count/comment_count/share_count`
4. `timelines`
  - 推模式下粉丝收件箱的持久化记录。
5. `likes`
  - 点赞关系（`user_id + feed_id` 唯一约束）。
6. `comments`
  - 评论记录（软删除）。

### 5.2 Redis 键设计（概念）

- `inbox:{user_id}`：收件箱（ZSET）
- `outbox:{user_id}`：发件箱（ZSET）
- `following:{user_id}` / `followers:{user_id}`：关系集合
- 用户/Feed 缓存键（按封装实现）

---

## 6. 模块设计

### 6.1 后端模块

- `config/`：配置加载与结构化映射。
- `models/`：数据模型定义、DB 初始化、自动迁移。
- `handlers/`：API 入口控制器。
  - `user_handler.go`
  - `follow_handler.go`
  - `feed_handler.go`
  - `upload_handler.go`
- `services/`：核心业务逻辑。
  - `user_service.go`：用户与大 V 状态相关。
  - `follow_service.go`：关注、取关、回填/清理收件箱。
  - `feed_service.go`：发布、转发、时间线聚合、点赞评论。
- `cache/redis.go`：Redis 操作封装。
- `mq/fanout.go`：扇出队列与 worker 消费。
- `middleware/`：JWT 鉴权、CORS。
- `router/router.go`：路由注册，含上传静态资源映射 `/uploads`。

### 6.2 前端模块

- `src/views/`
  - `Timeline.vue`：发布区 + 时间线列表。
  - `Profile.vue`：个人主页 + 用户动态。
  - `Discover.vue`：用户发现页。
  - `Login.vue` / `Register.vue`：认证页。
  - `Layout.vue`：全局布局。
- `src/components/FeedCard.vue`
  - 单条动态展示（文本/图片/视频/转发内容/互动区）。
- `src/api/`
  - `request.js`：Axios 实例、拦截器。
  - `index.js`：按模块封装 API。
- `src/stores/user.js`
  - 登录态与用户信息（使用 `sessionStorage`，支持多标签页独立登录）。

---

## 7. 功能设计（按用户视角）

### 7.1 账号与身份

- 注册 / 登录
- JWT 鉴权
- 未登录访问受限路由自动跳登录

### 7.2 社交关系

- 关注用户 / 取消关注
- 粉丝列表 / 关注列表
- 个人主页展示关系与动态

### 7.3 动态发布

- 支持文本、图片、视频任意组合
- 媒体上传后即时预览
- 图片/视频均走后端上传接口并返回 URL
- 前端在发布 payload 中提交 `images` 与 `videos`

### 7.4 时间线展示

- 拉取聚合后的 Feed 流
- 展示原创动态与转发动态
- 支持图片网格与视频播放器
- 支持分页加载更多

### 7.5 互动能力

- 点赞/取消点赞
- 评论/删除自己的评论
- 动态删除（仅自己主页可见）
- 互动区展示点赞昵称与评论详情
- 昵称可点击跳转主页（作者、转发作者、点赞用户、评论用户）

### 7.6 转发能力

- 转发后保留原动态引用
- 转发卡片展示原作者、原文案、原图/原视频

### 7.7 上传与媒体访问

- 上传接口：
  - `POST /api/upload/image`
  - `POST /api/upload/video`
- 静态访问：`/uploads/`**
- 前端开发代理支持 `/api` 与 `/uploads`

---

## 8. 核心流程设计

### 8.1 发布流程

1. 前端选择媒体并上传，获得 URL。
2. 调用 `POST /api/feeds` 写入 Feed。
3. 服务层写 DB 后将消息投递到扇出队列。
4. worker 根据作者是否大 V 决定推送策略：
  - 普通用户：推送到粉丝收件箱
  - 大 V：仅写发件箱

### 8.2 时间线读取流程

1. 读取当前用户收件箱（推模式结果）。
2. 读取关注的大 V 发件箱（拉模式结果）。
3. 合并、去重、按时间排序。
4. 批量查询 Feed、作者、点赞状态。
5. 返回前端渲染。

### 8.3 关注/取关流程

- 关注：
  - 若存在软删除关系则恢复，不重复插入（避免唯一键冲突）
  - 更新计数与缓存
  - 触发收件箱回填
- 取关：
  - 软删除关系
  - 更新计数与缓存
  - 清理该关注对象在收件箱/时间线中的内容

---

## 9. API 设计概览

### 9.1 认证

- `POST /api/auth/register`
- `POST /api/auth/login`

### 9.2 用户

- `GET /api/users/me`
- `GET /api/users/search`
- `GET /api/users/:id`

### 9.3 上传

- `POST /api/upload/image`
- `POST /api/upload/video`

### 9.4 关注

- `POST /api/follow/:id`
- `DELETE /api/follow/:id`
- `GET /api/users/:id/followers`
- `GET /api/users/:id/following`

### 9.5 Feed

- `POST /api/feeds`
- `POST /api/feeds/repost`
- `DELETE /api/feeds/:id`
- `GET /api/feeds/:id`
- `GET /api/users/:id/feeds`
- `GET /api/timeline`
- `POST /api/feeds/:id/like`
- `DELETE /api/feeds/:id/like`
- `GET /api/feeds/:id/likes`
- `POST /api/feeds/:id/comments`
- `GET /api/feeds/:id/comments`
- `DELETE /api/feeds/:id/comments/:comment_id`

---

## 10. 项目目录结构

```text
feed/
├─ backend/
│  ├─ main.go
│  ├─ config.yaml
│  ├─ init.sql
│  ├─ config/
│  ├─ models/
│  ├─ handlers/
│  ├─ services/
│  ├─ cache/
│  ├─ mq/
│  ├─ middleware/
│  ├─ router/
│  ├─ utils/
│  └─ uploads/
├─ frontend/
│  ├─ src/
│  │  ├─ api/
│  │  ├─ views/
│  │  ├─ components/
│  │  ├─ stores/
│  │  └─ router/
│  ├─ package.json
│  └─ vite.config.js
├─ docker-compose.yml
└─ README.md
```

---

## 11. 快速启动

### 11.1 方式一：本地直接启动

### 后端

```bash
cd backend
go mod tidy
go run main.go
```

### 前端

```bash
cd frontend
npm install
npm run dev
```

访问：

- 前端：`http://localhost:3000`
- 后端：`http://localhost:8080`

### 11.2 方式二：Docker 拉起基础依赖

```bash
docker compose up -d
```

然后按“方式一”启动前后端应用。

> 注意：`docker-compose.yml` 默认 `MYSQL_DATABASE=GopherAI`，而 `backend/config.yaml` 默认 `dbname=feed`。请二选一保持一致（改 compose 或改 config）。

---

## 12. 配置说明（`backend/config.yaml`）

- `server`：端口、运行模式
- `database`：MySQL 连接、连接池
- `redis`：Redis 地址、密码、连接池
- `jwt`：签名密钥、过期时间
- `feed`：推拉混合策略阈值与缓存容量
- `upload`：上传路径、扩展名白名单、大小限制

---

## 13. 前端运行与交互说明

1. 登录后进入主布局页。
2. 时间线顶部可发布文本/图片/视频。
3. 动态卡支持点赞、评论、转发。
4. 转发卡展示原作者与原媒体。
5. 点赞昵称、评论昵称、转发作者昵称可点击跳转主页。
6. `sessionStorage` 存储登录态：不同标签页可登录不同账号。

---

## 14. 性能与扩展设计

### 14.1 已实现优化

- 推拉混合降低单一策略瓶颈。
- 异步扇出削峰，避免发布请求阻塞。
- Redis 收/发件箱加速时间线读取。
- 批量查询减少 N+1。
- 统一 API 响应与错误处理。

### 14.2 可继续演进

- 将 channel MQ 升级为 Kafka/RabbitMQ。
- Feed 与关系表分库分表。
- Redis Cluster 与哨兵高可用。
- 热点用户缓存预热。
- CDN + 对象存储（OSS/COS/S3）替代本地 uploads。
- 增加审计、风控、内容审核。
- 增加指标监控与链路追踪（Prometheus/Grafana/OpenTelemetry）。

---

## 15. 风险与已知事项

1. 当前上传媒体存储在本地磁盘，生产环境建议对象存储。
2. Docker 文件中 RabbitMQ 当前未接入业务队列（预留组件）。
3. 大文件上传与转码能力尚可继续强化（断点续传、封面提取、码率转码）。
4. 超大规模下需配套限流与灰度策略。

---

## 16. 效果说明（体验层）

- 具备完整社交闭环：发布 → 分发 → 消费 → 互动 → 关系变化。
- 支持图文视频混合展示与转发引用展示。
- 个人主页与时间线体验统一，交互逻辑清晰。
- 可作为课程设计、架构演示、面试项目、团队内部 PoC 基线。

---

## 17. License

MIT