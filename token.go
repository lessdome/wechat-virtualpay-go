package wechat_virtualpay_go

import "context"

// TokenProvider 提供 access_token。
//
// 本包**不内置** token 的获取与缓存 —— 因为缓存策略（内存 / 文件 / Redis）
// 因部署形态而异，内置一种等于替使用者做决定。实现这个接口即可：
//
//	type RedisToken struct{ rdb *redis.Client; appID, secret string }
//
//	func (t *RedisToken) Token(ctx context.Context) (string, error) {
//		// 1. 先读缓存；2. 没有或快过期则调用 /cgi-bin/token 刷新；
//		// 3. 用分布式锁避免多实例并发刷新互相顶掉。
//	}
//
// 注意：微信的 access_token 全局唯一且会互相顶掉，**多实例部署务必集中缓存**。
type TokenProvider interface {
	Token(ctx context.Context) (string, error)
}
