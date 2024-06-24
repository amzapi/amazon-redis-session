# Amazon Redis Session

Amazon Redis Session 是一个用于亚马逊爬虫存储 Cookies 的库，基于 Redis 实现。它提供了方便的接口来管理和存储爬虫的 Session 数据。

## 特性

- 基于 Redis 实现高效的 Session 存储
- 支持多种操作，包括设置、获取和删除 Session
- 适用于亚马逊爬虫的 Cookie 管理

## 交流群

<img src="https://asin1.com/WechatGroup.jpg" alt="微信交流群" width="300">

## 安装

请确保已安装 Go 语言环境和 Redis 服务器。

```sh
go get github.com/amzapi/amazon-redis-session
```

## 快速开始

以下是一个简单的使用示例：

```go
package main

import (
    "context"
    "fmt"
    "log"
	
    "github.com/amzapi/amazon-redis-session"
)

func main() {
    // 创建 Redis 客户端
    cfg := &amazonsession.Config{
        Addr:     "localhost:6379",
        Password: "",
        Db:       0,
    }
    sessionManager, err := amazonsession.NewAmazonSession(cfg)
    if err != nil {
        log.Fatalf("无法连接到 Redis: %v", err)
    }

    ctx := context.Background()

    // 创建一个新的 Session
    session := &amazonsession.Session{
        Country: "US",
        // 其他字段根据需要填写
    }
    err = sessionManager.PushSession(ctx, session)
    if err != nil {
        log.Fatalf("设置 Session 失败: %v", err)
    }

    // 获取一个随机 Session
    randomSession, err := sessionManager.GetRandomSession(ctx, "US")
    if err != nil {
        log.Fatalf("获取 Session 失败: %v", err)
    }

    fmt.Printf("Random Session: %+v\n", randomSession)
}
```

## 配置

你可以通过以下参数配置 Redis 客户端：

- `Addr`: Redis 服务器地址，例如 "localhost:6379"
- `Password`: Redis 服务器密码（如果有）
- `Db`: Redis 数据库编号

## API

### NewAmazonSession

创建一个新的 AmazonSession 实例。

```go
func NewAmazonSession(cfg *Config) (*AmazonSession, error)
```

### PushSession

将一个新的 Session 存储到 Redis。

```go
func (j *AmazonSession) PushSession(ctx context.Context, session *Session) error
```

### GetRandomSession

获取一个随机的 Session。

```go
func (j *AmazonSession) GetRandomSession(ctx context.Context, country string) (*Session, error)
```

### PopSession

从 Redis 中弹出一个 Session 并将其从列表中移除。

```go
func (j *AmazonSession) PopSession(ctx context.Context, country string) (*Session, error)
```

### GetSession

根据国家和 sessionID 获取一个 Session。

```go
func (j *AmazonSession) GetSession(ctx context.Context, country, sessionID string) (*Session, error)
```

### GetCountrySessionIDs

获取特定国家的所有 Session ID。

```go
func (j *AmazonSession) GetCountrySessionIDs(ctx context.Context, country string) ([]string, error)
```

### GetAllSessions

获取所有国家的所有 Session。

```go
func (j *AmazonSession) GetAllSessions(ctx context.Context) ([]*Session, error)
```

### ListSession

列出特定国家的 Session，支持分页。

```go
func (j *AmazonSession) ListSession(ctx context.Context, country string, pgn Pagination) ([]*Session, error)
```

### ListCountrySession

列出特定国家的所有 Session。

```go
func (j *AmazonSession) ListCountrySession(ctx context.Context, country string) ([]*Session, error)
```

### UpdateLastCheckedTimestamp

更新特定 Session 的最后检查时间戳。

```go
func (j *AmazonSession) UpdateLastCheckedTimestamp(ctx context.Context, country, sessionID string) error
```

### DeleteSession

删除一个 Session。

```go
func (j *AmazonSession) DeleteSession(ctx context.Context, country, sessionID string) error
```

### CleanupSessions

清理过期或使用次数超过阈值的 Session。

```go
func (j *AmazonSession) CleanupSessions(ctx context.Context, timeDiffThreshold int64, usageCountThreshold int64) error
```

## 贡献

欢迎贡献代码！请遵循以下步骤进行贡献：

1. Fork 本仓库
2. 创建你的特性分支 (`git checkout -b feature/AmazingFeature`)
3. 提交你的修改 (`git commit -m 'Add some AmazingFeature'`)
4. 推送到分支 (`git push origin feature/AmazingFeature`)
5. 打开一个 Pull Request

## 许可证

该项目使用 MIT 许可证。详情请参阅 [LICENSE](LICENSE) 文件。
