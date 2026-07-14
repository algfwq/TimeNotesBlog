# TimeNotes Blog

TimeNotes 手账本的公开展示与协作上传服务。

- **前端**：React + TypeScript + Semi Design（毛玻璃 / 动态光效）
- **后端**：Go + Fiber v3 + WebSocket + SQLite
- **部署形态**：前端 `npm run build` 产物输出到 `web/`，由后端同端口托管
- **业务 API**：几乎全部走 WebSocket；HTTP 仅用于静态页、健康检查、`.tnote` 下载

<p align="center">
  <img alt="Version" src="https://img.shields.io/badge/version-2.9.0-blue?style=flat-square" />
  <img alt="Go" src="https://img.shields.io/badge/Go-1.26-00ADD8?style=flat-square" />
  <img alt="Platform" src="https://img.shields.io/badge/platform-Windows%20%7C%20Linux%20amd64-5B8DEF?style=flat-square" />
</p>

## 💝 支持项目

<p align="center">
  TimeNotes 是独立开发者用爱发电的开源项目，完全免费。<br/>
  如果它帮你写出了更棒的手账，欢迎请开发者喝杯咖啡 ☕
</p>

<p align="center">
  <a href="https://ifdian.net/a/algfwq">
    <img src="./afdian-sponsor.jpg" alt="爱发电赞助 TimeNotes" width="480" />
  </a>
</p>

<p align="center">
  👉 <a href="https://ifdian.net/a/algfwq"><strong>前往爱发电支持 TimeNotes</strong></a>
</p>

## 版本 2.9.0 更新摘要

- 生产构建产物统一命名：`TimeNotesBlog-2.9.0-{arch}-{os}`（内嵌当前 `web/` 前端构建）。
- 与客户端 2.9.0 对齐：桌面 / Android 均可「连接到 Blog」上传或更新完整 `.tnote`。
- 客户端 Android 侧通过原生 Go 代理发起 Blog WebSocket，避免 `https://wails.localhost` 混合内容拦截。
- 账号体系：管理员后台发号、argon2id 密码、PoW 登录、JWT 会话；后台路径 token 每次启动随机。
- 互动：按 IP 点赞、评论、访问统计与可插拔 GeoIP。

## 发布产物（v2.9.0）

位于 `bin/`，命名规则：`TimeNotesBlog-{version}-{arch}-{os}`。

| 文件 | 说明 |
|------|------|
| `bin/TimeNotesBlog-2.9.0-amd64-windows.exe` | Windows amd64 生产可执行文件（含前端静态资源） |
| `bin/TimeNotesBlog-2.9.0-amd64-linux` | Linux amd64 生产可执行文件（含前端静态资源） |

```powershell
# 先构建前端再打包二进制
cd frontend
npm install
npm run build   # 输出到 ../web/
cd ..

# Windows amd64
$env:CGO_ENABLED=0; $env:GOOS="windows"; $env:GOARCH="amd64"
go build -trimpath -ldflags="-s -w" -o bin/TimeNotesBlog-2.9.0-amd64-windows.exe .

# Linux amd64
$env:CGO_ENABLED=0; $env:GOOS="linux"; $env:GOARCH="amd64"
go build -trimpath -ldflags="-s -w" -o bin/TimeNotesBlog-2.9.0-amd64-linux .
```

运行前请复制并修改 `config.example.json` → `config.json`，生产必须设置强 `jwtSecret`。

---

## 功能概览

| 模块 | 说明 |
|------|------|
| 公开首页 | 展示所有可见手账，标注上传者 |
| 阅读页 | 浏览器解压完整 `.tnote` 并只读渲染；点赞（按 IP，不可取消）；评论侧栏 |
| 客户端上传 | TimeNotes 客户端连接 Blog 后上传 / 更新完整 `.tnote` |
| 账号 | 不开放注册；管理员后台添加用户；登录 API + PoW + JWT |
| 后台 | `/admin/{每次启动随机 token}/`；用户/手账/统计（VChart） |
| 编辑手账 | 后台调用本机客户端 `127.0.0.1:54088` 桥下载并打开 |

---

## 必做：最小可运行部署

### 1. 环境

- Go 1.26+
- Node.js 18+ / npm
- （可选）Linux 防火墙放行服务端口

### 2. 构建前端

```bash
cd TimeNotesBlog/frontend
npm install
npm run build
# 产物写入 ../web/
```

### 3. 配置（强烈建议）

```bash
cd TimeNotesBlog
cp config.example.json config.json
```

**必改（生产）：**

```json
{
  "addr": "0.0.0.0:8090",
  "jwtSecret": "请换成足够长的随机字符串"
}
```

也可用环境变量：

| 变量 | 含义 |
|------|------|
| `TIMENOTES_BLOG_CONFIG` | 配置文件路径 |
| `TIMENOTES_BLOG_ADDR` | 监听地址 |
| `TIMENOTES_BLOG_JWT_SECRET` | JWT 密钥 |
| `TIMENOTES_BLOG_DB` | SQLite 路径 |
| `TIMENOTES_BLOG_NOTES_DIR` | `.tnote` 存储目录 |
| `TIMENOTES_BLOG_LOG` | 日志路径 |
| `TIMENOTES_BLOG_CORS_ORIGINS` | 逗号分隔 Origin 白名单 |
| `TIMENOTES_BLOG_GEO_URL` | 自定义 GeoIP URL 模板（含 `{ip}`） |
| `TIMENOTES_BLOG_GEO_API_KEY` | GeoIP API Key（若需要） |

### 4. 运行

```bash
cd TimeNotesBlog
go run .
# 或使用 v2.9.0 发布产物
./bin/TimeNotesBlog-2.9.0-amd64-linux
# Windows:
# .\bin\TimeNotesBlog-2.9.0-amd64-windows.exe
```

启动日志会打印：

```
Admin UI: http://127.0.0.1:8090/admin/<随机token>/
Default admin account created: username=admin password=123456
```

**首次启动后请立刻登录后台修改管理员密码。**

### 5. Linux 防火墙（若需外网访问）

```bash
# ufw 示例
sudo ufw allow 8090/tcp
sudo ufw reload
```

若前面有反向代理，需放行代理到本机的端口，并开启 WebSocket 升级。

### 6. 客户端连接

1. 启动 TimeNotes 客户端（会在 `127.0.0.1:54088` 启动本地桥）
2. 首页点击「连接到 Blog」
3. 填写 Blog URL / 用户名 / 密码，可勾选「记住密码」
4. 连接成功后，在手账卡片菜单「上传到 Blog」/「更新到 Blog」
5. 上云手账会显示 Semi 云图标标签；本地删除不影响服务器副本

---

## 选做

| 项 | 说明 |
|----|------|
| HTTPS / 反代 | Nginx/Caddy 终止 TLS，并代理 `/` 与 `/ws` |
| systemd | 写 unit 守护进程（非必须） |
| Docker | 自行打包（非必须） |
| 高精度 GeoIP | 配置 `geo.provider=http_json` + 带 Key 的 URL |
| PostgreSQL | 业务依赖 `storage.Store` 接口，可后续新增实现 |
| CORS 精确白名单 | 公网部署时设置 `corsOrigins`，并视情况关闭 `allowLoopbackOrigins` |

### Nginx WebSocket 示例（选做）

```nginx
location / {
  proxy_pass http://127.0.0.1:8090;
  proxy_http_version 1.1;
  proxy_set_header Host $host;
  proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
  proxy_set_header Upgrade $http_upgrade;
  proxy_set_header Connection "upgrade";
}
```

若使用反代，请把代理 IP 写入 `trustedProxies`，以便正确识别访客 IP（点赞 / 限流 / Geo）。

---

## 安全说明

1. **默认管理员** `admin` / `123456` 仅首次初始化；务必修改。
2. **后台路径 token 每次重启都会变**，请从启动日志复制。
3. 密码使用 **argon2id** 哈希存储。
4. 登录需要 **PoW**；同一 IP 失败次数越多难度越高。上传等已登录操作使用 **JWT**，不再做 PoW。
5. 点赞按 `sha256(pepper|ip)` 去重，不可取消。
6. 评论支持「昵称+邮箱」或 GitHub 主页链接；GitHub 头像仅解析 `github.com/{user}`，不服务端抓任意 URL。
7. 上传文件名经路径净化；磁盘以 UUID 文件名存储。
8. 日志不记录密码、JWT、PoW 答案；Geo 失败时日志只写 IP 哈希前缀。
9. 生产必须设置强 `jwtSecret`。

---

## GeoIP（可切换多源）

默认：

```json
"geo": {
  "provider": "ip-api",
  "urlTemplate": "http://ip-api.com/json/{ip}?fields=status,message,country,regionName,city,lat,lon,query",
  "timeoutMs": 3000,
  "cacheTTLHours": 168
}
```

换成其它 HTTP JSON 源时：

```json
"geo": {
  "provider": "http_json",
  "urlTemplate": "https://example.com/json/{ip}?key={apiKey}",
  "apiKey": "YOUR_KEY",
  "countryField": "country",
  "regionField": "region",
  "cityField": "city",
  "latField": "lat",
  "lngField": "lon"
}
```

结果缓存到 SQLite `geo_cache`；查询失败不阻断访问统计。

---

## WebSocket 消息（摘要）

| type | 说明 |
|------|------|
| `auth.pow.challenge` / `auth.login` / `auth.session` / `auth.ping` | 鉴权 |
| `notes.list` / `notes.get` | 公开手账 |
| `notes.upload.*` / `notes.update.*` | 分片上传 / 更新完整 `.tnote` |
| `notes.like` / `notes.comment.create` / `notes.comments.list` | 互动 |
| `visit.track` | 访问上报 |
| `admin.*` | 后台管理 |

文件下载：`GET /files/{token}`（由 `notes.get` 签发短期 token）。

---

## 目录结构

```
TimeNotesBlog/
├── main.go / config.go
├── frontend/          # React 源码
├── web/               # 前端构建产物（后端托管）
├── internal/
│   ├── auth/          # 密码、JWT、PoW
│   ├── protocol/      # WS 信封
│   ├── server/        # Fiber + Hub
│   ├── storage/       # Store 接口 + sqlite
│   └── geo/           # 可插拔 GeoIP
├── data/              # 数据库与 .tnote
└── logs/
```

---

## 开发与检查

```bash
# 后端
cd TimeNotesBlog
go test ./...
go build -o bin/timenotesblog .

# 前端
cd frontend
npm run build
```

客户端相关：

```bash
cd TimeNotes
go test ./...
go build .
cd frontend
npm run build
```

---

## 默认端口

| 服务 | 地址 |
|------|------|
| Blog | `127.0.0.1:8090` |
| 客户端 Blog 桥 | `127.0.0.1:54088`（仅本机） |

---

## 常见问题

**Q: 后台 404？**  
A: token 每次重启变化，请看最新启动日志。

**Q: 后台「编辑」失败？**  
A: 需本机先启动 TimeNotes 客户端；桥只监听 loopback。

**Q: 上传提示文件名冲突？**  
A: 同一用户下同名 `.tnote` 拒绝新建；请用「更新到 Blog」。

**Q: 公网无法访问？**  
A: 检查 `addr` 是否绑定 `0.0.0.0`，以及系统防火墙 / 云安全组是否放行 TCP 端口。
