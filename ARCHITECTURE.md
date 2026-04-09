# Backend Architecture Convention

本项目后端统一采用：

- `handlers`：仅做参数校验、调用 service、返回响应
- `services`：业务编排层，不直接写复杂 SQL / 不直接散落 CRUD
- `repository`：唯一 MySQL 数据访问层
- `cache`：唯一 Redis 访问层
- `mq`：消息队列访问层（RabbitMQ）

## 约束

1. `services/*.go` 不允许新增 `models.DB` 直接 CRUD。
2. 所有数据库读写先扩展 `repository` 接口，再由 service 调用。
3. handler 构造函数统一依赖注入 service，例如：
   - `NewUserHandler(userService *services.UserService)`
4. 文件命名统一：
   - `*_handler.go`
   - `*_service.go`
   - `*_repository.go`
5. 构造函数统一：
   - `NewXxxService()`
   - `NewXxxRepository(db *gorm.DB)`

## 典型调用链

`router -> handler -> service -> repository/cache/mq`

## 重构目标

- 提升可维护性与可测试性
- 让业务逻辑与数据访问职责分离
- 支持后续 repository mock 单元测试
