

# 仿朋友圈与私信功能聚合
支持：

- 用户注册 / 登录（JWT）
- 关注 / 取关
- 发布动态（文字、图片、视频）
- 时间线（朋友圈）
- 点赞、评论、转发、删除
- 个人主页（头像、签名、访客）
- 私信消息（会话列表 + 聊天记录）

---

## 技术栈

### 后端（`backend/`）

- Go / Gin / GORM / MySQL / Redis / RabbitMQ / JWT

### 前端（`frontend/`）

- Vue 3（Composition API） Vue Router Pinia  Axios  Element Plus  Vite

---

## 项目截图
- 动态页面
![动态页面](./pic/屏幕截图%202026-04-08%20222747.png)
- 主页页面
![主页页面](./pic/屏幕截图%202026-04-08%20222758.png)
- 消息页面
![消息页面](./pic/屏幕截图%202026-04-08%20222806.png)




## 项目结构

```text
feed/
├─ backend/
│  ├─ main.go
│  ├─ config.yaml
│  ├─ go.mod
│  ├─ handlers/
│  ├─ services/
│  ├─ models/
│  ├─ middleware/
│  ├─ router/
│  ├─ cache/
│  ├─ utils/
│  └─ uploads/
├─ frontend/
│  ├─ package.json
│  ├─ vite.config.js
│  └─ src/
│     ├─ api/
│     ├─ components/
│     ├─ router/
│     ├─ stores/
│     └─ views/
├─ docker-compose.yml
└─ README.md
```

---

## 核心功能

### 1) 用户与关系

- 注册、登录、鉴权
- 搜索用户
- 关注 / 取消关注
- 个人主页展示粉丝、关注、访客

### 2) 动态系统

- 发布文字、图片、视频动态
- 查看自己和他人的动态
- 点赞 / 取消点赞
- 评论 / 删除评论
- 转发动态
- 删除自己的动态

### 3) 个人主页增强

- 修改头像
- 修改个性签名
- 记录最近访客

### 4) 私信消息

- 在他人主页点击“私信”可发起第一次聊天
- 消息页展示会话列表（头像、昵称、最后一条消息、时间）
- 聊天页展示完整对话记录
- 支持发送新消息并刷新会话

---

## API 概览

### 认证

- `POST /api/auth/register`
- `POST /api/auth/login`

### 用户

- `GET /api/users/me`
- `PUT /api/users/me`
- `GET /api/users/me/visitors`
- `GET /api/users/search`
- `GET /api/users/:id`

### 关注

- `POST /api/follow/:id`
- `DELETE /api/follow/:id`
- `GET /api/users/:id/followers`
- `GET /api/users/:id/following`

### 动态

- `POST /api/feeds`
- `POST /api/feeds/repost`
- `DELETE /api/feeds/:id`
- `GET /api/feeds/:id`
- `GET /api/users/:id/feeds`
- `GET /api/timeline`

### 点赞/评论

- `POST /api/feeds/:id/like`
- `DELETE /api/feeds/:id/like`
- `GET /api/feeds/:id/likes`
- `POST /api/feeds/:id/comments`
- `GET /api/feeds/:id/comments`
- `DELETE /api/feeds/:id/comments/:comment_id`

### 上传

- `POST /api/upload/image`
- `POST /api/upload/video`

### 私信

- `POST /api/messages`
- `GET /api/messages/conversations`
- `GET /api/messages/:target_id`

---

## 本地运行

## 1. 准备依赖

请先启动：

- MySQL
- Redis
- RabbitMQ

也可以使用：

```bash
docker compose up -d
```

> 注意检查 `backend/config.yaml` 中的数据库与 Redis 配置是否和本地环境一致。

## 2. 启动后端

```bash
cd backend
go mod tidy
go run main.go
```

默认后端地址：`http://localhost:8080`

## 3. 启动前端

```bash
cd frontend
npm install
npm run dev
```

默认前端地址：`http://localhost:3000`（或 Vite 控制台显示端口）

---

## 配置说明

后端核心配置文件：`backend/config.yaml`

可配置项包括：

- 服务端口、运行模式
- MySQL 连接
- Redis 连接
- JWT 密钥与过期时间
- 上传目录与大小限制
- Feed 相关阈值参数

---

## License

MIT
