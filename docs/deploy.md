# edge-proxy 部署手册

## 概览

`edge-proxy` 是单 Go binary，自带嵌入式 Web UI、SQLite 持久化、ACME 自动签证 / 续签、HTTPS 触达探测、钉钉 + Telegram 告警。每台节点完全自治、互不感知（B1 拓扑）。

```
       (用户)
         │ 443
         ▼
 ┌─────────────────────────┐
 │   edge-proxy (一台)      │
 │  /usr/local/bin/edge-proxy│  ← run 子命令长进程
 │  ├ Web UI :8080          │
 │  ├ SQLite /var/lib/edge-proxy/edge.db │
 │  ├ ACME cron (certbot)   │
 │  ├ probe cron            │
 │  └ renew cron (03:00)    │
 │            │             │
 │            ▼             │
 │   nginx (系统 nginx)     │
 │  /etc/nginx/conf.d/edge-*│
 │  ssl_certificate         │
 │  /etc/letsencrypt/live/  │
 └─────────────────────────┘
            │ 80 + Host header
            ▼
       (上游 / 源站)
```

---

## 0. 前置依赖

每台 edge 节点（Debian / Ubuntu 推荐）：

```bash
apt update
apt install -y nginx certbot python3-certbot-nginx
```

确认主 `nginx.conf` 含 `include /etc/nginx/conf.d/*.conf;`（Debian/Ubuntu 默认满足）。

```bash
grep -F 'include /etc/nginx/conf.d/*.conf' /etc/nginx/nginx.conf
```

DNS：将要服务的域名 A 记录解析到本机公网 IP。LE HTTP-01 验证依赖 80 端口可达。

---

## 1. 编译 binary（本地 → 目标机）

在开发机交叉编译（macOS 上无需 CGO）：

```bash
make release     # 产出 dist/edge-proxy-linux-amd64 + dist/edge-proxy-linux-arm64
```

或手动：

```bash
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o dist/edge-proxy-linux-amd64 ./cmd/edge-proxy
```

scp 到目标机：

```bash
scp dist/edge-proxy-linux-amd64 root@edge1:/usr/local/bin/edge-proxy
ssh root@edge1 chmod +x /usr/local/bin/edge-proxy
```

---

## 2. 初始化配置

```bash
ssh root@edge1
mkdir -p /etc/edge-proxy /var/lib/edge-proxy

# 写模板
edge-proxy init > /etc/edge-proxy/config.yaml
chmod 600 /etc/edge-proxy/config.yaml

# 生成密码 hash
edge-proxy gen-passwd "your-strong-password"
# → $2a$10$ABCDEF...

# 编辑 config.yaml: 填 admin.password_hash, acme.email,
# 以及（可选）alert.dingtalk.webhook / alert.telegram.bot_token 等
vim /etc/edge-proxy/config.yaml
```

关键字段：

| 字段 | 必填 | 备注 |
|------|------|------|
| `admin.bind` | 默认 `127.0.0.1:8080`，强烈建议保持，运维通过 SSH 隧道访问 | 改 `0.0.0.0` 公网暴露需自担风险 |
| `admin.username` / `admin.password_hash` | ✅ | 单管理员账号 |
| `acme.email` | ✅ | LE 注册邮箱（证书过期前发提醒邮件） |
| `alert.dingtalk.webhook` / `alert.telegram.*` | 留空则禁用对应通道 | 至少配一个，否则告警没出口 |
| `paths.data_dir` | 默认 `/var/lib/edge-proxy` | SQLite + 持久状态目录 |

---

## 3. 注册 systemd

```bash
edge-proxy install-systemd
systemctl daemon-reload
systemctl enable --now edge-proxy
systemctl status edge-proxy   # 验证 active (running)
```

---

## 4. 通过 SSH 隧道访问 Web UI

在你的开发机：

```bash
ssh -L 8080:127.0.0.1:8080 root@edge1
```

浏览器打开 `http://localhost:8080`，登录后：

1. **回源** 页录入 upstream IP（源站机器，如 `10.0.0.5:80`），可加多个 + 权重 + backup
2. **域名** 页录入一个测试域名（须真实 DNS 解析到本机）
3. 等待 ~30 秒：ACME cron 申请 LE 证书 → 写 `/etc/nginx/conf.d/edge-<host>.conf` → reload nginx
4. 浏览器访问 `https://<域名>/` 验证返回 200

---

## 5. 状态机一览

```
            录入
              ▼
         ┌────────┐ cron #1
         │pending │──────┐
         └────────┘      ▼
                 ┌───────────────┐
                 │ cert_applying │
                 └───────────────┘
                  │           │
              成功 │           │ 失败
                  ▼           ▼
              ┌────────┐  ┌─────────────┐ 5 次失败
              │ online │  │ cert_failed │──→ next_retry_at=now+1h
              └────────┘  └─────────────┘
              │      ▲
          probe失败 │ probe连续恢复
              ▼      │
            ┌──────────┐
            │ degraded │ ← 告警
            └──────────┘
              │
              │ UI 点"废弃"
              ▼
          ┌────────────┐    UI 点"回收"
          │ deprecated │──────────────→ DB 删除 + nginx conf 删除 + certbot delete
          └────────────┘
```

---

## 6. 故障排查

### 域名一直 `cert_failed`

UI 上看 `last_error`。常见原因：

- DNS 还没解析 / 解析错 → `dig +short <host>` 看是否指本机
- 80 端口被防火墙拦 → `nc -lv 80` 然后从外网 telnet
- 上一轮申请挂掉 → 点"重试"按钮清掉 backoff
- LE 速率限制 → 等 1 小时（连续 5 次失败后自动 backoff）

直接命令行复现：

```bash
certbot certonly --nginx --non-interactive -d <host> --email <email> --agree-tos --dry-run
```

### nginx -t 失败

```bash
nginx -t                                       # 看具体错误
ls /etc/nginx/conf.d/edge-*.conf               # 看 edge-proxy 写的配置
```

如 `edge-upstream.conf` 为空池子，UI 录入至少一个 enabled upstream。

### 域名状态长期 `degraded`

UI 上看 `last_probe_at` + 错误分类（HTTP 502 / TLS / 超时 / DNS 等）。重点排查：

- 上游服务是否 down → 直接在 edge 上 `curl -I http://<upstream-addr>/`
- 上游不返回 200/204/301/302 → 这是 OK 判定阈值，调整上游 health 端点或修改 `probe.health_path`

### Web UI 看不到 / SSH 隧道连不上

```bash
ss -tlnp | grep 8080           # 确认进程在监听
journalctl -u edge-proxy -n 50 # 看启动日志
```

---

## 7. 升级流程

```bash
# 1. 本地编译新版本
make release

# 2. scp 替换 binary
scp dist/edge-proxy-linux-amd64 root@edge1:/usr/local/bin/edge-proxy.new
ssh root@edge1 'chmod +x /usr/local/bin/edge-proxy.new && \
    mv /usr/local/bin/edge-proxy /usr/local/bin/edge-proxy.old && \
    mv /usr/local/bin/edge-proxy.new /usr/local/bin/edge-proxy && \
    systemctl restart edge-proxy && \
    systemctl status edge-proxy'
```

升级期间 Web UI 短暂不可访问；nginx 自身不动，已签证域名继续提供服务。

升级后回滚：

```bash
ssh root@edge1 'mv /usr/local/bin/edge-proxy.old /usr/local/bin/edge-proxy && systemctl restart edge-proxy'
```

---

## 8. 备份策略

每日定时备份 SQLite（运维 cron）：

```bash
# /etc/cron.daily/edge-proxy-backup
#!/bin/bash
sqlite3 /var/lib/edge-proxy/edge.db ".backup '/var/backups/edge-$(date +\%F).db'"
find /var/backups -name 'edge-*.db' -mtime +14 -delete
```

LE 证书（`/etc/letsencrypt/`）由 certbot 自管，无需额外备份；删除后下一轮 cron 自动重申请。

---

## 9. 完全卸载

```bash
systemctl disable --now edge-proxy
rm -f /usr/local/bin/edge-proxy /etc/systemd/system/edge-proxy.service
rm -rf /etc/edge-proxy /var/lib/edge-proxy
rm -f /etc/nginx/conf.d/edge-*.conf
systemctl reload nginx
# 可选：清 LE 证书
for d in $(ls /etc/letsencrypt/live/ 2>/dev/null); do certbot delete --cert-name "$d" --non-interactive; done
```
